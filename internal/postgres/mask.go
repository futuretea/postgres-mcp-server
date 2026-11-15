package postgres

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
)

const (
	maxMaskedNodes       = 10000
	maxMaskedDepth       = 32
	maxMaskedResultBytes = 1 << 20
)

type valueKind int

const (
	kindText valueKind = iota
	kindIdentifier
	kindNumber
	kindBool
)

type shape struct {
	fields map[string]valueKind
	lists  map[string]shape
}

type masker struct {
	policy masking.Policy
	budget *masking.Budget
}

func maskToolData(active masker, toolName string, data map[string]any) (map[string]any, error) {
	spec, ok := toolShapes[toolName]
	if !ok || data == nil {
		return nil, masking.ErrFailed
	}
	nodes := 0
	return active.maskObject(spec, data, 1, &nodes)
}

func materializeQuery(active masker, view queryResultView) (map[string]any, error) {
	if !active.policy.Enabled() {
		return queryResultMap(view), nil
	}
	nodes := 0
	columns := make([]map[string]any, len(view.columns))
	seen := make(map[string]struct{}, len(view.columns))
	for index, column := range view.columns {
		if !countNode(&nodes) {
			return nil, masking.ErrFailed
		}
		name, err := maskIdentifier(active.policy, column.name, active.budget)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name]; exists {
			return nil, masking.ErrFailed
		}
		seen[name] = struct{}{}
		postgresType, err := active.maskText(column.postgresType)
		if err != nil {
			return nil, err
		}
		columns[index] = map[string]any{"name": name, "postgres_type": postgresType}
	}
	rows := make([][]any, len(view.rows))
	for rowIndex, row := range view.rows {
		if len(row) != len(view.columns) {
			return nil, masking.ErrFailed
		}
		masked := make([]any, len(row))
		for columnIndex, cell := range row {
			if !countNode(&nodes) {
				return nil, masking.ErrFailed
			}
			value, err := active.maskQueryCell(view.columns[columnIndex], cell)
			if err != nil {
				return nil, err
			}
			masked[columnIndex] = value
		}
		rows[rowIndex] = masked
	}
	return map[string]any{"columns": columns, "rows": rows}, nil
}

func (m masker) maskQueryCell(column queryColumn, cell *string) (any, error) {
	if cell == nil {
		return nil, nil
	}
	class := scalarClass(column.typeOID)
	if m.policy.MatchName(column.name) {
		if class == classNumber || class == classBool {
			return nil, nil
		}
		return m.policy.Placeholder(), nil
	}
	switch class {
	case classNumber, classBool:
		return m.maskWholeScalar(*cell)
	case classBytea:
		return m.maskByteaHex(*cell)
	case classByteaArray:
		return nil, masking.ErrFailed
	default:
		return m.maskText(*cell)
	}
}

func scalarClass(oid uint32) int {
	switch oid {
	case pgtype.BoolOID:
		return classBool
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID, pgtype.Float4OID, pgtype.Float8OID, pgtype.NumericOID:
		return classNumber
	case pgtype.ByteaOID:
		return classBytea
	case pgtype.ByteaArrayOID:
		return classByteaArray
	default:
		return classText
	}
}

const (
	classText = iota
	classNumber
	classBool
	classBytea
	classByteaArray
)

func (m masker) maskByteaHex(text string) (string, error) {
	if !strings.HasPrefix(text, `\x`) {
		return "", masking.ErrFailed
	}
	digits := text[2:]
	if len(digits)%2 != 0 || !onlyHex(digits) {
		return "", masking.ErrFailed
	}
	raw, err := hex.DecodeString(digits)
	if err != nil {
		return "", masking.ErrFailed
	}
	masked, err := m.policy.MaskBytes(raw, m.budget)
	if err != nil {
		return "", err
	}
	if bytes.Equal(masked, raw) {
		return text, nil
	}
	return `\x` + hex.EncodeToString(masked), nil
}

