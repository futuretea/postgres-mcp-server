package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
)

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Port != 0 {
		t.Fatalf("expected port 0, got %d", cfg.Port)
	}
	if cfg.Listen != "127.0.0.1" {
		t.Fatalf("expected listen 127.0.0.1, got %q", cfg.Listen)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected log_level info, got %q", cfg.LogLevel)
	}
}

func TestLoadConfigAllowUnsafeDatabaseRole(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{name: "default", env: "", want: false},
		{name: "enabled by environment", env: "true", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("MCP_ALLOW_UNSAFE_DATABASE_ROLE", tt.env)
			cfg, err := config.LoadConfig("", viper.New())
			if err != nil {
				t.Fatalf("LoadConfig: %v", err)
			}
			if cfg.AllowUnsafeDatabaseRole != tt.want {
				t.Fatalf("AllowUnsafeDatabaseRole = %v, want %v", cfg.AllowUnsafeDatabaseRole, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.StaticConfig
		wantSub string
	}{
		{
			name:    "bad port",
			cfg:     config.StaticConfig{Port: 70000, LogLevel: "info", Listen: "127.0.0.1"},
			wantSub: "port must be between",
		},
		{
			name:    "empty listen",
			cfg:     config.StaticConfig{Port: 8080, LogLevel: "info", Listen: " "},
			wantSub: "listen must be set",
		},
		{
			name:    "bad log level",
			cfg:     config.StaticConfig{Port: 0, LogLevel: "nope", Listen: "127.0.0.1"},
			wantSub: "invalid log_level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestYAMLMaskingEnablesOnlyWhenExplicit(t *testing.T) {
	enabledPath := writeConfig(t, "masking:\n  enabled: true\n  names: [\"synthetic_field\"]\n")
	cfg, err := config.LoadConfig(enabledPath, viper.New())
	if err != nil || !cfg.Masking.Enabled() || !cfg.Masking.MatchName("synthetic_field") {
		t.Fatalf("yaml masking enabled=%v err=%v", cfg != nil && cfg.Masking.Enabled(), err)
	}
	disabledPath := writeConfig(t, "masking:\n  enabled: false\n  values: [\"synthetic-token-value\"]\n")
	cfg, err = config.LoadConfig(disabledPath, viper.New())
	if err != nil || cfg.Masking.Enabled() {
		t.Fatalf("explicit false masking enabled=%v err=%v", cfg != nil && cfg.Masking.Enabled(), err)
	}
}

func TestZeroWidthValuePatternIsRejectedWithoutThePatternText(t *testing.T) {
	const token = "sk_live_4242"
	for _, pattern := range []string{`\b|` + token, token + `|\b`} {
		payload, err := json.Marshal(map[string]any{
			"enabled":        true,
			"value_patterns": []string{pattern},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("MCP_MASKING_JSON", string(payload))
		_, err = config.LoadConfig("", viper.New())
		if err == nil || err.Error() != masking.InvalidConfigMessage || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), `\b`) {
			t.Fatalf("pattern %s error = %v", pattern, err)
		}
	}
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"value_patterns":["sk_live_[0-9]+"]}`)
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil || !cfg.Masking.Enabled() {
		t.Fatalf("ordinary pattern rejected: enabled=%v err=%v", cfg != nil && cfg.Masking.Enabled(), err)
	}
}

func TestNamePatternAnchorCannotEscapeTheGroup(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"enabled":       true,
		"name_patterns": []string{`a)|(b`},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MCP_MASKING_JSON", string(payload))
	_, err = config.LoadConfig("", viper.New())
	if err == nil || err.Error() != masking.InvalidConfigMessage || strings.Contains(err.Error(), `a)|(b`) {
		t.Fatalf("breakout pattern error = %v", err)
	}

	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"name_patterns":["(?:a)|(b)"]}`)
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Masking.MatchName("a") || !cfg.Masking.MatchName("b") {
		t.Fatal("grouped alternation missed a whole name")
	}
	if cfg.Masking.MatchName("aSECRET") || cfg.Masking.MatchName("zzb") || cfg.Masking.MatchName("ab") {
		t.Fatal("grouped alternation matched outside the whole name")
	}
}

func TestMaskingIsDisabledWithoutConfig(t *testing.T) {
	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Masking.Enabled() {
		t.Fatal("absent masking config is active")
	}
}

func TestMaskingJSONReplacesTheWholeYAMLObject(t *testing.T) {
	secret := "synthetic-token-value"
	path := writeConfig(t, "masking:\n  enabled: true\n  names: [\"from-yaml\"]\n  values: [\""+secret+"\"]\n")
	t.Setenv("MCP_MASKING_JSON", `{"enabled":true,"names":["from-env"]}`)
	cfg, err := config.LoadConfig(path, viper.New())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Masking.Enabled() || !cfg.Masking.MatchName("from-env") || cfg.Masking.MatchName("from-yaml") {
		t.Fatalf("env policy did not replace YAML masking: enabled=%t", cfg.Masking.Enabled())
	}
}

func TestMaskingConfigErrorsDoNotIncludeTheDocument(t *testing.T) {
	secret := "synthetic-token-value"
	tests := []struct {
		name    string
		yaml    string
		env     string
		want    string
		wantSub string
	}{
		{name: "empty env", env: "  ", want: masking.InvalidConfigMessage},
		{name: "unknown env field", env: `{"enabled":true,"names":["ok"],"extra":"` + secret + `"}`, want: masking.InvalidConfigMessage},
		{name: "enabled env without rules", env: `{"enabled":true}`, want: masking.InvalidConfigMessage},
		{name: "broken yaml masking", yaml: "masking:\n  values: [\"" + secret + "\"\n", want: masking.InvalidConfigMessage},
		{name: "yaml enabled without rules", yaml: "masking:\n  enabled: true\n", want: masking.InvalidConfigMessage},
		{name: "active policy hides invalid log level", yaml: "log_level: " + secret + "\n", env: `{"enabled":true,"values":["other-token"]}`, want: "[postgres_invalid_config] invalid configuration"},
		{name: "legacy yaml error remains", yaml: "port: [\n", wantSub: "read config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("MCP_MASKING_JSON", tt.env)
			}
			path := ""
			if tt.yaml != "" {
				path = writeConfig(t, tt.yaml)
			}
			_, err := config.LoadConfig(path, viper.New())
			if err == nil {
				t.Fatal("expected config error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("config error leaked the document: %v", err)
			}
			if tt.want != "" && err.Error() != tt.want {
				t.Fatalf("error = %q, want %q", err.Error(), tt.want)
			}
			if tt.wantSub != "" && !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGetListenAddress(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.StaticConfig
		want string
	}{
		{name: "stdio", cfg: config.StaticConfig{Port: 0, Listen: "127.0.0.1"}, want: ""},
		{name: "IPv4", cfg: config.StaticConfig{Port: 8080, Listen: "127.0.0.1"}, want: "127.0.0.1:8080"},
		{name: "hostname", cfg: config.StaticConfig{Port: 8080, Listen: "localhost"}, want: "localhost:8080"},
		{name: "IPv6", cfg: config.StaticConfig{Port: 8080, Listen: "::1"}, want: "[::1]:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.GetListenAddress(); got != tt.want {
				t.Fatalf("GetListenAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}
