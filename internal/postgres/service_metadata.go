package postgres

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type queryResult struct {
	fieldNames []string
	typeOIDs   []uint32
	rows       [][]*string
}

func (s *Service) queryText(ctx context.Context, sql string, args ...any) (result queryResult, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return queryResult{}, metadataError()
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	queryTransaction, ok := tx.(queryTx)
	if !ok {
		return queryResult{}, metadataError()
	}

	queryArgs := make([]any, 0, len(args)+2)
	queryArgs = append(queryArgs, pgx.QueryResultFormats{pgx.TextFormatCode}, pgx.QueryExecModeExec)
	queryArgs = append(queryArgs, args...)
	rows, err := queryTransaction.Query(ctx, sql, queryArgs...)
	if err != nil {
		return queryResult{}, metadataError()
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	result.fieldNames = make([]string, len(fields))
	result.typeOIDs = make([]uint32, len(fields))
	for index, field := range fields {
		result.fieldNames[index] = string(field.Name)
		result.typeOIDs[index] = field.DataTypeOID
	}

	for rows.Next() {
		values := make([]*string, len(fields))
		destinations := make([]any, len(fields))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return queryResult{}, metadataError()
		}
		result.rows = append(result.rows, values)
	}
	if err := rows.Err(); err != nil {
		return queryResult{}, metadataError()
	}
	return result, nil
}

// GetServerInfo returns server identity values without exposing configuration
// details or a pool/connection object.
func (s *Service) GetServerInfo(ctx context.Context) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT version()::text AS server_version,
		current_setting('server_version_num')::text AS server_version_num,
		current_database()::text AS database,
		current_user::text AS "user",
		host(inet_server_addr())::text AS host,
		inet_server_port()::text AS port`)
	if err != nil || len(result.rows) != 1 {
		return nil, metadataError()
	}
	return objectFromRow(result.fieldNames, result.rows[0]), nil
}

func (s *Service) ListDatabases(ctx context.Context) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT datname::text AS name,
		pg_get_userbyid(datdba)::text AS owner,
		pg_encoding_to_char(encoding)::text AS encoding
		FROM pg_database
		WHERE datallowconn
		ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	return map[string]any{"databases": objectsFromRows(result)}, nil
}

func (s *Service) ListSchemas(ctx context.Context) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT schema_name::text AS name
		FROM information_schema.schemata
		ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schemas": objectsFromRows(result)}, nil
}

func (s *Service) ListTables(ctx context.Context, schema string) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT table_name::text AS name, table_type::text AS kind
		FROM information_schema.tables
		WHERE table_schema = $1
		ORDER BY table_name`, schema)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "tables": objectsFromRows(result)}, nil
}

func (s *Service) DescribeTable(ctx context.Context, schema, table string) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT ordinal_position::text AS ordinal_position,
		column_name::text AS name,
		data_type::text AS postgres_type,
		(CASE WHEN is_nullable = 'YES' THEN 'true' ELSE 'false' END)::text AS nullable,
		column_default::text AS "default",
		NULLIF(is_identity, 'NO')::text AS identity,
		NULLIF(is_generated, 'NEVER')::text AS generated
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}

	columns := make([]map[string]any, 0, len(result.rows))
	for _, row := range result.rows {
		column := objectFromRow(result.fieldNames, row)
		ordinal, err := strconv.Atoi(stringValue(column["ordinal_position"]))
		if err != nil {
			return nil, metadataError()
		}
		column["ordinal_position"] = ordinal
		column["nullable"] = column["nullable"] == "true"
		columns = append(columns, column)
	}
	return map[string]any{"schema": schema, "table": table, "columns": columns}, nil
}

func (s *Service) ListIndexes(ctx context.Context, schema, table string) (map[string]any, error) {
	result, err := s.queryText(ctx, `SELECT indexes.indexname::text AS name, indexes.indexdef::text AS definition
		FROM pg_indexes AS indexes
		WHERE indexes.schemaname = $1 AND indexes.tablename = $2
		AND EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2
		)
		ORDER BY indexes.indexname`, schema, table)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": schema, "table": table, "indexes": objectsFromRows(result)}, nil
}

func (s *Service) Query(ctx context.Context, sql string) (map[string]any, error) {
	if err := ValidateStrictSelect(sql); err != nil {
		return nil, err
	}
	result, err := s.queryText(ctx, sql)
	if err != nil {
		return nil, queryError()
	}

	columns := make([]map[string]any, len(result.fieldNames))
	for index, name := range result.fieldNames {
		postgresType, err := s.formatType(ctx, result.typeOIDs[index])
		if err != nil {
			return nil, queryError()
		}
		columns[index] = map[string]any{"name": name, "postgres_type": postgresType}
	}

	rows := make([][]any, len(result.rows))
	for rowIndex, row := range result.rows {
		rows[rowIndex] = make([]any, len(row))
		for columnIndex, value := range row {
			if value != nil {
				rows[rowIndex][columnIndex] = *value
			}
		}
	}
	return map[string]any{"columns": columns, "rows": rows}, nil
}

func (s *Service) formatType(ctx context.Context, oid uint32) (string, error) {
	result, err := s.queryText(ctx, `SELECT format_type($1::oid, NULL)::text AS postgres_type`, int64(oid))
	if err != nil || len(result.rows) != 1 || len(result.rows[0]) != 1 || result.rows[0][0] == nil {
		return "", metadataError()
	}
	return *result.rows[0][0], nil
}

func objectsFromRows(result queryResult) []map[string]any {
	objects := make([]map[string]any, 0, len(result.rows))
	for _, row := range result.rows {
		objects = append(objects, objectFromRow(result.fieldNames, row))
	}
	return objects
}

func objectFromRow(fields []string, values []*string) map[string]any {
	object := make(map[string]any, len(fields))
	for index, field := range fields {
		if values[index] != nil {
			object[field] = *values[index]
		} else {
			object[field] = nil
		}
	}
	return object
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return ""
}
