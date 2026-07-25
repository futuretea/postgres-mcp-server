# PostgreSQL 只读 MCP Server

这是一个面向单个 PostgreSQL 17+ 实例的 MCP（Model Context Protocol）服务。它提供数据库目录发现和受限的 SELECT 查询，可通过 stdio 或 HTTP 使用，也提供 CLI。

## 快速开始

先为最小权限 PostgreSQL 角色设置连接目标，再构建并查看工具目录：

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
make build
./bin/postgres-mcp-server tools list
./bin/postgres-mcp-server mcp
```

`tools list` 与 `tools describe` 不会读取数据库连接配置。MCP/HTTP 启动或实际调用工具时才会校验连接，并在 ready 前校验服务端版本与角色。

## 工具

| 工具 | 参数 | 用途 |
|---|---|---|
| `postgres_get_server_info` | 无 | 返回服务端与当前连接身份信息。 |
| `postgres_list_databases` | 无 | 列出可见数据库。 |
| `postgres_list_schemas` | 无 | 列出可见 schema。 |
| `postgres_list_tables` | `schema` | 列出 schema 中的表和视图。 |
| `postgres_describe_table` | `schema`、`table` | 描述表的列。 |
| `postgres_list_indexes` | `schema`、`table` | 列出表索引。 |
| `postgres_query` | `sql` | 执行一条严格只读 SELECT。 |

所有成功响应都是一个 JSON text envelope，固定含有 `schema_version`、`tool` 和 `data`。参数必须是没有未知字段的 JSON object。无参数工具必须传 `{}`；缺失、`null`、数组和标量都会被拒绝。

## 数据库连接

使用裸 `DATABASE_URL`，或者 PostgreSQL 标准环境变量：`PGHOST`、`PGPORT`、`PGDATABASE`、`PGUSER`、`PGPASSWORD`、`PGSSLMODE`。设置了 `DATABASE_URL` 时它优先。配置只能解析到一个 `host:port`；不同的 URL 目标或逗号分隔的 `PGHOST` 会在创建连接前失败。

服务启动时要求 PostgreSQL 17 或更新版本，并拒绝 superuser、可绕过行级安全、具备复制权限，或属于服务端文件、服务端程序、后端信号角色的服务账号。请使用独立的最小权限数据库角色。

## 只读边界与部署风险

每次数据库操作都在 PostgreSQL read-only transaction 和 pgx extended execution protocol 中执行。`postgres_query` 会解析为恰好一条 SELECT，并拒绝 DML、DDL、行锁、SELECT INTO、修改型 CTE 以及一组已知有副作用的 PostgreSQL 内建函数。

这不是对所有用户函数、扩展、FDW 或未来 PostgreSQL 内建函数无副作用的完整证明。数据库账号只应授予该服务确实需要的权限。

HTTP transport 没有认证或 TLS。默认只监听 loopback。若通过 `--listen` 监听远程地址，只能部署在可信网络中，或放在提供所需保护的基础设施之后。本服务刻意不提供查询超时、行数限制、响应大小限制或并发预算。

## 运行 MCP 与 HTTP

```sh
# stdio MCP
./bin/postgres-mcp-server mcp

# loopback HTTP MCP
./bin/postgres-mcp-server mcp --port 8080

# 显式远程监听；须满足上面的可信网络要求
./bin/postgres-mcp-server mcp --port 8080 --listen 0.0.0.0
```

HTTP 健康检查为 `GET /healthz`。transport 和工具过滤配置见 `config.example.yaml`；数据库连接仍只从环境变量读取。

## CLI

```sh
./bin/postgres-mcp-server tools list --json
./bin/postgres-mcp-server tools describe postgres_query --json
./bin/postgres-mcp-server tools call postgres_list_schemas --params '{}'
./bin/postgres-mcp-server tools call postgres_query --params '{"sql":"SELECT 1"}'
./bin/postgres-mcp-server version
```

## 容器

`make docker` 会先构建 Alpine CGO parser-check target，再构建可运行镜像：

```sh
make docker
docker run --rm -i --env DATABASE_URL postgres-mcp-server:dev
```

镜像入口是 `postgres-mcp-server mcp`。

## 开发检查

```sh
make format
make lint
make test
make test-integration  # 需要 Docker daemon；会创建 PostgreSQL 17 fixture
make build
make docker
```
