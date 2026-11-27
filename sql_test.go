package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSplitSQL_SimpleStatements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single_statement",
			input:    "SELECT 1;",
			expected: []string{"SELECT 1"},
		},
		{
			name:     "two_statements",
			input:    "SELECT 1; SELECT 2;",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "three_statements",
			input:    "CREATE TABLE t(id INT); INSERT INTO t VALUES(1); SELECT * FROM t;",
			expected: []string{
				"CREATE TABLE t(id INT)",
				"INSERT INTO t VALUES(1)",
				"SELECT * FROM t",
			},
		},
		{
			name:     "no_trailing_semicolon",
			input:    "SELECT 1",
			expected: []string{"SELECT 1"},
		},
		{
			name:     "whitespace_between",
			input:    "  SELECT 1;   SELECT 2;  ",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "empty_input",
			input:    "",
			expected: nil,
		},
		{
			name:     "only_whitespace",
			input:    "   \n\t  ",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQL(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSplitSQL_DollarQuoted(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "dollar_quoted_function",
			input:    "CREATE FUNCTION f() RETURNS void AS $$ BEGIN RAISE NOTICE 'hello'; END; $$ LANGUAGE plpgsql;",
			expected: []string{"CREATE FUNCTION f() RETURNS void AS $$ BEGIN RAISE NOTICE 'hello'; END; $$ LANGUAGE plpgsql"},
		},
		{
			name: "dollar_quoted_do_block",
			input: "DO $$ BEGIN RAISE NOTICE 'step 1;'; RAISE NOTICE 'step 2;'; END; $$; " +
				"SELECT 1;",
			expected: []string{
				"DO $$ BEGIN RAISE NOTICE 'step 1;'; RAISE NOTICE 'step 2;'; END; $$",
				"SELECT 1",
			},
		},
		{
			name:     "tagged_dollar_quote",
			input:    "CREATE FUNCTION g() AS $fn$ BEGIN RETURN 1; END; $fn$ LANGUAGE plpgsql;",
			expected: []string{"CREATE FUNCTION g() AS $fn$ BEGIN RETURN 1; END; $fn$ LANGUAGE plpgsql"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQL(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSplitSQL_Comments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "line_comment",
			input:    "-- this is a comment\nSELECT 1;",
			expected: []string{"-- this is a comment\nSELECT 1"},
		},
		{
			name:     "block_comment",
			input:    "/* multi\nline comment */ SELECT 1;",
			expected: []string{"/* multi\nline comment */ SELECT 1"},
		},
		{
			name:     "comment_with_semicolon",
			input:    "-- comment with; semicolons\nSELECT 1;",
			expected: []string{"-- comment with; semicolons\nSELECT 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQL(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSplitSQL_QuotedStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single_quote_with_semicolon",
			input:    `INSERT INTO t VALUES('hello; world');`,
			expected: []string{`INSERT INTO t VALUES('hello; world')`},
		},
		{
			name:     "double_quote_with_semicolon",
			input:    `SELECT "col;name" FROM t;`,
			expected: []string{`SELECT "col;name" FROM t`},
		},
		{
			name:     "escaped_single_quote",
			input:    `INSERT INTO t VALUES('it''s a test');`,
			expected: []string{`INSERT INTO t VALUES('it''s a test')`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQL(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsSelectStatement(t *testing.T) {
	tests := []struct {
		sql      string
		expected bool
	}{
		{"SELECT * FROM t", true},
		{"select * from t", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"SHOW tables", true},
		{"EXPLAIN SELECT 1", true},
		{"INSERT INTO t VALUES(1)", false},
		{"CREATE TABLE t(id INT)", false},
		{"DROP TABLE t", false},
		{"BEGIN", false},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			result := isSelectStatement(tt.sql)
			if result != tt.expected {
				t.Errorf("isSelectStatement(%q) = %v, want %v", tt.sql, result, tt.expected)
			}
		})
	}
}

func TestAddLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{
			name:     "no_existing_limit",
			input:    "SELECT * FROM t",
			limit:    100,
			expected: "SELECT * FROM t LIMIT 100",
		},
		{
			name:     "existing_limit",
			input:    "SELECT * FROM t LIMIT 50",
			limit:    100,
			expected: "SELECT * FROM t LIMIT 50",
		},
		{
			name:     "trailing_semicolon",
			input:    "SELECT * FROM t;",
			limit:    100,
			expected: "SELECT * FROM t LIMIT 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addLimit(tt.input, tt.limit)
			if result != tt.expected {
				t.Errorf("addLimit() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSerializeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"nil", nil, nil},
		{"string_bytes", []byte("hello"), "hello"},
		{"int", 42, 42},
		{"float", 3.14, 3.14},
		{"bool", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serializeValue(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("serializeValue(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatError(t *testing.T) {
	err := fmt.Errorf("test error")
	result := formatError(err)
	if !strings.Contains(result, "test error") {
		t.Errorf("formatError() = %q, expected to contain 'test error'", result)
	}
}
