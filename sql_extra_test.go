package main

import (
	"reflect"
	"testing"
)

func TestFindDollarQuotedEnd_EdgeCases(t *testing.T) {
	// "DO $$ BEGIN END; $$"
	// D(0)O(1) (2)$(3)$(4) (5)B(6)E(7)G(8)I(9)N(10) (11)E(12)N(13)D(14);(15) (16)$(17)$(18)
	t.Run("dollar_block", func(t *testing.T) {
		input := "DO $$ BEGIN END; $$"
		// start=3 is position of opening $$
		result := findDollarQuotedEnd(input, 3)
		// closing $$ at pos 17-18, return 17+2-3 = 16
		if result != 16 {
			t.Errorf("findDollarQuotedEnd() = %d, want 16", result)
		}
	})

	// "AS $fn$ body $fn$"
	// A(0)S(1) (2)$(3)f(4)n(5)$(6) (7)b(8)o(9)d(10)y(11) (12)$(13)f(14)n(15)$(16)
	t.Run("tagged_end", func(t *testing.T) {
		input := "AS $fn$ body $fn$"
		// start=3 is position of opening $fn$
		result := findDollarQuotedEnd(input, 3)
		// closing $fn$ at pos 13-16, return 13+4-3 = 14
		if result != 14 {
			t.Errorf("findDollarQuotedEnd() = %d, want 14", result)
		}
	})

	t.Run("no_closing", func(t *testing.T) {
		input := "DO $$ BEGIN"
		result := findDollarQuotedEnd(input, 3)
		if result != -1 {
			t.Errorf("findDollarQuotedEnd() = %d, want -1", result)
		}
	})

	t.Run("invalid_start", func(t *testing.T) {
		input := "DO $$"
		result := findDollarQuotedEnd(input, 10)
		if result != -1 {
			t.Errorf("findDollarQuotedEnd() = %d, want -1", result)
		}
	})
}

func TestFindSingleQuoteEnd_EdgeCases(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		// "it's" - i(0)t(1)'(2)s(3)
		// start=2 (position of opening quote)
		result := findSingleQuoteEnd("it's", 2)
		// j=3, sql[3]='s' → j=4, j>=n(4) → return 4-2 = 2
		if result != 2 {
			t.Errorf("findSingleQuoteEnd() = %d, want 2", result)
		}
	})

	t.Run("escaped", func(t *testing.T) {
		// "it''s" - i(0)t(1)'(2)'(3)s(4)
		// start=2
		result := findSingleQuoteEnd("it''s", 2)
		// j=3, sql[3]=''' && sql[4]='s' (not '') → return 3+1-2 = 2
		// Wait: j=3, sql[3]=''', j+1=4, sql[4]='s' (not ''') → return 3+1-2=2
		// Actually: j=3, sql[3]=''', j+1=4 < n, sql[4]='s' ≠ ''' → return 3+1-2 = 2
		if result != 2 {
			t.Errorf("findSingleQuoteEnd() = %d, want 2", result)
		}
	})

	t.Run("proper_escaped", func(t *testing.T) {
		// "a''b'" - a(0)'(1)'(2)b(3)'(4)
		// start=1
		result := findSingleQuoteEnd("a''b'", 1)
		// j=2, sql[2]=''' && sql[3]='b' → not escape... wait
		// j=2, sql[2]=''', j+1=3, sql[3]='b' → not '' → return 2+1-1 = 2
		if result != 2 {
			t.Errorf("findSingleQuoteEnd() = %d, want 2", result)
		}
	})

	t.Run("double_escaped", func(t *testing.T) {
		// "'''" - '(0)'(1)'(2)
		// start=0
		result := findSingleQuoteEnd("'''", 0)
		// j=1, sql[1]=''' && sql[2]=''' → j+=2 → j=3, j>=n(3) → return 3-0 = 3
		if result != 3 {
			t.Errorf("findSingleQuoteEnd() = %d, want 3", result)
		}
	})
}

func TestFindDoubleQuoteEnd_EdgeCases(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		// "\"col\"" - "(0)c(1)o(2)l(3)"(4)
		// start=0
		result := findDoubleQuoteEnd("\"col\"", 0)
		// j=1 to j=3, sql[4]='"' → return 4+1-0 = 5
		if result != 5 {
			t.Errorf("findDoubleQuoteEnd() = %d, want 5", result)
		}
	})

	t.Run("no_close", func(t *testing.T) {
		// "\"col" - "(0)c(1)o(2)l(3)
		// start=0
		result := findDoubleQuoteEnd("\"col", 0)
		// j=1 to j=3, j>=n(4) → return 4-0 = 4
		if result != 4 {
			t.Errorf("findDoubleQuoteEnd() = %d, want 4", result)
		}
	})
}

func TestSplitSQL_ComplexBlock(t *testing.T) {
	input := `
DO $$
BEGIN
  -- This is a test;
  RAISE NOTICE 'done;';
END;
$$;
SELECT 'result' AS val;
`
	result := splitSQL(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(result), result)
	}
	// The first statement should contain the DO block
	if result[0][:2] != "DO" {
		t.Errorf("first statement should start with DO, got: %q", result[0][:10])
	}
	if result[1] != "SELECT 'result' AS val" {
		t.Errorf("second statement = %q, want %q", result[1], "SELECT 'result' AS val")
	}
}

func TestSplitSQL_NestedQuotes(t *testing.T) {
	input := `INSERT INTO t VALUES('a''b"c'); INSERT INTO t VALUES("d'e'f");`
	result := splitSQL(input)
	if len(result) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(result), result)
	}
}

func TestSplitSQL_EmptyStatements(t *testing.T) {
	input := ";;SELECT 1;;"
	result := splitSQL(input)
	if len(result) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(result), result)
	}
	if result[0] != "SELECT 1" {
		t.Errorf("result[0] = %q, want %q", result[0], "SELECT 1")
	}
}

func TestSplitSQL_MultiLine(t *testing.T) {
	input := "SELECT a,\n  b,\n  c\nFROM t;\nSELECT 1;"
	expected := []string{
		"SELECT a,\n  b,\n  c\nFROM t",
		"SELECT 1",
	}

	result := splitSQL(input)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("splitSQL() = %v, want %v", result, expected)
	}
}
