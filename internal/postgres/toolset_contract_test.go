package postgres

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPostgresToolsetPublishesExactlySevenSchemas(t *testing.T) {
	tools := NewToolset(nil).GetTools()
	byName := make(map[string][]byte, len(tools))
	domains := make(map[string]string, len(tools))
	for _, tool := range tools {
		byName[tool.Tool.Name] = tool.Tool.RawInputSchema
		domains[tool.Tool.Name] = tool.Domain
		if tool.Domain != "postgres" {
			t.Fatalf("tool %q domain = %q, want postgres", tool.Tool.Name, tool.Domain)
		}
	}

	wantRequired := map[string][]string{
		"postgres_get_server_info": nil,
		"postgres_list_databases":  nil,
		"postgres_list_schemas":    nil,
		"postgres_list_tables":     {"schema"},
		"postgres_describe_table":  {"schema", "table"},
		"postgres_list_indexes":    {"schema", "table"},
		"postgres_query":           {"sql"},
	}
	if len(byName) != len(wantRequired) {
		t.Fatalf("PostgreSQL tool count = %d, want exactly %d", len(byName), len(wantRequired))
	}

	for name, required := range wantRequired {
		rawSchema, ok := byName[name]
		if !ok {
			t.Fatalf("missing PostgreSQL tool %q", name)
		}
		if domains[name] != "postgres" {
			t.Fatalf("tool %q domain = %q, want postgres", name, domains[name])
		}
		assertToolSchema(t, name, rawSchema, required)
	}
	for _, removedTemplateTool := range []string{"ec" + "ho", "pi" + "ng"} {
		if _, found := byName[removedTemplateTool]; found {
			t.Fatalf("template tool %q remained in PostgreSQL catalog", removedTemplateTool)
		}
	}
}

func TestDecodeToolParamsKeepsRawArgumentShapeDistinct(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		raw      any
		want     map[string]any
		wantErr  bool
	}{
		{name: "no argument tool accepts object", toolName: "postgres_list_schemas", raw: map[string]any{}, want: map[string]any{}},
		{name: "single string parameter", toolName: "postgres_list_tables", raw: map[string]any{"schema": "public"}, want: map[string]any{"schema": "public"}},
		{name: "two string parameters", toolName: "postgres_describe_table", raw: map[string]any{"schema": "public", "table": "users"}, want: map[string]any{"schema": "public", "table": "users"}},
		{name: "query parameter", toolName: "postgres_query", raw: map[string]any{"sql": "SELECT 1"}, want: map[string]any{"sql": "SELECT 1"}},
		{name: "no argument tool missing arguments", toolName: "postgres_list_schemas", raw: nil, wantErr: true},
		{name: "no argument tool array arguments", toolName: "postgres_list_schemas", raw: []any{}, wantErr: true},
		{name: "no argument tool scalar arguments", toolName: "postgres_list_schemas", raw: "{}", wantErr: true},
		{name: "no argument tool unknown property", toolName: "postgres_list_schemas", raw: map[string]any{"unexpected": "value"}, wantErr: true},
		{name: "required schema missing", toolName: "postgres_list_tables", raw: map[string]any{}, wantErr: true},
		{name: "required schema null", toolName: "postgres_list_tables", raw: map[string]any{"schema": nil}, wantErr: true},
		{name: "required schema non string", toolName: "postgres_list_tables", raw: map[string]any{"schema": 42}, wantErr: true},
		{name: "required schema blank", toolName: "postgres_list_tables", raw: map[string]any{"schema": " \t "}, wantErr: true},
		{name: "required table missing", toolName: "postgres_describe_table", raw: map[string]any{"schema": "public"}, wantErr: true},
		{name: "required query blank", toolName: "postgres_query", raw: map[string]any{"sql": "\n"}, wantErr: true},
		{name: "required query unknown property", toolName: "postgres_query", raw: map[string]any{"sql": "SELECT 1", "extra": true}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := DecodeToolParams(tt.toolName, tt.raw)
			if tt.wantErr {
				if params != nil {
					t.Fatalf("DecodeToolParams() params = %#v, want nil on invalid raw arguments", params)
				}
				if err == nil || err.Error() != "[postgres_invalid_params] invalid parameters" {
					t.Fatalf("DecodeToolParams() error = %v, want stable invalid-parameters error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeToolParams() error = %v", err)
			}
			if !reflect.DeepEqual(params, tt.want) {
				t.Fatalf("DecodeToolParams() = %#v, want %#v", params, tt.want)
			}
		})
	}
}

