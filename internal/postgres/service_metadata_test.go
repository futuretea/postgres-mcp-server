package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestServiceMetadataAndQueryUseReadOnlyExtendedProtocol(t *testing.T) {
	tests := []struct {
		name          string
		results       []metadataResult
		invoke        func(*Service) (map[string]any, error)
		wantQueries   int
		assertPayload func(*testing.T, map[string]any)
	}{
		{
			name: "server info",
			results: []metadataResult{resultWithRows(
				[]field{{name: "server_version", oid: 25}, {name: "server_version_num", oid: 25}, {name: "database", oid: 25}, {name: "user", oid: 25}, {name: "host", oid: 25}, {name: "port", oid: 25}},
				[]string{"PostgreSQL 17", "170000", "app", "reader", "127.0.0.1", "5432"},
			)},
			invoke:      func(service *Service) (map[string]any, error) { return service.GetServerInfo(t.Context()) },
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				if data["database"] != "app" || data["host"] != "127.0.0.1" {
					t.Fatalf("GetServerInfo() = %#v", data)
				}
			},
		},
		{
			name: "databases",
			results: []metadataResult{resultWithRows(
				[]field{{name: "name", oid: 25}, {name: "owner", oid: 25}, {name: "encoding", oid: 25}},
				[]string{"app", "reader", "UTF8"},
			)},
			invoke:      func(service *Service) (map[string]any, error) { return service.ListDatabases(t.Context()) },
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				assertObjectList(t, data, "databases", "name", "app")
			},
		},
		{
			name:        "schemas",
			results:     []metadataResult{resultWithRows([]field{{name: "name", oid: 25}}, []string{"public"})},
			invoke:      func(service *Service) (map[string]any, error) { return service.ListSchemas(t.Context()) },
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				assertObjectList(t, data, "schemas", "name", "public")
			},
		},
		{
			name: "tables",
			results: []metadataResult{resultWithRows(
				[]field{{name: "name", oid: 25}, {name: "kind", oid: 25}},
				[]string{"widgets", "BASE TABLE"},
			)},
			invoke:      func(service *Service) (map[string]any, error) { return service.ListTables(t.Context(), "inventory") },
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				if data["schema"] != "inventory" {
					t.Fatalf("ListTables() schema = %#v", data["schema"])
				}
				assertObjectList(t, data, "tables", "name", "widgets")
			},
		},
		{
			name: "describe table",
			results: []metadataResult{resultWithRows(
				[]field{{name: "ordinal_position", oid: 25}, {name: "name", oid: 25}, {name: "postgres_type", oid: 25}, {name: "nullable", oid: 25}, {name: "default", oid: 25}, {name: "identity", oid: 25}, {name: "generated", oid: 25}},
				[]string{"1", "id", "integer", "false", "", "", ""},
			)},
			invoke: func(service *Service) (map[string]any, error) {
				return service.DescribeTable(t.Context(), "inventory", "widgets")
			},
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				columns, ok := data["columns"].([]map[string]any)
				if !ok || len(columns) != 1 || columns[0]["ordinal_position"] != 1 || columns[0]["nullable"] != false {
					t.Fatalf("DescribeTable() columns = %#v", data["columns"])
				}
			},
		},
		{
			name: "indexes",
			results: []metadataResult{resultWithRows(
				[]field{{name: "name", oid: 25}, {name: "definition", oid: 25}},
				[]string{"widgets_pkey", "CREATE UNIQUE INDEX widgets_pkey"},
			)},
			invoke: func(service *Service) (map[string]any, error) {
				return service.ListIndexes(t.Context(), "inventory", "widgets")
			},
			wantQueries: 1,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				assertObjectList(t, data, "indexes", "name", "widgets_pkey")
			},
		},
		{
			name: "strict query",
			results: []metadataResult{
				resultWithRows([]field{{name: "one", oid: 23}}, []string{"1"}),
				resultWithRows([]field{{name: "postgres_type", oid: 25}}, []string{"integer"}),
			},
			invoke:      func(service *Service) (map[string]any, error) { return service.Query(t.Context(), "SELECT 1 AS one") },
			wantQueries: 2,
			assertPayload: func(t *testing.T, data map[string]any) {
				t.Helper()
				rows, ok := data["rows"].([][]any)
				if !ok || len(rows) != 1 || len(rows[0]) != 1 || rows[0][0] != "1" {
					t.Fatalf("Query() rows = %#v", data["rows"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &metadataPool{results: append([]metadataResult(nil), tt.results...)}
			data, err := tt.invoke(&Service{pool: pool})
			if err != nil {
				t.Fatalf("service method error = %v", err)
			}
			tt.assertPayload(t, data)

			if len(pool.beginOptions) != tt.wantQueries || pool.rollbackCount != tt.wantQueries || len(pool.events) != tt.wantQueries {
				t.Fatalf("transactions=%d rollbacks=%d queries=%d, want %d each", len(pool.beginOptions), pool.rollbackCount, len(pool.events), tt.wantQueries)
			}
			for index, options := range pool.beginOptions {
				if options.AccessMode != pgx.ReadOnly {
					t.Fatalf("transaction %d access mode = %v, want read-only", index, options.AccessMode)
				}
			}
			for index, event := range pool.events {
				if !event.usesExecMode || !event.usesTextResults {
					t.Fatalf("query %d protocol event = %#v, want extended execution with text results", index, event)
				}
			}
			for index, result := range pool.allResults {
				if !result.rows.closed {
					t.Fatalf("result %d rows were not closed", index)
				}
			}
		})
	}
}

func TestServiceMetadataFailureClosesRowsAndRollsBack(t *testing.T) {
	pool := &metadataPool{results: []metadataResult{{fields: []field{{name: "name", oid: 25}}, rows: &metadataRows{values: [][]*string{{nil}}, scanErr: errors.New("scan failed")}}}}
	_, err := (&Service{pool: pool}).ListSchemas(t.Context())
	if err == nil || err.Error() != "[postgres_metadata_failed] metadata query failed" {
		t.Fatalf("ListSchemas() error = %v, want safe metadata error", err)
	}
	if pool.rollbackCount != 1 || !pool.allResults[0].rows.closed {
		t.Fatalf("failure cleanup rollback=%d rowsClosed=%v, want 1/true", pool.rollbackCount, pool.allResults[0].rows.closed)
	}
}

func assertObjectList(t *testing.T, data map[string]any, key, fieldName, want string) {
	t.Helper()
	objects, ok := data[key].([]map[string]any)
	if !ok || len(objects) != 1 || objects[0][fieldName] != want {
		t.Fatalf("%s = %#v, want one object with %s=%q", key, data[key], fieldName, want)
	}
}

type field struct {
	name string
	oid  uint32
}

type metadataEvent struct {
	usesExecMode    bool
	usesTextResults bool
}

type metadataResult struct {
	fields []field
	rows   *metadataRows
}

func resultWithRows(fields []field, rowValues ...[]string) metadataResult {
	rows := make([][]*string, len(rowValues))
	for rowIndex, values := range rowValues {
		rows[rowIndex] = make([]*string, len(values))
		for columnIndex, value := range values {
			if value == "" {
				continue
			}
			value := value
			rows[rowIndex][columnIndex] = &value
		}
	}
	return metadataResult{fields: fields, rows: &metadataRows{values: rows}}
}

type metadataPool struct {
	results       []metadataResult
	allResults    []metadataResult
	events        []metadataEvent
	beginOptions  []pgx.TxOptions
	rollbackCount int
}

func (p *metadataPool) BeginTx(_ context.Context, options pgx.TxOptions) (Tx, error) {
	p.beginOptions = append(p.beginOptions, options)
	return metadataTx{pool: p}, nil
}

func (p *metadataPool) Close() {}

type metadataTx struct {
	pool *metadataPool
}

func (tx metadataTx) QueryRow(context.Context, string, ...any) Row {
	return metadataRow{err: errors.New("unexpected QueryRow")}
}

func (tx metadataTx) Query(_ context.Context, _ string, args ...any) (Rows, error) {
	event := metadataEvent{}
	for _, arg := range args {
		if mode, ok := arg.(pgx.QueryExecMode); ok && mode == pgx.QueryExecModeExec {
			event.usesExecMode = true
		}
		if formats, ok := arg.(pgx.QueryResultFormats); ok && len(formats) == 1 && formats[0] == pgx.TextFormatCode {
			event.usesTextResults = true
		}
	}
	tx.pool.events = append(tx.pool.events, event)
	if len(tx.pool.results) == 0 {
		return nil, errors.New("unexpected Query")
	}
	result := tx.pool.results[0]
	tx.pool.results = tx.pool.results[1:]
	tx.pool.allResults = append(tx.pool.allResults, result)
	return result.rows.withFields(result.fields), nil
}

func (tx metadataTx) Rollback(context.Context) error {
	tx.pool.rollbackCount++
	return nil
}

type metadataRow struct{ err error }

func (r metadataRow) Scan(...any) error { return r.err }

type metadataRows struct {
	fields  []pgconn.FieldDescription
	values  [][]*string
	index   int
	scanErr error
	closed  bool
}

func (r *metadataRows) withFields(fields []field) *metadataRows {
	r.fields = make([]pgconn.FieldDescription, len(fields))
	for index, field := range fields {
		r.fields[index] = pgconn.FieldDescription{Name: field.name, DataTypeOID: field.oid}
	}
	return r
}

func (r *metadataRows) FieldDescriptions() []pgconn.FieldDescription { return r.fields }

func (r *metadataRows) Next() bool { return r.index < len(r.values) }

func (r *metadataRows) Scan(destinations ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	values := r.values[r.index]
	r.index++
	if len(destinations) != len(values) {
		return errors.New("unexpected destination count")
	}
	for index, destination := range destinations {
		value, ok := destination.(**string)
		if !ok {
			return errors.New("unexpected destination type")
		}
		*value = values[index]
	}
	return nil
}

func (r *metadataRows) Close() { r.closed = true }

func (r *metadataRows) Err() error { return nil }
