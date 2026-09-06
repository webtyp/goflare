//go:build !wasm

package goflare_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestAssetHash_MatchesManualSHA256(t *testing.T) {
	content := []byte("<h1>hello</h1>")
	ext := "html"

	b64 := base64.StdEncoding.EncodeToString(content)
	sum := sha256.Sum256([]byte(b64 + ext))
	want := hex.EncodeToString(sum[:])[:32]

	got := goflare.ExportAssetHash(content, ext)

	if len(got) != 32 {
		t.Fatalf("expected a 32-character hash, got %d: %q", len(got), got)
	}
	if got != want {
		t.Errorf("assetHash mismatch: want %q, got %q", want, got)
	}
}

func TestBuildAssetManifest_KeysUseLeadingSlashAndForwardSlashes(t *testing.T) {
	env := newTestEnv(t)
	// Build a nested path with filepath.Join, which uses the OS separator.
	nested := filepath.Join("css", "site.css")
	env.writePublic(nested, "body{}")

	g := goflare.New(&goflare.Config{PublicDir: env.PublicDir, OutputDir: env.OutputDir})

	manifest, byHash, err := g.ExportBuildAssetManifest(env.PublicDir)
	if err != nil {
		t.Fatalf("buildAssetManifest failed: %v", err)
	}

	wantKey := "/css/site.css"
	entry, ok := manifest[wantKey]
	if !ok {
		t.Fatalf("expected manifest key %q, got keys: %v", wantKey, keysOf(manifest))
	}
	if entry.Hash == "" {
		t.Error("expected a non-empty hash for the manifest entry")
	}
	if _, ok := byHash[entry.Hash]; !ok {
		t.Errorf("expected byHash index to contain hash %q", entry.Hash)
	}

	// index.html comes from newTestEnv itself.
	if _, ok := manifest["/index.html"]; !ok {
		t.Errorf("expected manifest key /index.html, got keys: %v", keysOf(manifest))
	}
}

func keysOf[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestUploadAssets_EmptyBucketsSkipsPhase2(t *testing.T) {
	env := newTestEnv(t)
	phase2Called := false

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case strings.Contains(r.URL.Path, "/workers/assets/upload"):
			phase2Called = true
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"should-not-be-used"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID:  "acc-123",
		WorkerName: "my-worker",
		PublicDir:  env.PublicDir,
		OutputDir:  env.OutputDir,
	})

	client := &goflare.CfClient{Token: "account-token", BaseURL: server.URL, HttpClient: http.DefaultClient}

	manifest, byHash, err := g.ExportBuildAssetManifest(env.PublicDir)
	if err != nil {
		t.Fatalf("buildAssetManifest failed: %v", err)
	}

	token, err := g.ExportUploadAssets(client, manifest, byHash)
	if err != nil {
		t.Fatalf("uploadAssets failed: %v", err)
	}

	if phase2Called {
		t.Error("expected phase 2 (/workers/assets/upload) not to be called when buckets is empty")
	}
	if token != "session-jwt" {
		t.Errorf("expected completion token to be the phase-1 jwt %q, got %q", "session-jwt", token)
	}
}

func TestUploadAssets_TwoBucketsReturnsLastJWT(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")
	os.WriteFile(file1, []byte("a"), 0644)
	os.WriteFile(file2, []byte("b"), 0644)

	byHash := map[string]string{
		"hash1": file1,
		"hash2": file2,
	}

	var phase2Calls int
	var authHeaders []string

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[["hash1"],["hash2"]]}}`))
		case strings.Contains(r.URL.Path, "/workers/assets/upload"):
			phase2Calls++
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			if phase2Calls == 1 {
				w.Write([]byte(`{"success":true,"result":{"jwt":"completion-jwt-1"}}`))
			} else {
				w.Write([]byte(`{"success":true,"result":{"jwt":"completion-jwt-2"}}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{AccountID: "acc-123", WorkerName: "my-worker"})
	client := &goflare.CfClient{Token: "account-token", BaseURL: server.URL, HttpClient: http.DefaultClient}

	// manifest itself is irrelevant here: the buckets to upload come from the
	// (mocked) phase-1 response, not from the manifest we send.
	token, err := g.ExportUploadAssets(client, nil, byHash)
	if err != nil {
		t.Fatalf("uploadAssets failed: %v", err)
	}

	if phase2Calls != 2 {
		t.Fatalf("expected 2 phase-2 uploads (one per bucket), got %d", phase2Calls)
	}
	if token != "completion-jwt-2" {
		t.Errorf("expected the completion token from the last response, got %q", token)
	}
	for _, h := range authHeaders {
		if h != "Bearer session-jwt" {
			t.Errorf("expected phase 2 to authenticate with the phase-1 jwt, got %q", h)
		}
	}
}
