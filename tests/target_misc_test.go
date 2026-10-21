//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/goflare"
	"webtyp.com/keyring"
	krtests "webtyp.com/keyring/tests"
)

func TestRuntime_W1_W2(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{Name: "test-script"}

	var requests []string
	var secretBody []byte

	const secretValue = "my-secret-val"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		isSecretPut := r.URL.Path == "/accounts/a/workers/scripts/test-script/secrets" && r.Method == http.MethodPut
		// W2: the value travels only in the body of the secrets PUT.
		if !isSecretPut && strings.Contains(r.URL.String()+string(body), secretValue) {
			t.Errorf("W2 failed: secret value found in %s %s", r.Method, r.URL)
		}
		if r.URL.Path == "/accounts" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []map[string]any{{"id": "a"}}})
			return
		}
		if r.URL.Path == "/accounts/a/workers/scripts/test-script/secrets" && r.Method == http.MethodGet {
			// Mock not found to trigger W1 upload
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/accounts/a/workers/scripts/test-script" && r.Method == http.MethodPut {
			json.NewEncoder(w).Encode(map[string]any{"success": true})
			return
		}
		if r.URL.Path == "/accounts/a/workers/scripts/test-script/secrets" && r.Method == http.MethodPut {
			secretBody = body
			json.NewEncoder(w).Encode(map[string]any{"success": true})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	dest := target.Runtime(p)

	has, err := dest.Has("MY_SEC")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if has {
		t.Fatalf("expected Has to return false for missing script")
	}

	err = dest.Put("MY_SEC", secretValue)
	if err != nil {
		t.Fatalf("unexpected Put error: %v", err)
	}

	// Verify W1 uploaded the placeholder
	foundUpload := false
	for _, req := range requests {
		if req == "PUT /accounts/a/workers/scripts/test-script" {
			foundUpload = true
		}
	}
	if !foundUpload {
		t.Fatalf("W1 failed: missing script upload request")
	}

	// The secrets PUT itself must carry the value.
	if !strings.Contains(string(secretBody), secretValue) {
		t.Fatalf("expected secret body to contain value, got %s", secretBody)
	}
}

func TestJob_J1(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: "."})
	p := deploy.Plan{
		Name:      "test-worker",
		Domain:    "ex.com",
		Databases: []deploy.Database{{Name: "d-name", Binding: "D_BIND"}},
		Buckets:   []deploy.Bucket{{Name: "b-name", Binding: "B_BIND"}},
		Vars: []deploy.Var{
			{Key: "K1", Value: "V1"},
			{Key: "K2", Value: "V2"},
		},
	}

	job := target.Job(p)

	if job.Uses != "webtyp/goflare@v1" {
		t.Fatalf("expected webtyp/goflare@v1")
	}

	if len(job.Secrets) != 2 || job.Secrets[0] != "CLOUDFLARE_API_TOKEN" || job.Secrets[1] != "CLOUDFLARE_ACCOUNT_ID" {
		t.Fatalf("expected exact secrets")
	}

	var hasPreDeploy bool
	var varsStr string
	for _, in := range job.With {
		if in.Name == "pre-deploy" {
			hasPreDeploy = true
		}
		if in.Name == "vars" {
			varsStr = in.Value
		}
	}

	if hasPreDeploy {
		t.Fatalf("did not expect pre-deploy when cmd/migrate/main.go missing")
	}
	if varsStr != "K1=V1\nK2=V2" {
		t.Fatalf("expected vars string formatted, got %q", varsStr)
	}

	// J1 with cmd/migrate
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "cmd", "migrate"), 0755)
	os.WriteFile(filepath.Join(tmp, "cmd", "migrate", "main.go"), []byte("package main"), 0644)

	target2, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: tmp})
	job2 := target2.Job(p)

	hasPreDeploy2 := false
	for _, in := range job2.With {
		if in.Name == "pre-deploy" {
			hasPreDeploy2 = true
			if in.Value != "go run ./cmd/migrate" {
				t.Fatalf("expected pre-deploy value go run ./cmd/migrate, got %s", in.Value)
			}
		}
	}
	if !hasPreDeploy2 {
		t.Fatalf("expected pre-deploy when cmd/migrate/main.go exists")
	}
}

