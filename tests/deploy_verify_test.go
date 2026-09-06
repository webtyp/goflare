//go:build !wasm

package goflare_test

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"webtyp.com/goflare"
)

func TestDeployVerify_Success(t *testing.T) {
	env := newTestEnv(t)
	env.writeOutput("edge.js", "console.log('edge')")
	env.writeOutput("edge.wasm", "wasm")

	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	probeCalled := false

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/assets-upload-session") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
			return
		}

		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/api/__goflare_probe") {
			probeCalled = true
			w.Header().Set("x-goflare", "dev")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "my-app",
		AccountID:   "acc-123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL
	g.SiteURL = server.URL
	g.RetryBackoff = time.Millisecond

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if !probeCalled {
		t.Error("Expected probe endpoint to have been requested")
	}
}

func TestDeployVerify_MissingHeaderFails(t *testing.T) {
	env := newTestEnv(t)
	env.writeOutput("edge.js", "console.log('edge')")
	env.writeOutput("edge.wasm", "wasm")

	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/assets-upload-session") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
			return
		}

		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}

		// HTML shell response without x-goflare header (reproduction of bug)
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/api/__goflare_probe") {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<!doctype html><html></html>"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "my-app",
		AccountID:   "acc-123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL
	g.SiteURL = server.URL
	g.RetryBackoff = time.Millisecond

	err := g.Deploy()
	if err == nil {
		t.Fatal("Expected Deploy to fail when probe responds without x-goflare header")
	}

	if !strings.Contains(err.Error(), "deploy verification failed") {
		t.Errorf("Unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), goflare.HeaderIdentity) {
		t.Errorf("Expected error to mention header %s: %v", goflare.HeaderIdentity, err)
	}
}

func TestDeployVerify_AssetOnlySkipsProbe(t *testing.T) {
	env := newTestEnv(t)
	env.writePublic("index.html", "<h1>hello</h1>")

	os.Setenv("CLOUDFLARE_API_TOKEN", "valid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	probeCalled := false

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/assets-upload-session") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
			return
		}

		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/api/__goflare_probe") {
			probeCalled = true
		}

		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	cfg := &goflare.Config{
		ProjectName: "my-app",
		AccountID:   "acc-123",
		PublicDir:   env.PublicDir,
		OutputDir:   env.OutputDir,
	}
	g := goflare.New(cfg)
	g.BaseURL = server.URL
	g.SiteURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if probeCalled {
		t.Error("Expected asset-only deployment to skip probe verification")
	}
}

func TestBuild_JSBundleMarkerReplaced(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(cwd)

	tmpDir := t.TempDir()
	entryDir := filepath.Join(tmpDir, "edge")
	outputDir := filepath.Join(tmpDir, ".build")
	os.MkdirAll(entryDir, 0755)

	modBytes, _ := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	newMod := strings.Replace(string(modBytes), "module webtyp.com/goflare", "module testapp", 1)
	newMod = strings.ReplaceAll(newMod, "webtyp.com/cloudflare v0.0.2", "webtyp.com/cloudflare v0.0.0")
	{
		var keep []string
		for _, ln := range strings.Split(newMod, "\n") {
			if strings.HasPrefix(strings.TrimSpace(ln), "replace webtyp.com/") {
				continue
			}
			keep = append(keep, ln)
		}
		newMod = strings.Join(keep, "\n") + "\n" + webtypReplaces(t)
	}
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(newMod), 0644)
	if sumBytes, err := os.ReadFile(filepath.Join(repoRoot, "go.sum")); err == nil {
		os.WriteFile(filepath.Join(tmpDir, "go.sum"), sumBytes, 0644)
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, string(out))
	}

	mainContent := `package main
import _ "webtyp.com/cloudflare/edge"
func main() {}
`
	os.WriteFile(filepath.Join(entryDir, "main.go"), []byte(mainContent), 0644)

	cfg := &goflare.Config{
		Entry:     entryDir,
		OutputDir: outputDir,
	}
	g := goflare.New(cfg)

	if err := g.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	edgeJs := filepath.Join(outputDir, "edge.js")
	jsContent, err := os.ReadFile(edgeJs)
	if err != nil {
		t.Fatalf("Failed to read edge.js: %v", err)
	}
	jsStr := string(jsContent)

	if !strings.Contains(jsStr, goflare.HeaderIdentity) {
		t.Errorf("Expected edge.js to contain identity header name %q", goflare.HeaderIdentity)
	}
	if strings.Contains(jsStr, "__GOFLARE_VERSION__") {
		t.Errorf("Expected edge.js NOT to contain raw marker __GOFLARE_VERSION__")
	}
}
