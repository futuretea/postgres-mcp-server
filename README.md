# PostgreSQL Read-only MCP Server

An MCP (Model Context Protocol) server for discovering one PostgreSQL 17+ instance and running a deliberately narrow SELECT subset. It exposes MCP over stdio or HTTP and includes a CLI.

## Quick start

Set a connection target for a minimally privileged PostgreSQL role, then build and inspect the catalog:

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
make build
./bin/postgres-mcp-server tools list
./bin/postgres-mcp-server mcp
```

`tools list` and `tools describe` do not read database connection settings. Starting MCP/HTTP or calling a tool validates the connection and verifies the server and role before it reports ready.

## Use a release

Release `v0.0.1` is available as an npm package and a multi-architecture Docker image. Both release entry points start MCP over stdio by default.

### npm

Run the published package with `npx`:

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
npx -y @futuretea/postgres-mcp-server@0.0.1
```

For an MCP client configuration, use:

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

The npm command is an MCP entry point, not a wrapper for the server's other CLI subcommands. Pin the package version you intend to run.

### Docker

Run the published image directly:

```sh
export DATABASE_URL='postgres://reader@db.example.internal:5432/app?sslmode=require'
docker run --rm -i \
  --env DATABASE_URL \
  ghcr.io/futuretea/postgres-mcp-server:v0.0.1
```

For an MCP client that launches Docker, pass the connection setting through the Docker process:

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

The database hostname must be reachable from the container. `localhost` inside the container is not the host machine; use a database hostname on the Docker network, or the platform-specific host gateway when appropriate.

The Docker examples use `sslmode=require` for a remote database. Set the PostgreSQL TLS mode and certificates required by your deployment.

### HTTP mode

Pass MCP flags after the release command to start HTTP instead of stdio. On the host, the default listener is loopback:

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
npx -y @futuretea/postgres-mcp-server@0.0.1 --port 8080
```

For Docker, the server must listen on all container interfaces, while the published port can remain loopback-only on the host:

```sh
export DATABASE_URL='postgres://reader@db.example.internal:5432/app?sslmode=require'
docker run --rm \
  --env DATABASE_URL \
  --publish 127.0.0.1:8080:8080 \
  ghcr.io/futuretea/postgres-mcp-server:v0.0.1 \
  --port 8080 --listen 0.0.0.0
```

Use `http://127.0.0.1:8080/mcp` for Streamable HTTP MCP clients. The server also provides SSE at `/sse` with messages at `/message`, and `GET /healthz` for health checks. HTTP has no authentication or TLS; keep it on a trusted network or place it behind infrastructure that provides both.

## Tools

| Tool | Parameters | Purpose |
|---|---|---|
| `postgres_get_server_info` | none | Return server and current connection identity. |
| `postgres_list_databases` | none | List visible databases. |
| `postgres_list_schemas` | none | List visible schemas. |
| `postgres_list_tables` | `schema` | List tables and views in a schema. |
| `postgres_describe_table` | `schema`, `table` | Describe table columns. |
| `postgres_list_indexes` | `schema`, `table` | List indexes for a table. |
| `postgres_query` | `sql` | Run one strict read-only SELECT query. |

Every successful tool response is one JSON text envelope with `schema_version`, `tool`, and `data`. Parameters must be a JSON object with no unknown keys. Empty-parameter tools require `{}`; missing, `null`, array, or scalar arguments are rejected.

## Database connection

Use a bare `DATABASE_URL`, or standard PostgreSQL environment variables such as `PGHOST`, `PGPORT`, `PGDATABASE`, `PGUSER`, `PGPASSWORD`, and `PGSSLMODE`. When `DATABASE_URL` is set it takes precedence. Configuration must resolve to one `host:port`; distinct URL or comma-separated `PGHOST` targets are rejected before a connection is created.

At startup the server requires PostgreSQL 17 or newer and rejects service roles that are superusers, bypass row-level security, have replication, or belong to server-file, server-program, or backend-signalling roles. Use a dedicated, least-privilege database role.

### Create a read-only service role

Run the following in `psql` as a database administrator. Replace `app`, `public`, and `app_owner` with your database, schema, and object-owner role. This creates a fresh login role with no inherited privileges; do not reuse an administrator or application-owner role.

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

The `\password` command prompts without placing the password in shell history. Store the resulting connection secret only in the runtime secret or environment manager, not in the repository. If the role must read tables created in the future, run this optional command as the role that creates those tables:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE app_owner IN SCHEMA public
  GRANT SELECT ON TABLES TO mcp_reader;
```

Before configuring `DATABASE_URL`, connect as `mcp_reader` and verify that every boolean below is `false`:

```sql
SELECT current_user, rolsuper, rolbypassrls, rolreplication,
  pg_has_role(current_user, 'pg_read_server_files', 'member') AS read_server_files,
  pg_has_role(current_user, 'pg_write_server_files', 'member') AS write_server_files,
  pg_has_role(current_user, 'pg_execute_server_program', 'member') AS execute_server_program,
  pg_has_role(current_user, 'pg_signal_backend', 'member') AS signal_backend
FROM pg_roles
WHERE rolname = current_user;
```

If any value is `true`, use a new dedicated role rather than removing privileges from a shared administrator or application role.

## Read-only boundary and deployment risk

Each database operation uses a PostgreSQL read-only transaction and pgx's extended execution protocol. `postgres_query` is parsed as exactly one SELECT, rejecting DML, DDL, row locks, SELECT INTO, modifying CTEs, and selected PostgreSQL built-ins with known side effects.

This is not a complete proof that every user-defined function, extension, FDW, or future PostgreSQL built-in is harmless. Grant only the database access you intend the server to have.

HTTP transports have no authentication or TLS layer. The default listener is loopback. If you set `--listen` to a remote address, place the service only on a trusted network or behind infrastructure that provides the required protection. This server intentionally has no query timeout, row limit, response-size limit, or concurrency budget.

## Running MCP and HTTP

```sh
# stdio MCP
./bin/postgres-mcp-server mcp

# HTTP MCP on loopback
./bin/postgres-mcp-server mcp --port 8080

# Explicit remote listener; see the trusted-network requirement above
./bin/postgres-mcp-server mcp --port 8080 --listen 0.0.0.0
```

The HTTP health endpoint is `GET /healthz`. See `config.example.yaml` for transport and tool-filter settings; database connection settings remain in the environment.

## CLI

```sh
./bin/postgres-mcp-server tools list --json
./bin/postgres-mcp-server tools describe postgres_query --json
./bin/postgres-mcp-server tools call postgres_list_schemas --params '{}'
./bin/postgres-mcp-server tools call postgres_query --params '{"sql":"SELECT 1"}'
./bin/postgres-mcp-server version
```

## Container

`make docker` first builds the Alpine CGO parser-check target and then builds the runnable image:

```sh
make docker
docker run --rm -i --env DATABASE_URL postgres-mcp-server:dev
```

The image entrypoint is `postgres-mcp-server mcp`.

## Development checks

```sh
make format
make lint
make test
make test-integration  # requires a running Docker daemon; creates a PostgreSQL 17 fixture
make build
make docker
```
