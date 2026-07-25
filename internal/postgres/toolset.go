package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

const postgresDomain = "postgres"

type handlerService interface {
	GetServerInfo(context.Context) (map[string]any, error)
	ListDatabases(context.Context) (map[string]any, error)
	ListSchemas(context.Context) (map[string]any, error)
	ListTables(context.Context, string) (map[string]any, error)
	DescribeTable(context.Context, string, string) (map[string]any, error)
	ListIndexes(context.Context, string, string) (map[string]any, error)
	Query(context.Context, string) (map[string]any, error)
}

// Toolset contains the PostgreSQL MCP tool definitions. A nil Service is
// intentional for catalog-only commands: handlers are never called by list or
// describe, and execution paths construct a Service before calling a handler.
type Toolset struct {
	service handlerService
}

func NewToolset(service handlerService) *Toolset {
	return &Toolset{service: service}
}

func (t *Toolset) GetName() string { return postgresDomain }

func (t *Toolset) GetDescription() string {
	return "Read-only PostgreSQL discovery and strict SELECT tools"
}

func (t *Toolset) GetTools() []toolset.ServerTool {
	return []toolset.ServerTool{
		t.newTool("postgres_get_server_info", "Get PostgreSQL server information", nil, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.GetServerInfo(ctx) })
		}),
		t.newTool("postgres_list_databases", "List visible PostgreSQL databases", nil, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.ListDatabases(ctx) })
		}),
		t.newTool("postgres_list_schemas", "List visible schemas in the current database", nil, func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.ListSchemas(ctx) })
		}),
		t.newTool("postgres_list_tables", "List visible tables and views in a schema", []string{"schema"}, func(ctx context.Context, params map[string]any) (map[string]any, error) {
			schema := params["schema"].(string)
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.ListTables(ctx, schema) })
		}),
		t.newTool("postgres_describe_table", "Describe table columns", []string{"schema", "table"}, func(ctx context.Context, params map[string]any) (map[string]any, error) {
			schema, table := params["schema"].(string), params["table"].(string)
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.DescribeTable(ctx, schema, table) })
		}),
		t.newTool("postgres_list_indexes", "List indexes for a visible table", []string{"schema", "table"}, func(ctx context.Context, params map[string]any) (map[string]any, error) {
			schema, table := params["schema"].(string), params["table"].(string)
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.ListIndexes(ctx, schema, table) })
		}),
		t.newTool("postgres_query", "Run one strict read-only SELECT query", []string{"sql"}, func(ctx context.Context, params map[string]any) (map[string]any, error) {
			sql := params["sql"].(string)
			return t.call(ctx, func(service handlerService) (map[string]any, error) { return service.Query(ctx, sql) })
		}),
	}
}

func (t *Toolset) newTool(name, description string, required []string, call func(context.Context, map[string]any) (map[string]any, error)) toolset.ServerTool {
	return toolset.ServerTool{
		Tool:                  mcp.NewToolWithRawSchema(name, description, schemaFor(required)),
		Domain:                postgresDomain,
		NormalizeRawArguments: func(raw any) (map[string]any, error) { return DecodeToolParams(name, raw) },
		Handler: func(ctx context.Context, params map[string]any) (string, error) {
			data, err := call(ctx, params)
			if err != nil {
				return "", err
			}
			return EncodeToolResult(name, data)
		},
	}
}

func (t *Toolset) call(_ context.Context, call func(handlerService) (map[string]any, error)) (map[string]any, error) {
	if t.service == nil {
		return nil, connectionError()
	}
	return call(t.service)
}

func schemaFor(required []string) json.RawMessage {
	properties := make(map[string]any, len(required))
	for _, name := range required {
		properties[name] = map[string]any{"type": "string", "minLength": 1, "pattern": ".*\\S.*"}
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return raw
}

var toolParameters = map[string][]string{
	"postgres_get_server_info": nil,
	"postgres_list_databases":  nil,
	"postgres_list_schemas":    nil,
	"postgres_list_tables":     {"schema"},
	"postgres_describe_table":  {"schema", "table"},
	"postgres_list_indexes":    {"schema", "table"},
	"postgres_query":           {"sql"},
}

// DecodeToolParams is the PostgreSQL-only decoder behind the generic raw MCP
// argument normalizer. It deliberately accepts only object values and rejects
// unknown, missing, non-string, null, and blank fields before a handler runs.
func DecodeToolParams(toolName string, raw any) (map[string]any, error) {
	required, knownTool := toolParameters[toolName]
	params, isObject := raw.(map[string]any)
	if !knownTool || !isObject {
		return nil, InvalidParametersError()
	}
	if len(params) != len(required) {
		return nil, InvalidParametersError()
	}
	for _, name := range required {
		value, found := params[name]
		text, isString := value.(string)
		if !found || !isString || strings.TrimSpace(text) == "" {
			return nil, InvalidParametersError()
		}
	}
	return params, nil
}

// InvalidParametersError is shared by the CLI parser and the PostgreSQL MCP
// decoder so malformed JSON and invalid raw MCP values have one safe contract.
func InvalidParametersError() error {
	return postgresError("invalid_params", "invalid parameters")
}

// EncodeToolResult writes the entire result before returning it, so an
// encoding failure cannot produce partial successful MCP content.
func EncodeToolResult(toolName string, data map[string]any) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"schema_version": 1,
		"tool":           toolName,
		"data":           data,
	})
	if err != nil {
		return "", postgresError("result_encoding_failed", "result encoding failed")
	}
	return string(raw), nil
}
