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

	cfg, err := config.LoadConfig("", viper.New())
	if err != nil {
		t.Fatalf("LoadConfig() must ignore PostgreSQL execution environment: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("StaticConfig.Validate() must ignore PostgreSQL execution environment: %v", err)
	}
}

func TestLoadPostgresExecutionConfig(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		pgHost      string
		pgPort      string
		pgDatabase  string
		pgUser      string
		pgPassword  string
		pgSSLMode   string
		pgService   string
		serviceFile string
		wantHost    string
		wantErr     string
	}{
		{
			name:        "bare DATABASE_URL takes precedence over standard environment",
			databaseURL: "postgres://reader:from-url@url-primary:5432/app?sslmode=disable",
			pgHost:      "not-used,second-target",
			pgPort:      "5432,5432",
			pgDatabase:  "ignored",
			pgUser:      "ignored",
			pgPassword:  "ignored",
			pgSSLMode:   "require",
		},
		{
			name:       "standard PostgreSQL environment fallback",
			pgHost:     "env-primary",
			pgPort:     "5432",
			pgDatabase: "app",
			pgUser:     "reader",
			pgPassword: "password-is-not-an-error-message",
			pgSSLMode:  "disable",
		},
		{
			name:        "same host TLS fallback is accepted",
			databaseURL: "postgres://reader@primary:5432/app?host=primary&port=5432&sslmode=prefer",
		},
		{
			name:       "same host standard environment fallback is accepted",
			pgHost:     "primary,primary",
			pgPort:     "5432,5432",
			pgDatabase: "app",
			pgUser:     "reader",
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
			name:    "missing connection configuration is rejected",
			wantErr: "[postgres_invalid_config] invalid configuration",
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
				if tt.wantHost != "" && cfg.PoolConfig.ConnConfig.Host != tt.wantHost {
					t.Fatalf("LoadPostgresExecutionConfig() host = %q, want %q", cfg.PoolConfig.ConnConfig.Host, tt.wantHost)
				}
				return
			}

			if cfg != nil {
				t.Fatalf("LoadPostgresExecutionConfig() config = %#v, want nil on rejected configuration", cfg)
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("LoadPostgresExecutionConfig() error = %v, want %q", err, tt.wantErr)
			}
			for _, marker := range []string{"CONFIG_SECRET_MARKER", "password-is-not-an-error-message"} {
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
