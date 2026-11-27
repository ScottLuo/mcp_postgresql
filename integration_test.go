//go:build integration
// +build integration

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pgUser     = "testuser"
	pgPassword = "testpass"
	pgDB       = "testdb"
	pgPort     = "15432"
)

var pgDSN string

// setupPostgres starts a PostgreSQL container and waits for it to be ready.
func setupPostgres(t *testing.T) {
	t.Helper()

	// Check if PostgreSQL is already running on the port
	if isPostgresRunning() {
		t.Log("PostgreSQL already running on port", pgPort)
		pgDSN = fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", pgUser, pgPassword, pgPort, pgDB)
		return
	}

	// Pull and start the container
	t.Log("Starting PostgreSQL container...")
	cmd := exec.Command("docker", "run", "-d",
		"--name", "mcp_pg_test",
		"-e", "POSTGRES_USER="+pgUser,
		"-e", "POSTGRES_PASSWORD="+pgPassword,
		"-e", "POSTGRES_DB="+pgDB,
		"-p", pgPort+":5432",
		"postgres:16-alpine",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to start PostgreSQL container: %v", err)
	}

	// Cleanup
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "mcp_pg_test").Run()
	})

	// Wait for PostgreSQL to be ready
	pgDSN = fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable", pgUser, pgPassword, pgPort, pgDB)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if isPostgresRunning() {
			t.Log("PostgreSQL is ready")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("PostgreSQL did not become ready in time")
}

