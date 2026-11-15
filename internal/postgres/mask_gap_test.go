package postgres

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

func TestTextAndNumericColumnsDoNotShareAMatch(t *testing.T) {
	const exact = "18446744073709551615"
	const decimal = "1.2300"
	service := &maskService{view: queryResultView{
		columns: []queryColumn{
			{name: "as_text", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "as_int", typeOID: pgtype.Int8OID, postgresType: "bigint"},
			{name: "shorter", typeOID: pgtype.Int4OID, postgresType: "integer"},
			{name: "shorter_text", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "decimal_text", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "decimal_num", typeOID: pgtype.NumericOID, postgresType: "numeric"},
			{name: "near_decimal", typeOID: pgtype.NumericOID, postgresType: "numeric"},
			{name: "bool_text", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "bool_col", typeOID: pgtype.BoolOID, postgresType: "boolean"},
			{name: "bool_protocol", typeOID: pgtype.BoolOID, postgresType: "boolean"},
			{name: "named_num", typeOID: pgtype.Int4OID, postgresType: "integer"},
			{name: "missing_num", typeOID: pgtype.Int4OID, postgresType: "integer"},
			{name: "large_text", typeOID: pgtype.TextOID, postgresType: "text"},
			{name: "large_num", typeOID: pgtype.NumericOID, postgresType: "numeric"},
		},
		rows: [][]*string{{
			strPtr("411111"),
			strPtr("411111"),
			strPtr("41111"),
			strPtr("41111"),
			strPtr(decimal),
			strPtr(decimal),
			strPtr("1.23"),
			strPtr("true"),
			strPtr("true"),
			strPtr("t"),
			strPtr("7"),
			nil,
			strPtr(exact),
			strPtr(exact),
		}},
	}}
	// The name rule is separate so only named_num is selected by field name.
	service.view.columns[10].name = "synthetic_field"
	text, err := callRuled(t, `{"enabled":true,"names":["synthetic_field"],"values":["411111","411","`+exact+`","`+decimal+`","true"]}`, service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "411111") || strings.Contains(text, exact) || strings.Contains(text, decimal) {
		t.Fatalf("matched originals remain: %s", text)
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
	if row[0] != "[masked]" || row[1] != nil || row[2] != nil || row[3] != "[masked]11" {
		t.Fatalf("text versus number = %#v", row[:4])
	}
	if row[4] != "[masked]" || row[5] != nil || row[6] != "1.23" {
		t.Fatalf("decimal provenance = %#v", row[4:7])
	}
	if row[7] != "[masked]" || row[8] != nil || row[9] != "t" || row[10] != nil || row[11] != nil {
		t.Fatalf("bool and name provenance = %#v", row[7:12])
	}
	if row[12] != "[masked]" || row[13] != nil {
		t.Fatalf("large integer provenance = %#v", row[12:])
	}
	if strings.Contains(text, exact) {
		t.Fatalf("large integer text was parsed as a float: %s", text)
	}
}

func TestJSONNumberAndBoolBecomeNull(t *testing.T) {
	const exact = "18446744073709551615"
	service := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "doc", typeOID: pgtype.JSONBOID, postgresType: "jsonb"}},
		rows:    [][]*string{{strPtr(`{"n":` + exact + `,"flag":true,"label":"411111","keep":1.2300}`)}},
	}}
	text, err := callRuled(t, `{"enabled":true,"values":["`+exact+`","true","411111"]}`, service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, exact) || strings.Contains(text, "411111") {
		t.Fatalf("json scalars leaked: %s", text)
	}
	var payload struct {
		Data struct {
			Rows [][]any `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	cell, _ := payload.Data.Rows[0][0].(string)
	var doc map[string]any
	if err := json.Unmarshal([]byte(cell), &doc); err != nil {
		t.Fatalf("cell %s: %v", cell, err)
	}
	if doc["n"] != nil || doc["flag"] != nil || doc["label"] != "[masked]" {
		t.Fatalf("json document = %#v", doc)
	}
	if !strings.Contains(cell, "1.2300") {
		t.Fatalf("unmatched decimal was rewritten: %s", cell)
	}
}

func TestOneBudgetCoversTheWholeResult(t *testing.T) {
	const token = "sk_live_4242"
	columns := make([]queryColumn, 60)
	row := make([]*string, 60)
	for index := range columns {
		columns[index] = queryColumn{name: "n" + strconv.Itoa(index), typeOID: pgtype.TextOID, postgresType: "text"}
		row[index] = strPtr(strings.Repeat("b", 20000))
	}
	row[len(row)-1] = strPtr(token)
	service := &maskService{view: queryResultView{columns: columns, rows: [][]*string{row}}}
	text, err := callRuled(t, `{"enabled":true,"values":["`+token+`"]}`, service, "postgres_query")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" || strings.Contains(err.Error(), token) {
		t.Fatalf("whole-result budget = %q, %v", text, err)
	}
}

func TestInvalidUTF8ByteaRegexIsNotReturned(t *testing.T) {
	const token = "sk_live_4242"
	raw := append([]byte{0xff}, []byte(token)...)
	hexText := `\x` + hex.EncodeToString(raw)
	service := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"}},
		rows:    [][]*string{{strPtr(hexText)}},
	}}
	text, err := callRuled(t, `{"enabled":true,"value_patterns":["sk_live_[0-9]+"]}`, service, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, token) || strings.Contains(text, hex.EncodeToString(raw)) {
		t.Fatalf("regex bytea leaked: %s", text)
	}
	if !strings.Contains(text, "ff") && !strings.Contains(text, "FF") {
		t.Fatalf("invalid UTF-8 neighbor was dropped: %s", text)
	}

	literal := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"}},
		rows:    [][]*string{{strPtr(hexText)}},
	}}
	text, err = callRuled(t, `{"enabled":true,"values":["`+token+`"]}`, literal, "postgres_query")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, token) || strings.Contains(text, hex.EncodeToString(raw)) {
		t.Fatalf("literal bytea leaked: %s", text)
	}
}

func TestByteaExpansionPastBudgetIsNotReturned(t *testing.T) {
	raw := bytes.Repeat([]byte("a"), 5000)
	service := &maskService{view: queryResultView{
		columns: []queryColumn{{name: "blob", typeOID: pgtype.ByteaOID, postgresType: "bytea"}},
		rows:    [][]*string{{strPtr(`\x` + hex.EncodeToString(raw))}},
	}}
	text, err := callRuled(t, `{"enabled":true,"values":["a"]}`, service, "postgres_query")
	if text != "" || err == nil || err.Error() != "[postgres_masking_failed] result was not returned" {
		t.Fatalf("expanded bytea result = %q, %v", text, err)
	}
	if strings.Contains(err.Error(), "[masked]") || strings.Contains(text, "aaaa") {
		t.Fatalf("expanded output escaped: %q %v", text, err)
	}
}

func TestMetadataNumberAndBoolUseWholeValues(t *testing.T) {
	service := maskingFixture()
	columns := service.describe["columns"].([]map[string]any)
	columns[0]["ordinal_position"] = 411111
	columns[0]["nullable"] = false
	columns[0]["default"] = "411111"
	text, err := callRuled(t, `{"enabled":true,"values":["411111"]}`, service, "postgres_describe_table")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "411111") {
		t.Fatalf("describe leaked the number: %s", text)
	}
	var payload struct {
		Data struct {
			Columns []map[string]any `json:"columns"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Columns[0]["ordinal_position"] != nil || payload.Data.Columns[0]["nullable"] != false {
		t.Fatalf("matched metadata scalars = %#v", payload.Data.Columns[0])
	}
	if payload.Data.Columns[0]["default"] != "[masked]" {
		t.Fatalf("text default = %#v", payload.Data.Columns[0]["default"])
	}
	if payload.Data.Columns[1]["ordinal_position"] != float64(2) || payload.Data.Columns[1]["nullable"] != true {
		t.Fatalf("unmatched metadata scalars = %#v", payload.Data.Columns[1])
	}
}

func callRuled(t *testing.T, raw string, service handlerService, name string) (string, error) {
	t.Helper()
	policy, err := masking.ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	created := NewToolset(service)
	created.SetPolicy(policy)
	var handler toolset.ToolHandler
	for _, tool := range created.GetTools() {
		if tool.Tool.Name == name {
			handler = tool.Handler
		}
	}
	if handler == nil {
		t.Fatalf("missing tool %s", name)
	}
	params := map[string]any{"sql": "SELECT note FROM widgets"}
	if name == "postgres_describe_table" {
		params = map[string]any{"schema": "public", "table": "widgets"}
	}
	return handler(context.Background(), params)
}
