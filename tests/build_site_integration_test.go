//go:build integration

package goflare_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestBuildSite_Integration(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// 1. Minimal go.mod
	goModContent := "module example.com/site\n\ngo 1.25.2\n"
	if err := os.WriteFile("go.mod", []byte(goModContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. web/client.go
	webDir := "web"
	if err := os.MkdirAll(webDir, 0755); err != nil {
		t.Fatal(err)
	}
	clientGo := "//go:build wasm\n\npackage main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(webDir, "client.go"), []byte(clientGo), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. web/public/
	publicDir := filepath.Join("web", "public")
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 4. config/css.go
	configDir := "config"
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	cssGo := `//go:build !wasm

package config

import "webtyp.com/css"

type Panel struct{}

func (p *Panel) RootCSS() *css.Stylesheet { return css.Theme() }
`
	if err := os.WriteFile(filepath.Join(configDir, "css.go"), []byte(cssGo), 0644); err != nil {
		t.Fatal(err)
	}

	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("no module cache/network access to resolve the fixture: %v\n%s", err, out)
	}

	cfg := &goflare.Config{
		ProjectName:  "integration-site",
		PublicDir:    publicDir,
		OutputDir:    ".build",
		CompilerMode: goflare.CompilerModeStdlib,
	}

	g := goflare.New(cfg)
	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	styleCss, err := os.ReadFile(filepath.Join(publicDir, "style.css"))
	if err != nil {
		t.Fatalf("failed to read style.css: %v", err)
	}
	if len(styleCss) == 0 {
		t.Error("expected style.css to be non-empty")
	}

	indexHtml, err := os.ReadFile(filepath.Join(publicDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	if !strings.Contains(string(indexHtml), `<div id="app">`) {
		t.Errorf("expected index.html to contain '<div id=\"app\">', got: %s", string(indexHtml))
	}
}
