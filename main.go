package main

import (
	"context"

	"talkin-mysql-mcp-server/internal/config"
	"talkin-mysql-mcp-server/internal/database"
	"talkin-mysql-mcp-server/internal/logging"
	"talkin-mysql-mcp-server/internal/server"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// 初始化日志
	logger, closeLogger, err := logging.New()
	if err != nil {
		panic(err)
	}
	defer closeLogger()

	// 初始化配置
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal(err)
	}

	// 初始化数据库连接
	databases, err := database.Open(cfg)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		if err := databases.Close(); err != nil {
			logger.Printf("close databases: %v", err)
		}
	}()

	logger.Printf("MCP server starting")
	// 启动 MCP 服务器
	if err := server.New(databases, logger).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Fatal(err)
	}
}
