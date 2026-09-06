//go:build !wasm

package goflare_test

import (
	"regexp"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestTinyGoVersionIsNotEmpty(t *testing.T) {
	if goflare.TinyGoVersion == "" {
		t.Fatal("expected TinyGoVersion to be non-empty")
	}

	matched, err := regexp.MatchString(`^\d+\.\d+\.\d+`, goflare.TinyGoVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Errorf("expected TinyGoVersion to match major.minor.patch, got: %s", goflare.TinyGoVersion)
	}
}

func TestRunTinyGoOutputFormat(t *testing.T) {
	dir := "/home/runner/.local/tinygo/bin"
	version := "tinygo version 0.41.1 linux/amd64"

	out := goflare.FormatTinyGoOutput(dir, version)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 lines of output, got %d: %q", len(lines), out)
	}

	if !strings.HasPrefix(lines[0], goflare.TinyGoBinDirPrefix) {
		t.Errorf("expected line 1 to start with %q, got: %s", goflare.TinyGoBinDirPrefix, lines[0])
	}

	if !strings.HasPrefix(lines[1], goflare.TinyGoVersionPrefix) {
		t.Errorf("expected line 2 to start with %q, got: %s", goflare.TinyGoVersionPrefix, lines[1])
	}

	if lines[0] != goflare.TinyGoBinDirPrefix+dir {
		t.Errorf("expected line 1 to be %q, got %q", goflare.TinyGoBinDirPrefix+dir, lines[0])
	}

	if lines[1] != goflare.TinyGoVersionPrefix+version {
		t.Errorf("expected line 2 to be %q, got %q", goflare.TinyGoVersionPrefix+version, lines[1])
	}
}
