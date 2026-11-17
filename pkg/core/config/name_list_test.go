package config_test

import (
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
)

func TestLoadConfigNameListBoundary(t *testing.T) {
	t.Setenv("MCP_ENABLED_TOOLS", "")
	t.Setenv("MCP_DISABLED_TOOLS", "")
	t.Setenv("MCP_ENABLED_DOMAINS", "")
	t.Setenv("MCP_DISABLED_DOMAINS", "")
	allowQuery := writeConfig(t, "enabled_tools: [postgres_query]\n")

	t.Run("yaml allow-list", func(t *testing.T) {
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "postgres_query")
	})

	t.Run("empty environment keeps yaml", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "")
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "postgres_query")
	})

	t.Run("comma separated environment replaces yaml", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", " postgres_list_schemas , postgres_list_tables ")
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "postgres_list_schemas", "postgres_list_tables")
	})

	t.Run("spaces stay inside one name", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "postgres_list_schemas postgres_list_tables")
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "postgres_list_schemas postgres_list_tables")
	})

	t.Run("mixed blanks keep names", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "postgres_query, ,postgres_list_schemas")
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "postgres_query", "postgres_list_schemas")
	})

	t.Run("unknown name stays a name", func(t *testing.T) {
		t.Setenv("MCP_ENABLED_TOOLS", "missing")
		cfg, err := config.LoadConfig(allowQuery, viper.New())
		if err != nil {
			t.Fatal(err)
		}
		assertNames(t, cfg.EnabledTools, "missing")
	})

	t.Run("empty yaml list stays unrestricted", func(t *testing.T) {
		cfg, err := config.LoadConfig(writeConfig(t, "enabled_tools: []\n"), viper.New())
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.EnabledTools) != 0 || len(cfg.DisabledTools) != 0 || len(cfg.EnabledDomains) != 0 || len(cfg.DisabledDomains) != 0 {
			t.Fatalf("empty lists = %#v %#v %#v %#v", cfg.EnabledTools, cfg.DisabledTools, cfg.EnabledDomains, cfg.DisabledDomains)
		}
	})

	blankCases := []struct {
		name  string
		yaml  string
		env   string
		field string
	}{
		{name: "space environment", env: " \t\u00a0 ", field: "enabled_tools"},
		{name: "comma blank environment", env: " , , ", field: "enabled_tools"},
		{name: "yaml blanks", yaml: "enabled_tools: [\" \", \"\"]\n", field: "enabled_tools"},
		{name: "disabled tools", yaml: "disabled_tools: [\" \"]\n", field: "disabled_tools"},
		{name: "enabled domains", env: " \t", field: "enabled_domains"},
		{name: "disabled domains", yaml: "disabled_domains: [\" \"]\n", field: "disabled_domains"},
	}
	for _, tt := range blankCases {
		t.Run(tt.name, func(t *testing.T) {
			path := allowQuery
			if tt.yaml != "" {
				path = writeConfig(t, tt.yaml)
			}
			switch tt.field {
			case "enabled_tools":
				t.Setenv("MCP_ENABLED_TOOLS", tt.env)
			case "enabled_domains":
				t.Setenv("MCP_ENABLED_DOMAINS", tt.env)
			}
			_, err := config.LoadConfig(path, viper.New())
			want := tt.field + " contains no names"
			if err == nil || err.Error() != want || strings.Contains(err.Error(), "\u00a0") {
				t.Fatalf("LoadConfig error = %v, want %q", err, want)
			}
		})
	}
}

func assertNames(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("names = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names = %#v, want %#v", got, want)
		}
	}
}
