# PostgreSQL 只读 MCP Server

这是一个面向单个 PostgreSQL 17+ 实例的 MCP（Model Context Protocol）服务。它提供数据库目录发现和受限的 SELECT 查询，可通过 stdio 或 HTTP 使用，也提供 CLI。

## 快速开始

先为最小权限 PostgreSQL 角色设置分字段连接配置，再构建并查看工具目录。这里的密码是普通环境变量值，不需要 URL 编码。

```sh
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
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
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
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
        "MCP_POSTGRES_HOST": "127.0.0.1",
        "MCP_POSTGRES_PORT": "5432",
        "MCP_POSTGRES_DATABASE": "app",
        "MCP_POSTGRES_USERNAME": "reader",
        "MCP_POSTGRES_PASSWORD": "<runtime-secret>",
        "MCP_POSTGRES_SSLMODE": "disable"
      }
    }
  }
}
```

npm 命令是 MCP 入口，不是服务器其它 CLI 子命令的包装器。请固定为你希望运行的版本。

### Docker

直接运行已发布的镜像：

```sh
export MCP_POSTGRES_HOST=db.example.internal
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=require
docker run --rm -i \
  --env MCP_POSTGRES_HOST \
  --env MCP_POSTGRES_PORT \
  --env MCP_POSTGRES_DATABASE \
  --env MCP_POSTGRES_USERNAME \
  --env MCP_POSTGRES_PASSWORD \
  --env MCP_POSTGRES_SSLMODE \
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
        "MCP_POSTGRES_HOST",
        "--env",
        "MCP_POSTGRES_PORT",
        "--env",
        "MCP_POSTGRES_DATABASE",
        "--env",
        "MCP_POSTGRES_USERNAME",
        "--env",
        "MCP_POSTGRES_PASSWORD",
        "--env",
        "MCP_POSTGRES_SSLMODE",
        "ghcr.io/futuretea/postgres-mcp-server:v0.0.1"
      ],
      "env": {
        "MCP_POSTGRES_HOST": "db.example.internal",
        "MCP_POSTGRES_PORT": "5432",
        "MCP_POSTGRES_DATABASE": "app",
        "MCP_POSTGRES_USERNAME": "reader",
        "MCP_POSTGRES_PASSWORD": "<runtime-secret>",
        "MCP_POSTGRES_SSLMODE": "require"
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
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
npx -y @futuretea/postgres-mcp-server@0.0.1 --port 8080
```

Docker 中服务必须监听容器的全部接口；宿主机发布端口仍可只绑定 loopback：

```sh
export MCP_POSTGRES_HOST=db.example.internal
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=require
docker run --rm \
  --env MCP_POSTGRES_HOST \
  --env MCP_POSTGRES_PORT \
  --env MCP_POSTGRES_DATABASE \
  --env MCP_POSTGRES_USERNAME \
  --env MCP_POSTGRES_PASSWORD \
  --env MCP_POSTGRES_SSLMODE \
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

使用分字段环境变量。不要把主机、密码或连接 URL 写进 YAML。这样设置的密码是原始环境变量值，不需要 URL 编码。

| 变量 | libpq 别名 | 是否必填 | 默认值 |
|---|---|---|---|
| `MCP_POSTGRES_HOST` | `PGHOST` | 是 | 无 |
| `MCP_POSTGRES_PORT` | `PGPORT` | 否 | `5432` |
| `MCP_POSTGRES_DATABASE` | `PGDATABASE` | 是 | 无 |
| `MCP_POSTGRES_USERNAME` | `PGUSER` | 是 | 无 |
| `MCP_POSTGRES_PASSWORD` | `PGPASSWORD` | 否 | 空 |
| `MCP_POSTGRES_SSLMODE` | `PGSSLMODE` | 否 | `prefer` |

`MCP_POSTGRES_*` 与本服务已有的 `MCP_` 环境变量前缀一致，也与 ClickHouse 服务的 `MCP_CLICKHOUSE_*` 连接字段一致（`MCP_CLICKHOUSE_DATABASE`、`MCP_CLICKHOUSE_USERNAME`、`MCP_CLICKHOUSE_PASSWORD`）。MySQL 使用 `MYSQL_MCP_*`，是因为该仓库把包括 HTTP 端口在内的每个变量都放在 `MYSQL_MCP_` 前缀下。`MCP_POSTGRES_PORT` 是数据库端口，HTTP 端口仍是 `MCP_PORT`。

每个 `MCP_POSTGRES_*` 变量都是同一行 `PG*` 变量的别名。同一对变量被设成不同的值时，启动失败。错误会写出这两个变量名，但不会打印变量值。`MCP_POSTGRES_*` 是显式连接参数，会覆盖 `PGSERVICE` 中的同名字段。未设置对应的 `MCP_POSTGRES_*` 时，service 文件仍会覆盖 `PG*`，这是 pgx 现有的 libpq 行为。

`PGSERVICE` 可以从 PostgreSQL service 文件提供必填字段。未使用 `DATABASE_URL` 或 `PGSERVICE` 时，host、database、username 必填。缺少必填字段时，错误会列出缺失的变量，例如 `[postgres_invalid_config] missing MCP_POSTGRES_HOST or PGHOST, MCP_POSTGRES_DATABASE or PGDATABASE, MCP_POSTGRES_USERNAME or PGUSER`。

配置只能解析到一个 `host:port`。不同的 URL 目标或逗号分隔的主机会在创建连接前失败。

### `DATABASE_URL`（可选）

`DATABASE_URL` 仍是已有 libpq URL 的高级入口。设置了它时，该 URL 就是连接串，所有 `MCP_POSTGRES_*` 变量都会被忽略。标准 `PG*` 变量仍会补上 URL 里没有的设置；URL 中已有的设置覆盖同名 `PG*` 变量。要使用上面的分字段，请不要设置 `DATABASE_URL`。

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
```

