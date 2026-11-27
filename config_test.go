package main

import (
	"os"
	"testing"
)

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset env %s: %v", key, err)
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	tests := []struct {
		name          string
		targetsJSON   string
		env           map[string]string
		expectedCount int
	}{
		{
			name:          "single_target",
			targetsJSON:   `{"app1":"test db"}`,
			env:           map[string]string{"POSTGRES_APP1_DSN": "postgres://user:pass@localhost:5432/db"},
			expectedCount: 1,
		},
		{
			name:          "multiple_targets",
			targetsJSON:   `{"app1":"test db","app2":"another db"}`,
			env: map[string]string{
				"POSTGRES_APP1_DSN": "postgres://user:pass@localhost:5432/db1",
				"POSTGRES_APP2_DSN": "postgres://user:pass@remote:5432/db2",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			setEnv(t, "POSTGRES_TARGETS", tt.targetsJSON)
			defer unsetEnv(t, "POSTGRES_TARGETS")
			for k, v := range tt.env {
				setEnv(t, k, v)
				defer unsetEnv(t, k)
			}

			// Execution
			cfg, err := LoadConfig()

			// Validation
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if len(cfg.Targets) != tt.expectedCount {
				t.Errorf("expected %d targets, got %d", tt.expectedCount, len(cfg.Targets))
			}
		})
	}
}

func TestLoadConfig_MissingEnv(t *testing.T) {
	setEnv(t, "POSTGRES_TARGETS", "")
	defer unsetEnv(t, "POSTGRES_TARGETS")

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error when POSTGRES_TARGETS is empty")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	setEnv(t, "POSTGRES_TARGETS", "not-valid-json")
	defer unsetEnv(t, "POSTGRES_TARGETS")

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_MissingDSN(t *testing.T) {
	setEnv(t, "POSTGRES_TARGETS", `{"app1":"test db"}`)
	defer unsetEnv(t, "POSTGRES_TARGETS")
	unsetEnv(t, "POSTGRES_APP1_DSN")

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error when DSN env var is missing")
	}
}

func TestLoadConfig_EmptyTargets(t *testing.T) {
	setEnv(t, "POSTGRES_TARGETS", `{}`)
	defer unsetEnv(t, "POSTGRES_TARGETS")

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error when targets object is empty")
	}
}

func TestConfig_GetTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", Description: "test", DSN: "postgres://..."},
		},
	}

	t.Run("existing_target", func(t *testing.T) {
		target, err := cfg.GetTarget("app1")
		if err != nil {
			t.Fatalf("GetTarget() error = %v", err)
		}
		if target.Description != "test" {
			t.Errorf("expected description 'test', got %q", target.Description)
		}
	})

	t.Run("nonexistent_target", func(t *testing.T) {
		_, err := cfg.GetTarget("unknown")
		if err == nil {
			t.Error("expected error for unknown target")
		}
	})
}

func TestConfig_TargetNames(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1"},
			"app2": {Name: "app2"},
		},
	}

	names := cfg.TargetNames()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}
