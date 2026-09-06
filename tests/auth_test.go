//go:build !wasm

package goflare_test

import (
	"net/http"
	"os"
	"testing"

	"webtyp.com/goflare"
)

func TestAuth_Validates(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user/tokens/verify" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true,"result":{"id":"test","status":"active"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	os.Setenv("CLOUDFLARE_API_TOKEN", "new-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err := g.Auth()
	if err != nil {
		t.Fatalf("Auth failed: %v", err)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"errors":[{"code":1000,"message":"Invalid token"}]}`))
	})
	defer server.Close()

	os.Setenv("CLOUDFLARE_API_TOKEN", "invalid-token")
	defer os.Unsetenv("CLOUDFLARE_API_TOKEN")

	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)
	g.BaseURL = server.URL

	err := g.Auth()
	if err == nil {
		t.Fatal("Expected error for invalid token")
	}
}

func TestAuth_NoToken(t *testing.T) {
	os.Unsetenv("CLOUDFLARE_API_TOKEN")

	cfg := &goflare.Config{ProjectName: "test-project"}
	g := goflare.New(cfg)

	err := g.Auth()
	if err == nil {
		t.Fatal("Expected error when token missing")
	}
}
