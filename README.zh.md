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

## 使用已发布版本

`v0.0.1` 已发布 npm 包和多架构 Docker 镜像。两个发布入口默认都会通过 stdio 启动 MCP。

### npm

用 `npx` 运行已发布的包：

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
npx -y @futuretea/postgres-mcp-server@0.0.1
```

MCP 客户端配置示例：

```json
{
  "mcpServers": {
    "postgres-readonly": {
      "command": "npx",
      "args": ["-y", "@futuretea/postgres-mcp-server@0.0.1"],
      "env": {
        "DATABASE_URL": "postgres://reader@127.0.0.1:5432/app?sslmode=disable"
      }
    }
  }
}
```

npm 命令是 MCP 入口，不是服务器其它 CLI 子命令的包装器。请固定为你希望运行的版本。

### Docker

直接运行已发布的镜像：

```sh
export DATABASE_URL='postgres://reader@db.example.internal:5432/app?sslmode=require'
docker run --rm -i \
  --env DATABASE_URL \
  ghcr.io/futuretea/postgres-mcp-server:v0.0.1
```

若 MCP 客户端通过 Docker 启动服务，把连接配置传递给 Docker 进程：

```json
{
  "mcpServers": {
    "postgres-readonly": {
      "command": "docker",
      "args": [
        "run",
        "--rm",
        "-i",
        "--env",
        "DATABASE_URL",
        "ghcr.io/futuretea/postgres-mcp-server:v0.0.1"
      ],
      "env": {
        "DATABASE_URL": "postgres://reader@db.example.internal:5432/app?sslmode=require"
      }
    }
  }
}
```

数据库主机名必须能从容器访问。容器内的 `localhost` 不是宿主机；请使用 Docker 网络内的数据库主机名，或在适用时使用平台对应的宿主机网关。

Docker 示例使用远程数据库时采用 `sslmode=require`。请按部署环境需要设置 PostgreSQL TLS 模式和证书。

### HTTP 模式

在发布命令后传入 MCP 参数，就会启动 HTTP 而不是 stdio。直接运行时默认监听 loopback：

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
npx -y @futuretea/postgres-mcp-server@0.0.1 --port 8080
```

Docker 中服务必须监听容器的全部接口；宿主机发布端口仍可只绑定 loopback：

```sh
export DATABASE_URL='postgres://reader@db.example.internal:5432/app?sslmode=require'
docker run --rm \
  --env DATABASE_URL \
  --publish 127.0.0.1:8080:8080 \
  ghcr.io/futuretea/postgres-mcp-server:v0.0.1 \
  --port 8080 --listen 0.0.0.0
```

Streamable HTTP MCP 客户端使用 `http://127.0.0.1:8080/mcp`。服务也提供 `/sse`（消息端点 `/message`）和健康检查 `GET /healthz`。HTTP 没有认证或 TLS；只能部署在可信网络，或置于同时提供认证和 TLS 的基础设施之后。

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
