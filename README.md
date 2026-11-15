# PostgreSQL Read-only MCP Server

[中文](README.zh.md)

An MCP (Model Context Protocol) server for discovering one PostgreSQL 17+ instance and running a deliberately narrow SELECT subset. It exposes MCP over stdio or HTTP and includes a CLI.

## Quick start

Set discrete connection fields for a minimally privileged PostgreSQL role, then build and inspect the catalog. A valid UTF-8 password is a normal environment value here and does not need URL encoding. A password that is not valid UTF-8 is rejected before connect, and the error does not include the password.

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

For an MCP client that launches Docker, pass the connection settings through the Docker process:

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

Set discrete environment variables. Do not put the host, password, or a connection URL in YAML. A valid UTF-8 password does not need URL encoding.

| Variable | libpq alias | Required without `DATABASE_URL` or `PGSERVICE` | Default |
|---|---|---|---|
| `MCP_POSTGRES_HOST` | `PGHOST` | yes | none |
| `MCP_POSTGRES_PORT` | `PGPORT` | no | `5432` |
| `MCP_POSTGRES_DATABASE` | `PGDATABASE` | yes | none |
| `MCP_POSTGRES_USERNAME` | `PGUSER` | yes | none |
| `MCP_POSTGRES_PASSWORD` | `PGPASSWORD` | no | empty |
| `MCP_POSTGRES_SSLMODE` | `PGSSLMODE` | no | `prefer` |

`MCP_POSTGRES_*` matches this server's `MCP_` environment prefix and the ClickHouse server's `MCP_CLICKHOUSE_*` connection fields (`MCP_CLICKHOUSE_DATABASE`, `MCP_CLICKHOUSE_USERNAME`, `MCP_CLICKHOUSE_PASSWORD`). MySQL uses `MYSQL_MCP_*` because that repository prefixes every variable, including its HTTP port, with `MYSQL_MCP_`. `MCP_POSTGRES_PORT` is the database port. The HTTP port remains `MCP_PORT`.

Each `MCP_POSTGRES_*` variable is an alias of the `PG*` variable in the same row. If both names are set to different values, startup fails. The error names the two variables and does not print their values. `MCP_POSTGRES_*` is an explicit connection parameter, so it overrides a `PGSERVICE` entry for that field. When the `MCP_POSTGRES_*` alias is unset, a service file still overrides `PG*`, which is pgx's existing libpq behavior.

An unset `MCP_POSTGRES_PASSWORD` leaves `PGPASSWORD` and `.pgpass` in place. An empty `PGPASSWORD` is unset, matching libpq. Setting `MCP_POSTGRES_PASSWORD` to an empty string is an explicit empty password: it overrides `PGPASSWORD` and `.pgpass`. If that empty value and a non-empty `PGPASSWORD` are both set, startup fails and the error names the two variables without printing them.

The resolved password must be valid UTF-8. This applies to `MCP_POSTGRES_PASSWORD`, `PGPASSWORD`, a `DATABASE_URL` password, a service-file password, and a `.pgpass` password. Invalid UTF-8 fails configuration with `[postgres_invalid_config] password must be valid UTF-8` and the value is not printed. Keeping the original bytes in the connection config is not authentication: pgx's SCRAM preparation replaces those bytes with U+FFFD and would send a different password.

`PGSERVICE` can supply the required fields from a PostgreSQL service file. Without `DATABASE_URL` or `PGSERVICE`, host, database, and username are required. Libpq defaults for those three fields are not used, so a partial `PG*` setup that omits one of them fails. A missing required field produces an error that names the absent variables, for example `[postgres_invalid_config] missing MCP_POSTGRES_HOST or PGHOST, MCP_POSTGRES_DATABASE or PGDATABASE, MCP_POSTGRES_USERNAME or PGUSER`.

Configuration must resolve to one `host:port`. Distinct URL targets or comma-separated hosts are rejected before a connection is created.

### `DATABASE_URL` (optional)

`DATABASE_URL` remains an advanced alternative for a libpq URL you already have. When it is set, that URL is the connection string and every `MCP_POSTGRES_*` variable is ignored. Standard `PG*` variables still fill settings the URL omits, and a setting present in the URL overrides the same `PG*` variable. Leave `DATABASE_URL` unset to use the discrete fields above.

```sh
export DATABASE_URL='postgres://reader@127.0.0.1:5432/app?sslmode=disable'
```

A password in `DATABASE_URL` must be URL-encoded. Prefer `MCP_POSTGRES_PASSWORD` when the password contains reserved characters.

At startup the server requires PostgreSQL 17 or newer and rejects service roles that are superusers, bypass row-level security, have replication, or belong to server-file, server-program, or backend-signalling roles. Use a dedicated, least-privilege database role.

If you cannot provision such a role, set `allow_unsafe_database_role: true` (`MCP_ALLOW_UNSAFE_DATABASE_ROLE=true` for the environment) to skip only this role check. The server still wraps every statement in a read-only transaction, but the role-level guarantee is gone: a superuser role can read tables you did not intend to expose, read server files, and run server-side programs that a read-only transaction does not constrain, and a `BYPASSRLS` role sees through row-level security policies. Enable it only against a database you control.

