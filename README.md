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
