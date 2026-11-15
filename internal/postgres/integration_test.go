//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

const (
	integrationDatabase = "app"
	integrationSchema   = "mcp_probe"
	integrationTable    = "widgets"
	integrationSentinel = "synthetic-token-value"
	integrationPassword = "synthetic-fixture-password"
	integrationQuerySQL = "SELECT id, name FROM mcp_probe.widgets WHERE name = 'synthetic-token-value'"
	unsafeRoleWarning   = "database service role is not permitted; allow_unsafe_database_role is set, so the startup role check is skipped"
)

type integrationFixture struct {
	major     int
	readerURL string
	host      string
	dangers   []integrationRole
}

type integrationRole struct {
	name string
	url  string
}

func TestIntegrationFixture(t *testing.T) {
	fixture := writeIntegrationFixture(t)
	t.Run("database URL handlers", func(t *testing.T) {
		service := openReaderURL(t, fixture.readerURL)
		defer service.Close()
		exerciseHandlers(t, service, fixture, masking.Disabled(), true)
		policy := integrationMaskPolicy(t)
		exerciseHandlers(t, service, fixture, policy, false)
		assertDedicatedAndPublicPrivileges(t, service, fixture.major)
		assertStrictSelectStillRejects(t, service)
	})
	t.Run("discrete fields without DATABASE_URL", func(t *testing.T) {
		service := openReaderDiscrete(t, fixture.readerURL)
		defer service.Close()
		exerciseHandlers(t, service, fixture, masking.Disabled(), true)
	})
	for _, role := range fixture.dangers {
		t.Run(role.name, func(t *testing.T) {
			assertDangerousRoleRejected(t, role.url)
			assertUnsafeRoleExemption(t, role.url, fixture.major)
		})
	}
}

