package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

const postgresInvalidConfigPrefix = "[postgres_invalid_config]"

// PostgresExecutionConfig is the validated connection configuration used only
// when a database operation is about to start. Static configuration and tool
// catalog discovery intentionally do not create it.
type PostgresExecutionConfig struct {
	PoolConfig *pgxpool.Config
}

type postgresEnvField struct {
	keyword  string
	mcpName  string
	pgName   string
	required bool
	secret   bool
}

type resolvedPostgresField struct {
	spec     postgresEnvField
	mcpValue string
	pgValue  string
	mcpSet   bool
	pgSet    bool
}

func (f resolvedPostgresField) set() bool {
	return f.mcpSet || f.pgSet
}

func discretePostgresFields() []postgresEnvField {
	return []postgresEnvField{
		{keyword: "host", mcpName: "MCP_POSTGRES_HOST", pgName: "PGHOST", required: true},
		{keyword: "port", mcpName: "MCP_POSTGRES_PORT", pgName: "PGPORT"},
		{keyword: "dbname", mcpName: "MCP_POSTGRES_DATABASE", pgName: "PGDATABASE", required: true},
		{keyword: "user", mcpName: "MCP_POSTGRES_USERNAME", pgName: "PGUSER", required: true},
		{keyword: "password", mcpName: "MCP_POSTGRES_PASSWORD", pgName: "PGPASSWORD", secret: true},
		{keyword: "sslmode", mcpName: "MCP_POSTGRES_SSLMODE", pgName: "PGSSLMODE"},
	}
}

// LoadPostgresExecutionConfig loads the connection used for a database
// operation.
//
// DATABASE_URL, when set, is the connection string: every MCP_POSTGRES_*
// variable is ignored. Standard PG* variables still fill settings the URL
// omits, and a URL setting overrides the same PG* variable. That is pgx's
// libpq precedence and is the conflict policy for URL versus discrete fields.
//
// When DATABASE_URL is unset, each MCP_POSTGRES_* variable is an alias of
// the matching PG* variable. If both names are set to different values,
// loading fails and the error names the two variables. MCP_POSTGRES_* is
// passed as an explicit connection parameter, so it overrides a PGSERVICE
// entry for that field. A service file still overrides PG* when the MCP
// alias is unset.
//
// Without DATABASE_URL or PGSERVICE, host, database, and username are
// required. Libpq's defaults for those fields are not used. The error names
// each missing variable. Port defaults to 5432 and sslmode defaults to
// prefer. An unset MCP_POSTGRES_PASSWORD leaves PGPASSWORD and .pgpass in
// place. An empty PGPASSWORD is unset. An empty MCP_POSTGRES_PASSWORD is an
// explicit empty password: it overrides PGPASSWORD and .pgpass, and it
// disagrees with a non-empty PGPASSWORD.
//
// The resolved password must be valid UTF-8. pgx SCRAM preparation replaces
// invalid UTF-8 with U+FFFD and would authenticate as a different password,
// so that input is rejected before connect. The error does not include the
// password. Other parser failures stay generic because a connection string
// can contain credentials.
func LoadPostgresExecutionConfig() (*PostgresExecutionConfig, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL != "" {
		if !utf8.ValidString(databaseURL) {
			return nil, invalidUTF8Password()
		}
		if !hasSingleURLTarget(databaseURL) {
			return nil, invalidPostgresConfig()
		}
		cfg, err := parsePostgresPoolConfig(databaseURL)
		return finishPassword(cfg, err, false)
	}

	fields, err := resolveDiscretePostgresFields()
	if err != nil {
		return nil, err
	}
	if !postgresServiceConfigured() {
		if missing := missingRequiredPostgresFields(fields); len(missing) > 0 {
			return nil, missingPostgresFields(missing)
		}
	}
	cfg, err := parsePostgresPoolConfig(discreteKeywordString(fields))
	return finishPassword(cfg, err, explicitEmptyMCPPassword(fields))
}