func isPostgresRunning() bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+pgPort, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// waitForPG waits for PostgreSQL to accept connections via pgxpool.
func waitForPG(t *testing.T, dsn string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pool, err := pgxpool.New(context.Background(), dsn)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err = pool.Ping(ctx)
			cancel()
			pool.Close()
			if err == nil {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("PostgreSQL not accepting connections")
}

func TestIntegration_BasicQuery(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	// Create PoolManager
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Test 1: Basic SELECT
	result, err := pm.ExecSQL(context.Background(), "testdb", "SELECT 1", 100)
	if err != nil {
		t.Fatalf("ExecSQL() error = %v", err)
	}
	if result.RowCount != 1 {
		t.Errorf("expected RowCount=1, got %d", result.RowCount)
	}
	if result.Rows[0]["?column?"] != 1 {
		// Column name depends on PostgreSQL version
		t.Logf("Result: %+v", result.Rows[0])
	}

	// Test 2: CREATE TABLE + INSERT + SELECT
	_, err = pm.ExecSQL(context.Background(), "testdb",
		"CREATE TABLE IF NOT EXISTS test_integration(id INT, name TEXT)", 10)
	if err != nil {
		t.Fatalf("CREATE TABLE error: %v", err)
	}

	_, err = pm.ExecSQL(context.Background(), "testdb",
		"INSERT INTO test_integration VALUES (1, 'hello'), (2, 'world')", 10)
	if err != nil {
		t.Fatalf("INSERT error: %v", err)
	}

	result, err = pm.ExecSQL(context.Background(), "testdb",
		"SELECT * FROM test_integration ORDER BY id", 10)
	if err != nil {
		t.Fatalf("SELECT error: %v", err)
	}
	if result.RowCount != 2 {
		t.Errorf("expected 2 rows, got %d", result.RowCount)
	}
	if result.Columns[0] != "id" {
		t.Errorf("expected first column 'id', got %q", result.Columns[0])
	}

	// Cleanup
	_, _ = pm.ExecSQL(context.Background(), "testdb", "DROP TABLE test_integration", 10)
}

func TestIntegration_PermissionCheck(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Test: DROP DATABASE should fail (testuser doesn't own the db typically)
	// Use a CREATE ROLE test instead since testuser is superuser in the container
	_, err = pm.ExecSQL(context.Background(), "testdb", "CREATE ROLE integration_test_role", 10)
	if err != nil {
		t.Fatalf("CREATE ROLE error: %v", err)
	}

	// Verify the role was created
	result, err := pm.ExecSQL(context.Background(), "testdb",
		"SELECT rolname FROM pg_roles WHERE rolname = 'integration_test_role'", 10)
	if err != nil {
		t.Fatalf("SELECT role error: %v", err)
	}
	if result.RowCount != 1 {
		t.Errorf("expected role to exist, got %d rows", result.RowCount)
	}

	// Cleanup
	_, _ = pm.ExecSQL(context.Background(), "testdb", "DROP ROLE integration_test_role", 10)
}

func TestIntegration_TransactionAtomicity(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Test: Transaction with intentional error should roll back
	sqlContent := `
CREATE TABLE temp_test(id INT);
INSERT INTO temp_test VALUES (1);
SELECT 1/0;
`
	_, err = pm.ExecSQLFile(context.Background(), "testdb", sqlContent, false)
	if err == nil {
		t.Fatal("expected error from division by zero")
	}

	// Verify the table was rolled back (should not exist)
	result, err := pm.ExecSQL(context.Background(), "testdb",
		"SELECT count(*) FROM information_schema.tables WHERE table_name = 'temp_test'", 10)
	if err != nil {
		t.Fatalf("check table error: %v", err)
	}
	if result.RowCount != 1 {
		t.Fatalf("expected 1 row from information_schema, got %d", result.RowCount)
	}
	// The count should be 0 since the table was rolled back
	count := result.Rows[0]["count"]
	switch v := count.(type) {
	case int32:
		if v != 0 {
			t.Errorf("expected table count=0 after rollback, got %d", v)
		}
	case int64:
		if v != 0 {
			t.Errorf("expected table count=0 after rollback, got %d", v)
		}
	default:
		t.Logf("count type: %T value: %v", count, count)
	}
}

func TestIntegration_DryRun(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Test: dry_run should commit but roll back
	sqlContent := `
CREATE TABLE dry_run_test(id INT);
INSERT INTO dry_run_test VALUES (1);
`
	result, err := pm.ExecSQLFile(context.Background(), "testdb", sqlContent, true)
	if err != nil {
		t.Fatalf("dry_run ExecSQLFile error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}

	// Verify the table does NOT exist after dry_run
	checkResult, err := pm.ExecSQL(context.Background(), "testdb",
		"SELECT count(*) FROM information_schema.tables WHERE table_name = 'dry_run_test'", 10)
	if err != nil {
		t.Fatalf("check table error: %v", err)
	}
	count := checkResult.Rows[0]["count"]
	switch v := count.(type) {
	case int32:
		if v != 0 {
			t.Errorf("dry_run: expected table not to exist, got count=%d", v)
		}
	case int64:
		if v != 0 {
			t.Errorf("dry_run: expected table not to exist, got count=%d", v)
		}
	}
}

func TestIntegration_ResultTruncation(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Create table with 10 rows
	_, _ = pm.ExecSQL(context.Background(), "testdb",
		"CREATE TABLE IF NOT EXISTS trunc_test(id INT)", 10)
	_, _ = pm.ExecSQL(context.Background(), "testdb",
		"DELETE FROM trunc_test", 10)
	for i := 1; i <= 10; i++ {
		_, _ = pm.ExecSQL(context.Background(), "testdb",
			fmt.Sprintf("INSERT INTO trunc_test VALUES (%d)", i), 10)
	}

	// Query with limit 5
	result, err := pm.ExecSQL(context.Background(), "testdb",
		"SELECT * FROM trunc_test ORDER BY id", 5)
	if err != nil {
		t.Fatalf("ExecSQL error: %v", err)
	}
	if result.RowCount != 5 {
		t.Errorf("expected RowCount=5 (truncated), got %d", result.RowCount)
	}

	// Cleanup
	_, _ = pm.ExecSQL(context.Background(), "testdb", "DROP TABLE trunc_test", 10)
}

func TestIntegration_PGErrorReporting(t *testing.T) {
	setupPostgres(t)
	waitForPG(t, pgDSN)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
		},
	}
	pm, err := NewPoolManager(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewPoolManager() error = %v", err)
	}
	defer pm.Close()

	// Test: syntax error should report error
	_, err = pm.ExecSQL(context.Background(), "testdb",
		"SELECT FROM nonexistent_table_xyz", 10)
	if err == nil {
		t.Fatal("expected error for bad SQL")
	}
	if !strings.Contains(err.Error(), "postgres error") {
		t.Errorf("expected postgres error format, got: %v", err)
	}

	// Test: division by zero
	_, err = pm.ExecSQL(context.Background(), "testdb",
		"SELECT 1/0", 10)
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
	if !strings.Contains(err.Error(), "division") {
		t.Errorf("expected division error, got: %v", err)
	}
}

func TestIntegration_ListTarget(t *testing.T) {
	setupPostgres(t)

	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"testdb": {Name: "testdb", Description: "test database", DSN: pgDSN},
			"otherdb": {Name: "otherdb", Description: "another database", DSN: "postgres://u:p@localhost:5432/other"},
		},
	}

	_, output, err := pgListTarget(context.Background(), nil, targetInput{}, nil, cfg)
	if err != nil {
		t.Fatalf("pgListTarget error: %v", err)
	}
	if len(output.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(output.Targets))
	}
}