`DATABASE_URL` 中的密码必须做 URL 编码。密码含有保留字符时，请使用 `MCP_POSTGRES_PASSWORD`。

服务启动时要求 PostgreSQL 17 或更新版本，并拒绝 superuser、可绕过行级安全、具备复制权限，或属于服务端文件、服务端程序、后端信号角色的服务账号。请使用独立的最小权限数据库角色。

若无法提供这样的角色，可设置 `allow_unsafe_database_role: true`（环境变量为 `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`）只跳过这一项角色检查。服务仍会把每条语句包在只读事务里，但角色层面的保证不再成立：superuser 能读取你本不想暴露的表、读取服务端文件、执行只读事务约束不到的服务端程序，`BYPASSRLS` 角色会看穿行级安全策略。仅对你掌控的数据库启用。

### 创建只读服务角色

使用数据库管理员账号在 `psql` 中执行下列命令。将 `app`、`public` 替换成实际的数据库和 schema。该命令会创建一个不继承权限的新登录角色；不要复用管理员或应用 owner 角色。

```sql
CREATE ROLE mcp_reader
  LOGIN
  NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOINHERIT NOREPLICATION NOBYPASSRLS;
\password mcp_reader

GRANT CONNECT ON DATABASE app TO mcp_reader;
\connect app
GRANT USAGE ON SCHEMA public TO mcp_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO mcp_reader;
```

### 验证 schema 的有效权限

配置连接前，请确认 `mcp_reader` 无法在目标 schema 中创建对象：

```sql
SELECT has_schema_privilege('mcp_reader', 'public', 'CREATE') AS can_create_objects;
```

结果必须为 `false`。若结果为 `true`，说明该角色在此 schema 中并非只读。若 PostgreSQL 14 或更早版本升级而来，`public` 可能保留共享的 `PUBLIC` `CREATE` 授权，从而出现此结果。不要只为该服务撤销共享的 `PUBLIC` 权限；应先由数据库管理员决定数据库级的 schema 策略。

`\password` 会交互式设置密码，不会把密码写入 shell 历史。连接密钥只应保存在运行环境的 secret 或环境变量管理器中，不能写入仓库。如果还需读取今后创建的表，且由 `postgres` 创建这些表，请用 `postgres` 执行以下可选命令；其他环境请把 `postgres` 替换成实际的对象创建角色：

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public
  GRANT SELECT ON TABLES TO mcp_reader;
```

配置连接前，请用 `mcp_reader` 连接，并确认下面每个布尔值都为 `false`：

```sql
SELECT current_user, rolsuper, rolbypassrls, rolreplication,
  pg_has_role(current_user, 'pg_read_server_files', 'member') AS read_server_files,
  pg_has_role(current_user, 'pg_write_server_files', 'member') AS write_server_files,
  pg_has_role(current_user, 'pg_execute_server_program', 'member') AS execute_server_program,
  pg_has_role(current_user, 'pg_signal_backend', 'member') AS signal_backend
FROM pg_roles
WHERE rolname = current_user;
```

若任一值为 `true`，请新建专用角色；不要为了 MCP 从共享管理员或应用角色中移除权限。

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
docker run --rm -i \
  --env MCP_POSTGRES_HOST \
  --env MCP_POSTGRES_PORT \
  --env MCP_POSTGRES_DATABASE \
  --env MCP_POSTGRES_USERNAME \
  --env MCP_POSTGRES_PASSWORD \
  --env MCP_POSTGRES_SSLMODE \
  postgres-mcp-server:dev
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
