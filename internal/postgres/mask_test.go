package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

const maskingSecret = "synthetic-token-value"

func TestDisabledToolResultsPreserveLegacyValues(t *testing.T) {
	service := maskingFixture()
	for _, name := range toolNames() {
		t.Run(name, func(t *testing.T) {
			text, err := callTool(t, masking.Disabled(), service, name)
			if err != nil {
				t.Fatalf("disabled %s error = %v", name, err)
			}
			if !strings.Contains(text, maskingSecret) {
				t.Fatalf("disabled %s dropped configured data: %s", name, text)
			}
		})
	}
}

func TestEnabledToolsRemoveConfiguredData(t *testing.T) {
	service := maskingFixture()
	for _, name := range toolNames() {
		t.Run(name, func(t *testing.T) {
			text, err := callTool(t, maskingPolicy(t), service, name)
			if err != nil {
				t.Fatalf("enabled %s error = %v", name, err)
			}
			if strings.Contains(text, maskingSecret) || strings.Contains(text, "synthetic_field") || strings.Contains(text, "token_visible") || strings.Contains(text, "sk_live_42") {
				t.Fatalf("enabled %s leaked configured data: %s", name, text)
			}
			if !strings.Contains(text, "[masked]") {
				t.Fatalf("enabled %s did not redact a matched string: %s", name, text)
			}
			assertEnvelopeConstants(t, name, text)
		})
	}
}

