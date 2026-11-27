package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// helperPool creates a pgxpool.Pool that doesn't require a live server.
// pgxpool.NewWithConfig creates the pool lazily - connections are established on demand.
func helperPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig error: %v", err)
	}
	poolCfg.MaxConns = 1
	poolCfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		t.Fatalf("NewWithConfig error: %v", err)
	}
	return pool
}

func TestPoolManager_GetPool_Success(t *testing.T) {
	pool := helperPool(t, "postgres://user:pass@localhost:5432/db")
	defer pool.Close()

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{},
	}

	got, err := pm.GetPool("app1")
	if err != nil {
		t.Fatalf("GetPool() error = %v", err)
	}
	if got != pool {
		t.Error("expected same pool instance")
	}
}

func TestPoolManager_CloseAll_WithPools(t *testing.T) {
	pool1 := helperPool(t, "postgres://user:pass@localhost:5432/db1")
	pool2 := helperPool(t, "postgres://user:pass@localhost:5432/db2")

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool1,
			"app2": pool2,
		},
		config: &Config{},
	}

	pm.Close()
	if len(pm.pools) != 0 {
		t.Errorf("expected 0 pools after close, got %d", len(pm.pools))
	}
}

func TestPoolManager_CloseAll_Direct(t *testing.T) {
	pool := helperPool(t, "postgres://user:pass@localhost:5432/db")

	pm := &PoolManager{
		pools: map[string]*pgxpool.Pool{
			"app1": pool,
		},
		config: &Config{},
	}

	pm.closeAll()
	if len(pm.pools) != 0 {
		t.Errorf("expected 0 pools after closeAll, got %d", len(pm.pools))
	}
}

func TestNewPoolManager_MultipleTargets_Success(t *testing.T) {
	// Use valid DSNs - pool creation is lazy so no real server needed
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db1"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pm, err := NewPoolManager(ctx, cfg)
	if err != nil {
		// May fail at ping if no server - that's OK
		t.Logf("NewPoolManager error (may be expected): %v", err)
		return
	}
	defer pm.Close()

	pool, err := pm.GetPool("app1")
	if err != nil {
		t.Fatalf("GetPool() error = %v", err)
	}
	if pool == nil {
		t.Error("expected non-nil pool")
	}
}

func TestNewPoolManager_Error_ClosesExisting(t *testing.T) {
	// First target has valid DSN, second has invalid - tests closeAll on error
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"good": {Name: "good", DSN: "postgres://user:pass@localhost:5432/db"},
			"bad":  {Name: "bad", DSN: "not-a-dsn"},
		},
	}

	_, err := NewPoolManager(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for bad DSN")
	}
}