func resolveDiscretePostgresFields() ([]resolvedPostgresField, error) {
	specs := discretePostgresFields()
	resolved := make([]resolvedPostgresField, 0, len(specs))
	for _, spec := range specs {
		mcpValue, mcpSet := lookupPostgresEnv(spec.mcpName, spec.secret, spec.secret)
		pgValue, pgSet := lookupPostgresEnv(spec.pgName, spec.secret, false)
		if mcpSet && pgSet && mcpValue != pgValue {
			return nil, fmt.Errorf("%s %s and %s disagree", postgresInvalidConfigPrefix, spec.mcpName, spec.pgName)
		}
		resolved = append(resolved, resolvedPostgresField{
			spec:     spec,
			mcpValue: mcpValue,
			pgValue:  pgValue,
			mcpSet:   mcpSet,
			pgSet:    pgSet,
		})
	}
	return resolved, nil
}

func lookupPostgresEnv(key string, secret bool, keepEmpty bool) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	if secret {
		if value == "" && !keepEmpty {
			return "", false
		}
		return value, true
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func postgresServiceConfigured() bool {
	_, ok := lookupPostgresEnv("PGSERVICE", false, false)
	return ok
}

func explicitEmptyMCPPassword(fields []resolvedPostgresField) bool {
	for _, field := range fields {
		if field.spec.secret && field.mcpSet && field.mcpValue == "" {
			return true
		}
	}
	return false
}

func missingRequiredPostgresFields(fields []resolvedPostgresField) []string {
	missing := make([]string, 0, 3)
	for _, field := range fields {
		if field.spec.required && !field.set() {
			missing = append(missing, field.spec.mcpName+" or "+field.spec.pgName)
		}
	}
	return missing
}

func missingPostgresFields(names []string) error {
	return fmt.Errorf("%s missing %s", postgresInvalidConfigPrefix, strings.Join(names, ", "))
}

// discreteKeywordString quotes set MCP_POSTGRES_* values. Unset fields stay
// in the environment so a service file can still override PG*. Quoting walks
// bytes. A rune walk would replace invalid UTF-8 with U+FFFD before the
// UTF-8 check could reject that password.
func discreteKeywordString(fields []resolvedPostgresField) string {
	var builder strings.Builder
	for _, field := range fields {
		if !field.mcpSet {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(field.spec.keyword)
		builder.WriteByte('=')
		builder.WriteString(quotePostgresKeyword(field.mcpValue))
	}
	return builder.String()
}

func quotePostgresKeyword(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte('\'')
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' || value[i] == '\'' {
			builder.WriteByte('\\')
		}
		builder.WriteByte(value[i])
	}
	builder.WriteByte('\'')
	return builder.String()
}

func parsePostgresPoolConfig(connectionString string) (*PostgresExecutionConfig, error) {
	poolConfig, err := pgxpool.ParseConfig(connectionString)
	if err != nil || poolConfig == nil || poolConfig.ConnConfig == nil {
		return nil, invalidPostgresConfig()
	}
	if !hasSingleTarget(poolConfig) {
		return nil, invalidPostgresConfig()
	}
	return &PostgresExecutionConfig{PoolConfig: poolConfig}, nil
}

func finishPassword(cfg *PostgresExecutionConfig, err error, explicitEmpty bool) (*PostgresExecutionConfig, error) {
	if err != nil {
		return nil, err
	}
	if explicitEmpty {
		cfg.PoolConfig.ConnConfig.Password = ""
	}
	if !utf8.ValidString(cfg.PoolConfig.ConnConfig.Password) {
		return nil, invalidUTF8Password()
	}
	return cfg, nil
}

func invalidUTF8Password() error {
	return fmt.Errorf("%s password must be valid UTF-8", postgresInvalidConfigPrefix)
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
	return fmt.Errorf("%s invalid configuration", postgresInvalidConfigPrefix)
}
