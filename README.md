# PostgreSQL Read-only MCP Server

[中文](README.zh.md)

Source and issues are in the [GitHub repository](https://github.com/futuretea/postgres-mcp-server). The license is [MIT](LICENSE).

An MCP (Model Context Protocol) server for discovering one PostgreSQL 14 or newer instance and running a deliberately narrow SELECT subset. It exposes MCP over stdio or HTTP and includes a CLI.

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

[Release v0.4.0](https://github.com/futuretea/postgres-mcp-server/releases/tag/v0.4.0) is the current npm package and the current GHCR image. The npm dist-tag `latest` and the image tag `latest` select that release. Both release entry points start MCP over stdio by default.

GitHub Releases `v0.0.1` through `v0.4.0` attach no executable files. Each page has generated notes, plus the Git tag source tarball and zipball. Prebuilt programs are the npm platform packages and the GHCR image. You can also build a binary from the tag.

The examples in this section leave `DATABASE_URL` unset and use discrete connection variables. A set `DATABASE_URL` ignores every `MCP_POSTGRES_*` variable. The shell examples use POSIX `export`. On Windows, set the same names in the process environment or in the client `env` object.

### npm

The wrapper package is [`@futuretea/postgres-mcp-server`](https://www.npmjs.com/package/@futuretea/postgres-mcp-server). Version `0.4.0` publishes these platform packages. The wrapper selects one from Node's `process.platform` and `process.arch`:

| Package | Node platform | Node arch |
|---|---|---|
| `@futuretea/postgres-mcp-server-darwin-amd64` | `darwin` | `x64` |
| `@futuretea/postgres-mcp-server-darwin-arm64` | `darwin` | `arm64` |
| `@futuretea/postgres-mcp-server-linux-amd64` | `linux` | `x64` |
| `@futuretea/postgres-mcp-server-linux-arm64` | `linux` | `arm64` |
| `@futuretea/postgres-mcp-server-windows-amd64` | `win32` | `x64` |

Any other pair stops with `Unsupported platform/architecture: <platform>/<arch>`. If the matching package is not installed, the wrapper stops with `Could not resolve binary path for platform/architecture: <platform>/<arch>`.

The published `postgres-mcp-server` command always inserts the `mcp` subcommand and then appends the arguments you passed. It does not run `tools`, `version`, or `completion`. Those commands are on the binary from `make build`. They are also on the platform-package file `bin/postgres-mcp-server`, or `bin/postgres-mcp-server.exe` on Windows.

Flags are passed through. `--help` prints `mcp` help and does not connect. A positional word such as `version` is not another subcommand. The process still starts `mcp` and validates the database connection.

The README file inside the published `0.4.0` npm package still pins `v0.0.1`. Use this file when you configure release `0.4.0`.

Pin `@0.4.0` for this release. Startup connects and checks the server version and role before the process reports ready:

```sh
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
npx -y @futuretea/postgres-mcp-server@0.4.0
```

For an MCP client configuration, use:

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

This client starts a stdio process. The process speaks MCP on stdin and stdout. There is no authorization header, API key, or client certificate.

`npx -y @futuretea/postgres-mcp-server@0.4.0 --help` prints `mcp` help and does not open a database connection.

### Docker

The image repository is [`ghcr.io/futuretea/postgres-mcp-server`](https://github.com/futuretea/postgres-mcp-server/pkgs/container/postgres-mcp-server). Tags `v0.4.0` and `latest` are the same image index. Runnable platforms are `linux/amd64` and `linux/arm64`. The index also contains build-attestation manifests. Those entries are not images you can run.

The published image runs as user `mcp`. Its entrypoint is `/usr/local/bin/postgres-mcp-server mcp`. Arguments after the image name are `mcp` flags.

The database hostname must be reachable from the container. `localhost` inside the container is not the host machine. Use a database hostname on the Docker network, or the platform-specific host gateway when that is the route you intend. This remote example sets `sslmode=require`. Set the PostgreSQL TLS mode and certificates required by your deployment.

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

### HTTP mode

Pass MCP flags after the release command to start HTTP instead of stdio. On the host, the default listener is loopback. Startup still connects before the process reports ready:

```sh
export MCP_POSTGRES_HOST=127.0.0.1
export MCP_POSTGRES_PORT=5432
export MCP_POSTGRES_DATABASE=app
export MCP_POSTGRES_USERNAME=reader
export MCP_POSTGRES_PASSWORD='<runtime-secret>'
export MCP_POSTGRES_SSLMODE=disable
npx -y @futuretea/postgres-mcp-server@0.4.0 --port 8080
```

For Docker, the server must listen on all container interfaces, while the published port can remain loopback-only on the host:

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

Streamable HTTP clients use `http://127.0.0.1:8080/mcp`. That transport stores no session and returns no session id. SSE clients use `http://127.0.0.1:8080/sse` and post to `/message`. The SSE response appends the original request query to the message endpoint URL.

While the process is serving, `GET /healthz` returns `200` and the body `healthy`. `HEAD /healthz` returns `200`, an empty body, and `Content-Length: 7`. Any other method returns `405`.

HTTP has no authentication or TLS. The server does not check an authorization header, an API key, or a client certificate. Keep it on a trusted network or place it behind infrastructure that provides both.

The HTTP server uses a 5 second read-header timeout, a 30 second read timeout, no write timeout, and a 120 second idle timeout. These are not SQL statement timeouts.

`--sse-base-url` sets the public base URL for SSE message endpoints. The value must be an absolute `http` or `https` URL with a host and no query string. A value that does not match is ignored, and the base URL stays empty. Leave the flag unset when clients can use the request host.

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

Every successful tool response is one JSON text envelope with `schema_version`, `tool`, and `data`. `schema_version` is the number `1`. Parameters must be a JSON object with no unknown keys. Empty-parameter tools require `{}`. Missing, `null`, array, or scalar arguments are rejected.

### Result fields

| Tool | `data` |
|---|---|
| `postgres_get_server_info` | `server_version`, `server_version_num`, `database`, `user`, `host`, `port` |
| `postgres_list_databases` | `databases`, each with `name`, `owner`, and `encoding` |
| `postgres_list_schemas` | `schemas`, each with `name` |
| `postgres_list_tables` | `schema`, and `tables`, each with `name` and `kind` |
| `postgres_describe_table` | `schema`, `table`, and `columns`, each with `ordinal_position`, `name`, `postgres_type`, `nullable`, `default`, `identity`, and `generated` |
| `postgres_list_indexes` | `schema`, `table`, and `indexes`, each with `name` and `definition` |
| `postgres_query` | `columns`, each with `name` and `postgres_type`, and `rows` as arrays of cell values |

With masking off, query cells and most metadata fields are PostgreSQL text, or JSON null when the SQL value is null. `ordinal_position` is a JSON number. `nullable` is a JSON boolean. `host` and `port` are null when `inet_server_addr()` is null. `identity` is null unless the column is an identity column. `generated` is null when the column is not generated.

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

At startup the server requires PostgreSQL 14 or newer and rejects service roles that are superusers, bypass row-level security, have replication, or belong to server-file, server-program, or backend-signalling roles. Use a dedicated, least-privilege database role.

The version check reads `server_version_num` and rejects a value below `140000` with `[postgres_unsupported_server_version] PostgreSQL 14 or later is required`. That check still runs when the unsafe-role setting is on. A rejected role fails with `[postgres_unsafe_service_role] database service role is not permitted`. The integration fixture accepts `postgres:14-alpine` and `postgres:17-alpine` only. `make test-integration` rejects any other `POSTGRES_IMAGE`. The GitHub test job runs both images.

If you cannot provision such a role, set `allow_unsafe_database_role: true` (`MCP_ALLOW_UNSAFE_DATABASE_ROLE=true` for the environment) to skip only this role check. A non-empty string is parsed with `strconv.ParseBool`. True values are `1`, `t`, `T`, `true`, `TRUE`, and `True`. False values are `0`, `f`, `F`, `false`, `FALSE`, and `False`. An empty `MCP_ALLOW_UNSAFE_DATABASE_ROLE` is not a value. YAML `true` stays true. The default `false` applies only when the file does not set the key. A non-empty false value, including `0`, overrides YAML `true`. Any other text, including `yes`, `on`, `2`, and a space, fails startup. There is no command-line flag for this setting. The server still wraps every statement in a read-only transaction, but the role-level guarantee is gone: a superuser role can read tables you did not intend to expose, read server files, and run server-side programs that a read-only transaction does not constrain, and a `BYPASSRLS` role sees through row-level security policies. Enable it only against a database you control.

### Create a read-only service role

Run the following in `psql` as a database administrator. Replace `app` and `mcp_probe` with your database and schema. This creates a fresh login role with no inherited privileges; do not reuse an administrator or application-owner role.

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

### Verify effective schema privileges

Before configuring the connection, verify the dedicated schema, not `public`:

```sql
SELECT has_schema_privilege('mcp_reader', 'mcp_probe', 'CREATE') AS can_create_objects,
       has_schema_privilege('mcp_reader', 'mcp_probe', 'USAGE') AS can_use_objects;
```

`can_create_objects` must be `false` and `can_use_objects` must be `true`. On a fresh database, `has_schema_privilege('mcp_reader', 'public', 'CREATE')` is `true` on PostgreSQL 14 and `false` on PostgreSQL 17. That is the server default. Do not revoke a shared `PUBLIC` privilege solely for this server; have the database administrator decide the database-wide schema policy first.

The `\password` command prompts without placing the password in shell history. Store the resulting connection secret only in the runtime secret or environment manager, not in the repository. If the role must read tables created in the future and `postgres` creates those tables, run this optional command as `postgres`. Replace `postgres` with the actual object-creator role in other environments:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA mcp_probe
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

Masking is off unless it is explicitly enabled. With no `masking` key, and with `enabled` omitted or false, tool output is the previous text. Set `enabled: true` and at least one rule to turn it on. `MCP_MASKING_JSON`, when the variable is present, replaces the whole YAML `masking` object. `ParseJSON` only parses that value. An empty or whitespace-only value is invalid. An unset variable leaves the YAML object in place.

Unknown fields, and `enabled: true` with no rules, fail startup. The error is `invalid masking config` and does not include the document. `{}` is valid and leaves masking off.

Each of `names`, `name_patterns`, `values`, and `value_patterns` allows at most 32 entries. A name is at most 128 bytes. A value literal is at most 256 bytes. A pattern is at most 128 bytes. An empty entry is invalid. The placeholder is 1 to 64 bytes. The default is `[masked]`. Exact `names` are case-sensitive.

When masking is on, one result may visit at most 10000 nodes, one scanned input is at most 1 MiB, and the encoded JSON result is at most 1 MiB (`maxMaskedResultBytes`). A result past that encoded-JSON cap fails with `[postgres_masking_failed] result was not returned` and no partial JSON. With masking off, a result has no response-size limit.

CLI `tools describe` and `tools call` hide a name that is absent from the enabled catalog, including a filtered name, only when masking is on. The error is `[postgres_invalid_params] invalid parameters` and does not include the name. With masking off, those commands return `unknown tool "<name>"; run 'postgres-mcp-server tools list' to see available tools`. MCP `tools/call` does not use that check. A missing or filtered name returns `tool '<name>' not found: tool not found`, which includes the name.

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

Each database operation uses a PostgreSQL read-only transaction and pgx's extended execution protocol. `postgres_query` is parsed as exactly one SELECT, rejecting DML, DDL, row locks, SELECT INTO, modifying CTEs, and selected PostgreSQL built-ins with known side effects. A rejected statement returns `[postgres_query_rejected] query is not an allowed SELECT statement`. The rejected built-in names and prefixes are the fixed list in [`internal/postgres/strict_select.go`](internal/postgres/strict_select.go).

This is not a complete proof that every user-defined function, extension, FDW, or future PostgreSQL built-in is harmless. Grant only the database access you intend the server to have.

HTTP transports have no authentication or TLS layer. The default listener is loopback. If you set `--listen` to a remote address, place the service only on a trusted network or behind infrastructure that provides the required protection. This server sets no query timeout, row limit, or concurrency budget. With masking off, it also sets no response-size limit. With masking on, an encoded JSON result larger than 1 MiB is not returned. The call fails with `[postgres_masking_failed] result was not returned`.

## Running MCP and HTTP

```sh
# stdio MCP
./bin/postgres-mcp-server mcp

# HTTP MCP on loopback
./bin/postgres-mcp-server mcp --port 8080

# Explicit remote listener; see the trusted-network requirement above
./bin/postgres-mcp-server mcp --port 8080 --listen 0.0.0.0
```

`GET /healthz` and `HEAD /healthz` are the health endpoints. See [`config.example.yaml`](config.example.yaml) for transport and tool-filter settings. Database connection settings remain in the environment.

An explicit flag overrides a non-empty environment variable for the same setting. A non-empty environment variable overrides YAML. YAML overrides the default. A flag you do not pass does not block the environment or the file. This loader uses Viper `AutomaticEnv` and does not call `AllowEmptyEnv`, so an empty environment variable is absent. It leaves the YAML value, or the default when the file has no value. `MCP_MASKING_JSON` is the exception. `LoadConfig` reads it with `LookupEnv`. A present empty or whitespace-only value is invalid and does not leave the YAML object.

| YAML key | Flag | Environment | Default |
|---|---|---|---|
| `port` | `--port` | `MCP_PORT` | `0` |
| `listen` | `--listen` | `MCP_LISTEN` | `127.0.0.1` |
| `sse_base_url` | `--sse-base-url` | `MCP_SSE_BASE_URL` | empty |
| `log_level` | `--log-level` | `MCP_LOG_LEVEL` | `info` |
| `enabled_tools` | `--enabled-tools` | `MCP_ENABLED_TOOLS` | empty |
| `disabled_tools` | `--disabled-tools` | `MCP_DISABLED_TOOLS` | empty |
| `enabled_domains` | `--enable-domains` | `MCP_ENABLED_DOMAINS` | empty |
| `disabled_domains` | `--disable-domains` | `MCP_DISABLED_DOMAINS` | empty |
| `allow_unsafe_database_role` | none | `MCP_ALLOW_UNSAFE_DATABASE_ROLE` | `false` |

`port` must be from 0 through 65535. `0` keeps stdio. Any other port starts HTTP, and `listen` must be set. Log levels are `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`, and `disabled`. Stdio mode discards logs so MCP messages stay on stdout. HTTP mode writes logs to stderr.

`MCP_ENABLED_TOOLS` and the other list variables are comma-separated. A space is part of one name, not a separator. An empty enable list allows every tool or domain. A non-empty enable list is an allow-list. A list that is present but trims to no names is invalid. The error names the field, for example `enabled_tools contains no names`, and omits the list text. Disable lists then remove matches. The only domain is `postgres`. If the filter removes every tool, startup fails with `no tools registered; check enabled_tools / disabled_tools / domain filters`. `tools list` uses the same filter and still does not connect.

## CLI

These commands are on a binary you build, or on the platform-package binary. The npm wrapper command does not run them, because it always inserts `mcp`.

```sh
./bin/postgres-mcp-server tools list --json
./bin/postgres-mcp-server tools describe postgres_query --json
./bin/postgres-mcp-server tools call postgres_list_schemas --params '{}'
./bin/postgres-mcp-server tools call postgres_query --params '{"sql":"SELECT 1"}'
./bin/postgres-mcp-server tools call postgres_query --params-file ./query.json
./bin/postgres-mcp-server version
./bin/postgres-mcp-server completion bash
```

`--params` defaults to `{}`. `--params-file -` reads the JSON object from stdin. `version` prints `postgres-mcp-server version=<version> commit=<commit> date=<date>`. `completion` writes a bash, zsh, fish, or powershell script. `completion install` prints the lines to add to a shell profile. `tools` also accepts the alias `tool`.

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

The image entrypoint is `/usr/local/bin/postgres-mcp-server mcp`.

## Build, test, and contribute

Go 1.25.10 matches `go.mod`, `.tool-versions`, and CI. The SQL parser package uses cgo, so the build needs a C compiler. `make build` writes `bin/postgres-mcp-server`. It sets the version from `git describe --tags --always --dirty`, the commit from `HEAD`, and the date from the current UTC time. A clean checkout of tag `v0.4.0` prints `version=v0.4.0`. A dirty worktree appends `-dirty`. `go build` without those linker settings prints `version=dev`, `commit=unknown`, and `date=unknown`.

```sh
make format
make lint
make test
make test-integration  # Docker; default postgres:17-alpine. POSTGRES_IMAGE=postgres:14-alpine selects 14.
make build
make docker
```

`make lint` runs `go vet` and checks `gofmt`. It runs `golangci-lint` only when that command is on `PATH`. CI runs golangci-lint v2.12.2. `make test` is `go test ./...`. The GitHub Build workflow also runs the race detector and both PostgreSQL 14 and 17 integration fixtures. `make ci` runs lint, unit tests, the default PostgreSQL 17 fixture, and `make build`. The Build workflow does not start when every changed file is `.gitignore`, `LICENSE`, or a Markdown file in the repository root.

Open an [issue](https://github.com/futuretea/postgres-mcp-server/issues) for a defect or a change.

## Release

A push of a stable tag `vX.Y.Z` runs the npm and Docker release workflows. A prerelease suffix does not match the tag check.

The npm workflow builds cgo binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, and `windows/amd64`. It publishes `@futuretea/postgres-mcp-server` and the five platform packages, and it opens a GitHub Release with generated notes. That release step does not upload the binaries. The release asset list stays empty.

The Docker workflow pushes `ghcr.io/futuretea/postgres-mcp-server:vX.Y.Z` and `:latest` for `linux/amd64` and `linux/arm64`.

A release binary's version string is the tag with the leading `v` removed. The npm workflow records the tagged commit's committer date. The Docker workflow records the image build time.