func TestConfig_C1_C2_V1_V2(t *testing.T) {
	// Setup env
	os.Setenv("D1_BINDING", "DB")
	os.Setenv("D1_NAME", "my-d1")
	os.Setenv("CLOUDFLARE_ACCOUNT_ID", "a")
	defer os.Unsetenv("CLOUDFLARE_ACCOUNT_ID")
	os.Setenv("VARS", "K1=V1\nK2=V2\nINVALID")
	defer os.Unsetenv("D1_BINDING")
	defer os.Unsetenv("D1_NAME")
	defer os.Unsetenv("VARS")

	// C1: CI side config loading
	_, err := goflare.LoadConfigFromEnv("")
	if err == nil || !strings.Contains(err.Error(), "ErrInvalidVarLine") {
		t.Fatalf("expected ErrInvalidVarLine on INVALID, got %v", err)
	}

	os.Setenv("VARS", "K1=V1\nK2=V2")
	cfg, err := goflare.LoadConfigFromEnv("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// We can't access cfg.Vars directly as it is not exported if tests are in a different package,
	// but actually it is exported. `Vars []deploy.Var`
	if len(cfg.Vars) != 2 || cfg.Vars[0].Key != "K1" || cfg.Vars[1].Value != "V2" {
		t.Fatalf("V1 parsing failed: %v", cfg.Vars)
	}

	// We will just verify Deploy returns ErrD1NotFound if the server returns empty.
	srvErr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/user/tokens/verify") {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": map[string]any{"status": "active"}})
			return
		}
		if strings.Contains(r.URL.Path, "/d1/database") {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}}) // empty
			return
		}
	}))
	defer srvErr.Close()

	// Need valid Deploy config
	cfg.ProjectName = "test"
	cfg.Entry = "web/edge.go"
	t.Chdir(t.TempDir())
	os.MkdirAll("web", 0755)
	os.WriteFile("web/edge.go", []byte("package main\nfunc main(){}"), 0644)
	os.MkdirAll("web", 0755)
	os.WriteFile("web/edge.js", []byte(""), 0644)
	os.WriteFile("web/edge.wasm", []byte(""), 0644)

	g2 := goflare.New(cfg)
	g2.BaseURL = srvErr.URL

	g2.SetLog(func(msgs ...any) {})
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	err = g2.Deploy()
	if err == nil || !strings.Contains(err.Error(), "ErrD1NotFound") {
		t.Fatalf("expected ErrD1NotFound, got %v", err)
	}
}

// rewriteTransport sends every request to the fake Cloudflare server.
type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme = rt.target.Scheme
	r.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func TestRunD1ID_C2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/v4/accounts/a/d1/database" || r.URL.Query().Get("name") != "my-d1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []map[string]any{
			{"name": "my-d1-other", "uuid": "wrong"},
			{"name": "my-d1", "uuid": "uuid-123"},
		}})
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)

	orig := http.DefaultClient.Transport
	http.DefaultClient.Transport = rewriteTransport{target: target}
	defer func() { http.DefaultClient.Transport = orig }()

	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "a")
	t.Setenv("D1_BINDING", "DB")
	t.Setenv("D1_NAME", "my-d1")
	t.Setenv("PROJECT_NAME", "test")

	var out strings.Builder
	if err := goflare.RunD1ID("", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "uuid-123\n" {
		t.Fatalf("expected only the ID on stdout, got %q", out.String())
	}

	// Not found: error and nothing on stdout.
	t.Setenv("D1_NAME", "missing")
	out.Reset()
	if err := goflare.RunD1ID("", &out); err == nil {
		t.Fatalf("expected error for unknown database")
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty stdout on error, got %q", out.String())
	}
}

func TestActionYml_A1(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", goflare.ActionFilePath))
	if err != nil {
		t.Fatal(err)
	}
	action := string(raw)
	for _, want := range []string{"  d1-name:", "  r2-bucket:", "  vars:", "D1_NAME:", "R2_BUCKET:", "VARS:"} {
		if !strings.Contains(action, want) {
			t.Errorf("action.yml lacks %q", want)
		}
	}
	// The ID-by-hand inputs are gone; the database ID only reaches
	// pre-deploy through `goflare d1-id`.
	removed := regexp.MustCompile(`(?i)(d1[-_]database[-_]name|r2[-_]bucket[-_](id|name)|secrets\.d1[-_]database[-_]id|d1-database-id)`)
	if m := removed.FindString(action); m != "" {
		t.Errorf("action.yml still carries %q", m)
	}
}
