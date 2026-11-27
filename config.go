package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// TargetConfig holds the configuration for a single PostgreSQL target.
type TargetConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DSN         string `json:"dsn"`
}

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Targets map[string]*TargetConfig `json:"targets"`
}

// LoadConfig reads configuration from environment variables.
//
// POSTGRES_TARGETS: JSON object mapping target names to descriptions.
// Example: {"app1":"description of app1","app2":"description of app2"}
//
// POSTGRES_<TARGET>_DSN: The DSN for each target (uppercase target name).
// Example: POSTGRES_APP1_DSN=postgres://user:pass@host:5432/db?sslmode=disable
func LoadConfig() (*Config, error) {
	targetsJSON := os.Getenv("POSTGRES_TARGETS")
	if targetsJSON == "" {
		return nil, fmt.Errorf("required environment variable POSTGRES_TARGETS is not set")
	}

	var targetDescs map[string]string
	if err := json.Unmarshal([]byte(targetsJSON), &targetDescs); err != nil {
		return nil, fmt.Errorf("failed to parse POSTGRES_TARGETS as JSON: %w", err)
	}

	if len(targetDescs) == 0 {
		return nil, fmt.Errorf("POSTGRES_TARGETS is empty; at least one target must be defined")
	}

	cfg := &Config{Targets: make(map[string]*TargetConfig, len(targetDescs))}

	for name, desc := range targetDescs {
		// The DSN env var is POSTGRES_<UPPERCASE_NAME>_DSN
		dsnVar := "POSTGRES_" + strings.ToUpper(name) + "_DSN"
		dsn := os.Getenv(dsnVar)
		if dsn == "" {
			return nil, fmt.Errorf("required environment variable %s is not set for target %q", dsnVar, name)
		}

		cfg.Targets[name] = &TargetConfig{
			Name:        name,
			Description: desc,
			DSN:         dsn,
		}
	}

	return cfg, nil
}

// GetTarget returns the TargetConfig for the given name.
func (c *Config) GetTarget(name string) (*TargetConfig, error) {
	t, ok := c.Targets[name]
	if !ok {
		return nil, fmt.Errorf("unknown target %q", name)
	}
	return t, nil
}

// TargetNames returns a sorted list of all target names.
func (c *Config) TargetNames() []string {
	names := make([]string, 0, len(c.Targets))
	for name := range c.Targets {
		names = append(names, name)
	}
	return names
}
