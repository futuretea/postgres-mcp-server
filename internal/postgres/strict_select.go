package postgres

import (
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var rejectedFunctionNames = map[string]struct{}{
	"pg_advisory_lock":                    {},
	"pg_advisory_lock_shared":             {},
	"pg_try_advisory_lock":                {},
	"pg_try_advisory_lock_shared":         {},
	"pg_advisory_xact_lock":               {},
	"pg_advisory_xact_lock_shared":        {},
	"pg_try_advisory_xact_lock":           {},
	"pg_try_advisory_xact_lock_shared":    {},
	"pg_read_file":                        {},
	"pg_read_binary_file":                 {},
	"pg_stat_file":                        {},
	"pg_current_logfile":                  {},
	"loread":                              {},
	"lowrite":                             {},
	"pg_cancel_backend":                   {},
	"pg_terminate_backend":                {},
	"pg_log_backend_memory_contexts":      {},
	"pg_notify":                           {},
	"pg_export_snapshot":                  {},
	"pg_stat_clear_snapshot":              {},
	"set_config":                          {},
	"setseed":                             {},
	"nextval":                             {},
	"setval":                              {},
	"currval":                             {},
	"lastval":                             {},
	"pg_reload_conf":                      {},
	"pg_rotate_logfile":                   {},
	"pg_promote":                          {},
	"pg_switch_wal":                       {},
	"pg_log_standby_snapshot":             {},
	"pg_wal_replay_pause":                 {},
	"pg_wal_replay_resume":                {},
	"pg_create_restore_point":             {},
	"pg_backup_start":                     {},
	"pg_backup_stop":                      {},
	"pg_stat_force_next_flush":            {},
	"pg_create_physical_replication_slot": {},
	"pg_drop_replication_slot":            {},
	"pg_create_logical_replication_slot":  {},
	"pg_copy_logical_replication_slot":    {},
	"pg_logical_slot_get_changes":         {},
	"pg_logical_slot_get_binary_changes":  {},
	"pg_logical_slot_peek_changes":        {},
	"pg_logical_slot_peek_binary_changes": {},
	"pg_replication_slot_advance":         {},
	"pg_logical_emit_message":             {},
	"ts_rewrite":                          {},
	"ts_stat":                             {},
}

var rejectedFunctionPrefixes = []string{
	"pg_ls_",
	"lo_",
	"pg_stat_reset",
	"pg_replication_origin_",
	"query_to_",
}

// ValidateStrictSelect accepts exactly one PostgreSQL SELECT statement with no
// row locks, SELECT INTO, modifying CTE, or explicitly rejected PostgreSQL 17
// built-in function invocation. It deliberately does not claim to classify
// custom functions, extensions, FDW calls, or future built-ins.
func ValidateStrictSelect(sql string) error {
	tree, err := pg_query.Parse(sql)
	if err != nil || tree == nil || len(tree.GetStmts()) != 1 {
		return rejectedQuery()
	}
	root := tree.GetStmts()[0].GetStmt()
	if root == nil || root.GetSelectStmt() == nil {
		return rejectedQuery()
	}

	valid := true
	walkMessage(root.ProtoReflect(), func(message protoreflect.Message) {
		switch node := message.Interface().(type) {
		case *pg_query.SelectStmt:
			if node.GetIntoClause() != nil || len(node.GetLockingClause()) > 0 {
				valid = false
			}
		case *pg_query.CommonTableExpr:
			if node.GetCtequery() == nil || node.GetCtequery().GetSelectStmt() == nil {
				valid = false
			}
		case *pg_query.FuncCall:
			if isRejectedFunction(node) {
				valid = false
			}
		}
	})
	if !valid {
		return rejectedQuery()
	}
	return nil
}

func rejectedQuery() error {
	return postgresError("query_rejected", "query is not an allowed SELECT statement")
}

func isRejectedFunction(call *pg_query.FuncCall) bool {
	parts := call.GetFuncname()
	if len(parts) == 0 {
		return true
	}
	terminal := parts[len(parts)-1].GetString_()
	if terminal == nil {
		return true
	}
	name := strings.ToLower(terminal.GetSval())
	if _, found := rejectedFunctionNames[name]; found {
		return true
	}
	for _, prefix := range rejectedFunctionPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func walkMessage(message protoreflect.Message, visit func(protoreflect.Message)) {
	if !message.IsValid() {
		return
	}
	visit(message)
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsList() {
			list := value.List()
			for index := 0; index < list.Len(); index++ {
				walkValue(list.Get(index), visit)
			}
			return true
		}
		if field.IsMap() {
			value.Map().Range(func(_ protoreflect.MapKey, mapValue protoreflect.Value) bool {
				walkValue(mapValue, visit)
				return true
			})
			return true
		}
		walkValue(value, visit)
		return true
	})
}

func walkValue(value protoreflect.Value, visit func(protoreflect.Message)) {
	if message, ok := value.Interface().(protoreflect.Message); ok {
		walkMessage(message, visit)
	}
}
