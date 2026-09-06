//go:build !wasm

package goflare_test

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"webtyp.com/goflare"
)

// TestEdgeSize checks the hard Cloudflare limit (<1 MB) for .build/edge.wasm.
// SRP: migrated from veltylabs/misitio/tests/edge_size_test.go — it is a
// platform limit (goflare/Cloudflare), not a misitio domain one.
func TestEdgeSize(t *testing.T) {
	if err := goflare.EnsureTinyGo(io.Discard); err != nil && os.Getenv("CI") != "" {
		t.Fatalf("could not install tinygo in CI: %v", err)
	}
	tinygoPath, err := exec.LookPath("tinygo")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("tinygo is not installed in PATH in CI: %v", err)
		}
		t.Skip("tinygo is not installed in PATH")
	}
	tmpFile, err := os.CreateTemp("", "edge-*.wasm")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Create a minimal temporary main importing cloudflare/edge to measure the
	// platform's baseline cost without any domain logic.
	//
	// It gets its OWN module rather than borrowing goflare's. edge pulls in the
	// whole cloudflare tree, and goflare's go.sum only covers what goflare
	// itself imports — so `go mod tidy` prunes the entries this build needs and
	// the test breaks for a reason that has nothing to do with the size it
	// measures.
	tmpDir, err := os.MkdirTemp("", "edge-main-*")
	if err != nil {
		t.Fatalf("failed to create temporary dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	mainPath := tmpDir + "/main.go"
	if err := os.WriteFile(mainPath, []byte("package main\nimport _ \"webtyp.com/cloudflare/edge\"\nfunc main(){}\n"), 0644); err != nil {
		t.Fatalf("failed to write temporary main: %v", err)
	}

	cfVersion, err := goflare.ProjectCloudflareVersion("..")
	if err != nil {
		t.Fatalf("failed to read the cloudflare version from go.mod: %v", err)
	}
	for _, args := range [][]string{
		{"mod", "init", "edgesize"},
		{"get", "webtyp.com/cloudflare@" + cfVersion},
		{"mod", "tidy"},
	} {
		c := exec.Command("go", args...)
		c.Dir = tmpDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Skipf("no module cache/network access to resolve the fixture (go %v): %v\n%s", args, err, out)
		}
	}

	cmd := exec.Command(tinygoPath, "build", "-target", "wasm", "-no-debug", "-o", tmpPath, ".")
	cmd.Dir = tmpDir
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build edge with tinygo: %v\n%s", err, string(out))
	}
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("failed to stat the binary size: %v", err)
	}
	const maxBytes = 1048576
	if info.Size() > maxBytes {
		t.Fatalf("edge.wasm exceeds the Cloudflare limit: %d bytes (max 1048576)", info.Size())
	}
}
