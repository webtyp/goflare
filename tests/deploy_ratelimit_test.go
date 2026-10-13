//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestDeploy_RateLimit_NoDomain(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/rulesets") {
			t.Errorf("expected no requests to /rulesets without domain, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
}

func TestDeploy_RateLimit_CreateNew(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)

	var putBody map[string]any

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/zones"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":[{"id":"zone-123","name":"velty.cl"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/domains"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"success":false,"errors":[{"code":10007,"message":"not found"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &putBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		Domain: "api.velty.cl",
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	rules, ok := putBody["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("expected 1 rule in PUT body, got %v", putBody)
	}

	rule := rules[0].(map[string]any)
	if rule["ref"] != "goflare-my-worker" {
		t.Errorf("expected ref goflare-my-worker, got %v", rule["ref"])
	}
	if rule["expression"] != "(http.host eq \"api.velty.cl\")" {
		t.Errorf("expected expression (http.host eq \"api.velty.cl\"), got %v", rule["expression"])
	}

	rl, ok := rule["ratelimit"].(map[string]any)
	if !ok {
		t.Fatalf("missing ratelimit field")
	}
	if rl["requests_per_period"].(float64) != 100 {
		t.Errorf("expected 100 requests per period, got %v", rl["requests_per_period"])
	}
}

func TestDeploy_RateLimit_UpdateExisting(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)

	var putBody map[string]any

	foreignRuleJSON := `{"ref":"manual","id":"r1","description":"x","expression":"true","action":"block","ratelimit":{"characteristics":["ip.src"],"period":10,"requests_per_period":50},"extra_field":7}`
	oldGoflareRuleJSON := `{"ref":"goflare-my-worker","description":"goflare: old rule","expression":"false","action":"block"}`

	existingRuleset := `{"success":true,"result":{"rules":[` + foreignRuleJSON + `,` + oldGoflareRuleJSON + `]}}`

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/zones"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":[{"id":"zone-123","name":"velty.cl"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/domains"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(existingRuleset))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &putBody)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		Domain: "api.velty.cl",
		RateLimitPer10s: 40,
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	rules, ok := putBody["rules"].([]any)
	if !ok || len(rules) != 2 {
		t.Fatalf("expected 2 rules in PUT body, got %v", putBody)
	}

	rule0 := rules[0].(map[string]any)
	if rule0["ref"] != "manual" || rule0["extra_field"].(float64) != 7 {
		t.Errorf("expected foreign rule to be preserved exactly, got %v", rule0)
	}

	rule1 := rules[1].(map[string]any)
	if rule1["ref"] != "goflare-my-worker" {
		t.Errorf("expected ref goflare-my-worker, got %v", rule1["ref"])
	}

	rl, ok := rule1["ratelimit"].(map[string]any)
	if !ok {
		t.Fatalf("missing ratelimit field")
	}
	if rl["requests_per_period"].(float64) != 40 {
		t.Errorf("expected 40 requests per period, got %v", rl["requests_per_period"])
	}
}

func TestDeploy_RateLimit_GETError(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/zones"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":[{"id":"zone-123","name":"velty.cl"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/domains"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"internal error"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		Domain: "api.velty.cl",
	})
	g.BaseURL = server.URL

	err := g.Deploy()
	if err == nil {
		t.Fatal("expected Deploy to fail")
	}
	if !strings.Contains(err.Error(), "rate limit:") {
		t.Errorf("expected error to mention rate limit, got %v", err)
	}
}

func TestDeploy_RateLimit_PUTPermissionError(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t)

	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/assets-upload-session"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"jwt":"session-jwt","buckets":[]}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/zones"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":[{"id":"zone-123","name":"velty.cl"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/domains"):
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"success":false,"errors":[{"code":10007,"message":"not found"}]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/rulesets/phases/http_ratelimit/entrypoint"):
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"success":false,"errors":[{"code":10000,"message":"forbidden"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		Domain: "api.velty.cl",
	})
	g.BaseURL = server.URL

	err := g.Deploy()
	if err == nil {
		t.Fatal("expected Deploy to fail")
	}
	if err.Error() != goflare.ErrMsgRateLimitPermission {
		t.Errorf("expected permission error %q, got %q", goflare.ErrMsgRateLimitPermission, err.Error())
	}
}
