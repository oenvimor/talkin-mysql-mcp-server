package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultQueryRowLimit = 100
	maximumQueryRowLimit = 500
	queryRowsMaxBytes    = 63 * 1024
)

type DataSource interface {
	ListDatabases(ctx context.Context, connection string) ([]string, error)
	ListTables(ctx context.Context, connection, database string) ([]map[string]any, error)
	DescribeTable(ctx context.Context, connection, database, table string) ([]map[string]any, error)
	Query(ctx context.Context, connection, database, query string, limit, maxBytes int) ([]map[string]any, bool, error)
}

type queryArgs struct {
	Connection string `json:"connection" jsonschema:"connection target: test or prod_readonly"`
	Database   string `json:"database" jsonschema:"MySQL database/schema to use, from list_databases"`
	SQL        string `json:"sql" jsonschema:"SQL statement to execute"`
	Limit      *int   `json:"limit,omitempty" jsonschema:"maximum rows to return; defaults to 100, maximum 500"`
}

type listDatabasesArgs struct {
	Connection string `json:"connection" jsonschema:"connection target: test or prod_readonly"`
}

type describeTableArgs struct {
	Connection string `json:"connection" jsonschema:"connection target: test or prod_readonly"`
	Database   string `json:"database" jsonschema:"MySQL database/schema name"`
	Table      string `json:"table" jsonschema:"table name to describe"`
}

type listTablesArgs struct {
	Connection string `json:"connection" jsonschema:"connection target: test or prod_readonly"`
	Database   string `json:"database" jsonschema:"MySQL database/schema name"`
}

