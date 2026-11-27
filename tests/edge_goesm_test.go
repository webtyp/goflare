package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/goflare"
)

func TestBuildWorker_GoESM(t *testing.T) {
	t.Setenv("WEBTYP_EDGE", "js")

	dir := t.TempDir()
	edgeDir := filepath.Join(dir, "web", "edge")
	outDir := filepath.Join(dir, "dist")
	if err := os.MkdirAll(edgeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module edgepoc\n\ngo 1.26.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(edgeDir, "main.go")
	if err := os.WriteFile(entry, []byte("package main\n\nfunc Handle() string { return \"ok\" }\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := goflare.New(&goflare.Config{
		RootDir:   dir,
		Entry:     entry,
		OutputDir: outDir,
	})
	if err := g.Build(); err != nil {
		t.Fatalf("Build with WEBTYP_EDGE=js failed: %v", err)
	}
	jsPath := filepath.Join(outDir, "edge.js")
	info, err := os.Stat(jsPath)
	if err != nil || info.Size() == 0 {
		t.Fatalf("expected non-empty edge.js, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "edge.wasm")); err == nil {
		t.Fatal("expected zero edge.wasm under WEBTYP_EDGE=js")
	}
}
