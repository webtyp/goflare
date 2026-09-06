//go:build !wasm

package goflare_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
	"webtyp.com/sitec"
)

type fakeSite struct {
	cfg     sitec.BuildConfig
	written bool
	files   map[string][]byte
}

func (f *fakeSite) WriteTo(fs sitec.FS) error {
	f.written = true
	for relPath, data := range f.files {
		outPath := filepath.Join(f.cfg.OutputDir, relPath)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func newFakeSiteBuilder(files map[string][]byte, buildErr error) (goflare.SiteBuilder, *fakeSite) {
	s := &fakeSite{files: files}
	return func(cfg sitec.BuildConfig) (goflare.SiteOutput, error) {
		s.cfg = cfg
		if buildErr != nil {
			return nil, buildErr
		}
		return s, nil
	}, s
}

func TestBuildPages_PassesModuleRootAndPublicDir(t *testing.T) {
	env := newTestEnv(t)
	builder, fake := newFakeSiteBuilder(nil, nil)

	cfg := &goflare.Config{
		ProjectName: "my-app",
		AccountID:   "123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	g.SetSiteBuilder(builder)
	g.SetLog(func(msg ...any) {})

	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if fake.cfg.RootDir != "." {
		t.Errorf("expected RootDir '.', got %q", fake.cfg.RootDir)
	}
	if fake.cfg.OutputDir != env.PublicDir {
		t.Errorf("expected OutputDir %q, got %q", env.PublicDir, fake.cfg.OutputDir)
	}
	if fake.cfg.AppName != "my-app" {
		t.Errorf("expected AppName 'my-app', got %q", fake.cfg.AppName)
	}
	if fake.cfg.Log == nil {
		t.Error("expected Log function to be non-nil")
	}
}

func TestBuildPages_ModeFollowsCompilerMode(t *testing.T) {
	tests := []struct {
		compilerMode string
		expectedMode sitec.Mode
	}{
		{compilerMode: "L", expectedMode: sitec.ModeDev},
		{compilerMode: "S", expectedMode: sitec.ModeRelease},
		{compilerMode: "", expectedMode: sitec.ModeRelease},
	}

	for _, tt := range tests {
		t.Run("CompilerMode="+tt.compilerMode, func(t *testing.T) {
			env := newTestEnv(t)
			builder, fake := newFakeSiteBuilder(nil, nil)

			cfg := &goflare.Config{
				ProjectName:  "test",
				PublicDir:    env.PublicDir,
				OutputDir:    env.OutputDir,
				CompilerMode: tt.compilerMode,
			}

			g := goflare.New(cfg)
			g.SetSiteBuilder(builder)

			if err := g.Build(); err != nil {
				t.Fatalf("Build failed: %v", err)
			}

			if fake.cfg.Mode != tt.expectedMode {
				t.Errorf("expected Mode %v, got %v", tt.expectedMode, fake.cfg.Mode)
			}
		})
	}
}

func TestBuildPages_WritesArtifactsToDisk(t *testing.T) {
	env := newTestEnv(t)
	files := map[string][]byte{
		"index.html": []byte("<html><body>app</body></html>"),
		"style.css":  []byte("body { color: red; }"),
	}
	builder, fake := newFakeSiteBuilder(files, nil)

	cfg := &goflare.Config{
		ProjectName: "test",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	g.SetSiteBuilder(builder)

	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !fake.written {
		t.Error("expected WriteTo to have been called")
	}

	indexContent, err := os.ReadFile(filepath.Join(env.PublicDir, "index.html"))
	if err != nil {
		t.Fatalf("failed to read index.html: %v", err)
	}
	if string(indexContent) != "<html><body>app</body></html>" {
		t.Errorf("unexpected index.html content: %s", string(indexContent))
	}

	styleContent, err := os.ReadFile(filepath.Join(env.PublicDir, "style.css"))
	if err != nil {
		t.Fatalf("failed to read style.css: %v", err)
	}
	if string(styleContent) != "body { color: red; }" {
		t.Errorf("unexpected style.css content: %s", string(styleContent))
	}
}

func TestBuildPages_PropagatesSiteBuildError(t *testing.T) {
	env := newTestEnv(t)
	siteErr := errors.New("sitec compiler failure: syntax error in css.go")
	builder, _ := newFakeSiteBuilder(nil, siteErr)

	cfg := &goflare.Config{
		ProjectName: "test",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}

	g := goflare.New(cfg)
	g.SetSiteBuilder(builder)

	err := g.Build()
	if err == nil {
		t.Fatal("expected Build to fail when siteBuilder returns an error")
	}

	if !strings.Contains(err.Error(), siteErr.Error()) {
		t.Errorf("expected error message to contain %q, got %q", siteErr.Error(), err.Error())
	}
}
