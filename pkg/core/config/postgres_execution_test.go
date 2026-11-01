package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/futuretea/postgres-mcp-server/pkg/core/config"
)

func TestStaticConfigAndCatalogOnlyDoNotReadPostgresEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://reader:STATIC_CONFIG_MARKER@not-a-real-host:5432/app")
	t.Setenv("PGHOST", "also-not-a-real-host,another-host")
	t.Setenv("MCP_POSTGRES_HOST", "mcp-static-marker")
	t.Setenv("MCP_POSTGRES_PASSWORD", "STATIC_CONFIG_MARKER")

	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig() must ignore PostgreSQL execution environment: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("StaticConfig.Validate() must ignore PostgreSQL execution environment: %v", err)
	}
}

func TestLoadPostgresExecutionConfig(t *testing.T) {
	const (
		specialPassword = "p@ss:/?# w'or\\d host=evil"
		mcpPassword     = "MCP_PASSWORD_SHOULD_BE_IGNORED"
		rawPassword     = "a\xffb'\\c host=evil"
	)

	tests := []struct {
		name            string
		databaseURL     string
		pgHost          string
		pgPort          string
		pgDatabase      string
		pgUser          string
		pgPassword      string
		pgSSLMode       string
		pgService       string
		serviceFile     string
		mcpHost         string
		mcpPort         string
		mcpDatabase     string
		mcpUsername     string
		mcpPassword     string
		mcpSSLMode      string
		wantHost        string
		wantPort        uint16
		wantDatabase    string
		wantUser        string
		checkPassword   bool
		wantPassword    string
		wantTLSDisabled bool
		wantErr         string
	}{
		{
			name:            "bare DATABASE_URL takes precedence over standard environment and MCP fields",
			databaseURL:     "postgres://reader:from-url@url-primary:5432/app?sslmode=disable",
			pgHost:          "not-used,second-target",
			pgPort:          "5432,5432",
			pgDatabase:      "ignored",
			pgUser:          "ignored",
			pgPassword:      "ignored-pg-password",
			pgSSLMode:       "require",
			mcpHost:         "mcp-ignored",
			mcpDatabase:     "mcp-ignored",
			mcpUsername:     "mcp-ignored",
			mcpPassword:     mcpPassword,
			mcpSSLMode:      "require",
			wantHost:        "url-primary",
			wantDatabase:    "app",
			wantUser:        "reader",
			checkPassword:   true,
			wantPassword:    "from-url",
			wantTLSDisabled: true,
		},
		{
			name:          "DATABASE_URL keeps PG password gap fill and ignores MCP password",
			databaseURL:   "postgres://url-user@url-primary:5432/url-db?sslmode=disable",
			pgPassword:    "from-pg",
			mcpHost:       "mcp-host",
			mcpUsername:   "mcp-user",
			mcpPassword:   mcpPassword,
			wantHost:      "url-primary",
			wantDatabase:  "url-db",
			wantUser:      "url-user",
			checkPassword: true,
			wantPassword:  "from-pg",
		},
		{
			name:            "MCP discrete fields are accepted without DATABASE_URL or PG variables",
			mcpHost:         "mcp-primary",
			mcpPort:         "5433",
			mcpDatabase:     "mcp-app",
			mcpUsername:     "mcp-reader",
			mcpPassword:     specialPassword,
			mcpSSLMode:      "disable",
			wantHost:        "mcp-primary",
			wantPort:        5433,
			wantDatabase:    "mcp-app",
			wantUser:        "mcp-reader",
			checkPassword:   true,
			wantPassword:    specialPassword,
			wantTLSDisabled: true,
		},
		{
			name:            "MCP password preserves non-UTF-8 bytes",
			mcpHost:         "mcp-primary",
			mcpDatabase:     "mcp-app",
			mcpUsername:     "mcp-reader",
			mcpPassword:     rawPassword,
			mcpSSLMode:      "disable",
			wantHost:        "mcp-primary",
			wantDatabase:    "mcp-app",
			wantUser:        "mcp-reader",
			checkPassword:   true,
			wantPassword:    rawPassword,
			wantTLSDisabled: true,
		},
		{
			name:          "trimmed MCP host is used",
			mcpHost:       "  trimmed-host  ",
			mcpDatabase:   "app",
			mcpUsername:   "reader",
			wantHost:      "trimmed-host",
			wantPort:      5432,
			wantDatabase:  "app",
			wantUser:      "reader",
			checkPassword: true,
		},
		{
			name:          "matching MCP and PG aliases are accepted",
			pgHost:        "alias-primary",
			pgDatabase:    "app",
			pgUser:        "reader",
			pgPassword:    specialPassword,
			mcpHost:       "alias-primary",
			mcpDatabase:   "app",
			mcpUsername:   "reader",
			mcpPassword:   specialPassword,
			wantHost:      "alias-primary",
			wantDatabase:  "app",
			wantUser:      "reader",
			checkPassword: true,
			wantPassword:  specialPassword,
		},
		{
			name:        "disagreeing MCP and PG host aliases are rejected",
			pgHost:      "pg-host",
			pgDatabase:  "app",
			pgUser:      "reader",
			mcpHost:     "mcp-host",
			mcpDatabase: "app",
			mcpUsername: "reader",
			wantErr:     "[postgres_invalid_config] MCP_POSTGRES_HOST and PGHOST disagree",
		},
		{
			name:        "disagreeing password aliases are rejected without leaking either value",
			pgHost:      "env-primary",
			pgDatabase:  "app",
			pgUser:      "reader",
			pgPassword:  "pg-" + specialPassword,
			mcpHost:     "env-primary",
			mcpDatabase: "app",
			mcpUsername: "reader",
			mcpPassword: "mcp-" + specialPassword,
			wantErr:     "[postgres_invalid_config] MCP_POSTGRES_PASSWORD and PGPASSWORD disagree",
		},
		{
			name:            "standard PostgreSQL environment fallback",
			pgHost:          "env-primary",
			pgPort:          "5432",
			pgDatabase:      "app",
			pgUser:          "reader",
			pgPassword:      "password-is-not-an-error-message",
			pgSSLMode:       "disable",
			wantHost:        "env-primary",
			checkPassword:   true,
			wantPassword:    "password-is-not-an-error-message",
			wantTLSDisabled: true,
		},
		{
			name:        "same host TLS fallback is accepted",
			databaseURL: "postgres://reader@primary:5432/app?host=primary&port=5432&sslmode=prefer",
			wantHost:    "primary",
		},
		{
			name:       "same host standard environment fallback is accepted",
			pgHost:     "primary,primary",
			pgPort:     "5432,5432",
			pgDatabase: "app",
			pgUser:     "reader",
			wantHost:   "primary",
		},
		{
			name:      "single target standard service file is accepted",
			pgService: "reader",
			serviceFile: "[reader]\n" +
				"host=service-primary\n" +
				"port=5432\n" +
				"dbname=app\n" +
				"user=reader\n" +
				"sslmode=disable\n",
			wantHost: "service-primary",
		},
		{
			name:      "service file overrides PGHOST when the MCP alias is unset",
			pgHost:    "env-host",
			pgService: "reader",
			serviceFile: "[reader]\n" +
				"host=service-primary\n" +
				"port=5432\n" +
				"dbname=app\n" +
				"user=reader\n" +
				"sslmode=disable\n",
			wantHost: "service-primary",
		},
		{
			name:      "explicit MCP host overrides the service file",
			pgService: "reader",
			mcpHost:   "explicit-primary",
			serviceFile: "[reader]\n" +
				"host=service-primary\n" +
				"port=5432\n" +
				"dbname=app\n" +
				"user=reader\n" +
				"sslmode=disable\n",
			wantHost:     "explicit-primary",
			wantDatabase: "app",
			wantUser:     "reader",
		},
		{
			name:      "distinct standard service file targets are rejected",
			pgService: "reader",
			serviceFile: "[reader]\n" +
				"host=service-primary,service-secondary\n" +
				"port=5432,5432\n" +
				"dbname=app\n" +
				"user=reader\n" +
				"sslmode=disable\n",
			wantErr: "[postgres_invalid_config] invalid configuration",
		},
		{
			name:        "distinct DATABASE_URL fallback targets are rejected",
			databaseURL: "postgres://reader@primary:5432/app?host=secondary&port=5432",
			wantErr:     "[postgres_invalid_config] invalid configuration",
		},
		{
			name:        "distinct DATABASE_URL port override is rejected",
			databaseURL: "postgres://reader@primary:5432/app?port=5433",
			wantErr:     "[postgres_invalid_config] invalid configuration",
		},
		{
			name:       "comma separated distinct PGHOST targets are rejected",
			pgHost:     "primary,secondary",
			pgPort:     "5432,5432",
			pgDatabase: "app",
			pgUser:     "reader",
			wantErr:    "[postgres_invalid_config] invalid configuration",
		},
		{
			name:        "malformed DATABASE_URL is rejected without leaking it",
			databaseURL: "postgres://reader:CONFIG_SECRET_MARKER@%zz",
			wantErr:     "[postgres_invalid_config] invalid configuration",
		},
		{
			name: "missing connection configuration names the required fields",
			wantErr: "[postgres_invalid_config] missing MCP_POSTGRES_HOST or PGHOST, " +
				"MCP_POSTGRES_DATABASE or PGDATABASE, MCP_POSTGRES_USERNAME or PGUSER",
		},
		{
			name:    "partial discrete configuration names only the absent fields",
			mcpHost: "mcp-primary",
			wantErr: "[postgres_invalid_config] missing MCP_POSTGRES_DATABASE or PGDATABASE, " +
				"MCP_POSTGRES_USERNAME or PGUSER",
		},
		{
			name:       "PG database still counts when the MCP alias is unset",
			mcpHost:    "mcp-primary",
			pgDatabase: "app",
			wantErr:    "[postgres_invalid_config] missing MCP_POSTGRES_USERNAME or PGUSER",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setPostgresEnv(t, "DATABASE_URL", tt.databaseURL)
			setPostgresEnv(t, "PGHOST", tt.pgHost)
			setPostgresEnv(t, "PGPORT", tt.pgPort)
			setPostgresEnv(t, "PGDATABASE", tt.pgDatabase)
			setPostgresEnv(t, "PGUSER", tt.pgUser)
			setPostgresEnv(t, "PGPASSWORD", tt.pgPassword)
			setPostgresEnv(t, "PGSSLMODE", tt.pgSSLMode)
			setPostgresEnv(t, "PGSERVICE", tt.pgService)
			setPostgresEnv(t, "MCP_POSTGRES_HOST", tt.mcpHost)
			setPostgresEnv(t, "MCP_POSTGRES_PORT", tt.mcpPort)
			setPostgresEnv(t, "MCP_POSTGRES_DATABASE", tt.mcpDatabase)
			setPostgresEnv(t, "MCP_POSTGRES_USERNAME", tt.mcpUsername)
			setPostgresEnv(t, "MCP_POSTGRES_PASSWORD", tt.mcpPassword)
			setPostgresEnv(t, "MCP_POSTGRES_SSLMODE", tt.mcpSSLMode)
			serviceFile := ""
			if tt.serviceFile != "" {
				serviceFile = writeServiceFile(t, tt.serviceFile)
			}
			setPostgresEnv(t, "PGSERVICEFILE", serviceFile)

			cfg, err := config.LoadPostgresExecutionConfig()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadPostgresExecutionConfig() error = %v", err)
				}
				if cfg == nil {
					t.Fatal("LoadPostgresExecutionConfig() config = nil, want prevalidated execution configuration")
				}
				got := cfg.PoolConfig.ConnConfig
				if tt.wantHost != "" && got.Host != tt.wantHost {
					t.Fatalf("LoadPostgresExecutionConfig() host = %q, want %q", got.Host, tt.wantHost)
				}
				if tt.wantPort != 0 && got.Port != tt.wantPort {
					t.Fatalf("LoadPostgresExecutionConfig() port = %d, want %d", got.Port, tt.wantPort)
				}
				if tt.wantDatabase != "" && got.Database != tt.wantDatabase {
					t.Fatalf("LoadPostgresExecutionConfig() database = %q, want %q", got.Database, tt.wantDatabase)
				}
				if tt.wantUser != "" && got.User != tt.wantUser {
					t.Fatalf("LoadPostgresExecutionConfig() user = %q, want %q", got.User, tt.wantUser)
				}
				if tt.checkPassword && got.Password != tt.wantPassword {
					t.Fatal("LoadPostgresExecutionConfig() password did not match the expected discrete or URL value")
				}
				if tt.wantTLSDisabled && got.TLSConfig != nil {
					t.Fatal("LoadPostgresExecutionConfig() TLSConfig set, want sslmode=disable")
				}
				return
			}

			if cfg != nil {
				t.Fatalf("LoadPostgresExecutionConfig() config = %#v, want nil on rejected configuration", cfg)
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("LoadPostgresExecutionConfig() error = %v, want %q", err, tt.wantErr)
			}
			for _, marker := range []string{
				"CONFIG_SECRET_MARKER",
				"password-is-not-an-error-message",
				"STATIC_CONFIG_MARKER",
				specialPassword,
				mcpPassword,
				rawPassword,
				"from-pg",
				"from-url",
			} {
				if strings.Contains(err.Error(), marker) {
					t.Fatalf("safe configuration error leaked marker %q: %q", marker, err)
				}
			}
		})
	}
}

func setPostgresEnv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func writeServiceFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pg_service.conf")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write service file: %v", err)
	}
	return path
}
