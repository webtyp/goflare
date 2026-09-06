//go:build !wasm

package goflare_test

import (
	"reflect"
	"testing"

	"webtyp.com/gobuild"
	"webtyp.com/goflare"
)

func TestIsStdlib(t *testing.T) {
	tests := []struct {
		pkg  string
		want bool
	}{
		{"bytes", true},
		{"crypto/sha256", true},
		{"internal/poll", true},
		{"webtyp.com/fmt", false},
		{"golang.org/x/net/html", false},
		{"", false},
	}

	for _, tt := range tests {
		got := goflare.IsStdlib(tt.pkg)
		if got != tt.want {
			t.Errorf("IsStdlib(%q) = %v, want %v", tt.pkg, got, tt.want)
		}
	}
}

func TestFilterActionable(t *testing.T) {
	// 1. Actionable: direct import of forbidden package by non-stdlib package
	chain1 := gobuild.ImportChain{
		Forbidden: "bytes",
		Path:      []string{"webtyp.com/example", "bytes"},
	}

	// 2. Non-actionable: forbidden package reached via another stdlib package (crypto/sha256)
	chain2 := gobuild.ImportChain{
		Forbidden: "bytes",
		Path:      []string{"webtyp.com/example", "crypto/sha256", "bytes"},
	}

	chains := []gobuild.ImportChain{chain1, chain2}
	filtered := goflare.FilterActionable(chains)

	if len(filtered) != 1 {
		t.Fatalf("expected 1 actionable chain, got %d", len(filtered))
	}

	if !reflect.DeepEqual(filtered[0], chain1) {
		t.Errorf("expected filtered[0] to equal chain1, got: %+v", filtered[0])
	}
}
