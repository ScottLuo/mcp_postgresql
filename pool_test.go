package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mockRows implements pgx.Rows for testing.
type mockRows struct {
	fields   []pgconn.FieldDescription
	values   [][]any
	err      error
	closed   bool
	rowIndex int
}

func (m *mockRows) Close() {
	m.closed = true
}

func (m *mockRows) Err() error {
	return m.err
}

func (m *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription {
	return m.fields
}

func (m *mockRows) Next() bool {
	return m.rowIndex < len(m.values)
}

func (m *mockRows) Scan(dest ...any) error {
	if m.rowIndex >= len(m.values) {
		return fmt.Errorf("no row")
	}
	for i, v := range m.values[m.rowIndex] {
		if i < len(dest) {
			switch d := dest[i].(type) {
			case *any:
				*d = v
			}
		}
	}
	m.rowIndex++
	return nil
}

func (m *mockRows) Values() ([]any, error) {
	if m.rowIndex >= len(m.values) {
		return nil, fmt.Errorf("no row")
	}
	result := m.values[m.rowIndex]
	m.rowIndex++
	return result, nil
}

func (m *mockRows) RawValues() [][]byte {
	return nil
}

func (m *mockRows) Conn() *pgx.Conn {
	return nil
}

func TestCollectRows_Basic(t *testing.T) {
	fields := []pgconn.FieldDescription{
		{Name: "id"},
		{Name: "name"},
	}
	values := [][]any{
		{1, "alice"},
		{2, "bob"},
	}

	rows := &mockRows{fields: fields, values: values}

	result, err := collectRows(rows, 100)
	if err != nil {
		t.Fatalf("collectRows() error = %v", err)
	}

	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Columns[0] != "id" {
		t.Errorf("expected column[0] = 'id', got %q", result.Columns[0])
	}
	if result.Columns[1] != "name" {
		t.Errorf("expected column[1] = 'name', got %q", result.Columns[1])
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	if result.RowCount != 2 {
		t.Errorf("expected RowCount=2, got %d", result.RowCount)
	}
	if result.Truncated {
		t.Error("expected Truncated=false")
	}
	if result.Rows[0]["name"] != "alice" {
		t.Errorf("expected row[0]['name']='alice', got %v", result.Rows[0]["name"])
	}
	if result.Rows[1]["name"] != "bob" {
		t.Errorf("expected row[1]['name']='bob', got %v", result.Rows[1]["name"])
	}
}

func TestCollectRows_Truncated(t *testing.T) {
	fields := []pgconn.FieldDescription{{Name: "id"}}
	values := [][]any{{1}, {2}, {3}, {4}, {5}}

	rows := &mockRows{fields: fields, values: values}

	result, err := collectRows(rows, 3)
	if err != nil {
		t.Fatalf("collectRows() error = %v", err)
	}

	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(result.Rows))
	}
	if result.RowCount != 5 {
		t.Errorf("expected RowCount=5, got %d", result.RowCount)
	}
	if !result.Truncated {
		t.Error("expected Truncated=true")
	}
}

func TestCollectRows_Empty(t *testing.T) {
	fields := []pgconn.FieldDescription{{Name: "x"}}
	rows := &mockRows{fields: fields, values: nil}

	result, err := collectRows(rows, 100)
	if err != nil {
		t.Fatalf("collectRows() error = %v", err)
	}
	if result.RowCount != 0 {
		t.Errorf("expected RowCount=0, got %d", result.RowCount)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result.Rows))
	}
}

func TestCollectRows_WithBytes(t *testing.T) {
	fields := []pgconn.FieldDescription{{Name: "val"}}
	values := [][]any{{[]byte("123.45")}}

	rows := &mockRows{fields: fields, values: values}

	result, err := collectRows(rows, 100)
	if err != nil {
		t.Fatalf("collectRows() error = %v", err)
	}
	if result.Rows[0]["val"] != "123.45" {
		t.Errorf("expected '123.45', got %v", result.Rows[0]["val"])
	}
}

func TestCollectRows_WithError(t *testing.T) {
	fields := []pgconn.FieldDescription{{Name: "x"}}
	rows := &mockRows{fields: fields, err: fmt.Errorf("db error")}

	_, err := collectRows(rows, 100)
	if err == nil {
		t.Error("expected error")
	}
}

func TestMapPGXError_PGError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "42501",
		Message: "Insufficient Privilege",
		Detail:  "user does not have permission",
	}

	err := mapPGXError(pgErr)
	if err == nil {
		t.Fatal("expected error")
	}
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}
}

func TestMapPGXError_GenericError(t *testing.T) {
	origErr := fmt.Errorf("connection refused")
	err := mapPGXError(origErr)
	if err.Error() != "connection refused" {
		t.Errorf("expected original error message, got %q", err.Error())
	}
}

func TestMapPGXError_WrappedPGError(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "23505",
		Message: "duplicate key",
	}
	wrapped := fmt.Errorf("exec: %w", pgErr)

	err := mapPGXError(wrapped)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPoolManager_GetPool_NotFound(t *testing.T) {
	pm := &PoolManager{
		pools: make(map[string]*pgxpool.Pool),
	}

	_, err := pm.GetPool("unknown")
	if err == nil {
		t.Error("expected error for unknown pool")
	}
}

func TestPoolManager_Close_Empty(t *testing.T) {
	pm := &PoolManager{
		pools: make(map[string]*pgxpool.Pool),
	}
	pm.Close()
	// Should not panic
}

func TestPoolManager_ExecSQL_UnknownTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, err := pm.ExecSQL(context.Background(), "unknown", "SELECT 1", 100)
	if err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestPoolManager_ExecSQLFile_UnknownTarget(t *testing.T) {
	cfg := &Config{
		Targets: map[string]*TargetConfig{
			"app1": {Name: "app1", DSN: "postgres://user:pass@localhost:5432/db"},
		},
	}
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool),
		config: cfg,
	}

	_, err := pm.ExecSQLFile(context.Background(), "unknown", "SELECT 1", false)
	if err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestExecuteInTransaction_EmptyContent(t *testing.T) {
	result := splitSQL("   \n\t  ")
	if len(result) != 0 {
		t.Errorf("expected 0 statements for whitespace, got %d", len(result))
	}
}
