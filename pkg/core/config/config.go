package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/futuretea/postgres-mcp-server/pkg/core/masking"
)

// StaticConfig contains configuration that is fixed when the server starts.
type StaticConfig struct {
	Port            int      `mapstructure:"port"`
	Listen          string   `mapstructure:"listen"`
	SSEBaseURL      string   `mapstructure:"sse_base_url"`
	LogLevel        string   `mapstructure:"log_level"`
	EnabledTools    []string `mapstructure:"enabled_tools"`
	DisabledTools   []string `mapstructure:"disabled_tools"`
	EnabledDomains  []string `mapstructure:"enabled_domains"`
	DisabledDomains []string `mapstructure:"disabled_domains"`

	AllowUnsafeDatabaseRole bool `mapstructure:"allow_unsafe_database_role"`

	// Masking is parsed separately so unknown masking fields fail closed.
	Masking masking.Policy `mapstructure:"-"`
}

// Validate checks whether the configuration can be used to start the server.
func (c *StaticConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", c.Port)
	}
	if c.Port != 0 && strings.TrimSpace(c.Listen) == "" {
		return fmt.Errorf("listen must be set when port is non-zero")
	}
	if _, err := zerolog.ParseLevel(c.LogLevel); err != nil {
		return fmt.Errorf("invalid log_level %q: %w", c.LogLevel, err)
	}
	return nil
}

// GetListenAddress returns the configured HTTP listen address (host:port).
func (c *StaticConfig) GetListenAddress() string {
	if c.Port == 0 {
		return ""
	}
	return net.JoinHostPort(c.Listen, strconv.Itoa(c.Port))
}

// LoadConfig loads configuration from defaults, optional YAML, environment, and flags.
func LoadConfig(configPath string, v *viper.Viper) (*StaticConfig, error) {
	if v == nil {
		v = viper.New()
	}

	defaults := map[string]any{
		"port":             0,
		"listen":           "127.0.0.1",
		"sse_base_url":     "",
		"log_level":        "info",
		"enabled_tools":    []string{},
		"disabled_tools":   []string{},
		"enabled_domains":  []string{},
		"disabled_domains": []string{},

		"allow_unsafe_database_role": false,
	}
	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	envPolicy, envPresent, err := lookupMaskingEnv()
	if err != nil {
		return nil, err
	}

	if configPath != "" {
		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, configReadError(configPath, envPresent, envPolicy, "read config file", err)
		}
	}

	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	cfg := &StaticConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, configReadError(configPath, envPresent, envPolicy, "unmarshal config", err)
	}

	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if cfg.EnabledTools, err = normalizeNameList("enabled_tools", cfg.EnabledTools); err != nil {
		return nil, err
	}
	if cfg.DisabledTools, err = normalizeNameList("disabled_tools", cfg.DisabledTools); err != nil {
		return nil, err
	}
	if cfg.EnabledDomains, err = normalizeNameList("enabled_domains", cfg.EnabledDomains); err != nil {
		return nil, err
	}
	if cfg.DisabledDomains, err = normalizeNameList("disabled_domains", cfg.DisabledDomains); err != nil {
		return nil, err
	}

	if envPresent {
		cfg.Masking = envPolicy
	} else {
		policy, err := maskingFromConfig(v)
		if err != nil {
			return nil, err
		}
		cfg.Masking = policy
	}

	if err := cfg.Validate(); err != nil {
		if cfg.Masking.Enabled() {
			return nil, invalidConfiguration()
		}
		return nil, err
	}
	return cfg, nil
}

func lookupMaskingEnv() (masking.Policy, bool, error) {
	raw, ok := os.LookupEnv("MCP_MASKING_JSON")
	if !ok {
		return masking.Disabled(), false, nil
	}
	policy, err := masking.ParseJSON(raw)
	if err != nil {
		return masking.Policy{}, true, err
	}
	return policy, true, nil
}

func maskingFromConfig(v *viper.Viper) (masking.Policy, error) {
	if v == nil || !v.InConfig("masking") {
		return masking.Disabled(), nil
	}
	return masking.ParseRaw(v.Get("masking"))
}

func configReadError(configPath string, envPresent bool, envPolicy masking.Policy, prefix string, err error) error {
	if envPresent && envPolicy.Enabled() {
		return invalidConfiguration()
	}
	if configFileHasMaskingKey(configPath) {
		return errors.New(masking.InvalidConfigMessage)
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

func configFileHasMaskingKey(path string) bool {
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte("masking:"))
}

func invalidConfiguration() error {
	return fmt.Errorf("%s invalid configuration", postgresInvalidConfigPrefix)
}

func normalizeNameList(field string, values []string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	if len(values) > 0 && len(normalized) == 0 {
		return nil, fmt.Errorf("%s contains no names", field)
	}
	return normalized, nil
}
