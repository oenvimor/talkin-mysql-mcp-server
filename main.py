"""Minimal stdio MCP server for the Talkin MySQL databases."""

import json
import os
import sys
from pathlib import Path

import pymysql
import yaml

DATABASE_NAMES = ("test", "prod_readonly")
PROTOCOL_VERSION = "2024-11-05"


def load_config():
    config_path = Path(os.environ.get("TALKIN_MCP_CONFIG", Path.cwd() / "config.yaml"))
    with config_path.open("r", encoding="utf-8") as config_file:
        config = yaml.safe_load(config_file)
    databases = config.get("databases", {})
    for name in DATABASE_NAMES:
        db = databases.get(name, {})
        required = ("host", "port", "user", "password", "database")
        missing = [key for key in required if key not in db or db[key] in (None, "")]
        if missing:
            raise ValueError("Missing configuration for {}: {}".format(name, ", ".join(missing)))
    return databases


def send(message):
    sys.stdout.write(json.dumps(message, ensure_ascii=False, default=str) + "\n")
    sys.stdout.flush()


def tool_definitions():
    return [
        {
            "name": "list_databases",
            "description": "列出已配置的 Talkin 数据库连接名称。",
            "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
        },
        {
            "name": "query_database",
            "description": "在选定的 Talkin MySQL 数据库上执行 SQL。操作权限由对应数据库账号权限决定。",
            "inputSchema": {
                "type": "object",
                "properties": {
                    "database": {"type": "string", "enum": list(DATABASE_NAMES), "description": "目标数据库"},
                    "sql": {"type": "string", "minLength": 1, "description": "要执行的 SQL 语句"},
                },
                "required": ["database", "sql"],
                "additionalProperties": False,
            },
        },
    ]


def run_tool(name, arguments, databases):
    if name == "list_databases":
        return {"content": [{"type": "text", "text": "\n".join(DATABASE_NAMES)}]}
    if name != "query_database":
        raise ValueError("Unknown tool: {}".format(name))
    database = arguments.get("database")
    sql = arguments.get("sql")
    if database not in DATABASE_NAMES or not isinstance(sql, str) or not sql.strip():
        raise ValueError("database and non-empty sql are required")
    db = databases[database]
    connection = pymysql.connect(
        host=db["host"], port=int(db["port"]), user=db["user"], password=db["password"],
        database=db["database"], charset="utf8mb4", connect_timeout=10,
        read_timeout=60, write_timeout=60, autocommit=True,
    )
    try:
        with connection.cursor() as cursor:
            cursor.execute(sql)
            if cursor.description:
                columns = [column[0] for column in cursor.description]
                rows = [dict(zip(columns, row)) for row in cursor.fetchall()]
                result = {"rows": rows, "row_count": len(rows)}
            else:
                result = {"affected_rows": cursor.rowcount, "last_insert_id": cursor.lastrowid}
        return {"content": [{"type": "text", "text": json.dumps(result, ensure_ascii=False, default=str)}]}
    finally:
        connection.close()


def main():
    databases = load_config()
    for line in sys.stdin:
        try:
            request = json.loads(line)
            method = request.get("method")
            request_id = request.get("id")
            if method == "initialize":
                send({"jsonrpc": "2.0", "id": request_id, "result": {
                    "protocolVersion": PROTOCOL_VERSION,
                    "capabilities": {"tools": {}},
                    "serverInfo": {"name": "talkin-mysql-mcp-server", "version": "1.0.0"},
                }})
            elif method == "notifications/initialized" or method == "notifications/cancelled":
                continue
            elif method == "ping":
                send({"jsonrpc": "2.0", "id": request_id, "result": {}})
            elif method == "tools/list":
                send({"jsonrpc": "2.0", "id": request_id, "result": {"tools": tool_definitions()}})
            elif method == "tools/call":
                params = request.get("params", {})
                try:
                    result = run_tool(params.get("name"), params.get("arguments", {}), databases)
                    send({"jsonrpc": "2.0", "id": request_id, "result": result})
                except Exception as error:
                    code = getattr(error, "args", ["QUERY_FAILED"])[0]
                    send({"jsonrpc": "2.0", "id": request_id, "result": {
                        "isError": True,
                        "content": [{"type": "text", "text": "Database operation failed ({}). Check SQL, connectivity, and account permissions.".format(code)}],
                    }})
            elif request_id is not None:
                send({"jsonrpc": "2.0", "id": request_id, "error": {"code": -32601, "message": "Method not found"}})
        except Exception as error:
            print("MCP server error: {}".format(error), file=sys.stderr)
            if isinstance(locals().get("request"), dict) and request.get("id") is not None:
                send({"jsonrpc": "2.0", "id": request["id"], "error": {"code": -32603, "message": "Internal error"}})


if __name__ == "__main__":
    main()
