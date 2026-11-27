package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPoolManager_ExecSQL_AcquireFail(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:59999/db")
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	poolCfg.MaxConns = 1
	poolCfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig error: %v", err)
	}
	defer pool.Close()

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{
			Targets: map[string]*TargetConfig{
				"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = pm.ExecSQL(ctx, "app1", "SELECT 1", 100)
	if err == nil {
		t.Error("expected error when pool cannot connect")
	}
	if err != nil && err.Error() == "" {
		t.Error("expected non-empty error")
	}
}

func TestPoolManager_ExecSQLFile_AcquireFail(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:59999/db")
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	poolCfg.MaxConns = 1
	poolCfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig error: %v", err)
	}
	defer pool.Close()

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{
			Targets: map[string]*TargetConfig{
				"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = pm.ExecSQLFile(ctx, "app1", "SELECT 1", false)
	if err == nil {
		t.Error("expected error when pool cannot connect")
	}
}

func TestPgExecuteSQL_AcquireFail(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:59999/db")
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	poolCfg.MaxConns = 1
	poolCfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig error: %v", err)
	}
	defer pool.Close()

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{
			Targets: map[string]*TargetConfig{
				"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, _, err := pgExecuteSQL(ctx, nil, executeSQLInput{
		TargetName: "app1",
		SQL:        "SELECT 1",
	}, pm, &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected CallToolResult with error info")
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestPgExecuteSQLFile_AcquireFail(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:59999/db")
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	poolCfg.MaxConns = 1
	poolCfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig error: %v", err)
	}
	defer pool.Close()

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{
			Targets: map[string]*TargetConfig{
				"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	result, _, err := pgExecuteSQLFile(ctx, nil, executeSQLFileInput{
		TargetName: "app1",
		FileName:   "/dev/null",
	}, pm, &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:59999/db"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// /dev/null reads as empty → "no SQL statements" error from executeInTransaction
	// OR acquire fails → error in CallToolResult
	if result != nil && !result.IsError {
		t.Error("expected IsError=true")
	}
}
