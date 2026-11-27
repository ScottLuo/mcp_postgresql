package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockQueryRunner implements queryRunner for unit testing.
type mockQueryRunner struct {
	queryRows   pgx.Rows
	queryErr    error
	execTag     pgconn.CommandTag
	execErr     error
	queriedSQL  string
	executedSQL string
	queryCalled bool
	execCalled  bool
}

func (m *mockQueryRunner) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	m.queryCalled = true
	m.queriedSQL = sql
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	return m.queryRows, nil
}

func (m *mockQueryRunner) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	m.execCalled = true
	m.executedSQL = sql
	if m.execErr != nil {
		return pgconn.CommandTag{}, m.execErr
	}
	return m.execTag, nil
}

// mockTxRunner implements txRunner for unit testing.
type mockTxRunner struct {
	mockQueryRunner
	commitErr      error
	rollbackErr    error
	commitCalled   bool
	rollbackCalled bool
	rollbackCount  int
}

func (m *mockTxRunner) Commit(ctx context.Context) error {
	m.commitCalled = true
	return m.commitErr
}

func (m *mockTxRunner) Rollback(ctx context.Context) error {
	m.rollbackCalled = true
	m.rollbackCount++
	return m.rollbackErr
}

func TestExecuteQuery_SelectSuccess(t *testing.T) {
	rows := &mockRows{
		fields: []pgconn.FieldDescription{{Name: "id"}},
		values: [][]any{{1}, {2}},
	}

	runner := &mockQueryRunner{queryRows: rows}

	result, err := executeQuery(context.Background(), runner, "SELECT * FROM t", 100)
	if err != nil {
		t.Fatalf("executeQuery() error = %v", err)
	}
	if !runner.queryCalled {
		t.Error("expected Query to be called")
	}
	if runner.execCalled {
		t.Error("expected Exec not to be called for SELECT")
	}
	if result.RowCount != 2 {
		t.Errorf("expected RowCount=2, got %d", result.RowCount)
	}
	// Verify LIMIT was added
	if runner.queriedSQL != "SELECT * FROM t LIMIT 100" {
		t.Errorf("expected SQL with LIMIT, got %q", runner.queriedSQL)
	}
}

func TestExecuteQuery_SelectError(t *testing.T) {
	runner := &mockQueryRunner{queryErr: fmt.Errorf("query failed")}

	_, err := executeQuery(context.Background(), runner, "SELECT 1", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !runner.queryCalled {
		t.Error("expected Query to be called")
	}
}

func TestExecuteQuery_ExecSuccess(t *testing.T) {
	tag := pgconn.NewCommandTag("INSERT 0 1")
	runner := &mockQueryRunner{execTag: tag}

	result, err := executeQuery(context.Background(), runner, "INSERT INTO t VALUES(1)", 100)
	if err != nil {
		t.Fatalf("executeQuery() error = %v", err)
	}
	if !runner.execCalled {
		t.Error("expected Exec to be called")
	}
	if runner.queryCalled {
		t.Error("expected Query not to be called for INSERT")
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].RowsAffected != 1 {
		t.Errorf("expected RowsAffected=1, got %d", result.Results[0].RowsAffected)
	}
}

func TestExecuteQuery_ExecError(t *testing.T) {
	runner := &mockQueryRunner{execErr: fmt.Errorf("exec failed")}

	_, err := executeQuery(context.Background(), runner, "DELETE FROM t", 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteQuery_DefaultLimit(t *testing.T) {
	rows := &mockRows{
		fields: []pgconn.FieldDescription{{Name: "x"}},
		values: nil,
	}
	runner := &mockQueryRunner{queryRows: rows}

	_, _ = executeQuery(context.Background(), runner, "SELECT 1", 0)
	// limitRows=0 should default to 1000
	if runner.queriedSQL != "SELECT 1 LIMIT 1000" {
		t.Errorf("expected default limit 1000, got %q", runner.queriedSQL)
	}
}

func TestExecuteQuery_PGError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "42501", Message: "Insufficient Privilege"}
	runner := &mockQueryRunner{execErr: pgErr}

	_, err := executeQuery(context.Background(), runner, "DROP TABLE t", 100)
	if err == nil {
		t.Fatal("expected error")
	}
	expected := "postgres error [42501] Insufficient Privilege: "
	if err.Error() != expected {
		t.Errorf("error = %q, expected %q", err.Error(), expected)
	}
}

func TestExecuteInTransaction_Success(t *testing.T) {
	tx := &mockTxRunner{}
	tx.queryRows = &mockRows{
		fields: []pgconn.FieldDescription{{Name: "val"}},
		values: [][]any{{42}},
	}
	tx.execTag = pgconn.NewCommandTag("INSERT 0 1")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	result, err := executeInTransaction(context.Background(), beginTx, "INSERT INTO t VALUES(1); SELECT 1;", false)
	if err != nil {
		t.Fatalf("executeInTransaction() error = %v", err)
	}
	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}
	if !tx.commitCalled {
		t.Error("expected Commit to be called")
	}
	if tx.rollbackCalled {
		t.Error("expected Rollback not to be called on success")
	}
}

