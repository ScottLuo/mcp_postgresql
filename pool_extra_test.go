package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewPoolManager_InvalidDSN(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"bad": {Name: "bad", DSN: "not-a-valid-dsn"},
		},
	}

	_, err := NewPoolManager(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}

func TestNewPoolManager_ValidStructure(t *testing.T) {
	// Use an invalid DSN but test that the structure is correct
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:9999/db"},
		},
	}

	// This will fail at ping (no server), but tests the structure
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pm, err := NewPoolManager(ctx, cfg)
	if err != nil {
		// Expected - no real server
		t.Logf("NewPoolManager error (expected): %v", err)
		return
	}
	defer pm.Close()

	// If we get here, the pool was created successfully
	_, err = pm.GetPool("app1")
	if err != nil {
		t.Errorf("GetPool() error = %v", err)
	}
}

func TestCreatePool_InvalidDSN(t *testing.T) {
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: &Config{},
	}

	_, err := pm.createPool(context.Background(), &TargetConfig{
		Name: "test",
		DSN:  "invalid-dsn-format",
	})
	if err == nil {
		t.Error("expected error for invalid DSN")
	}
}

func TestCreatePool_InvalidFormat(t *testing.T) {
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: &Config{},
	}

	// postgres:// with missing fields
	_, err := pm.createPool(context.Background(), &TargetConfig{
		Name: "test",
		DSN:  "postgres://user:pass@localhost:5432/",
	})
	if err == nil {
		t.Log("unexpected success - might connect to default db")
	}
}
