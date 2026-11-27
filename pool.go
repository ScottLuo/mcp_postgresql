package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolManager manages connection pools for all configured PostgreSQL targets.
type PoolManager struct {
	mu     sync.RWMutex
	pools  map[string]*pgxpool.Pool
	config *Config
}

// NewPoolManager creates a new PoolManager and initializes all connection pools.
func NewPoolManager(ctx context.Context, cfg *Config) (*PoolManager, error) {
	pm := &PoolManager{
		pools:  make(map[string]*pgxpool.Pool, len(cfg.Targets)),
		config: cfg,
	}

	for name, target := range cfg.Targets {
		pool, err := pm.createPool(ctx, target)
		if err != nil {
			pm.closeAll()
			return nil, fmt.Errorf("failed to create pool for target %q: %w", name, err)
		}
		pm.pools[name] = pool
	}

	return pm, nil
}

func (pm *PoolManager) createPool(ctx context.Context, target *TargetConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(target.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse DSN: %w", err)
	}

	poolCfg.MaxConns = 10
	poolCfg.MinConns = 1
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET statement_timeout = '30s'")
		if err != nil {
			return fmt.Errorf("set statement_timeout: %w", err)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping pool: %w", err)
	}

	return pool, nil
}

// GetPool returns the pool for the given target name.
func (pm *PoolManager) GetPool(targetName string) (*pgxpool.Pool, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	pool, ok := pm.pools[targetName]
	if !ok {
		return nil, fmt.Errorf("no pool for target %q", targetName)
	}
	return pool, nil
}

// Close closes all connection pools.
func (pm *PoolManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.closeAll()
}

func (pm *PoolManager) closeAll() {
	for name, pool := range pm.pools {
		pool.Close()
		delete(pm.pools, name)
	}
}

// queryRunner is a narrow interface satisfied by *pgxpool.Conn,
// allowing executeQuery to be unit-tested with mocks.
type queryRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// txRunner is a narrow interface for the transaction operations used by
// executeInTransaction. It is satisfied by pgx.Tx.
type txRunner interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

// ExecSQL executes a single SQL statement on the given target.
func (pm *PoolManager) ExecSQL(ctx context.Context, targetName string, sql string, limitRows int) (*QueryResult, error) {
	pool, err := pm.GetPool(targetName)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	return executeQuery(ctx, conn, sql, limitRows)
}