### Create a read-only service role

Run the following in `psql` as a database administrator. Replace `app` and `public` with your database and schema. This creates a fresh login role with no inherited privileges; do not reuse an administrator or application-owner role.

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

### Verify effective schema privileges

Before configuring the connection, verify that `mcp_reader` cannot create objects in the target schema:

```sql
SELECT has_schema_privilege('mcp_reader', 'public', 'CREATE') AS can_create_objects;
```

The result must be `false`. A result of `true` means the role is not restricted to read access in that schema. This can occur when an installation upgraded from PostgreSQL 14 or earlier retains a shared `PUBLIC` `CREATE` grant on `public`. Do not revoke a shared `PUBLIC` privilege solely for this server; have the database administrator decide the database-wide schema policy first.

The `\password` command prompts without placing the password in shell history. Store the resulting connection secret only in the runtime secret or environment manager, not in the repository. If the role must read tables created in the future and `postgres` creates those tables, run this optional command as `postgres`. Replace `postgres` with the actual object-creator role in other environments:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public
  GRANT SELECT ON TABLES TO mcp_reader;
```

Before configuring the connection, connect as `mcp_reader` and verify that every boolean below is `false`:

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

## Output masking

Masking is off unless it is explicitly enabled. With no `masking` key, and with `enabled` omitted or false, tool output is the previous text. Set `enabled: true` and at least one rule to turn it on. `MCP_MASKING_JSON`, when the variable is present, replaces the whole YAML `masking` object. An empty value is invalid. Unset leaves the YAML object in place.

Empty documents, unknown fields, and `enabled: true` with no rules fail startup. The error is `invalid masking config` and does not include the document.

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

`names` and `name_patterns` select a whole field name. They also select database, schema, table, column, index, owner, and role identifiers returned by these tools. Name patterns are anchored to the whole name. `a)|(b` is rejected. `(?:a)|(b)` matches only the whole names `a` and `b`. `values` and `value_patterns` are substring matches, including a one-character literal, and each match is replaced once. The placeholder is not searched again. A one-character literal can match inside ordinary words.

A matched string becomes the placeholder, `[masked]` unless you set another one. A matched non-string becomes JSON null. An existing SQL NULL stays null. A null in a result can be a stored NULL or a redacted value. Types and nullability in the JSON can change when a rule matches. `ordinal_position` stays a number and `nullable` stays a boolean unless that whole value matches a rule.

The column type decides which rule applies. `text` and other ordinary strings stay strings, even when every character is a digit. A match replaces that span with the placeholder. `smallint`, `integer`, `bigint`, `real`, `double precision`, `numeric`, and `boolean` use the exact text PostgreSQL sent. If a value rule hits that text, the whole value becomes null and the digits are not edited in place. A boolean cell whose text is `t` does not match a rule for `true`. An unmatched number keeps its original literal.

JSON and JSONB cells that are one JSON object or array are masked structurally, including escaped keys and JSON nested inside a string, for two JSON-in-string rounds. A deeper JSON value fails the result. Text that only looks like JSON stays text, apart from value matches. A JSON number or boolean that matches a value rule becomes null. An unmatched number keeps its original literal, including trailing zeros.

`bytea` is recognized by the column type. Known `\x` hex is decoded, masked, and written back as hex. A hex cell longer than 1 MiB fails before it is decoded. Value patterns also run on ASCII text inside invalid UTF-8. Escape-format `bytea`, invalid hex, and `bytea[]` fail the result when masking is on, and the original bytes are not returned. A text value that looks like hex is not decoded. A masked value past the size limit fails the result.

A value pattern that can match without consuming a character is rejected at startup. `\b|sk_live_4242` is one example. The error is `invalid masking config` and does not include the pattern. The shared masking package is copied from a pin that still awaits an independent review. This server does not claim that review is finished.

These tools do not return column or table comments. Index expressions are the `definition` string, and defaults are the `default` string. Name rules do not search inside those strings. Value rules do.

`schema_version` and the tool name are server constants and are not scanned. Fixed public errors and the placeholder are not scanned. A short literal that also occurs there can remain. Masking does not find every private value, compressed payload, or encoding.

If a result cannot be masked, including a name collision, an unsupported shape, or a value past the size limit, the call fails with `[postgres_masking_failed] result was not returned` and no partial JSON. Masking changes emitted text only. The SQL sent to PostgreSQL, and the stored rows, stay as they were.

HTTP debug logs record a route class (`mcp`, `sse`, `message`, or `other`), not the request path or query.

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
docker run --rm -i \
  --env MCP_POSTGRES_HOST \
  --env MCP_POSTGRES_PORT \
  --env MCP_POSTGRES_DATABASE \
  --env MCP_POSTGRES_USERNAME \
  --env MCP_POSTGRES_PASSWORD \
  --env MCP_POSTGRES_SSLMODE \
  postgres-mcp-server:dev
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
