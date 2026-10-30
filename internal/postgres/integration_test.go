//go:build integration

package postgres

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
)

func TestPostgres17FixtureServesReadOnlyDiscoveryAndQuery(t *testing.T) {
	connectionURL := strings.TrimSpace(os.Getenv("POSTGRES_INTEGRATION_URL"))
	if connectionURL == "" {
		t.Fatal("POSTGRES_INTEGRATION_URL is required for the PostgreSQL integration fixture")
	}
	t.Setenv("DATABASE_URL", connectionURL)
	for _, key := range []string{"PGHOST", "PGPORT", "PGDATABASE", "PGUSER", "PGPASSWORD", "PGSSLMODE"} {
		t.Setenv(key, "")
	}

	executionConfig, err := config.LoadPostgresExecutionConfig()
	if err != nil {
		t.Fatalf("LoadPostgresExecutionConfig(): %v", err)
	}
	service, err := NewService(t.Context(), executionConfig, nil, false)
	if err != nil {
		t.Fatalf("NewService(): %v", err)
	}
	defer service.Close()

	serverInfo, err := service.GetServerInfo(t.Context())
	if err != nil {
		t.Fatalf("GetServerInfo(): %v", err)
	}
	versionNumber, err := strconv.Atoi(serverInfo["server_version_num"].(string))
	if err != nil || versionNumber < minimumServerVersion {
		t.Fatalf("server_version_num = %#v, want PostgreSQL 17 fixture", serverInfo["server_version_num"])
	}
	if _, ok := serverInfo["host"].(string); !ok {
		t.Fatalf("server host = %#v, want TCP host string", serverInfo["host"])
	}

	schemas, err := service.ListSchemas(t.Context())
	if err != nil {
		t.Fatalf("ListSchemas(): %v", err)
	}
	if len(schemas["schemas"].([]map[string]any)) == 0 {
		t.Fatal("ListSchemas() returned no visible schemas")
	}
	databases, err := service.ListDatabases(t.Context())
	assertIntegrationObjectList(t, databases, err, "databases", "name", "app")
	tables, err := service.ListTables(t.Context(), "public")
	assertIntegrationObjectList(t, tables, err, "tables", "name", "widgets")
	columns, err := service.DescribeTable(t.Context(), "public", "widgets")
	assertIntegrationColumns(t, columns, err)
	indexes, err := service.ListIndexes(t.Context(), "public", "widgets")
	assertIntegrationObjectList(t, indexes, err, "indexes", "name", "widgets_name_idx")
	privileges, err := service.Query(t.Context(), "SELECT has_schema_privilege(current_user, 'public', 'CREATE') AS can_create_objects")
	assertIntegrationQueryCell(t, privileges, err, "can_create_objects", "f")
	widgetCount, err := service.Query(t.Context(), "SELECT count(*)::int AS widget_count FROM public.widgets")
	assertIntegrationQueryCell(t, widgetCount, err, "widget_count", "0")

	result, err := service.Query(t.Context(), "SELECT 1::int AS one, NULL::text AS empty")
	if err != nil {
		t.Fatalf("Query(): %v", err)
	}
	rows, ok := result["rows"].([][]any)
	if !ok || len(rows) != 1 || len(rows[0]) != 2 || rows[0][0] != "1" || rows[0][1] != nil {
		t.Fatalf("Query() rows = %#v, want string and null cells", result["rows"])
	}

	for _, sql := range []string{
		"INSERT INTO any_table VALUES (1)",
		"SELECT pg_advisory_lock(4242)",
	} {
		if _, err := service.Query(t.Context(), sql); err == nil || !strings.HasPrefix(err.Error(), "[postgres_query_rejected]") {
			t.Fatalf("Query(%q) error = %v, want rejected strict SELECT", sql, err)
		}
	}
}

func assertIntegrationObjectList(t *testing.T, data map[string]any, err error, key, field, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	objects, ok := data[key].([]map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object list", key, data[key])
	}
	for _, object := range objects {
		if object[field] == want {
			return
		}
	}
	t.Fatalf("%s = %#v, want %s=%q", key, objects, field, want)
}

func assertIntegrationColumns(t *testing.T, data map[string]any, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("DescribeTable: %v", err)
	}
	columns, ok := data["columns"].([]map[string]any)
	if !ok || len(columns) != 2 {
		t.Fatalf("DescribeTable columns = %#v, want two columns", data["columns"])
	}
	if columns[0]["name"] != "id" || columns[0]["ordinal_position"] != 1 || columns[1]["name"] != "name" || columns[1]["nullable"] != false {
		t.Fatalf("DescribeTable columns = %#v, want id/name contract", columns)
	}
}

func assertIntegrationQueryCell(t *testing.T, data map[string]any, err error, column, want string) {
	t.Helper()
	if err != nil {
		t.Fatalf("Query(): %v", err)
	}
	columns, ok := data["columns"].([]map[string]any)
	if !ok || len(columns) != 1 || columns[0]["name"] != column {
		t.Fatalf("Query() columns = %#v, want %q", data["columns"], column)
	}
	rows, ok := data["rows"].([][]any)
	if !ok || len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] != want {
		t.Fatalf("Query() rows = %#v, want %q", data["rows"], want)
	}
}