func TestEncodeToolResultUsesStableJSONTextEnvelope(t *testing.T) {
	text, err := EncodeToolResult("postgres_describe_table", map[string]any{
		"schema": "public",
		"table":  "users",
		"columns": []any{map[string]any{
			"ordinal_position": float64(1),
			"name":             "id",
			"postgres_type":    "bigint",
			"nullable":         false,
			"default":          nil,
			"identity":         nil,
			"generated":        nil,
		}},
	})
	if err != nil {
		t.Fatalf("EncodeToolResult() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("EncodeToolResult() returned non-JSON text %q: %v", text, err)
	}
	want := map[string]any{
		"schema_version": float64(1),
		"tool":           "postgres_describe_table",
		"data": map[string]any{
			"schema": "public",
			"table":  "users",
			"columns": []any{map[string]any{
				"ordinal_position": float64(1),
				"name":             "id",
				"postgres_type":    "bigint",
				"nullable":         false,
				"default":          nil,
				"identity":         nil,
				"generated":        nil,
			}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EncodeToolResult() = %#v, want %#v", got, want)
	}
}

func TestEncodeToolResultPreservesEveryToolFixedDataShape(t *testing.T) {
	tests := []struct {
		tool string
		data map[string]any
	}{
		{
			tool: "postgres_get_server_info",
			data: map[string]any{
				"server_version":     "17.0",
				"server_version_num": "170000",
				"database":           "app",
				"user":               "reader",
				"host":               nil,
				"port":               nil,
			},
		},
		{tool: "postgres_list_databases", data: map[string]any{"databases": []any{}}},
		{tool: "postgres_list_schemas", data: map[string]any{"schemas": []any{}}},
		{tool: "postgres_list_tables", data: map[string]any{"schema": "public", "tables": []any{}}},
		{tool: "postgres_describe_table", data: map[string]any{"schema": "public", "table": "users", "columns": []any{}}},
		{tool: "postgres_list_indexes", data: map[string]any{"schema": "public", "table": "users", "indexes": []any{}}},
		{tool: "postgres_query", data: map[string]any{"columns": []any{}, "rows": []any{}}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			text, err := EncodeToolResult(tt.tool, tt.data)
			if err != nil {
				t.Fatalf("EncodeToolResult() error = %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(text), &got); err != nil {
				t.Fatalf("EncodeToolResult() returned non-JSON text %q: %v", text, err)
			}
			want := map[string]any{
				"schema_version": float64(1),
				"tool":           tt.tool,
				"data":           tt.data,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("EncodeToolResult() = %#v, want %#v", got, want)
			}
		})
	}
}

func assertToolSchema(t *testing.T, name string, rawSchema []byte, required []string) {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("tool %q RawInputSchema = %q, want JSON schema: %v", name, rawSchema, err)
	}
	if got := schema["type"]; got != "object" {
		t.Fatalf("tool %q schema type = %#v, want object", name, got)
	}
	if got, ok := schema["additionalProperties"].(bool); !ok || got {
		t.Fatalf("tool %q additionalProperties = %#v, want false", name, schema["additionalProperties"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool %q properties = %#v, want object", name, schema["properties"])
	}
	if len(properties) != len(required) {
		t.Fatalf("tool %q properties = %#v, want exactly %v", name, properties, required)
	}
	for _, property := range required {
		rawProperty, ok := properties[property].(map[string]any)
		if !ok {
			t.Fatalf("tool %q property %q = %#v, want schema object", name, property, properties[property])
		}
		if rawProperty["type"] != "string" || rawProperty["minLength"] != float64(1) || rawProperty["pattern"] != ".*\\S.*" {
			t.Fatalf("tool %q property %q schema = %#v, want non-blank string", name, property, rawProperty)
		}
	}
	var gotRequired []string
	if rawRequired, ok := schema["required"].([]any); ok {
		for _, item := range rawRequired {
			stringItem, ok := item.(string)
			if !ok {
				t.Fatalf("tool %q required item = %#v, want string", name, item)
			}
			gotRequired = append(gotRequired, stringItem)
		}
	}
	if !reflect.DeepEqual(gotRequired, required) {
		t.Fatalf("tool %q required = %#v, want %#v", name, gotRequired, required)
	}
}
