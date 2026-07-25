package mcp_test

import (
	"strings"
	"testing"

	"github.com/futuretea/postgres-mcp-server/internal/postgres"
	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
	mcpserver "github.com/futuretea/postgres-mcp-server/pkg/server/mcp"
	"github.com/futuretea/postgres-mcp-server/pkg/toolset"
)

func TestNewServerRegistersPostgresTools(t *testing.T) {
	server, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets:     []toolset.Toolset{postgres.NewToolset(nil)},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer server.Close()

	if !server.IsHealthy() {
		t.Fatal("expected healthy server")
	}
	tools := server.GetEnabledTools()
	if len(tools) != 7 {
		t.Fatalf("expected 7 PostgreSQL tools, got %v", tools)
	}
	names := map[string]bool{}
	for _, name := range tools {
		names[name] = true
	}
	for _, name := range []string{
		"postgres_get_server_info",
		"postgres_list_databases",
		"postgres_list_schemas",
		"postgres_list_tables",
		"postgres_describe_table",
		"postgres_list_indexes",
		"postgres_query",
	} {
		if !names[name] {
			t.Fatalf("missing PostgreSQL tool %q from %v", name, tools)
		}
	}
	for _, removedTemplateTool := range []string{"ec" + "ho", "pi" + "ng"} {
		if names[removedTemplateTool] {
			t.Fatalf("removed tool %q remained registered: %v", removedTemplateTool, tools)
		}
	}
}

func TestNewServerRequiresStaticConfig(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{})
	if err == nil || !strings.Contains(err.Error(), "static config is required") {
		t.Fatalf("expected static config error, got %v", err)
	}
}

func TestNewServerRejectsEmptyToolFilter(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{
			LogLevel:     "info",
			EnabledTools: []string{"missing"},
		},
		Toolsets: []toolset.Toolset{postgres.NewToolset(nil)},
	})
	if err == nil || !strings.Contains(err.Error(), "no tools registered") {
		t.Fatalf("expected no tools registered error, got %v", err)
	}
}

func TestNewServerRejectsDuplicateToolNames(t *testing.T) {
	_, err := mcpserver.NewServer(mcpserver.Configuration{
		StaticConfig: &config.StaticConfig{LogLevel: "info"},
		Toolsets: []toolset.Toolset{
			postgres.NewToolset(nil),
			postgres.NewToolset(nil),
		},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate tool name "postgres_get_server_info"`) {
		t.Fatalf("expected duplicate tool name error, got %v", err)
	}
}

func TestNewTextResult(t *testing.T) {
	ok := mcpserver.NewTextResult("hello", nil)
	if ok.IsError || len(ok.Content) != 1 {
		t.Fatalf("unexpected ok result: %+v", ok)
	}

	fail := mcpserver.NewTextResult("", errString("boom"))
	if !fail.IsError {
		t.Fatal("expected error result")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
