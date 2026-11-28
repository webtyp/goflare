package goflare_test

import (
	"path/filepath"
	"testing"

	"webtyp.com/goflare"
)

func TestEventsPaths(t *testing.T) {
	cfg := &goflare.Config{
		Entry:     "web/edge/main.go",
		OutputDir: "web/edge",
	}

	h := goflare.New(cfg)

	gotMain := h.MainInputFileRelativePath()
	wantMain := filepath.Join("web", "edge", "main.go")
	if gotMain != wantMain {
		t.Errorf("MainInputFileRelativePath() = %q, want %q", gotMain, wantMain)
	}

	gotUnobserved := h.UnobservedFiles()
	if len(gotUnobserved) != 2 {
		t.Fatalf("UnobservedFiles() returned %d files, want 2", len(gotUnobserved))
	}
	wantEdge := filepath.Join("web", "edge", goflare.EdgeModule)
	wantApp := filepath.Join("web", "edge", goflare.AppModule)
	if gotUnobserved[0] != wantEdge || gotUnobserved[1] != wantApp {
		t.Errorf("UnobservedFiles() = %v, want [%q, %q]", gotUnobserved, wantEdge, wantApp)
	}
}
