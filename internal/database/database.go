package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"talkin-mysql-mcp-server/internal/config"
)

type Store struct {
	databases map[string]*sql.DB
}

func Open(cfg config.Config) (*Store, error) {
	store := &Store{databases: make(map[string]*sql.DB, len(cfg.Databases))}
	for _, name := range []string{"test", "prod_readonly"} {
		settings := cfg.Databases[name]
		driverConfig := mysql.NewConfig()
		driverConfig.User = settings.User
		driverConfig.Passwd = settings.Password
		driverConfig.Net = "tcp"
		driverConfig.Addr = fmt.Sprintf("%s:%d", settings.Host, settings.Port)
		driverConfig.ParseTime = true
		driverConfig.Timeout = 10 * time.Second
		driverConfig.ReadTimeout = 60 * time.Second
		driverConfig.WriteTimeout = 60 * time.Second

		db, err := sql.Open("mysql", driverConfig.FormatDSN())
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("configure database %q: %w", name, err)
		}
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)
		store.databases[name] = db
	}
	return store, nil
}

func (s *Store) Close() error {
	var firstErr error
	for _, db := range s.databases {
		if err := db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Store) ListDatabases(ctx context.Context, name string) ([]string, error) {
	db, ok := s.databases[name]
	if !ok {
		return nil, fmt.Errorf("connection must be test or prod_readonly")
	}
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var database string
		if err := rows.Scan(&database); err != nil {
			return nil, err
		}
		databases = append(databases, database)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return databases, nil
}

func (s *Store) ListTables(ctx context.Context, connection, database string) ([]map[string]any, error) {
	conn, err := s.schemaConn(ctx, connection, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, `
		SELECT TABLE_NAME, TABLE_TYPE, TABLE_COMMENT
		FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE()
		ORDER BY TABLE_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) Query(ctx context.Context, connection, database, query string, limit, maxBytes int) ([]map[string]any, bool, error) {
	conn, err := s.schemaConn(ctx, connection, database)
	if err != nil {
		return nil, false, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, false, err
	}

	results := make([]map[string]any, 0)
	usedBytes := 2 // []
	truncated := false
	for rows.Next() {
		if len(results) >= limit {
			truncated = true
			break
		}
		row, err := scanRow(rows, columns)
		if err != nil {
			return nil, false, err
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil, false, err
		}
		separatorBytes := 0
		if len(results) > 0 {
			separatorBytes = 1
		}
		if usedBytes+separatorBytes+len(encoded) > maxBytes {
			truncated = true
			break
		}
		results = append(results, row)
		usedBytes += separatorBytes + len(encoded)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return results, truncated, nil
}

func (s *Store) DescribeTable(ctx context.Context, connection, database, table string) ([]map[string]any, error) {
	if strings.TrimSpace(table) == "" {
		return nil, fmt.Errorf("table must not be empty")
	}
	conn, err := s.schemaConn(ctx, connection, database)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(ctx, "SHOW FULL COLUMNS FROM "+quoteIdentifier(table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (s *Store) schemaConn(ctx context.Context, connection, database string) (*sql.Conn, error) {
	if strings.TrimSpace(database) == "" {
		return nil, fmt.Errorf("database must not be empty")
	}
	db, ok := s.databases[connection]
	if !ok {
		return nil, fmt.Errorf("connection must be test or prod_readonly")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "USE "+quoteIdentifier(database)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0)
	for rows.Next() {
		row, err := scanRow(rows, columns)
		if err != nil {
			return nil, err
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func scanRow(rows *sql.Rows, columns []string) (map[string]any, error) {
	values := make([]any, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	row := make(map[string]any, len(columns))
	for i, column := range columns {
		if bytes, ok := values[i].([]byte); ok {
			row[column] = string(bytes)
		} else {
			row[column] = values[i]
		}
	}
	return row, nil
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}
