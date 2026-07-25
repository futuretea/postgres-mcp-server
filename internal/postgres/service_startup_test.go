package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
)

func TestNewServiceStartupUsesReadOnlyExtendedProtocolAndRollsBack(t *testing.T) {
	pool := newFakePool([]fakeRow{
		{values: []any{"170000"}},
		{values: []any{false, false, false, false, false, false, false}},
	})

	service, err := NewService(t.Context(), validExecutionConfig(t), fakePoolFactory(pool))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() service = nil, want initialized service")
	}
	if len(pool.beginOptions) != 1 || pool.beginOptions[0].AccessMode != pgx.ReadOnly {
		t.Fatalf("startup transactions = %#v, want one read-only transaction", pool.beginOptions)
	}
	if pool.rollbackCount != 1 {
		t.Fatalf("startup rollback count = %d, want 1", pool.rollbackCount)
	}
	if len(pool.events) != 2 {
		t.Fatalf("startup SQL events = %#v, want version and service-role probes", pool.events)
	}
	for _, event := range pool.events {
		if !event.usesExecMode {
			t.Fatalf("startup query %q did not receive pgx.QueryExecModeExec", event.sql)
		}
	}
	if pool.closeCount != 0 {
		t.Fatalf("pool close count before Service.Close() = %d, want 0", pool.closeCount)
	}

	service.Close()
	service.Close()
	if pool.closeCount != 1 {
		t.Fatalf("pool close count after Service.Close() twice = %d, want 1", pool.closeCount)
	}
}

func TestNewServiceRejectsUnsupportedOrUnsafeTargetBeforeToolSQL(t *testing.T) {
	tests := []struct {
		name       string
		rows       []fakeRow
		wantPrefix string
		wantEvents int
	}{
		{
			name:       "PostgreSQL 16",
			rows:       []fakeRow{{values: []any{"160999"}}},
			wantPrefix: "[postgres_unsupported_server_version] PostgreSQL 17 or later is required",
			wantEvents: 1,
		},
	}

	unsafeRoles := []struct {
		name  string
		flags []any
	}{
		{name: "superuser", flags: []any{true, false, false, false, false, false, false}},
		{name: "BYPASSRLS", flags: []any{false, true, false, false, false, false, false}},
		{name: "replication", flags: []any{false, false, true, false, false, false, false}},
		{name: "pg_read_server_files member", flags: []any{false, false, false, true, false, false, false}},
		{name: "pg_write_server_files member", flags: []any{false, false, false, false, true, false, false}},
		{name: "pg_execute_server_program member", flags: []any{false, false, false, false, false, true, false}},
		{name: "pg_signal_backend member", flags: []any{false, false, false, false, false, false, true}},
	}
	for _, unsafeRole := range unsafeRoles {
		tests = append(tests, struct {
			name       string
			rows       []fakeRow
			wantPrefix string
			wantEvents int
		}{
			name: "unsafe service role: " + unsafeRole.name,
			rows: []fakeRow{
				{values: []any{"170000"}},
				{values: unsafeRole.flags},
			},
			wantPrefix: "[postgres_unsafe_service_role]",
			wantEvents: 2,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := newFakePool(tt.rows)
			service, err := NewService(t.Context(), validExecutionConfig(t), fakePoolFactory(pool))
			if service != nil {
				t.Fatalf("NewService() service = %#v, want nil on rejected startup", service)
			}
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Fatalf("NewService() error = %v, want safe error prefix %q", err, tt.wantPrefix)
			}
			if pool.closeCount != 1 {
				t.Fatalf("pool close count = %d, want 1", pool.closeCount)
			}
			if pool.rollbackCount != 1 {
				t.Fatalf("startup rollback count = %d, want 1", pool.rollbackCount)
			}
			if len(pool.events) != tt.wantEvents {
				t.Fatalf("startup SQL events = %#v, want %d; tool SQL must not run", pool.events, tt.wantEvents)
			}
		})
	}
}

