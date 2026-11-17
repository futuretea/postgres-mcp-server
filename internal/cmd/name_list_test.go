package cmd_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/futuretea/postgres-mcp-server/internal/cmd"
)

func TestToolsListNameListBoundary(t *testing.T) {
	path := writeNameListConfig(t, "enabled_tools: [postgres_query]\n")

	t.Run("yaml allow-list", func(t *testing.T) {
		names, err := runToolsList(t, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(names, ",") != "postgres_query" {
			t.Fatalf("tools = %v, want postgres_query", names)
		}
	})

	t.Run("empty environment keeps yaml", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "")
		names, err := runToolsList(t, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(names, ",") != "postgres_query" {
			t.Fatalf("tools = %v, want postgres_query", names)
		}
	})

	t.Run("flag overrides environment", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "postgres_query")
		names, err := runToolsList(t, path, []string{"--enabled-tools", "postgres_list_databases"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(names, ",") != "postgres_list_databases" {
			t.Fatalf("tools = %v, want postgres_list_databases", names)
		}
	})

	t.Run("omitted flag keeps environment", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "postgres_list_databases")
		names, err := runToolsList(t, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(names, ",") != "postgres_list_databases" {
			t.Fatalf("tools = %v, want postgres_list_databases", names)
		}
	})

	t.Run("explicit empty flag allows every tool", func(t *testing.T) {
		names, err := runToolsList(t, path, []string{"--enabled-tools", ""})
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 7 {
			t.Fatalf("tools = %v, want all 7 tools", names)
		}
	})

	t.Run("unknown name stays a name", func(t *testing.T) {
		names, err := runToolsList(t, path, []string{"--enabled-tools", "missing"})
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 0 {
			t.Fatalf("tools = %v, want no listed tools", names)
		}
	})

	blankFlags := []struct {
		name string
		args []string
		want string
	}{
		{name: "blank enabled tools", args: []string{"--enabled-tools", " "}, want: "enabled_tools contains no names"},
		{name: "comma blank enabled tools", args: []string{"--enabled-tools", " , "}, want: "enabled_tools contains no names"},
		{name: "blank disabled tools", args: []string{"--disabled-tools", " "}, want: "disabled_tools contains no names"},
		{name: "blank enabled domains", args: []string{"--enable-domains", " "}, want: "enabled_domains contains no names"},
		{name: "blank disabled domains", args: []string{"--disable-domains", " "}, want: "disabled_domains contains no names"},
	}
	for _, tt := range blankFlags {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &bytes.Buffer{}})
			root.SetArgs(append([]string{"--config", path}, append(tt.args, "tools", "list")...))
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(out.String(), "postgres_") {
				t.Fatalf("error = %v stdout = %q, want %q and no tool list", err, out.String(), tt.want)
			}
		})
	}

	t.Run("blank environment does not clear the allow-list into every tool", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " ")
		var out bytes.Buffer
		root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &bytes.Buffer{}})
		root.SetArgs([]string{"--config", path, "tools", "list"})
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "enabled_tools contains no names") || strings.Contains(out.String(), "postgres_list_schemas") {
			t.Fatalf("error = %v stdout = %q, want enabled_tools contains no names and no expanded catalog", err, out.String())
		}
	})
}

func TestMCPBlankEnabledToolsDoesNotDial(t *testing.T) {
	listener := listenLocal(t)
	accepts := countAccepts(t, listener)
	setSyntheticPostgresEnv(t, listener)

	t.Run("blank list", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " ")
		err := runMCP(t)
		if err == nil || !strings.Contains(err.Error(), "enabled_tools contains no names") || accepts.Load() != 0 {
			t.Fatalf("mcp error = %v accepts = %d, want enabled_tools contains no names and no connection", err, accepts.Load())
		}
	})

	t.Run("legal list reaches the database boundary", func(t *testing.T) {
		before := accepts.Load()
		t.Setenv("MCP_ENABLED_TOOLS", "postgres_query")
		err := runMCP(t)
		if err == nil || strings.Contains(err.Error(), "contains no names") {
			t.Fatalf("mcp error = %v, want a database failure", err)
		}
		if accepts.Load() <= before {
			t.Fatal("legal enabled_tools did not reach the synthetic database listener")
		}
	})
}

func runToolsList(t *testing.T, path string, extra []string) ([]string, error) {
	t.Helper()
	var out bytes.Buffer
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &out, ErrOut: &bytes.Buffer{}})
	args := []string{"--config", path}
	args = append(args, extra...)
	args = append(args, "tools", "list")
	root.SetArgs(args)
	err := root.Execute()
	return listedToolNames(out.String()), err
}

func listedToolNames(text string) []string {
	var names []string
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "NAME" || !strings.HasPrefix(fields[0], "postgres_") {
			continue
		}
		names = append(names, fields[0])
	}
	return names
}

func writeNameListConfig(t *testing.T, contents string) string {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func countAccepts(t *testing.T, listener net.Listener) *atomic.Int32 {
	t.Helper()
	var accepts atomic.Int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close()
		}
	}()
	return &accepts
}

func setSyntheticPostgresEnv(t *testing.T, listener net.Listener) {
	t.Helper()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGSERVICE", "")
	t.Setenv("PGPASSWORD", "")
	for _, key := range []string{"MCP_POSTGRES_HOST", "PGHOST"} {
		t.Setenv(key, "127.0.0.1")
	}
	for _, key := range []string{"MCP_POSTGRES_PORT", "PGPORT"} {
		t.Setenv(key, port)
	}
	for _, key := range []string{"MCP_POSTGRES_DATABASE", "PGDATABASE"} {
		t.Setenv(key, "postgres")
	}
	for _, key := range []string{"MCP_POSTGRES_USERNAME", "PGUSER"} {
		t.Setenv(key, "postgres")
	}
	for _, key := range []string{"MCP_POSTGRES_SSLMODE", "PGSSLMODE"} {
		t.Setenv(key, "disable")
	}
}

func runMCP(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	root := cmd.NewRootCommand(cmd.IOStreams{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}})
	root.SetContext(ctx)
	root.SetArgs([]string{"mcp"})
	return root.Execute()
}
