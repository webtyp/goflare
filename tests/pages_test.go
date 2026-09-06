//go:build integration

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/goflare"
)

func TestGeneratePagesFiles(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "goflare-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy main.go file
	webDir := filepath.Join(tmpDir, "web")
	err = os.MkdirAll(webDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create web dir: %v", err)
	}

	mainGoContent := `package main
func main() {}
`
	err = os.WriteFile(filepath.Join(webDir, "main.go"), []byte(mainGoContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	// Create public dir
	publicDir := filepath.Join(tmpDir, "web/public")
	err = os.MkdirAll(publicDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create public dir: %v", err)
	}

	// Initialize Goflare with test configuration
	cfg := &goflare.Config{
		ProjectName: "test-project",
		AccountID:   "test-account",
		Entry:       webDir,
		PublicDir:   publicDir,
		OutputDir:   filepath.Join(tmpDir, ".build/"),
	}
	g := goflare.New(cfg)
	builder, _ := newFakeSiteBuilder(map[string][]byte{"index.html": []byte("<html></html>")}, nil)
	g.SetSiteBuilder(builder)

	err = g.GeneratePagesFiles()
	if err != nil {
		t.Fatalf("GeneratePagesFiles failed: %v", err)
	}
}