func TestNewServiceMapsFactoryAndProbeCausesToSafeErrors(t *testing.T) {
	const marker = "POSTGRES_RAW_CAUSE_MARKER"
	cfg := validExecutionConfig(t)

	service, err := NewService(t.Context(), cfg, func(context.Context, *config.PostgresExecutionConfig) (Pool, error) {
		return nil, errors.New(marker)
	})
	if service != nil {
		t.Fatalf("NewService() service = %#v, want nil when pool factory fails", service)
	}
	assertSafePostgresError(t, err, marker)

	pool := newFakePool([]fakeRow{{err: errors.New(marker)}})
	service, err = NewService(t.Context(), cfg, fakePoolFactory(pool))
	if service != nil {
		t.Fatalf("NewService() service = %#v, want nil when version probe fails", service)
	}
	assertSafePostgresError(t, err, marker)
	if pool.rollbackCount != 1 || pool.closeCount != 1 {
		t.Fatalf("probe failure cleanup = rollback:%d close:%d, want rollback:1 close:1", pool.rollbackCount, pool.closeCount)
	}
}

func validExecutionConfig(t *testing.T) *config.PostgresExecutionConfig {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://reader@postgres.test:5432/app?sslmode=disable&default_query_exec_mode=simple_protocol")
	cfg, err := config.LoadPostgresExecutionConfig()
	if err != nil {
		t.Fatalf("LoadPostgresExecutionConfig() error = %v", err)
	}
	return cfg
}

func fakePoolFactory(pool *fakePool) PoolFactory {
	return func(context.Context, *config.PostgresExecutionConfig) (Pool, error) {
		return pool, nil
	}
}

func assertSafePostgresError(t *testing.T, err error, marker string) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), "[postgres_") {
		t.Fatalf("error = %v, want stable PostgreSQL error", err)
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("safe PostgreSQL error leaked marker %q: %q", marker, err)
	}
}

type fakePool struct {
	rows          []fakeRow
	events        []fakeQueryEvent
	beginOptions  []pgx.TxOptions
	rollbackCount int
	closeCount    int
}

type fakeQueryEvent struct {
	sql          string
	usesExecMode bool
}

func newFakePool(rows []fakeRow) *fakePool {
	return &fakePool{rows: append([]fakeRow(nil), rows...)}
}

func (p *fakePool) BeginTx(_ context.Context, options pgx.TxOptions) (Tx, error) {
	p.beginOptions = append(p.beginOptions, options)
	return fakeTx{pool: p}, nil
}

func (p *fakePool) Close() {
	p.closeCount++
}

type fakeTx struct {
	pool *fakePool
}

func (tx fakeTx) QueryRow(_ context.Context, sql string, args ...any) Row {
	usesExecMode := false
	for _, arg := range args {
		if mode, ok := arg.(pgx.QueryExecMode); ok && mode == pgx.QueryExecModeExec {
			usesExecMode = true
		}
	}
	tx.pool.events = append(tx.pool.events, fakeQueryEvent{sql: sql, usesExecMode: usesExecMode})
	if len(tx.pool.rows) == 0 {
		return fakeRow{err: errors.New("unexpected startup SQL")}
	}
	row := tx.pool.rows[0]
	tx.pool.rows = tx.pool.rows[1:]
	return row
}

func (tx fakeTx) Rollback(context.Context) error {
	tx.pool.rollbackCount++
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("Scan destinations = %d, values = %d", len(dest), len(r.values))
	}
	for index, value := range r.values {
		switch destination := dest[index].(type) {
		case *string:
			stringValue, ok := value.(string)
			if !ok {
				return fmt.Errorf("value %d = %T, cannot scan into string", index, value)
			}
			*destination = stringValue
		case *bool:
			boolValue, ok := value.(bool)
			if !ok {
				return fmt.Errorf("value %d = %T, cannot scan into bool", index, value)
			}
			*destination = boolValue
		default:
			return fmt.Errorf("unsupported scan destination %T", destination)
		}
	}
	return nil
}
