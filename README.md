# talkin-mysql-mcp-server
本地 MCP 服务，供本机 Codex 查询 Talkin 的测试库和线上只读库。服务启动时从本机环境变量或未提交的配置文件读取连接信息；调用工具时选择目标库。SQL 权限由对应数据库账号控制