func New(source DataSource, logger *log.Logger) *mcp.Server {
	if logger == nil {
		logger = log.Default()
	}
	s := mcp.NewServer(&mcp.Implementation{Name: "talkin-mysql-mcp-server", Version: "1.0.0"}, nil)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_databases",
		Description: "列出指定连接账号可访问的 MySQL 数据库。connection 填 test 或 prod_readonly。",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listDatabasesArgs) (*mcp.CallToolResult, any, error) {
		started := time.Now()
		databases, err := source.ListDatabases(ctx, args.Connection)
		if err != nil {
			logger.Printf("tool=list_databases connection=%q duration=%s error=%q", args.Connection, time.Since(started), err)
			return toolError("Could not list databases. Check the connection and account permissions."), nil, nil
		}
		encoded, err := json.MarshalIndent(map[string]any{"databases": databases}, "", "  ")
		if err != nil {
			logger.Printf("tool=list_databases connection=%q duration=%s error=encode_result:%q", args.Connection, time.Since(started), err)
			return toolError("Could not encode database list."), nil, nil
		}
		logger.Printf("tool=list_databases connection=%q database_count=%d duration=%s", args.Connection, len(databases), time.Since(started))
		return textResult(string(encoded)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_tables",
		Description: "列出指定 MySQL 数据库中的表和视图名称、类型及表注释。connection 填 test 或 prod_readonly，database 填实际数据库名。",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listTablesArgs) (*mcp.CallToolResult, any, error) {
		started := time.Now()
		if strings.TrimSpace(args.Connection) == "" || strings.TrimSpace(args.Database) == "" {
			return nil, nil, errors.New("connection and database must not be empty")
		}
		tables, err := source.ListTables(ctx, args.Connection, args.Database)
		if err != nil {
			logger.Printf("tool=list_tables connection=%q database=%q duration=%s error=%q", args.Connection, args.Database, time.Since(started), err)
			return toolError("Could not list tables. Check the database, connectivity, and account permissions."), nil, nil
		}
		encoded, err := json.MarshalIndent(map[string]any{"tables": tables, "table_count": len(tables)}, "", "  ")
		if err != nil {
			logger.Printf("tool=list_tables connection=%q database=%q duration=%s error=encode_result:%q", args.Connection, args.Database, time.Since(started), err)
			return toolError("Could not encode table list."), nil, nil
		}
		logger.Printf("tool=list_tables connection=%q database=%q table_count=%d duration=%s", args.Connection, args.Database, len(tables), time.Since(started))
		return textResult(string(encoded)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "describe_table",
		Description: "查看指定 MySQL 表的字段结构，包括字段类型、是否可空、键、默认值、额外属性和字段注释。connection 填 test 或 prod_readonly，database 填实际数据库名。",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args describeTableArgs) (*mcp.CallToolResult, any, error) {
		started := time.Now()
		if strings.TrimSpace(args.Connection) == "" || strings.TrimSpace(args.Database) == "" || strings.TrimSpace(args.Table) == "" {
			return nil, nil, errors.New("connection, database, and table must not be empty")
		}
		columns, err := source.DescribeTable(ctx, args.Connection, args.Database, args.Table)
		if err != nil {
			logger.Printf("tool=describe_table connection=%q database=%q table=%q duration=%s error=%q", args.Connection, args.Database, args.Table, time.Since(started), err)
			return toolError("Could not describe table. Check the database, table name, connectivity, and account permissions."), nil, nil
		}
		encoded, err := json.MarshalIndent(map[string]any{
			"table":        args.Table,
			"columns":      columns,
			"column_count": len(columns),
		}, "", "  ")
		if err != nil {
			logger.Printf("tool=describe_table connection=%q database=%q table=%q duration=%s error=encode_result:%q", args.Connection, args.Database, args.Table, time.Since(started), err)
			return toolError("Could not encode table structure."), nil, nil
		}
		logger.Printf("tool=describe_table connection=%q database=%q table=%q column_count=%d duration=%s", args.Connection, args.Database, args.Table, len(columns), time.Since(started))
		return textResult(string(encoded)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "query_database",
		Description: "在指定连接的 MySQL 数据库上执行 SQL。先用 list_databases 查询可访问的数据库；connection 填 test 或 prod_readonly，database 填实际数据库名。默认最多返回 100 行，可用 limit 指定 1 到 500 行；结果还受约 64 KiB 大小上限约束，超出时会标记 truncated。测试连接可读写，线上连接的权限由数据库账号控制。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args queryArgs) (*mcp.CallToolResult, any, error) {
		started := time.Now()
		if strings.TrimSpace(args.SQL) == "" {
			return nil, nil, errors.New("sql must not be empty")
		}
		if strings.TrimSpace(args.Connection) == "" || strings.TrimSpace(args.Database) == "" {
			return nil, nil, errors.New("connection and database must not be empty")
		}
		limit := defaultQueryRowLimit
		if args.Limit != nil {
			if *args.Limit < 1 || *args.Limit > maximumQueryRowLimit {
				return nil, nil, errors.New("limit must be between 1 and 500")
			}
			limit = *args.Limit
		}
		rows, truncated, err := source.Query(ctx, args.Connection, args.Database, args.SQL, limit, queryRowsMaxBytes)
		queryHash := sha256.Sum256([]byte(args.SQL))
		if err != nil {
			logger.Printf("tool=query_database connection=%q database=%q sql_bytes=%d sql_sha256=%x limit=%d duration=%s error=%q", args.Connection, args.Database, len(args.SQL), queryHash[:8], limit, time.Since(started), err)
			return toolError("Database operation failed. Check the SQL, connectivity, and database account permissions."), nil, nil
		}
		encoded, err := json.Marshal(map[string]any{"rows": rows, "row_count": len(rows), "truncated": truncated})
		if err != nil {
			logger.Printf("tool=query_database connection=%q database=%q sql_bytes=%d sql_sha256=%x duration=%s error=encode_result:%q", args.Connection, args.Database, len(args.SQL), queryHash[:8], time.Since(started), err)
			return toolError("Could not encode query results."), nil, nil
		}
		logger.Printf("tool=query_database connection=%q database=%q sql_bytes=%d sql_sha256=%x row_count=%d truncated=%t duration=%s", args.Connection, args.Database, len(args.SQL), queryHash[:8], len(rows), truncated, time.Since(started))
		return textResult(string(encoded)), nil, nil
	})
	return s
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}
