package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/futuretea/postgres-mcp-server/internal/cmd"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	text := out.String()
	for _, want := range []string{"mcp", "tools", "version", "completion"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
}

func TestRootWithoutArgsShowsHelp(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("root with no args: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Available Commands") {
		t.Fatalf("expected help output, got:\n%s", text)
	}
	if strings.Contains(text, "registered MCP tools") || strings.Contains(text, "starting HTTP MCP server") {
		t.Fatalf("root must not start the MCP server:\n%s", text)
	}
}

func TestToolsCatalogIsPostgresAndDoesNotNeedExecutionConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://reader:CATALOG_ONLY_MARKER@%zz")
	t.Setenv("PGHOST", "one-target,another-target")

	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools list must be catalog-only: %v", err)
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
		if !strings.Contains(out.String(), name) {
			t.Fatalf("tools list missing %q:\n%s", name, out.String())
		}
	}
	for _, removedTemplateTool := range []string{"ec" + "ho", "pi" + "ng"} {
		if strings.Contains(out.String(), removedTemplateTool) {
			t.Fatalf("tools list retained template tool %q:\n%s", removedTemplateTool, out.String())
		}
	}

	out.Reset()
	root = cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "describe", "postgres_query", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("tools describe postgres_query must be catalog-only: %v", err)
	}
	var description struct {
		Name        string         `json:"name"`
		InputSchema map[string]any `json:"inputSchema"`
	}
	if err := json.Unmarshal(out.Bytes(), &description); err != nil {
		t.Fatalf("tools describe output is not JSON: %v\n%s", err, out.String())
	}
	if description.Name != "postgres_query" || description.InputSchema["type"] != "object" {
		t.Fatalf("postgres_query description = %#v, want object schema", description)
	}
}

func TestToolsCallRejectsRawInvalidParametersBeforeDatabaseStartup(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: io.Discard})
	root.SetArgs([]string{"tools", "call", "postgres_query", "--params", "null"})
	err := root.Execute()
	if err == nil || err.Error() != "[postgres_invalid_params] invalid parameters" {
		t.Fatalf("tools call invalid parameters error = %v, want stable invalid-parameters error", err)
	}
	if out.Len() != 0 {
		t.Fatalf("tools call invalid parameters stdout = %q, want no success output", out.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "postgres-mcp-server") {
		t.Fatalf("unexpected version output: %q", out.String())
	}
}

func TestToolsUnknownTool(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "describe", "missing-tool"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}

	root = cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "missing-tool", "--params", "{}"})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error on call, got %v", err)
	}
}

func TestToolsRejectsNullParams(t *testing.T) {
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &out})
	root.SetArgs([]string{"tools", "call", "postgres_list_schemas", "--params", "null"})
	err := root.Execute()
	if err == nil || err.Error() != "[postgres_invalid_params] invalid parameters" {
		t.Fatalf("expected stable invalid-parameters error, got %v", err)
	}
}

func TestCommandsReturnOutputErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name string
		args []string
	}{
		{name: "completion script", args: []string{"completion", "bash"}},
		{name: "completion install", args: []string{"completion", "install"}},
		{name: "version", args: []string{"version"}},
		{name: "tools list", args: []string{"tools", "list"}},
		{name: "tools describe", args: []string{"tools", "describe", "postgres_query"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := cmd.NewRootCommand(cmd.IOStreams{
				In:     strings.NewReader(""),
				Out:    errorWriter{err: writeErr},
				ErrOut: io.Discard,
			})
			root.SetArgs(tt.args)
			if err := root.Execute(); !errors.Is(err, writeErr) {
				t.Fatalf("Execute() error = %v, want %v", err, writeErr)
			}
		})
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
