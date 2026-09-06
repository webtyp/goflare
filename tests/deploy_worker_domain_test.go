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

func TestDeploy_ConfiguresWorkerDomainWithLongestMatchingZone(t *testing.T) {
	withCloudflareToken(t)
	env := newTestEnv(t) // assets-only deploy is enough to exercise the domain step

	var domainPUTBody map[string]any
	domainPUTCalled := false

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
			w.Write([]byte(`{"success":true,"result":[
				{"id":"zone-short","name":"velty.cl"},
				{"id":"zone-long","name":"misitio.velty.cl"}
			]}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/domains"):
			domainPUTCalled = true
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &domainPUTBody)
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
		Domain: "misitio.velty.cl",
	})
	g.BaseURL = server.URL

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	if !domainPUTCalled {
		t.Fatal("expected PUT /accounts/{id}/workers/domains to be called")
	}
	if domainPUTBody["zone_id"] != "zone-long" {
		t.Errorf("expected the longest-matching zone (zone-long), got: %v", domainPUTBody["zone_id"])
	}
	if domainPUTBody["service"] != "my-worker" {
		t.Errorf("expected service = WorkerName (my-worker), got: %v", domainPUTBody["service"])
	}
	if domainPUTBody["hostname"] != "misitio.velty.cl" {
		t.Errorf("expected hostname = misitio.velty.cl, got: %v", domainPUTBody["hostname"])
	}
}

func TestDeploy_DomainFailureIsLoggedNotFatal(t *testing.T) {
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
			// No zone matches the configured domain.
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":[{"id":"zone-other","name":"otrositio.cl"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer server.Close()

	g := goflare.New(&goflare.Config{
		AccountID: "acc-123", WorkerName: "my-worker",
		PublicDir: env.PublicDir, OutputDir: env.OutputDir,
		Domain: "misitio.velty.cl",
	})
	g.BaseURL = server.URL

	var loggedMessages []string
	g.SetLog(func(msg ...any) {
		var parts []string
		for _, m := range msg {
			if s, ok := m.(string); ok {
				parts = append(parts, s)
			} else if e, ok := m.(error); ok {
				parts = append(parts, e.Error())
			}
		}
		loggedMessages = append(loggedMessages, strings.Join(parts, " "))
	})

	if err := g.Deploy(); err != nil {
		t.Fatalf("Deploy should not fail when domain configuration fails, got: %v", err)
	}

	found := false
	for _, m := range loggedMessages {
		if strings.Contains(m, "domain") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a logged warning mentioning the domain failure, got: %v", loggedMessages)
	}
}
