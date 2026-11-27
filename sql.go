package main

import (
	"strings"
)

// splitSQL splits a SQL script into individual statements.
// It handles:
//   - Semicolon-delimited statements
//   - DO $$ ... END; blocks (dollar-quoted strings)
//   - Comments (both -- line comments and /* */ block comments)
//   - Single-quoted and double-quoted strings
//
// The returned slice contains the statements WITHOUT trailing semicolons.
func splitSQL(sql string) []string {
	var statements []string
	var current strings.Builder

	i := 0
	n := len(sql)

	for i < n {
		c := sql[i]

		switch {
		// Line comment
		case c == '-' && i+1 < n && sql[i+1] == '-':
			// Skip to end of line
			j := i
			for j < n && sql[j] != '\n' {
				j++
			}
			current.WriteString(sql[i:j])
			i = j

		// Block comment
		case c == '/' && i+1 < n && sql[i+1] == '*':
			// Find closing */
			j := i + 2
			for j < n-1 {
				if sql[j] == '*' && sql[j+1] == '/' {
					j += 2
					break
				}
				j++
			}
			if j >= n {
				// Unterminated block comment; include rest
				current.WriteString(sql[i:])
				i = n
			} else {
				current.WriteString(sql[i:j])
				i = j
			}

		// Dollar-quoted string ($$ ... $$ or $tag$ ... $tag$)
		case c == '$':
			dollarEnd := findDollarQuotedEnd(sql, i)
			if dollarEnd > 0 {
				// Include the entire dollar-quoted string
				current.WriteString(sql[i : i+dollarEnd])
				i += dollarEnd
			} else {
				current.WriteByte(c)
				i++
			}

		// Single-quoted string
		case c == '\'':
			j := findSingleQuoteEnd(sql, i)
			current.WriteString(sql[i : i+j])
			i += j

		// Double-quoted string
		case c == '"':
			j := findDoubleQuoteEnd(sql, i)
			current.WriteString(sql[i : i+j])
			i += j

		// Semicolon: end of statement
		case c == ';':
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			i++

		default:
			// Skip pure whitespace between statements
			if current.Len() == 0 && c <= ' ' {
				i++
				continue
			}
			current.WriteByte(c)
			i++
		}
	}

	// Final statement (no trailing semicolon)
	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// findDollarQuotedEnd finds the end of a dollar-quoted string starting at position i.
// Returns the total length of the dollar-quoted string including the opening and closing dollar signs.
// Returns -1 if not found.
func findDollarQuotedEnd(sql string, start int) int {
	n := len(sql)
	if start >= n || sql[start] != '$' {
		return -1
	}

	// Check for $$ (empty tag)
	if start+1 < n && sql[start+1] == '$' {
		// Look for closing $$
		j := start + 2
		for j < n-1 {
			if sql[j] == '$' && sql[j+1] == '$' {
				return j + 2 - start
			}
			j++
		}
		return -1
	}

	// Check for $tag$ pattern
	// The tag is alphanumeric + underscore
	j := start + 1
	for j < n && (isAlphaNum(sql[j]) || sql[j] == '_') {
		j++
	}
	if j < n && sql[j] == '$' {
		tag := sql[start : j+1]
		tagLen := len(tag)
		// Find closing tag
		k := j + 1
		for k+tagLen <= n {
			if sql[k:k+tagLen] == tag {
				return k + tagLen - start
			}
			k++
		}
		return -1
	}

	return -1
}

func isAlphaNum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// findSingleQuoteEnd finds the end of a single-quoted string starting at position i.
// Handles escaped quotes ('').
func findSingleQuoteEnd(sql string, start int) int {
	n := len(sql)
	j := start + 1
	for j < n {
		if sql[j] == '\'' {
			// Check for escaped quote ''
			if j+1 < n && sql[j+1] == '\'' {
				j += 2
				continue
			}
			return j + 1 - start
		}
		j++
	}
	return n - start
}

// findDoubleQuoteEnd finds the end of a double-quoted string starting at position i.
func findDoubleQuoteEnd(sql string, start int) int {
	n := len(sql)
	j := start + 1
	for j < n {
		if sql[j] == '"' {
			return j + 1 - start
		}
		j++
	}
	return n - start
}