func TestDescribeTableKeepsNumberAndBool(t *testing.T) {
	text, err := callTool(t, maskingPolicy(t), maskingFixture(), "postgres_describe_table")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `"ordinal_position":1`) || !strings.Contains(text, `"nullable":false`) {
		t.Fatalf("describe types changed: %s", text)
	}
	if strings.Contains(text, `"ordinal_position":"1"`) || strings.Contains(text, `"nullable":"false"`) {
		t.Fatalf("describe types became strings: %s", text)
	}
	var payload struct {
		Data struct {
			Columns []map[string]any `json:"columns"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Columns[0]["ordinal_position"] != float64(1) || payload.Data.Columns[0]["nullable"] != false {
		t.Fatalf("describe column = %#v", payload.Data.Columns[0])
	}
	if payload.Data.Columns[0]["default"] != nil {
		t.Fatalf("stored null default = %#v", payload.Data.Columns[0]["default"])
	}
	if payload.Data.Columns[1]["default"] != "nextval('[masked]'::regclass)" {
		t.Fatalf("default = %#v", payload.Data.Columns[1]["default"])
	}
}

func TestQueryMasksByteaHexAndLeavesTextHex(t *testing.T) {
	secretHex := `\x` + hex.EncodeToString([]byte(maskingSecret))
	upper := `\x` + strings.ToUpper(hex.EncodeToString([]byte("ordinary")))
	service := &maskService{view: queryResultView{
		columns: []queryColumn{
			{name: "note", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
			{name: "looks_hex", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "plain", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
			{name: "missing", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
			{name: "token_visible", typeOID: pgtype.TextOID, postgresType: "text"},
		},
		rows: [][]*string{{
			strPtr(maskingSecret),
			strPtr(secretHex),
			strPtr(secretHex),
			strPtr(upper),
			nil,
			strPtr("ordinary-visible"),
		}},
	}}

	disabled, err := callTool(t, masking.Disabled(), service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disabled, maskingSecret) || !strings.Contains(disabled, upper) || !strings.Contains(disabled, secretHex) {
		t.Fatalf("disabled query changed text protocol output: %s", disabled)
	}

	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, maskingSecret) || strings.Contains(text, "token_visible") {
		t.Fatalf("query leaked configured data: %s", text)
	}
	if !strings.Contains(text, upper) {
		t.Fatalf("unchanged bytea hex was rewritten: %s", text)
	}
	var payload struct {
		Data struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	row := payload.Data.Rows[0]
	if row[0] != "[masked]" || row[4] != nil || row[5] != "[masked]" {
		t.Fatalf("row = %#v", row)
	}
	blob, ok := row[2].(string)
	if !ok || blob != secretHex {
		t.Fatalf("text hex cell = %#v", row[2])
	}
	maskedBlob, ok := row[1].(string)
	if !ok || !strings.HasPrefix(maskedBlob, `\x`) || strings.Contains(maskedBlob, secretHex[2:]) {
		t.Fatalf("bytea cell = %#v", row[1])
	}
}

func TestNameMatchedByteaBecomesPlaceholder(t *testing.T) {
	secretHex := `\x` + hex.EncodeToString([]byte(maskingSecret))
	service := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "synthetic_field", typeOID: pgtype.ByteaOID, postgresType: "bytea"}},
		rows:    [][]*string{{strPtr(secretHex)}},
	}}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, maskingSecret) || strings.Contains(text, secretHex) || strings.Contains(text, "synthetic_field") {
		t.Fatalf("named bytea leaked: %s", text)
	}
	var payload struct {
		Data struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Rows[0][0] != "[masked]" {
		t.Fatalf("named bytea cell = %#v", payload.Data.Rows[0][0])
	}
}

func TestUnsupportedByteaFailsClosed(t *testing.T) {
	secretHex := `\x` + hex.EncodeToString([]byte(maskingSecret))
	tests := []queryColumn{
		{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
		{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
		{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"},
		{name: "blobs", typeOID: pgtype.ByteaArrayOID, postgresType: "bytea[]"},
	}
	values := []string{`\001` + maskingSecret, `\xzz` + maskingSecret, `\xa`, secretHex}
	for index, column := range tests {
		t.Run(column.postgresType+values[index], func(t *testing.T) {
			service := &maskService{view: queryResultView{
				columns: []queryColumn{column},
				rows:    [][]*string{{strPtr(values[index])}},
			}}
			text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
			if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" {
				t.Fatalf("result = %q, %v", text, err)
			}
			if strings.Contains(err.Error(), maskingSecret) || strings.Contains(err.Error(), values[index]) {
				t.Fatalf("error leaked raw bytea: %v", err)
			}
		})
	}
}

func TestQueryNameCollisionReturnsNoPartialResult(t *testing.T) {
	service := &maskService{view: queryResultView{
		columns: []queryColumn{
			{name: "token_one", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "token_two", typeOID: pgtype.TextOID, postgresType: "text"},
		},
		rows: [][]*string{{strPtr(maskingSecret), strPtr("ordinary-visible")}},
	}}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" || strings.Contains(err.Error(), maskingSecret) {
		t.Fatalf("result = %q, %v", text, err)
	}
}

func TestNestedJSONAndNulls(t *testing.T) {
	service := &maskService{view: queryResultView{
		columns: []queryColumn{
			{name: "doc", typeOID: pgtype.JSONBOID, postgresType: "jsonb"},
			{name: "empty", typeOID: pgtype.JSONBOID, postgresType: "jsonb"},
		},
		rows: [][]*string{{
			strPtr(`{"synthetic_field":"ordinary-visible","keep":1}`),
			nil,
		}},
	}}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "synthetic_field") || strings.Contains(text, "ordinary-visible") {
		t.Fatalf("nested JSON leaked: %s", text)
	}
	var payload struct {
		Data struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Rows[0][1] != nil {
		t.Fatalf("stored null = %#v", payload.Data.Rows[0][1])
	}
	var doc map[string]any
	cell, _ := payload.Data.Rows[0][0].(string)
	if err := json.Unmarshal([]byte(cell), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["keep"] != float64(1) || doc["[masked]"] != "[masked]" {
		t.Fatalf("jsonb document = %#v", doc)
	}
}

func TestDeepJSONAndUnknownShapeFailClosed(t *testing.T) {
	inner := `{"synthetic_field":1}`
	mid, err := json.Marshal(map[string]string{"b": inner})
	if err != nil {
		t.Fatal(err)
	}
	deep, err := json.Marshal(map[string]string{"a": string(mid)})
	if err != nil {
		t.Fatal(err)
	}
	service := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "doc", typeOID: pgtype.TextOID, postgresType: "text"}},
		rows:    [][]*string{{strPtr(string(deep))}},
	}}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if text != "" || err == nil || strings.Contains(err.Error(), "synthetic_field") {
		t.Fatalf("deep JSON result = %q, %v", text, err)
	}

	bad := maskingFixture()
	bad.describe["comment"] = maskingSecret
	text, err = callTool(t, maskingPolicy(t), bad, "postgres_describe_table")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" || strings.Contains(err.Error(), maskingSecret) {
		t.Fatalf("unknown field result = %q, %v", text, err)
	}
}

func TestActiveModeHidesDriverErrorAndPanic(t *testing.T) {
	service := &maskService{err: fmt.Errorf("dial %s", maskingSecret)}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_list_schemas")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" || strings.Contains(err.Error(), maskingSecret) {
		t.Fatalf("driver error result = %q, %v", text, err)
	}

	legacy, legacyErr := callTool(t, masking.Disabled(), service, "postgres_list_schemas")
	if legacy != "" || legacyErr == nil || !strings.Contains(legacyErr.Error(), maskingSecret) {
		t.Fatalf("disabled driver error = %q, %v", legacy, legacyErr)
	}

	panicker := &maskService{panicMessage: "panic " + maskingSecret}
	text, err = callTool(t, maskingPolicy(t), panicker, "postgres_list_schemas")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" || strings.Contains(err.Error(), maskingSecret) {
		t.Fatalf("panic result = %q, %v", text, err)
	}

	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(fmt.Sprint(recovered), maskingSecret) {
			t.Fatalf("disabled panic = %#v", recovered)
		}
	}()
	_, _ = callTool(t, masking.Disabled(), panicker, "postgres_list_schemas")
	t.Fatal("disabled policy swallowed a panic")
}

func TestQueryCommandTextStaysUnmasked(t *testing.T) {
	sql := "SELECT note FROM widgets"
	pool := &metadataPool{results: []metadataResult{
		resultWithRows([]field{{name: "note", oid: pgtype.TextOID}}, []string{maskingSecret}),
		resultWithRows([]field{{name: "postgres_type", oid: pgtype.TextOID}}, []string{"text"}),
	}}
	service := &Service{pool: pool}
	text, err := callTool(t, maskingPolicy(t), service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, maskingSecret) {
		t.Fatalf("output leaked the cell: %s", text)
	}
	if len(pool.events) != 2 || pool.events[0].sql != sql {
		t.Fatalf("commands = %#v, want the original SELECT", pool.events)
	}
	if !pool.events[0].usesExecMode || !pool.events[0].usesTextResults {
		t.Fatalf("protocol event = %#v", pool.events[0])
	}
	if strings.Contains(pool.events[0].sql, "[masked]") || strings.Contains(pool.events[1].sql, maskingSecret) {
		t.Fatalf("commands changed: %#v", pool.events)
	}
}

func TestEnvelopeIgnoresValueRuleOnToolName(t *testing.T) {
	policy, err := masking.ParseJSON(`{"enabled":true,"values":["postgres_list_schemas"]}`)
	if err != nil {
		t.Fatal(err)
	}
	service := &maskService{schemas: map[string]any{"schemas": []map[string]any{{"name": "public"}}}}
	text, err := callTool(t, policy, service, "postgres_list_schemas")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `"tool":"postgres_list_schemas"`) {
		t.Fatalf("tool constant was masked: %s", text)
	}
}

func callTool(t *testing.T, policy masking.Policy, service handlerService, name string) (string, error) {
	t.Helper()
	created := NewToolset(service)
	created.SetPolicy(policy)
	var handler toolset.ToolHandler
	for _, tool := range created.GetTools() {
		if tool.Tool.Name == name {
			handler = tool.Handler
			break
		}
	}
	if handler == nil {
		t.Fatalf("missing tool %s", name)
	}
	params := map[string]any{}
	switch name {
	case "postgres_list_tables":
		params["schema"] = "token_visible"
	case "postgres_describe_table", "postgres_list_indexes":
		params["schema"] = "token_visible"
		params["table"] = "synthetic_field"
	case "postgres_query":
		params["sql"] = "SELECT note FROM widgets"
	}
	return handler(context.Background(), params)
}

func maskingPolicy(t *testing.T) masking.Policy {
	t.Helper()
	policy, err := masking.ParseJSON(`{"enabled":true,"names":["synthetic_field"],"name_patterns":["token_.+"],"values":["synthetic-token-value"],"value_patterns":["sk_live_[0-9]+"]}`)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func toolNames() []string {
	return []string{
		"postgres_get_server_info",
		"postgres_list_databases",
		"postgres_list_schemas",
		"postgres_list_tables",
		"postgres_describe_table",
		"postgres_list_indexes",
		"postgres_query",
	}
}

func maskingFixture() *maskService {
	return &maskService{
		info: map[string]any{
			"server_version":     "PostgreSQL " + maskingSecret,
			"server_version_num": "170000",
			"database":           "token_visible",
			"user":               "synthetic_field",
			"host":               nil,
			"port":               "5432",
		},
		databases: map[string]any{"databases": []map[string]any{{
			"name": "app-" + maskingSecret, "owner": "synthetic_field", "encoding": "UTF8",
		}}},
		schemas: map[string]any{"schemas": []map[string]any{{"name": maskingSecret}}},
		tables: map[string]any{
			"schema": "token_visible",
			"tables": []map[string]any{{"name": "synthetic_field", "kind": "BASE " + maskingSecret}},
		},
		describe: map[string]any{
			"schema": "token_visible",
			"table":  "synthetic_field",
			"columns": []map[string]any{
				{"ordinal_position": 1, "name": "id", "postgres_type": "integer", "nullable": false, "default": nil, "identity": nil, "generated": nil},
				{"ordinal_position": 2, "name": "note", "postgres_type": "text", "nullable": true, "default": "nextval('" + maskingSecret + "'::regclass)", "identity": nil, "generated": "sk_live_42"},
			},
		},
		indexes: map[string]any{
			"schema": "token_visible",
			"table":  "synthetic_field",
			"indexes": []map[string]any{{
				"name":       "token_visible",
				"definition": "CREATE INDEX ON t (" + maskingSecret + ")",
			}},
		},
		view: queryResultView{
			columns: []queryColumn{{name: "note", typeOID: pgtype.TextOID, postgresType: "text"}},
			rows:    [][]*string{{strPtr(maskingSecret)}},
		},
	}
}

func assertEnvelopeConstants(t *testing.T, toolName, text string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["schema_version"] != float64(1) || payload["tool"] != toolName {
		t.Fatalf("envelope = %#v", payload)
	}
}

func strPtr(value string) *string { return &value }

type maskService struct {
	info         map[string]any
	databases    map[string]any
	schemas      map[string]any
	tables       map[string]any
	describe     map[string]any
	indexes      map[string]any
	view         queryResultView
	err          error
	panicMessage string
}

func (s *maskService) fail() error {
	if s.panicMessage != "" {
		panic(s.panicMessage)
	}
	return s.err
}

func (s *maskService) GetServerInfo(context.Context) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.info, nil
}

func (s *maskService) ListDatabases(context.Context) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.databases, nil
}

func (s *maskService) ListSchemas(context.Context) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.schemas, nil
}

func (s *maskService) ListTables(context.Context, string) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.tables, nil
}

func (s *maskService) DescribeTable(context.Context, string, string) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.describe, nil
}

func (s *maskService) ListIndexes(context.Context, string, string) (map[string]any, error) {
	if err := s.fail(); err != nil {
		return nil, err
	}
	return s.indexes, nil
}

func (s *maskService) queryResultView(context.Context, string) (queryResultView, error) {
	if err := s.fail(); err != nil {
		return queryResultView{}, err
	}
	return s.view, nil
}

var _ handlerService = (*maskService)(nil)
