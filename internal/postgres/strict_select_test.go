package postgres

import (
	"strings"
	"testing"
)

func TestValidateStrictSelectAcceptsOneSelectStatement(t *testing.T) {
	if err := ValidateStrictSelect("SELECT id, email FROM public.users WHERE id = 42"); err != nil {
		t.Fatalf("ValidateStrictSelect(single SELECT) error = %v", err)
	}
}

func TestValidateStrictSelectRejectsNonStrictStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{name: "multiple statements", sql: "SELECT 1; SELECT 2"},
		{name: "insert", sql: "INSERT INTO public.users (email) VALUES ('reader@example.test')"},
		{name: "update", sql: "UPDATE public.users SET email = 'reader@example.test'"},
		{name: "delete", sql: "DELETE FROM public.users"},
		{name: "DDL", sql: "CREATE TABLE public.blocked (id integer)"},
		{name: "transaction control", sql: "BEGIN"},
		{name: "row locking", sql: "SELECT * FROM public.users FOR UPDATE"},
		{name: "modifying CTE", sql: "WITH changed AS (DELETE FROM public.users RETURNING id) SELECT * FROM changed"},
		{name: "select into", sql: "SELECT id INTO public.blocked FROM public.users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertStrictSelectRejected(t, tt.sql)
		})
	}
}

func TestValidateStrictSelectRejectsDangerousBuiltinNamesAtEveryASTDepth(t *testing.T) {
	fullNames := []string{
		"pg_advisory_lock",
		"pg_advisory_lock_shared",
		"pg_try_advisory_lock",
		"pg_try_advisory_lock_shared",
		"pg_advisory_xact_lock",
		"pg_advisory_xact_lock_shared",
		"pg_try_advisory_xact_lock",
		"pg_try_advisory_xact_lock_shared",
		"pg_read_file",
		"pg_read_binary_file",
		"pg_stat_file",
		"pg_current_logfile",
		"loread",
		"lowrite",
		"pg_cancel_backend",
		"pg_terminate_backend",
		"pg_log_backend_memory_contexts",
		"pg_notify",
		"pg_export_snapshot",
		"pg_stat_clear_snapshot",
		"set_config",
		"setseed",
		"nextval",
		"setval",
		"currval",
		"lastval",
		"pg_reload_conf",
		"pg_rotate_logfile",
		"pg_promote",
		"pg_switch_wal",
		"pg_log_standby_snapshot",
		"pg_wal_replay_pause",
		"pg_wal_replay_resume",
		"pg_create_restore_point",
		"pg_backup_start",
		"pg_backup_stop",
		"pg_stat_force_next_flush",
		"pg_create_physical_replication_slot",
		"pg_drop_replication_slot",
		"pg_create_logical_replication_slot",
		"pg_copy_logical_replication_slot",
		"pg_logical_slot_get_changes",
		"pg_logical_slot_get_binary_changes",
		"pg_logical_slot_peek_changes",
		"pg_logical_slot_peek_binary_changes",
		"pg_replication_slot_advance",
		"pg_logical_emit_message",
		"ts_rewrite",
		"ts_stat",
	}
	prefixNames := []string{
		"pg_ls_dir",
		"lo_open",
		"pg_stat_reset",
		"pg_replication_origin_create",
		"query_to_xml",
	}

	for _, functionName := range fullNames {
		for _, qualifiedName := range []string{functionName, "pg_catalog." + functionName} {
			t.Run(qualifiedName, func(t *testing.T) {
				assertStrictSelectRejected(t, "SELECT "+qualifiedName+"('x')")
			})
			t.Run("nested "+qualifiedName, func(t *testing.T) {
				assertStrictSelectRejected(t, "SELECT 1 WHERE EXISTS (SELECT "+qualifiedName+"('x'))")
			})
		}
	}

	for _, functionName := range prefixNames {
		for _, qualifiedName := range []string{functionName, "pg_catalog." + functionName} {
			t.Run(qualifiedName, func(t *testing.T) {
				assertStrictSelectRejected(t, "SELECT "+qualifiedName+"('x')")
			})
			t.Run("nested "+qualifiedName, func(t *testing.T) {
				assertStrictSelectRejected(t, "SELECT 1 WHERE EXISTS (SELECT "+qualifiedName+"('x'))")
			})
		}
	}
}

func assertStrictSelectRejected(t *testing.T, sql string) {
	t.Helper()
	err := ValidateStrictSelect(sql)
	if err == nil {
		t.Fatalf("ValidateStrictSelect(%q) error = nil, want rejection", sql)
	}
	if !strings.HasPrefix(err.Error(), "[postgres_query_rejected]") {
		t.Fatalf("ValidateStrictSelect(%q) error = %q, want safe postgres_query_rejected code", sql, err)
	}
}
