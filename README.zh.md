# PostgreSQL 只读 MCP Server

[English](README.md)

源码和 issue 在 [GitHub 仓库](https://github.com/futuretea/postgres-mcp-server)。许可证是 [MIT](LICENSE)。

这是一个面向单个 PostgreSQL 14 或更新版本实例的 MCP（Model Context Protocol）服务。它提供数据库目录发现和受限的 SELECT 查询，可通过 stdio 或 HTTP 使用，也提供 CLI。

## 快速开始

先为最小权限 PostgreSQL 角色设置分字段连接配置，再构建并查看工具目录。有效 UTF-8 密码是普通环境变量值，不需要 URL 编码。不是有效 UTF-8 的密码会在连接前被拒绝，错误信息不会包含密码。

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

[Release v0.4.0](https://github.com/futuretea/postgres-mcp-server/releases/tag/v0.4.0) 是当前 npm 包和当前 GHCR 镜像。npm dist-tag `latest` 与镜像标签 `latest` 都指向该版本。两个发布入口默认都通过 stdio 启动 MCP。

GitHub Release `v0.0.1` 到 `v0.4.0` 都没有附带可执行文件。每个页面有生成的说明，以及该 Git tag 的源码 tarball 和 zipball。预编译程序是 npm 平台包和 GHCR 镜像。也可以从该 tag 自行构建二进制。

本节示例不设置 `DATABASE_URL`，使用分字段连接变量。一旦设置 `DATABASE_URL`，所有 `MCP_POSTGRES_*` 变量都会被忽略。shell 示例使用 POSIX `export`。在 Windows 上，请在进程环境或客户端 `env` 对象里设置同名变量。

### npm

包装包是 [`@futuretea/postgres-mcp-server`](https://www.npmjs.com/package/@futuretea/postgres-mcp-server)。`0.4.0` 发布下列平台包。包装器按 Node 的 `process.platform` 和 `process.arch` 选择其中一个：

| 包 | Node platform | Node arch |
|---|---|---|
| `@futuretea/postgres-mcp-server-darwin-amd64` | `darwin` | `x64` |
| `@futuretea/postgres-mcp-server-darwin-arm64` | `darwin` | `arm64` |
| `@futuretea/postgres-mcp-server-linux-amd64` | `linux` | `x64` |
| `@futuretea/postgres-mcp-server-linux-arm64` | `linux` | `arm64` |
| `@futuretea/postgres-mcp-server-windows-amd64` | `win32` | `x64` |

其它组合会以 `Unsupported platform/architecture: <platform>/<arch>` 停止。匹配的平台包未安装时，包装器以 `Could not resolve binary path for platform/architecture: <platform>/<arch>` 停止。

已发布的 `postgres-mcp-server` 命令总会先插入 `mcp` 子命令，再追加你传入的参数。它不运行 `tools`、`version` 或 `completion`。这些命令在 `make build` 生成的二进制上。它们也在平台包文件 `bin/postgres-mcp-server` 上，Windows 上是 `bin/postgres-mcp-server.exe`。

flag 会被传给 `mcp`。`--help` 打印 `mcp` 帮助，并且不会连接数据库。`version` 这样的位置参数不是另一个子命令。进程仍会启动 `mcp`，并校验数据库连接。

已发布的 `0.4.0` npm 包内的 README 仍固定写着 `v0.0.1`。配置 `0.4.0` 时请使用本文。

本版本请固定 `@0.4.0`。进程在报告 ready 之前会连接数据库，并检查服务端版本和角色：

```sh
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
npx -y @futuretea/postgres-mcp-server@0.4.0
```

MCP 客户端配置示例：

```json
{
  "mcpServers": {
    "postgres-readonly": {
      "command": "npx",
      "args": ["-y", "@futuretea/postgres-mcp-server@0.4.0"],
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

该客户端启动一个 stdio 进程。进程通过 stdin 和 stdout 使用 MCP。没有 authorization header、API key 或客户端证书。

`npx -y @futuretea/postgres-mcp-server@0.4.0 --help` 打印 `mcp` 帮助，不会打开数据库连接。

### Docker

镜像仓库是 [`ghcr.io/futuretea/postgres-mcp-server`](https://github.com/futuretea/postgres-mcp-server/pkgs/container/postgres-mcp-server)。标签 `v0.4.0` 和 `latest` 是同一个镜像索引。可运行平台是 `linux/amd64` 和 `linux/arm64`。索引里还有构建 attestation manifest。那些条目不是可运行镜像。

已发布镜像以用户 `mcp` 运行。入口是 `/usr/local/bin/postgres-mcp-server mcp`。镜像名后面的参数是 `mcp` 的 flag。

数据库主机名必须能从容器访问。容器内的 `localhost` 不是宿主机。请使用 Docker 网络内的数据库主机名；数据库在宿主机上时，使用平台对应的宿主机网关。下面的远程示例设置 `sslmode=require`。请按部署环境需要设置 PostgreSQL TLS 模式和证书。

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
  ghcr.io/futuretea/postgres-mcp-server:v0.4.0
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
        "ghcr.io/futuretea/postgres-mcp-server:v0.4.0"
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

### HTTP 模式

在发布命令后传入 MCP 参数，就会启动 HTTP 而不是 stdio。直接在宿主机运行时，默认监听 loopback。进程在报告 ready 之前仍会连接数据库：

```sh
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
npx -y @futuretea/postgres-mcp-server@0.4.0 --port 8080
```

Docker 中服务必须监听容器的全部接口。宿主机发布端口仍可只绑定 loopback：

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
  ghcr.io/futuretea/postgres-mcp-server:v0.4.0 \
  --port 8080 --listen 0.0.0.0
```

Streamable HTTP 客户端使用 `http://127.0.0.1:8080/mcp`。该传输不保存会话，也不返回 session id。SSE 客户端使用 `http://127.0.0.1:8080/sse`，并向 `/message` 发送消息。SSE 响应会把原始请求的 query 追加到消息端点 URL。

进程在提供服务时，`GET /healthz` 和 `HEAD /healthz` 返回 `200`，正文是 `healthy`。其它方法返回 `405`。

HTTP 没有认证或 TLS。服务不检查 authorization header、API key 或客户端证书。只能部署在可信网络，或置于同时提供认证和 TLS 的基础设施之后。

HTTP 服务的读头超时是 5 秒，读超时是 30 秒，没有写超时，空闲超时是 120 秒。这些不是 SQL 语句超时。

`--sse-base-url` 设置 SSE 消息端点的公开 base URL。该值必须是带 host、且没有 query string 的绝对 `http` 或 `https` URL。不符合的值会被忽略，base URL 保持为空。客户端可以使用请求 host 时，不要设置该 flag。

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

所有成功响应都是一个 JSON text envelope，固定含有 `schema_version`、`tool` 和 `data`。`schema_version` 是数字 `1`。参数必须是没有未知字段的 JSON object。无参数工具必须传 `{}`。缺失、`null`、数组和标量都会被拒绝。

### 结果字段

| 工具 | `data` |
|---|---|
| `postgres_get_server_info` | `server_version`、`server_version_num`、`database`、`user`、`host`、`port` |
| `postgres_list_databases` | `databases`，每项含 `name`、`owner`、`encoding` |
| `postgres_list_schemas` | `schemas`，每项含 `name` |
| `postgres_list_tables` | `schema`，以及 `tables`，每项含 `name`、`kind` |
| `postgres_describe_table` | `schema`、`table`，以及 `columns`，每项含 `ordinal_position`、`name`、`postgres_type`、`nullable`、`default`、`identity`、`generated` |
| `postgres_list_indexes` | `schema`、`table`，以及 `indexes`，每项含 `name`、`definition` |
| `postgres_query` | `columns`，每项含 `name`、`postgres_type`，以及作为单元格值数组的 `rows` |

脱敏关闭时，查询单元格和大多数元数据字段是 PostgreSQL 文本；SQL 值为 null 时对应 JSON null。`ordinal_position` 是 JSON number。`nullable` 是 JSON boolean。`inet_server_addr()` 为 null 时，`host` 和 `port` 为 null。列不是 identity 列时，`identity` 为 null。列不是 generated 列时，`generated` 为 null。

## 数据库连接

使用分字段环境变量。不要把主机、密码或连接 URL 写进 YAML。有效 UTF-8 密码不需要 URL 编码。

| 变量 | libpq 别名 | 未设置 `DATABASE_URL` 或 `PGSERVICE` 时是否必填 | 默认值 |
|---|---|---|---|
| `MCP_POSTGRES_HOST` | `PGHOST` | 是 | 无 |
| `MCP_POSTGRES_PORT` | `PGPORT` | 否 | `5432` |
| `MCP_POSTGRES_DATABASE` | `PGDATABASE` | 是 | 无 |
| `MCP_POSTGRES_USERNAME` | `PGUSER` | 是 | 无 |
| `MCP_POSTGRES_PASSWORD` | `PGPASSWORD` | 否 | 空 |
| `MCP_POSTGRES_SSLMODE` | `PGSSLMODE` | 否 | `prefer` |

`MCP_POSTGRES_*` 与本服务已有的 `MCP_` 环境变量前缀一致，也与 ClickHouse 服务的 `MCP_CLICKHOUSE_*` 连接字段一致（`MCP_CLICKHOUSE_DATABASE`、`MCP_CLICKHOUSE_USERNAME`、`MCP_CLICKHOUSE_PASSWORD`）。MySQL 使用 `MYSQL_MCP_*`，是因为该仓库把包括 HTTP 端口在内的每个变量都放在 `MYSQL_MCP_` 前缀下。`MCP_POSTGRES_PORT` 是数据库端口，HTTP 端口仍是 `MCP_PORT`。

每个 `MCP_POSTGRES_*` 变量都是同一行 `PG*` 变量的别名。同一对变量被设成不同的值时，启动失败。错误会写出这两个变量名，但不会打印变量值。`MCP_POSTGRES_*` 是显式连接参数，会覆盖 `PGSERVICE` 中的同名字段。未设置对应的 `MCP_POSTGRES_*` 时，service 文件仍会覆盖 `PG*`，这是 pgx 现有的 libpq 行为。

未设置 `MCP_POSTGRES_PASSWORD` 时，仍使用 `PGPASSWORD` 和 `.pgpass`。空的 `PGPASSWORD` 视为未设置，与 libpq 一致。把 `MCP_POSTGRES_PASSWORD` 设成空字符串是显式空密码：它覆盖 `PGPASSWORD` 和 `.pgpass`。这个空值与非空 `PGPASSWORD` 同时存在时，启动失败，错误只写出这两个变量名，不打印值。

最终解析出的密码必须是有效 UTF-8。这包括 `MCP_POSTGRES_PASSWORD`、`PGPASSWORD`、`DATABASE_URL` 中的密码、service 文件密码和 `.pgpass` 密码。无效 UTF-8 会以 `[postgres_invalid_config] password must be valid UTF-8` 失败，且不会打印密码。连接配置里保留原始字节并不等于认证成功：pgx 的 SCRAM 准备会把这些字节替换成 U+FFFD，从而发送另一个密码。

`PGSERVICE` 可以从 PostgreSQL service 文件提供必填字段。未使用 `DATABASE_URL` 或 `PGSERVICE` 时，host、database、username 必填。这三个字段不使用 libpq 的默认值，因此缺少其中一项的部分 `PG*` 配置会失败。缺少必填字段时，错误会列出缺失的变量，例如 `[postgres_invalid_config] missing MCP_POSTGRES_HOST or PGHOST, MCP_POSTGRES_DATABASE or PGDATABASE, MCP_POSTGRES_USERNAME or PGUSER`。

配置只能解析到一个 `host:port`。不同的 URL 目标或逗号分隔的主机会在创建连接前失败。

### `DATABASE_URL`（可选）

`DATABASE_URL` 仍是已有 libpq URL 的高级入口。设置了它时，该 URL 就是连接串，所有 `MCP_POSTGRES_*` 变量都会被忽略。标准 `PG*` 变量仍会补上 URL 里没有的设置；URL 中已有的设置覆盖同名 `PG*` 变量。要使用上面的分字段，请不要设置 `DATABASE_URL`。

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
```

`DATABASE_URL` 中的密码必须做 URL 编码。密码含有保留字符时，请使用 `MCP_POSTGRES_PASSWORD`。

服务启动时要求 PostgreSQL 14 或更新版本，并拒绝 superuser、可绕过行级安全、具备复制权限，或属于服务端文件、服务端程序、后端信号角色的服务账号。请使用独立的最小权限数据库角色。

版本检查读取 `server_version_num`，低于 `140000` 时以 `[postgres_unsupported_server_version] PostgreSQL 14 or later is required` 失败。打开不安全角色设置时，这项检查仍然执行。被拒绝的角色以 `[postgres_unsafe_service_role] database service role is not permitted` 失败。集成测试夹具只接受 `postgres:14-alpine` 和 `postgres:17-alpine`。`make test-integration` 会拒绝其它 `POSTGRES_IMAGE`。GitHub 测试作业会跑这两个镜像。

若无法提供这样的角色，可设置 `allow_unsafe_database_role: true`（环境变量为 `MCP_ALLOW_UNSAFE_DATABASE_ROLE=true`）只跳过这一项角色检查。`true`、`TRUE` 和 `1` 会跳过检查。`false` 保持检查开启。这项设置没有命令行 flag。服务仍会把每条语句包在只读事务里，但角色层面的保证不再成立：superuser 能读取你本不想暴露的表、读取服务端文件、执行只读事务约束不到的服务端程序，`BYPASSRLS` 角色会看穿行级安全策略。仅对你掌控的数据库启用。

### 创建只读服务角色

使用数据库管理员账号在 `psql` 中执行下列命令。将 `app`、`mcp_probe` 替换成实际的数据库和 schema。该命令会创建一个不继承权限的新登录角色；不要复用管理员或应用 owner 角色。

```sql
CREATE ROLE mcp_reader
  LOGIN
  NOSUPERUSER NOCREATEDB NOCREATEROLE
  NOINHERIT NOREPLICATION NOBYPASSRLS;
\password mcp_reader

GRANT CONNECT ON DATABASE app TO mcp_reader;
\connect app
CREATE SCHEMA mcp_probe;
GRANT USAGE ON SCHEMA mcp_probe TO mcp_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA mcp_probe TO mcp_reader;
```

### 验证 schema 的有效权限

配置连接前，请确认专用 schema，而不是 `public`：

```sql
SELECT has_schema_privilege('mcp_reader', 'mcp_probe', 'CREATE') AS can_create_objects,
       has_schema_privilege('mcp_reader', 'mcp_probe', 'USAGE') AS can_use_objects;
```

`can_create_objects` 必须为 `false`，`can_use_objects` 必须为 `true`。在全新数据库上，`has_schema_privilege('mcp_reader', 'public', 'CREATE')` 在 PostgreSQL 14 上为 `true`，在 PostgreSQL 17 上为 `false`。这是服务器默认授权。不要只为该服务撤销共享的 `PUBLIC` 权限；应先由数据库管理员决定数据库级的 schema 策略。

`\password` 会交互式设置密码，不会把密码写入 shell 历史。连接密钥只应保存在运行环境的 secret 或环境变量管理器中，不能写入仓库。如果还需读取今后创建的表，且由 `postgres` 创建这些表，请用 `postgres` 执行以下可选命令；其他环境请把 `postgres` 替换成实际的对象创建角色：

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA mcp_probe
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

## 输出脱敏

未配置 `masking`，或 `enabled` 省略、为 false 时，脱敏关闭，工具输出保持原有文本。只有 `enabled: true` 且至少有一条规则时才会启用。环境变量 `MCP_MASKING_JSON` 只要存在，就整份替换 YAML 里的 `masking` 对象。空值无效。未设置该变量时仍使用 YAML。

空文档、未知字段，以及 `enabled: true` 但没有任何规则，都会让进程启动失败。错误文本是 `invalid masking config`，不包含配置原文。

`names`、`name_patterns`、`values` 和 `value_patterns` 各自最多 32 项。名字最多 128 字节。值字面量最多 256 字节。模式最多 128 字节。空项无效。占位符为 1 到 64 字节，默认是 `[masked]`。精确的 `names` 区分大小写。

脱敏开启时，一次结果最多访问 10000 个节点，单次扫描的输入最多 1 MiB，编码后的 JSON 结果最多 1 MiB。失败时返回 `[postgres_masking_failed] result was not returned`，不会返回半份 JSON。未知工具名会报告为 `[postgres_invalid_params] invalid parameters`，因此错误不会回显该名字。

```yaml
masking:
  enabled: true
  placeholder: "[masked]"
  names: ["synthetic_field"]
  name_patterns: ["token_.+"]
  values: ["synthetic-token-value"]
  value_patterns: ["sk_live_[0-9]+"]
```

```sh
export MCP_MASKING_JSON='{"enabled":true,"names":["synthetic_field"],"values":["synthetic-token-value"]}'
```

`names` 和 `name_patterns` 匹配完整字段名，也匹配这些工具返回的数据库名、schema 名、表名、列名、索引名、所有者和角色名。名字模式锚定整段名字。`a)|(b` 会被拒绝。`(?:a)|(b)` 只匹配完整名字 `a` 和 `b`。`values` 和 `value_patterns` 是子串匹配，包括单字符字面量，并且只替换一次。占位符不会再次被搜索。单字符字面量可能命中普通单词。

命中的字符串换成占位符，默认是 `[masked]`。命中的非字符串变成 JSON null。原本的 SQL NULL 仍是 null。结果里的 null 可能是库内 NULL，也可能是被隐去的值。规则命中时，JSON 里的类型和可空性可能改变。`ordinal_position` 和 `nullable` 在整值未命中时仍分别是数字和布尔值。

列类型决定用哪条规则。`text` 和其他普通字符串始终按字符串处理，即使每个字符都是数字。命中的只是那段子串，换成占位符。`smallint`、`integer`、`bigint`、`real`、`double precision`、`numeric` 和 `boolean` 使用 PostgreSQL 发来的原始文本。值规则只要命中这段文本，整个值变成 null，不会在数字中间改写。文本为 `t` 的布尔单元格不会命中针对 `true` 的规则。未命中的数字保留原来的字面量。

整段都是 JSON 对象或数组的 JSON、JSONB 单元格会按结构脱敏，包括转义后的键和字符串里再嵌套的 JSON，最多两轮“字符串里的 JSON”。更深的嵌套会使该次结果失败。只是看起来像 JSON 的文本仍按文本处理，只做值匹配。命中值规则的 JSON 数字或布尔值变成 null。未命中的数字保留原来的字面量，包括末尾的零。

`bytea` 只按列类型识别。已知的 `\x` 十六进制会先解码、再脱敏、然后重新写成十六进制。超过 1 MiB 的十六进制单元格在解码前失败。值模式也会作用在非法 UTF-8 内部的 ASCII 文本上。转义格式的 `bytea`、非法十六进制和 `bytea[]` 在脱敏开启时会使该次结果失败，原始字节不会返回。看起来像十六进制的 text 不会被当作 `bytea` 解码。超过长度限制的脱敏结果会使该次调用失败。

不消耗字符就能匹配的值模式会在启动时被拒绝，例如 `\b|sk_live_4242`。错误是 `invalid masking config`，不包含模式原文。这里复制的共享脱敏包仍在等待独立复审。本服务不把那次复审说成已经完成。

这些工具不返回列注释或表注释。索引表达式在 `definition` 字符串里，默认值在 `default` 字符串里。名字规则不搜索这些字符串内部，值规则会。

`schema_version` 和工具名是服务端常量，不参与扫描。固定公开错误和占位符也不扫描。同时出现在其中的短字面量仍可能看见。脱敏不能发现每一种私密值、压缩载荷或编码。

无法完成脱敏时，包括名字冲突、不支持的结果形状或超过长度限制，调用失败并返回 `[postgres_masking_failed] result was not returned`，不会返回半份 JSON。脱敏只改变发出的文本。发给 PostgreSQL 的 SQL 和库里的数据保持不变。

HTTP 调试日志只记录路由类别（`mcp`、`sse`、`message` 或 `other`），不记录请求路径或 query。

## 只读边界与部署风险

每次数据库操作都在 PostgreSQL read-only transaction 和 pgx extended execution protocol 中执行。`postgres_query` 会解析为恰好一条 SELECT，并拒绝 DML、DDL、行锁、SELECT INTO、修改型 CTE 以及一组已知有副作用的 PostgreSQL 内建函数。被拒绝的语句返回 `[postgres_query_rejected] query is not an allowed SELECT statement`。被拒绝的内建函数名和前缀是 [`internal/postgres/strict_select.go`](internal/postgres/strict_select.go) 里的固定列表。

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

`GET /healthz` 和 `HEAD /healthz` 是健康检查端点。transport 和工具过滤配置见 [`config.example.yaml`](config.example.yaml)。数据库连接仍只从环境变量读取。

显式 flag 覆盖同一设置的环境变量。环境变量覆盖 YAML。YAML 覆盖默认值。未传入的 flag 不会挡住环境变量或配置文件。

| YAML 键 | Flag | 环境变量 | 默认值 |
|---|---|---|---|
| `port` | `--port` | `MCP_PORT` | `0` |
| `listen` | `--listen` | `MCP_LISTEN` | `127.0.0.1` |
| `sse_base_url` | `--sse-base-url` | `MCP_SSE_BASE_URL` | 空 |
| `log_level` | `--log-level` | `MCP_LOG_LEVEL` | `info` |
| `enabled_tools` | `--enabled-tools` | `MCP_ENABLED_TOOLS` | 空 |
| `disabled_tools` | `--disabled-tools` | `MCP_DISABLED_TOOLS` | 空 |
| `enabled_domains` | `--enable-domains` | `MCP_ENABLED_DOMAINS` | 空 |
| `disabled_domains` | `--disable-domains` | `MCP_DISABLED_DOMAINS` | 空 |
| `allow_unsafe_database_role` | 无 | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` |

`port` 必须在 0 到 65535 之间。`0` 保持 stdio。其它端口启动 HTTP，且必须设置 `listen`。日志级别是 `trace`、`debug`、`info`、`warn`、`error`、`fatal`、`panic` 和 `disabled`。stdio 模式丢弃日志，使 MCP 消息留在 stdout。HTTP 模式把日志写到 stderr。

`MCP_ENABLED_TOOLS` 和其它列表变量用逗号分隔。空格属于同一个名字，不是分隔符。空的启用列表允许全部工具或 domain。非空启用列表是允许列表。禁用列表随后移除匹配项。唯一的 domain 是 `postgres`。过滤器去掉全部工具时，启动失败，错误是 `no tools registered; check enabled_tools / disabled_tools / domain filters`。`tools list` 使用同一过滤器，并且仍然不连接数据库。

## CLI

这些命令属于你构建的二进制，或平台包里的二进制。npm 包装命令不会运行它们，因为它总会插入 `mcp`。

```sh
./bin/postgres-mcp-server tools list --json
./bin/postgres-mcp-server tools describe postgres_query --json
./bin/postgres-mcp-server tools call postgres_list_schemas --params '{}'
./bin/postgres-mcp-server tools call postgres_query --params '{"sql":"SELECT 1"}'
./bin/postgres-mcp-server tools call postgres_query --params-file ./query.json
./bin/postgres-mcp-server version
./bin/postgres-mcp-server completion bash
```

`--params` 默认为 `{}`。`--params-file -` 从 stdin 读取 JSON object。`version` 打印 `postgres-mcp-server version=<version> commit=<commit> date=<date>`。`completion` 写出 bash、zsh、fish 或 powershell 脚本。`completion install` 打印要加入 shell 配置文件的行。`tools` 也可以用别名 `tool`。

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

镜像入口是 `/usr/local/bin/postgres-mcp-server mcp`。

## 构建、测试与贡献

Go 1.25.10 与 `go.mod`、`.tool-versions` 和 CI 一致。SQL 解析包使用 cgo，因此构建需要 C 编译器。`make build` 写出 `bin/postgres-mcp-server`。版本来自 `git describe --tags --always --dirty`，commit 来自 `HEAD`，日期是当前 UTC 时间。干净检出的 tag `v0.4.0` 会打印 `version=v0.4.0`。工作区有未提交改动时会追加 `-dirty`。不带这些链接参数的 `go build` 会打印 `version=dev`、`commit=unknown` 和 `date=unknown`。

```sh
make format
make lint
make test
make test-integration  # 需要 Docker；默认 postgres:17-alpine。POSTGRES_IMAGE=postgres:14-alpine 选择 14。
make build
make docker
```

`make lint` 运行 `go vet` 并检查 `gofmt`。只有 `golangci-lint` 在 `PATH` 上时才会运行它。CI 使用 golangci-lint v2.12.2。`make test` 是 `go test ./...`。GitHub Build 工作流还会运行 race detector，以及 PostgreSQL 14 和 17 两套集成夹具。`make ci` 运行 lint、单元测试、默认的 PostgreSQL 17 夹具和 `make build`。当改动的每个文件都是 `.gitignore`、`LICENSE`，或仓库根目录的 Markdown 文件时，Build 工作流不会启动。

缺陷或改动请开一个 [issue](https://github.com/futuretea/postgres-mcp-server/issues)。

## 发布

稳定 tag `vX.Y.Z` 的一次推送会运行 npm 和 Docker 发布工作流。带 prerelease 后缀的 tag 不符合 tag 检查。

npm 工作流为 `darwin/amd64`、`darwin/arm64`、`linux/amd64`、`linux/arm64` 和 `windows/amd64` 构建 cgo 二进制。它发布 `@futuretea/postgres-mcp-server` 和五个平台包，并创建一个带生成说明的 GitHub Release。该发布步骤不上传这些二进制。Release 的资源列表保持为空。

Docker 工作流为 `linux/amd64` 和 `linux/arm64` 推送 `ghcr.io/futuretea/postgres-mcp-server:vX.Y.Z` 和 `:latest`。

发布二进制里的版本字符串是去掉前导 `v` 的 tag。npm 工作流记录该 tag commit 的 committer date。Docker 工作流记录镜像构建时间。