func TestExecuteInTransaction_DryRun(t *testing.T) {
	tx := &mockTxRunner{}
	tx.execTag = pgconn.NewCommandTag("INSERT 0 1")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	result, err := executeInTransaction(context.Background(), beginTx, "INSERT INTO t VALUES(1);", true)
	if err != nil {
		t.Fatalf("executeInTransaction() error = %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if !tx.rollbackCalled {
		t.Error("expected Rollback to be called for dry_run")
	}
	if tx.commitCalled {
		t.Error("expected Commit not to be called for dry_run")
	}
}

func TestExecuteInTransaction_ExecError_Rollback(t *testing.T) {
	tx := &mockTxRunner{}
	tx.execErr = fmt.Errorf("constraint violation")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	_, err := executeInTransaction(context.Background(), beginTx, "INSERT INTO t VALUES(1);", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !tx.rollbackCalled {
		t.Error("expected Rollback to be called on error")
	}
}

func TestExecuteInTransaction_QueryError_Rollback(t *testing.T) {
	tx := &mockTxRunner{}
	tx.queryErr = fmt.Errorf("query error")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	_, err := executeInTransaction(context.Background(), beginTx, "SELECT 1/0;", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !tx.rollbackCalled {
		t.Error("expected Rollback to be called on query error")
	}
}

func TestExecuteInTransaction_BeginTxError(t *testing.T) {
	beginTx := func(ctx context.Context) (txRunner, error) {
		return nil, fmt.Errorf("cannot begin")
	}

	_, err := executeInTransaction(context.Background(), beginTx, "SELECT 1;", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteInTransaction_CommitError(t *testing.T) {
	tx := &mockTxRunner{}
	tx.execTag = pgconn.NewCommandTag("INSERT 0 1")
	tx.commitErr = fmt.Errorf("commit failed")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	_, err := executeInTransaction(context.Background(), beginTx, "INSERT INTO t VALUES(1);", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteInTransaction_DryRun_RollbackError(t *testing.T) {
	tx := &mockTxRunner{}
	tx.execTag = pgconn.NewCommandTag("INSERT 0 1")
	tx.rollbackErr = fmt.Errorf("rollback failed")

	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	_, err := executeInTransaction(context.Background(), beginTx, "INSERT INTO t VALUES(1);", true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExecuteInTransaction_EmptyStatements(t *testing.T) {
	tx := &mockTxRunner{}
	beginTx := func(ctx context.Context) (txRunner, error) {
		return tx, nil
	}

	_, err := executeInTransaction(context.Background(), beginTx, "   \n  ", false)
	if err == nil {
		t.Fatal("expected error for empty statements")
	}
}