// ExecSQLFile executes multiple SQL statements in a transaction.
// If dryRun is true, the transaction is rolled back at the end.
func (pm *PoolManager) ExecSQLFile(ctx context.Context, targetName string, sqlContent string, dryRun bool) (*QueryResult, error) {
	pool, err := pm.GetPool(targetName)
	if err != nil {
		return nil, err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	beginTx := func(ctx context.Context) (txRunner, error) {
		tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		return tx, nil
	}

	return executeInTransaction(ctx, beginTx, sqlContent, dryRun)
}

// QueryResult is the result of executing SQL statements.
type QueryResult struct {
	Columns   []string          `json:"columns,omitempty"`
	Rows      []map[string]any  `json:"rows,omitempty"`
	RowCount  int               `json:"row_count"`
	Truncated bool              `json:"truncated"`
	Results   []StatementResult `json:"results,omitempty"`
}

// StatementResult holds the result of a single statement.
type StatementResult struct {
	SQL          string           `json:"sql"`
	Columns      []string         `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowCount     int             `json:"row_count"`
	Truncated    bool            `json:"truncated"`
	RowsAffected int             `json:"rows_affected"`
}

// executeQuery executes a single SQL query.
func executeQuery(ctx context.Context, conn queryRunner, sql string, limitRows int) (*QueryResult, error) {
	if limitRows <= 0 {
		limitRows = 1000
	}

	trimmed := strings.TrimSpace(sql)
	if isSelectStatement(trimmed) {
		limitedSQL := addLimit(trimmed, limitRows)
		rows, err := conn.Query(ctx, limitedSQL)
		if err != nil {
			return nil, mapPGXError(err)
		}
		defer rows.Close()

		return collectRows(rows, limitRows)
	}

	commandTag, err := conn.Exec(ctx, trimmed)
	if err != nil {
		return nil, mapPGXError(err)
	}

	return &QueryResult{
		Results: []StatementResult{{SQL: trimmed, RowsAffected: int(commandTag.RowsAffected())}},
	}, nil
}

// executeInTransaction executes multiple SQL statements in a single transaction.
// The beginTx function is called to start the transaction; it must return a txRunner.
func executeInTransaction(ctx context.Context, beginTx func(context.Context) (txRunner, error), sqlContent string, dryRun bool) (*QueryResult, error) {
	statements := splitSQL(sqlContent)
	if len(statements) == 0 {
		return nil, fmt.Errorf("no SQL statements found in input")
	}

	tx, err := beginTx(ctx)
	if err != nil {
		return nil, err
	}

	var results []StatementResult

	for _, stmt := range statements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			continue
		}

		if isSelectStatement(trimmed) {
			rows, err := tx.Query(ctx, trimmed)
			if err != nil {
				_ = tx.Rollback(ctx)
				return &QueryResult{Results: results}, mapPGXError(err)
			}
			defer rows.Close()

			result, err := collectRows(rows, 1000)
			if err != nil {
				_ = tx.Rollback(ctx)
				return &QueryResult{Results: results}, err
			}
			sr := StatementResult{
				SQL:       trimmed,
				Columns:   result.Columns,
				Rows:      result.Rows,
				RowCount:  result.RowCount,
				Truncated: result.Truncated,
			}
			results = append(results, sr)
		} else {
			commandTag, err := tx.Exec(ctx, trimmed)
			if err != nil {
				_ = tx.Rollback(ctx)
				return &QueryResult{Results: results}, mapPGXError(err)
			}
			results = append(results, StatementResult{
				SQL:          trimmed,
				RowsAffected: int(commandTag.RowsAffected()),
			})
		}
	}

	if dryRun {
		if err := tx.Rollback(ctx); err != nil {
			return &QueryResult{Results: results}, fmt.Errorf("dry_run rollback: %w", err)
		}
	} else {
		if err := tx.Commit(ctx); err != nil {
			return &QueryResult{Results: results}, fmt.Errorf("commit transaction: %w", err)
		}
	}

	totalRows := 0
	for _, r := range results {
		totalRows += r.RowCount
	}

	return &QueryResult{
		RowCount: totalRows,
		Results:  results,
	}, nil
}

// collectRows collects all rows from pgx.Rows into a QueryResult.
func collectRows(rows pgx.Rows, limit int) (*QueryResult, error) {
	fields := rows.FieldDescriptions()
	var columns []string
	for _, f := range fields {
		columns = append(columns, string(f.Name))
	}

	result := &QueryResult{Columns: columns}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row values: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = serializeValue(values[i])
		}

		if len(result.Rows) < limit {
			result.Rows = append(result.Rows, row)
		} else {
			result.Truncated = true
		}
		result.RowCount++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return result, nil
}

// serializeValue converts a pgx value to a JSON-serializable type.
func serializeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case nil:
		return nil
	default:
		return val
	}
}

// isSelectStatement checks if the SQL is a SELECT or similar read query.
func isSelectStatement(sql string) bool {
	upper := strings.ToUpper(sql)
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "WITH") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "EXPLAIN")
}

// addLimit adds a LIMIT clause to a SELECT statement if not already present.
func addLimit(sql string, limit int) string {
	upper := strings.ToUpper(sql)
	if strings.Contains(upper, " LIMIT ") || strings.HasSuffix(upper, "LIMIT") {
		return sql
	}
	trimmed := strings.TrimRight(sql, ";\t\n ")
	return fmt.Sprintf("%s LIMIT %d", trimmed, limit)
}

// mapPGXError converts a pgx error to a more descriptive error with code info.
func mapPGXError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("postgres error [%s] %s: %s", pgErr.Code, pgErr.Message, pgErr.Detail)
	}
	return err
}
