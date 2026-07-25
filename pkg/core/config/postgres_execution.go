package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const postgresInvalidConfigMessage = "[postgres_invalid_config] invalid configuration"

// PostgresExecutionConfig is the validated connection configuration used only
// when a database operation is about to start. Static configuration and tool
// catalog discovery intentionally do not create it.
type PostgresExecutionConfig struct {
	PoolConfig *pgxpool.Config
}

// LoadPostgresExecutionConfig parses a bare DATABASE_URL when it is set, or
// otherwise lets pgx apply standard PostgreSQL environment variables. It never
// returns a parser cause because connection strings can contain credentials.
func LoadPostgresExecutionConfig() (*PostgresExecutionConfig, error) {
	connectionString := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if connectionString == "" && !hasPostgresEnvironment() {
		return nil, invalidPostgresConfig()
	}
	if connectionString != "" && !hasSingleURLTarget(connectionString) {
		return nil, invalidPostgresConfig()
	}

	poolConfig, err := pgxpool.ParseConfig(connectionString)
	if err != nil || poolConfig == nil || poolConfig.ConnConfig == nil {
		return nil, invalidPostgresConfig()
	}
	if !hasSingleTarget(poolConfig) {
		return nil, invalidPostgresConfig()
	}

	return &PostgresExecutionConfig{PoolConfig: poolConfig}, nil
}

// hasSingleURLTarget rejects a URL whose authority and explicit host/port
// query settings point at different targets. pgx correctly applies the latter
// as an override, but silently accepting the discarded authority would violate
// the one-target configuration contract before pool creation.
func hasSingleURLTarget(connectionString string) bool {
	parsed, err := url.Parse(connectionString)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return true
	}
	if parsed.Hostname() == "" {
		return true
	}
	primaryHost, primaryPort := parsed.Hostname(), portOrDefault(parsed.Port())
	query := parsed.Query()
	hosts := splitConfiguredValues(query.Get("host"))
	ports := splitConfiguredValues(query.Get("port"))
	if len(hosts) == 0 && len(ports) > 0 && portOrDefault(ports[0]) != primaryPort {
		return false
	}
	for index, host := range hosts {
		port := primaryPort
		if len(ports) > 0 {
			port = portOrDefault(ports[min(index, len(ports)-1)])
		}
		if host != primaryHost || port != primaryPort {
			return false
		}
	}
	return true
}

func splitConfiguredValues(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func portOrDefault(value string) uint16 {
	if value == "" {
		return 5432
	}
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0
	}
	return uint16(port)
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func hasPostgresEnvironment() bool {
	for _, key := range []string{
		"PGHOST",
		"PGPORT",
		"PGDATABASE",
		"PGUSER",
		"PGPASSWORD",
		"PGSSLMODE",
		"PGSERVICE",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func hasSingleTarget(poolConfig *pgxpool.Config) bool {
	primaryHost, primaryPort := poolConfig.ConnConfig.Host, poolConfig.ConnConfig.Port
	for _, fallback := range poolConfig.ConnConfig.Fallbacks {
		if fallback == nil {
			continue
		}
		if fallback.Host != primaryHost || fallback.Port != primaryPort {
			return false
		}
	}
	return true
}

func invalidPostgresConfig() error {
	return fmt.Errorf("%s", postgresInvalidConfigMessage)
}
