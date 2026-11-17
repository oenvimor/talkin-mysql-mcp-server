# Talkin MySQL MCP Server

Python 实现的本地 MCP 服务，通过 `config.yaml` 连接 Talkin 测试库和线上只读库，并由 Codex 通过 stdio 调用。兼容 Python 3.9+。

## 配置

编辑目录下的 `config.yaml`，填写两套数据库的 host、port、user、password 和 database。该文件已加入 `.gitignore`。线上连接应使用数据库侧配置为只读权限的账号；MCP 服务不会额外限制 SQL 类型。也可以设置 `TALKIN_MCP_CONFIG` 指向其他 YAML 文件。

## 安装和启动

```sh
python3 -m venv .venv
source .venv/bin/activate
python -m pip install -r requirements.txt
python main.py
```

Codex MCP 配置示例（将路径替换为本目录的绝对路径）：

```toml
[mcp_servers.talkin_mysql]
command = "/absolute/path/to/talkin-mysql-mcp-server/.venv/bin/python"
args = ["/absolute/path/to/talkin-mysql-mcp-server/main.py"]
cwd = "/absolute/path/to/talkin-mysql-mcp-server"
```

重启 Codex 后即可使用 `list_databases` 查看已配置目标，并用 `query_database` 指定 `test` 或 `prod_readonly` 执行 SQL。

stdio 模式下普通日志写入 stderr，stdout 专用于 MCP 协议通信。