func writeIntegrationFixture(t *testing.T) integrationFixture {
	t.Helper()
	major, err := strconv.Atoi(strings.TrimSpace(os.Getenv("POSTGRES_INTEGRATION_MAJOR")))
	if err != nil || (major != 14 && major < 17) {
		t.Fatalf("POSTGRES_INTEGRATION_MAJOR=%q, want 14 or 17+", os.Getenv("POSTGRES_INTEGRATION_MAJOR"))
	}
	adminURL := strings.TrimSpace(os.Getenv("POSTGRES_INTEGRATION_ADMIN_URL"))
	if adminURL == "" {
		t.Fatal("POSTGRES_INTEGRATION_ADMIN_URL is required")
	}
	admin, err := pgx.Connect(t.Context(), adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(t.Context())

	execFixture(t, admin, "reader role", `
CREATE ROLE mcp_reader LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS PASSWORD `+sqlLiteral(integrationPassword))
	roles := dangerousRoles()
	for _, role := range roles {
		execFixture(t, admin, role.name, role.createSQL)
		if role.grantSQL != "" {
			execFixture(t, admin, role.name+" membership", role.grantSQL)
		}
	}
	execFixture(t, admin, "database", `CREATE DATABASE app`)

	appAdmin, err := pgx.Connect(t.Context(), integrationURL(adminURL, "", "", integrationDatabase))
	if err != nil {
		t.Fatalf("admin connect app: %v", err)
	}
	defer appAdmin.Close(t.Context())
	execFixture(t, appAdmin, "schema", `CREATE SCHEMA mcp_probe`)
	execFixture(t, appAdmin, "schema usage", `GRANT USAGE ON SCHEMA mcp_probe TO mcp_reader`)
	execFixture(t, appAdmin, "table", `CREATE TABLE mcp_probe.widgets (id integer PRIMARY KEY, name text NOT NULL)`)
	execFixture(t, appAdmin, "index", `CREATE INDEX widgets_name_idx ON mcp_probe.widgets (name)`)
	execFixture(t, appAdmin, "table select", `GRANT SELECT ON mcp_probe.widgets TO mcp_reader`)
	execFixture(t, appAdmin, "sentinel", `INSERT INTO mcp_probe.widgets (id, name) VALUES (1, `+sqlLiteral(integrationSentinel)+`)`)

	readerURL := integrationURL(adminURL, "mcp_reader", integrationPassword, integrationDatabase)
	parsed, err := url.Parse(readerURL)
	if err != nil {
		t.Fatalf("reader URL: %v", err)
	}
	dangers := make([]integrationRole, 0, len(roles))
	for _, role := range roles {
		dangers = append(dangers, integrationRole{
			name: role.name,
			url:  integrationURL(adminURL, role.name, integrationPassword, integrationDatabase),
		})
	}
	return integrationFixture{major: major, readerURL: readerURL, host: parsed.Hostname(), dangers: dangers}
}

type dangerousRole struct {
	name      string
	createSQL string
	grantSQL  string
}

func dangerousRoles() []dangerousRole {
	password := sqlLiteral(integrationPassword)
	roles := []struct {
		name       string
		attributes string
		grant      string
	}{
		{name: "mcp_danger_superuser", attributes: "SUPERUSER"},
		{name: "mcp_danger_bypassrls", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION BYPASSRLS"},
		{name: "mcp_danger_replication", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT REPLICATION NOBYPASSRLS"},
		{name: "mcp_danger_read_server_files", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS", grant: "pg_read_server_files"},
		{name: "mcp_danger_write_server_files", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS", grant: "pg_write_server_files"},
		{name: "mcp_danger_execute_server_program", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS", grant: "pg_execute_server_program"},
		{name: "mcp_danger_signal_backend", attributes: "NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS", grant: "pg_signal_backend"},
	}
	out := make([]dangerousRole, 0, len(roles))
	for _, role := range roles {
		grantSQL := ""
		if role.grant != "" {
			grantSQL = "GRANT " + role.grant + " TO " + role.name
		}
		out = append(out, dangerousRole{
			name:      role.name,
			createSQL: "CREATE ROLE " + role.name + " LOGIN " + role.attributes + " PASSWORD " + password,
			grantSQL:  grantSQL,
		})
	}
	return out
}

func openReaderURL(t *testing.T, readerURL string) *Service {
	t.Helper()
	clearPostgresConnectionEnv(t)
	t.Setenv("DATABASE_URL", readerURL)
	return openService(t, false)
}

func openReaderDiscrete(t *testing.T, readerURL string) *Service {
	t.Helper()
	parsed, err := url.Parse(readerURL)
	if err != nil {
		t.Fatalf("parse reader URL: %v", err)
	}
	clearPostgresConnectionEnv(t)
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) != "" {
		t.Fatal("DATABASE_URL is set; discrete fields must be the only connection input")
	}
	t.Setenv("MCP_POSTGRES_HOST", parsed.Hostname())
	t.Setenv("MCP_POSTGRES_PORT", parsed.Port())
	t.Setenv("MCP_POSTGRES_DATABASE", strings.TrimPrefix(parsed.Path, "/"))
	t.Setenv("MCP_POSTGRES_USERNAME", parsed.User.Username())
	if password, ok := parsed.User.Password(); ok {
		t.Setenv("MCP_POSTGRES_PASSWORD", password)
	}
	if sslmode := parsed.Query().Get("sslmode"); sslmode != "" {
		t.Setenv("MCP_POSTGRES_SSLMODE", sslmode)
	}
	cfg := loadExecutionConfig(t)
	if cfg.PoolConfig.ConnConfig.Host != parsed.Hostname() {
		t.Fatalf("host = %q, want %q", cfg.PoolConfig.ConnConfig.Host, parsed.Hostname())
	}
	service, err := NewService(t.Context(), cfg, nil, false)
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	return service
}

func openService(t *testing.T, allowUnsafe bool) *Service {
	t.Helper()
	service, err := NewService(t.Context(), loadExecutionConfig(t), nil, allowUnsafe)
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	return service
}

func loadExecutionConfig(t *testing.T) *config.PostgresExecutionConfig {
	t.Helper()
	cfg, err := config.LoadPostgresExecutionConfig()
	if err != nil {
		t.Fatalf("LoadPostgresExecutionConfig(): %v", err)
	}
	return cfg
}

func exerciseHandlers(t *testing.T, service *Service, fixture integrationFixture, policy masking.Policy, keepSentinel bool) {
	t.Helper()
	toolset := NewToolset(service)
	toolset.SetPolicy(policy)
	tools := toolset.GetTools()
	if len(tools) != 7 {
		t.Fatalf("GetTools() = %d, want 7", len(tools))
	}
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		params, ok := handlerParams(tool.Tool.Name)
		if !ok || seen[tool.Tool.Name] {
			t.Fatalf("GetTools() returned unexpected tool %q", tool.Tool.Name)
		}
		seen[tool.Tool.Name] = true
		text, err := tool.Handler(t.Context(), params)
		if err != nil {
			t.Fatalf("%s: %v", tool.Tool.Name, err)
		}
		assertHandlerPayload(t, fixture, tool.Tool.Name, text, keepSentinel)
	}
}

func handlerParams(name string) (map[string]any, bool) {
	switch name {
	case "postgres_get_server_info", "postgres_list_databases", "postgres_list_schemas":
		return map[string]any{}, true
	case "postgres_list_tables":
		return map[string]any{"schema": integrationSchema}, true
	case "postgres_describe_table", "postgres_list_indexes":
		return map[string]any{"schema": integrationSchema, "table": integrationTable}, true
	case "postgres_query":
		return map[string]any{"sql": integrationQuerySQL}, true
	default:
		return nil, false
	}
}

func assertHandlerPayload(t *testing.T, fixture integrationFixture, name, text string, keepSentinel bool) {
	t.Helper()
	data := decodeToolData(t, name, text)
	if keepSentinel {
		if name == "postgres_query" && !strings.Contains(text, integrationSentinel) {
			t.Fatalf("mask off query dropped sentinel: %s", text)
		}
	} else if strings.Contains(text, integrationSentinel) {
		t.Fatalf("mask on %s kept sentinel: %s", name, text)
	}
	switch name {
	case "postgres_get_server_info":
		assertServerInfo(t, fixture, data, "mcp_reader")
	case "postgres_list_databases":
		assertObjectPresent(t, data, "databases", "name", integrationDatabase)
	case "postgres_list_schemas":
		assertObjectPresent(t, data, "schemas", "name", integrationSchema)
		assertObjectPresent(t, data, "schemas", "name", "public")
	case "postgres_list_tables":
		if data["schema"] != integrationSchema {
			t.Fatalf("schema = %#v", data["schema"])
		}
		tables := objectList(t, data, "tables")
		if len(tables) != 1 || tables[0]["name"] != integrationTable || tables[0]["kind"] != "BASE TABLE" {
			t.Fatalf("tables = %#v", tables)
		}
	case "postgres_describe_table":
		assertDescribedWidgets(t, data)
	case "postgres_list_indexes":
		assertWidgetIndexes(t, data)
	case "postgres_query":
		assertSentinelQuery(t, data, keepSentinel)
	default:
		t.Fatalf("unasserted tool %s", name)
	}
}

func assertServerInfo(t *testing.T, fixture integrationFixture, data map[string]any, user string) {
	t.Helper()
	versionText, _ := data["server_version"].(string)
	versionNum, _ := data["server_version_num"].(string)
	major, err := strconv.Atoi(versionNum)
	if err != nil || major/10000 != fixture.major {
		t.Fatalf("server_version_num = %#v, want major %d", data["server_version_num"], fixture.major)
	}
	port, _ := data["port"].(string)
	t.Logf("server_version=%s server_version_num=%s port=%s", versionText, versionNum, port)
	if data["database"] != integrationDatabase || data["user"] != user || data["host"] != fixture.host || port == "" {
		t.Fatalf("server info = %#v, want database %s user %s host %s", data, integrationDatabase, user, fixture.host)
	}
}

func assertDescribedWidgets(t *testing.T, data map[string]any) {
	t.Helper()
	if data["schema"] != integrationSchema || data["table"] != integrationTable {
		t.Fatalf("describe target = %#v", data)
	}
	columns := objectList(t, data, "columns")
	if len(columns) != 2 {
		t.Fatalf("columns = %#v", columns)
	}
	if columns[0]["name"] != "id" || columns[0]["ordinal_position"] != float64(1) || columns[0]["nullable"] != false || columns[0]["postgres_type"] != "integer" || columns[0]["default"] != nil {
		t.Fatalf("id column = %#v", columns[0])
	}
	if columns[1]["name"] != "name" || columns[1]["ordinal_position"] != float64(2) || columns[1]["nullable"] != false || columns[1]["postgres_type"] != "text" || columns[1]["default"] != nil {
		t.Fatalf("name column = %#v", columns[1])
	}
}

func assertWidgetIndexes(t *testing.T, data map[string]any) {
	t.Helper()
	if data["schema"] != integrationSchema || data["table"] != integrationTable {
		t.Fatalf("index target = %#v", data)
	}
	found := map[string]bool{}
	for _, index := range objectList(t, data, "indexes") {
		name, _ := index["name"].(string)
		definition, _ := index["definition"].(string)
		if strings.Contains(definition, integrationSentinel) {
			t.Fatalf("index definition contains sentinel: %#v", index)
		}
		found[name] = definition != ""
	}
	if !found["widgets_name_idx"] || !found["widgets_pkey"] {
		t.Fatalf("indexes = %#v, want widgets_name_idx and widgets_pkey", objectList(t, data, "indexes"))
	}
}

func assertSentinelQuery(t *testing.T, data map[string]any, keepSentinel bool) {
	t.Helper()
	columns := objectList(t, data, "columns")
	if len(columns) != 2 || columns[0]["name"] != "id" || columns[0]["postgres_type"] != "integer" || columns[1]["name"] != "name" || columns[1]["postgres_type"] != "text" {
		t.Fatalf("query columns = %#v", columns)
	}
	rows := arrayRows(t, data["rows"])
	if len(rows) != 1 || len(rows[0]) != 2 || rows[0][0] != "1" {
		t.Fatalf("query rows = %#v, want the original WHERE match", data["rows"])
	}
	want := integrationSentinel
	if !keepSentinel {
		want = "[masked]"
	}
	if rows[0][1] != want {
		t.Fatalf("query name = %#v, want %#v", rows[0][1], want)
	}
}

func assertDedicatedAndPublicPrivileges(t *testing.T, service *Service, major int) {
	t.Helper()
	text := callQuery(t, service, `SELECT has_schema_privilege(current_user, 'public', 'CREATE') AS public_create, has_schema_privilege(current_user, 'mcp_probe', 'CREATE') AS probe_create, has_schema_privilege(current_user, 'mcp_probe', 'USAGE') AS probe_usage`)
	data := decodeToolData(t, "postgres_query", text)
	rows := arrayRows(t, data["rows"])
	wantPublic := "f"
	if major == 14 {
		wantPublic = "t"
	}
	if len(rows) != 1 || len(rows[0]) != 3 || rows[0][0] != wantPublic || rows[0][1] != "f" || rows[0][2] != "t" {
		t.Fatalf("privileges = %#v, want public CREATE %s, mcp_probe CREATE f, USAGE t", data["rows"], wantPublic)
	}
}

func assertStrictSelectStillRejects(t *testing.T, service *Service) {
	t.Helper()
	toolset := NewToolset(service)
	query := findTool(t, toolset.GetTools(), "postgres_query")
	for _, sql := range []string{
		"INSERT INTO mcp_probe.widgets VALUES (2, 'blocked')",
		"SELECT pg_advisory_lock(4242)",
	} {
		_, err := query.Handler(t.Context(), map[string]any{"sql": sql})
		if err == nil || !strings.HasPrefix(err.Error(), "[postgres_query_rejected]") {
			t.Fatalf("Query(%q) error = %v, want rejected strict SELECT", sql, err)
		}
	}
}

func assertDangerousRoleRejected(t *testing.T, roleURL string) {
	t.Helper()
	clearPostgresConnectionEnv(t)
	t.Setenv("DATABASE_URL", roleURL)
	logs := &bytes.Buffer{}
	restore := swapLogger(logs)
	defer restore()
	service, err := NewService(t.Context(), loadExecutionConfig(t), nil, false)
	if service != nil {
		service.Close()
		t.Fatal("dangerous role created a service")
	}
	if err == nil || !strings.HasPrefix(err.Error(), "[postgres_unsafe_service_role]") {
		t.Fatalf("NewService() error = %v, want unsafe service role", err)
	}
	if strings.Contains(logs.String(), "allow_unsafe_database_role") {
		t.Fatalf("default startup logged an exemption: %s", logs.String())
	}
}

func assertUnsafeRoleExemption(t *testing.T, roleURL string, major int) {
	t.Helper()
	clearPostgresConnectionEnv(t)
	t.Setenv("DATABASE_URL", roleURL)
	logs := &bytes.Buffer{}
	restore := swapLogger(logs)
	defer restore()
	service := openService(t, true)
	defer service.Close()
	if !strings.Contains(logs.String(), unsafeRoleWarning) {
		t.Fatalf("allow_unsafe startup log = %q, want the role exemption warning", logs.String())
	}
	toolset := NewToolset(service)
	info := findTool(t, toolset.GetTools(), "postgres_get_server_info")
	text, err := info.Handler(t.Context(), map[string]any{})
	if err != nil {
		t.Fatalf("exempted get_server_info: %v", err)
	}
	data := decodeToolData(t, "postgres_get_server_info", text)
	versionNum, _ := data["server_version_num"].(string)
	got, convErr := strconv.Atoi(versionNum)
	if convErr != nil || got/10000 != major {
		t.Fatalf("exempted server_version_num = %#v, want major %d", data["server_version_num"], major)
	}
}

func callQuery(t *testing.T, service *Service, sql string) string {
	t.Helper()
	toolset := NewToolset(service)
	query := findTool(t, toolset.GetTools(), "postgres_query")
	text, err := query.Handler(t.Context(), map[string]any{"sql": sql})
	if err != nil {
		t.Fatalf("postgres_query: %v", err)
	}
	return text
}

func findTool(t *testing.T, tools []toolset.ServerTool, name string) toolset.ServerTool {
	t.Helper()
	for _, tool := range tools {
		if tool.Tool.Name == name {
			return tool
		}
	}
	t.Fatalf("GetTools() missing %s", name)
	return toolset.ServerTool{}
}

func integrationMaskPolicy(t *testing.T) masking.Policy {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"enabled": true,
		"values":  []string{integrationSentinel},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := masking.ParseJSON(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func decodeToolData(t *testing.T, name, text string) map[string]any {
	t.Helper()
	var payload struct {
		SchemaVersion int            `json:"schema_version"`
		Tool          string         `json:"tool"`
		Data          map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("%s JSON: %v", name, err)
	}
	if payload.SchemaVersion != 1 || payload.Tool != name || payload.Data == nil {
		t.Fatalf("%s envelope = %#v", name, payload)
	}
	return payload.Data
}

func objectList(t *testing.T, data map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := data[key].([]any)
	if !ok {
		t.Fatalf("%s = %#v, want object list", key, data[key])
	}
	objects := make([]map[string]any, len(raw))
	for index, item := range raw {
		object, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%s[%d] = %#v", key, index, item)
		}
		objects[index] = object
	}
	return objects
}

func assertObjectPresent(t *testing.T, data map[string]any, key, field, want string) {
	t.Helper()
	for _, object := range objectList(t, data, key) {
		if object[field] == want {
			return
		}
	}
	t.Fatalf("%s = %#v, want %s=%q", key, data[key], field, want)
}

func arrayRows(t *testing.T, raw any) [][]any {
	t.Helper()
	rows, ok := raw.([]any)
	if !ok {
		t.Fatalf("rows = %#v", raw)
	}
	out := make([][]any, len(rows))
	for index, row := range rows {
		cells, ok := row.([]any)
		if !ok {
			t.Fatalf("row %d = %#v", index, row)
		}
		out[index] = cells
	}
	return out
}

func execFixture(t *testing.T, conn *pgx.Conn, label, sql string) {
	t.Helper()
	if _, err := conn.Exec(t.Context(), sql); err != nil {
		t.Fatalf("fixture %s: %v", label, err)
	}
}

func integrationURL(base, user, password, database string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		return base
	}
	if user != "" {
		parsed.User = url.UserPassword(user, password)
	}
	if database != "" {
		parsed.Path = "/" + database
	}
	return parsed.String()
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func clearPostgresConnectionEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DATABASE_URL",
		"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE", "PGSERVICE", "PGSERVICEFILE",
		"MCP_POSTGRES_HOST", "MCP_POSTGRES_PORT", "MCP_POSTGRES_DATABASE",
		"MCP_POSTGRES_USERNAME", "MCP_POSTGRES_PASSWORD", "MCP_POSTGRES_SSLMODE",
	} {
		t.Setenv(key, "")
	}
}

func swapLogger(destination *bytes.Buffer) func() {
	previous := log.Logger
	log.Logger = zerolog.New(destination)
	return func() { log.Logger = previous }
}