func onlyHex(digits string) bool {
	for i := 0; i < len(digits); i++ {
		c := digits[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func (m masker) maskObject(spec shape, data map[string]any, depth int, nodes *int) (map[string]any, error) {
	if depth > maxMaskedDepth || !countNode(nodes) {
		return nil, masking.ErrFailed
	}
	if len(data) != len(spec.fields)+len(spec.lists) {
		return nil, masking.ErrFailed
	}
	out := make(map[string]any, len(data))
	for name, kind := range spec.fields {
		value, ok := data[name]
		if !ok {
			return nil, masking.ErrFailed
		}
		masked, err := m.maskScalar(kind, value)
		if err != nil {
			return nil, err
		}
		out[name] = masked
	}
	for name, child := range spec.lists {
		value, ok := data[name]
		if !ok {
			return nil, masking.ErrFailed
		}
		items, ok := value.([]map[string]any)
		if !ok {
			return nil, masking.ErrFailed
		}
		maskedItems := make([]map[string]any, len(items))
		seenNames := make(map[string]struct{}, len(items))
		checkNames := child.fields["name"] == kindIdentifier
		for index, item := range items {
			masked, err := m.maskObject(child, item, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			if checkNames {
				name, ok := masked["name"].(string)
				if !ok {
					return nil, masking.ErrFailed
				}
				if _, exists := seenNames[name]; exists {
					return nil, masking.ErrFailed
				}
				seenNames[name] = struct{}{}
			}
			maskedItems[index] = masked
		}
		out[name] = maskedItems
	}
	return out, nil
}

func (m masker) maskScalar(kind valueKind, value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	switch kind {
	case kindNumber:
		number, ok := value.(int)
		if !ok {
			return nil, masking.ErrFailed
		}
		return m.maskTypedScalar(strconv.Itoa(number), number)
	case kindBool:
		flag, ok := value.(bool)
		if !ok {
			return nil, masking.ErrFailed
		}
		word := "false"
		if flag {
			word = "true"
		}
		return m.maskTypedScalar(word, flag)
	case kindText:
		text, ok := value.(string)
		if !ok {
			return nil, masking.ErrFailed
		}
		return m.maskText(text)
	case kindIdentifier:
		text, ok := value.(string)
		if !ok {
			return nil, masking.ErrFailed
		}
		return maskIdentifier(m.policy, text, m.budget)
	default:
		return nil, masking.ErrFailed
	}
}

func (m masker) maskWholeScalar(original string) (any, error) {
	hit, err := m.policy.MatchScalar(original, m.budget)
	if err != nil {
		return nil, err
	}
	if hit {
		return nil, nil
	}
	return original, nil
}

func (m masker) maskTypedScalar(original string, unmatched any) (any, error) {
	hit, err := m.policy.MatchScalar(original, m.budget)
	if err != nil {
		return nil, err
	}
	if hit {
		return nil, nil
	}
	return unmatched, nil
}

func (m masker) maskText(text string) (string, error) {
	return m.policy.MaskText(text, m.budget)
}

func maskIdentifier(policy masking.Policy, value string, budget *masking.Budget) (string, error) {
	if policy.MatchName(value) {
		return policy.Placeholder(), nil
	}
	return policy.MaskText(value, budget)
}

func countNode(nodes *int) bool {
	*nodes++
	return *nodes <= maxMaskedNodes
}

var toolShapes = map[string]shape{
	"postgres_get_server_info": {
		fields: map[string]valueKind{
			"server_version":     kindText,
			"server_version_num": kindText,
			"database":           kindIdentifier,
			"user":               kindIdentifier,
			"host":               kindText,
			"port":               kindText,
		},
	},
	"postgres_list_databases": {
		lists: map[string]shape{
			"databases": {fields: map[string]valueKind{"name": kindIdentifier, "owner": kindIdentifier, "encoding": kindText}},
		},
	},
	"postgres_list_schemas": {
		lists: map[string]shape{
			"schemas": {fields: map[string]valueKind{"name": kindIdentifier}},
		},
	},
	"postgres_list_tables": {
		fields: map[string]valueKind{"schema": kindIdentifier},
		lists: map[string]shape{
			"tables": {fields: map[string]valueKind{"name": kindIdentifier, "kind": kindText}},
		},
	},
	"postgres_describe_table": {
		fields: map[string]valueKind{"schema": kindIdentifier, "table": kindIdentifier},
		lists: map[string]shape{
			"columns": {fields: map[string]valueKind{
				"ordinal_position": kindNumber,
				"name":             kindIdentifier,
				"postgres_type":    kindText,
				"nullable":         kindBool,
				"default":          kindText,
				"identity":         kindText,
				"generated":        kindText,
			}},
		},
	},
	"postgres_list_indexes": {
		fields: map[string]valueKind{"schema": kindIdentifier, "table": kindIdentifier},
		lists: map[string]shape{
			"indexes": {fields: map[string]valueKind{"name": kindIdentifier, "definition": kindText}},
		},
	},
}
