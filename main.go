package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

type Config struct {
	Databases map[string]DatabaseConfig `yaml:"databases"`
}

type QueryArgs struct {
	Database string `json:"database" jsonschema:"target database: test or prod_readonly"`
	SQL      string `json:"sql" jsonschema:"SQL statement to execute"`
}

func loadConfig() (Config, error) {
	configPath := os.Getenv("TALKIN_MCP_CONFIG")
	if configPath == "" {
		configPath = filepath.Join(".", "config.yaml")
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", configPath, err)
	}
	var config Config
	if err := yaml.Unmarshal(contents, &config); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", configPath, err)
	}
	for _, name := range []string{"test", "prod_readonly"} {
		db, ok := config.Databases[name]
		if !ok || db.Host == "" || db.Port <= 0 || db.User == "" || db.Password == "" || db.Database == "" {
			return Config{}, fmt.Errorf("database configuration %q is incomplete", name)
		}
	}
	return config, nil
}

func openDatabases(config Config) (map[string]*sql.DB, error) {
	databases := make(map[string]*sql.DB, len(config.Databases))
	for _, name := range []string{"test", "prod_readonly"} {
		cfg := config.Databases[name]
		driverConfig := mysql.NewConfig()
		driverConfig.User = cfg.User
		driverConfig.Passwd = cfg.Password
		driverConfig.Net = "tcp"
		driverConfig.Addr = fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		driverConfig.DBName = cfg.Database
		driverConfig.ParseTime = true
		driverConfig.Timeout = 10 * time.Second
		driverConfig.ReadTimeout = 60 * time.Second
		driverConfig.WriteTimeout = 60 * time.Second
		db, err := sql.Open("mysql", driverConfig.FormatDSN())
		if err != nil {
			return nil, fmt.Errorf("configure database %q: %w", name, err)
		}
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)
		databases[name] = db
	}
	return databases, nil
}

func queryDatabase(ctx context.Context, databases map[string]*sql.DB, args QueryArgs) (*mcp.CallToolResult, any, error) {
	db, ok := databases[args.Database]
	if !ok {
		return nil, nil, errors.New("database must be test or prod_readonly")
	}
	if strings.TrimSpace(args.SQL) == "" {
		return nil, nil, errors.New("sql must not be empty")
	}

	rows, err := db.QueryContext(ctx, args.SQL)
	if err != nil {
		return toolError("Database operation failed. Check the SQL, connectivity, and database account permissions.")
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return toolError("Could not read query result columns.")
	}
	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range values {
			scanTargets[i] = &values[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return toolError("Could not read query result rows.")
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			if bytes, ok := values[i].([]byte); ok {
				row[column] = string(bytes)
			} else {
				row[column] = values[i]
			}
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return toolError("Query failed while reading results.")
	}

	encoded, err := json.MarshalIndent(map[string]any{"rows": results, "row_count": len(results)}, "", "  ")
	if err != nil {
		return toolError("Could not encode query results.")
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}, nil, nil
}

func toolError(message string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}, nil, nil
}

func main() {
	config, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	databases, err := openDatabases(config)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		for _, db := range databases {
			_ = db.Close()
		}
	}()

	server := mcp.NewServer(&mcp.Implementation{Name: "talkin-mysql-mcp-server", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_databases",
		Description: "列出已配置的 Talkin 数据库连接名称。",
	}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "test\nprod_readonly"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query_database",
		Description: "在选定的 Talkin MySQL 数据库上执行 SQL。操作权限由对应数据库账号权限决定。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args QueryArgs) (*mcp.CallToolResult, any, error) {
		return queryDatabase(ctx, databases, args)
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
