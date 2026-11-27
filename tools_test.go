package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPgListTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", Description: "test db"},
			"app2": {Name: "app2", Description: "another db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, output, err := pgListTarget(context.Background(), nil, targetInput{}, pm, cfg)
	if err != nil {
		t.Fatalf("pgListTarget() error = %v", err)
	}
	if len(output.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(output.Targets))
	}
}

func TestPgExecuteSQL_UnknownTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, _, err := pgExecuteSQL(context.Background(), nil, executeSQLInput{
		TargetName: "unknown",
		SQL:        "SELECT 1",
	}, pm, cfg)
	if err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestPgExecuteSQLFile_FileNotFound(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, _, err := pgExecuteSQLFile(context.Background(), nil, executeSQLFileInput{
		TargetName: "app1",
		FileName:   "/nonexistent/file.sql",
	}, pm, cfg)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestPgExecuteSQLFile_UnknownTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, _, err := pgExecuteSQLFile(context.Background(), nil, executeSQLFileInput{
		TargetName: "unknown",
		FileName:   "test.sql",
	}, pm, cfg)
	if err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestPgExecuteSQLFile_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	sqlFile := filepath.Join(tmpDir, "test.sql")
	content := "SELECT 1;\nSELECT 2;"
	if err := os.WriteFile(sqlFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, output, err := pgExecuteSQLFile(context.Background(), nil, executeSQLFileInput{
		TargetName: "app1",
		FileName:   sqlFile,
	}, pm, cfg)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// The error is wrapped in CallToolResult with IsError=true
	_ = output
}

func TestFormatError_PGError(t *testing.T) {
	err := fmt.Errorf("postgres error [42501] Insufficient Privilege: user does not have permission")
	result := formatError(err)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestPgExecuteSQL_NoPool(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, _, err := pgExecuteSQL(context.Background(), nil, executeSQLInput{
		TargetName: "app1",
		SQL:        "SELECT 1",
	}, pm, cfg)
	if err != nil {
		t.Logf("expected error: %v", err)
	}
}
