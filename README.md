# Talkin MySQL MCP Server

本地 Go MCP 服务，供本机 Codex 查询 Talkin 的测试库和线上只读库。服务默认通过 `internal/config/config.yaml` 连接数据库，并通过 stdio 提供工具调用。使用 Go 官方 MCP SDK。

## 配置

复制 `internal/config/config.example.yaml` 为 `internal/config/config.yaml`，填写两套连接的 host、port、user 和 password；连接配置不需要指定默认 database。实际配置文件已加入 `.gitignore`。线上连接应使用数据库侧配置为只读权限的账号；MCP 服务不会额外限制 SQL 类型。也可以设置 `TALKIN_MCP_CONFIG` 指向其他 YAML 文件。

## 构建和启动

```sh
go mod download
go build -o talkin-mysql-mcp-server .
./talkin-mysql-mcp-server
```

Codex MCP 配置示例（将路径替换为本目录的绝对路径）：

```toml
[mcp_servers.talkin_mysql]
command = "/absolute/path/to/talkin-mysql-mcp-server/talkin-mysql-mcp-server"
args = []
cwd = "/absolute/path/to/talkin-mysql-mcp-server"
```

重启 Codex 后，先调用 `list_databases` 并传入 `connection`（`test` 或 `prod_readonly`）查看该连接账号可访问的数据库，再调用 `list_tables` 获取指定数据库中的表和视图。调用 `describe_table` 时传入 `connection`、`database` 和 `table`，查看字段类型、键、默认值及字段注释。调用 `query_database` 时传入 `connection`、实际的 `database` 名称和 SQL；默认最多返回 100 行，可设置 `limit`（最大 500 行），结果还限制在约 64 KiB，并通过 `truncated` 标明是否截断。

stdio 模式下普通日志写入 stderr，stdout 专用于 MCP 协议通信。

服务日志默认追加写入 `logs/talkin-mysql-mcp-server.log`，同时写入 stderr；日志文件权限为仅当前用户可读写。可以设置 `TALKIN_MCP_LOG` 修改日志文件路径。查询日志会记录 SQL 长度和 SHA-256 前缀用于关联，不记录 SQL 原文。
