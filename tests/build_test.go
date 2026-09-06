//go:build !wasm

package goflare_test

import (
	"os"
	"path/filepath"
	"testing"

	"webtyp.com/goflare"
)

func TestBuild_PagesOnly(t *testing.T) {
	env := newTestEnv(t)

	builder, _ := newFakeSiteBuilder(map[string][]byte{
		"index.html":       []byte("<html></html>"),
		"assets/style.css": []byte("body {}"),
	}, nil)

	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	g.SetSiteBuilder(builder)
	err := g.Build()
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(env.PublicDir, "index.html")); err != nil {
		t.Errorf("index.html missing in PublicDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.PublicDir, "assets", "style.css")); err != nil {
		t.Errorf("style.css missing in PublicDir: %v", err)
	}
}

func TestBuild_NothingToBuild(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		Entry:       "",
		PublicDir:   "",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when nothing to build")
	}
}

func TestBuild_MissingEntry(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		Entry:       "non-existent.go",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when Entry does not exist")
	}
}

func TestBuild_MissingPublicDir(t *testing.T) {
	cfg := &goflare.Config{
		ProjectName: "test",
		AccountID:   "123",
		PublicDir:   "non-existent-dir",
	}
	g := goflare.New(cfg)
	err := g.Build()
	if err == nil {
		t.Fatal("Expected error when PublicDir does not exist")
	}
}

// TestNew_StagingDirNotInProjectRoot verifies that goflare.New() does not
// create its staging directory inside the caller's working directory.
//
// Regression: client.New() defaults AppRootDir=".", so filepath.Join(".", "/tmp/xxx")
// produced the relative path "tmp/xxx". The staging dir was written to the project
// root instead of /tmp, and the subsequent move to functions/ failed because
// goflare looked for the file at the absolute /tmp path.
func TestNew_StagingDirNotInProjectRoot(t *testing.T) {
	env := newTestEnv(t)

	cfg := &goflare.Config{
		PublicDir: env.PublicDir,
		OutputDir: env.OutputDir,
	}

	// Snapshot CWD entries before New()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	beforeEntries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}

	g := goflare.New(cfg)
	_ = g

	afterEntries, err := os.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}

	if len(afterEntries) != len(beforeEntries) {
		for _, e := range afterEntries {
			if e.IsDir() && len(e.Name()) > 7 && e.Name()[:7] == "goflare" {
				os.RemoveAll(filepath.Join(cwd, e.Name()))
				t.Errorf("staging dir %q was created in CWD — must be in os.TempDir()", e.Name())
			}
		}
	}
}
