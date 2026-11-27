//go:build unit

package main

import "testing"

func TestVersion_Default(t *testing.T) {
	if version != "dev" {
		t.Errorf("expected default version to be %q, got %q", "dev", version)
	}
}
