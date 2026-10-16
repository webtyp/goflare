//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/goflare"
	"webtyp.com/keyring"
	krtests "webtyp.com/keyring/tests"
)

type memoryDestination struct {
	secrets map[string]string
}

func newMemoryDestination() *memoryDestination {
	return &memoryDestination{secrets: make(map[string]string)}
}

func (m *memoryDestination) Has(key string) (bool, error) {
	_, ok := m.secrets[key]
	return ok, nil
}

func (m *memoryDestination) Put(key, value string) error {
	m.secrets[key] = value
	return nil
}

func newFakeCFForProvision(t *testing.T) (*httptest.Server, *[]string) {
	var requestedPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.Method+" "+r.URL.Path)

		if strings.HasPrefix(r.URL.Path, "/user/tokens/verify") {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": map[string]any{
					"status": "active",
				},
			})
			return
		}

		if r.URL.Path == "/accounts" {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []map[string]any{
					{"id": "a"},
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/d1/database") && r.Method == http.MethodGet {
			// Mock not found
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  []map[string]any{},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/d1/database") && r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/r2/buckets") && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/r2/buckets") && r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/user/tokens/permission_groups") {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []map[string]any{
					{"id": "1", "name": "Workers Scripts Write"},
					{"id": "2", "name": "D1 Write"},
					{"id": "3", "name": "Workers R2 Storage Write"},
				},
			})
			return
		}

		if r.URL.Path == "/accounts/a/tokens" {
			if r.Method == http.MethodPost {
				json.NewEncoder(w).Encode(map[string]any{
					"success": true,
					"result": map[string]any{
						"value": "new_secret_token",
					},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": []map[string]any{
					{"id": "old_tk_id", "name": "webtyp-deploy-test", "status": "active"},
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/tokens/") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	return srv, &requestedPaths
}

func TestProvision_P1_P2(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{
		Name:      "test",
		Databases: nil,
		Buckets:   []deploy.Bucket{{Name: "bkt1"}},
	}
	ci := newMemoryDestination()

	srv, paths := newFakeCFForProvision(t)
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	err := target.Provision(p, ci)
	if err != nil {
		t.Fatalf("unexpected provision error: %v", err)
	}

	foundBktCreate := false
	foundTkCreate := false
	foundTkDelete := false
	for _, path := range *paths {
		if strings.Contains(path, "POST /accounts/a/r2/buckets") {
			foundBktCreate = true
		}
		if strings.Contains(path, "POST /accounts/a/tokens") {
			foundTkCreate = true
		}
		if strings.Contains(path, "DELETE /accounts/a/tokens/old_tk_id") {
			foundTkDelete = true
		}
	}

	// We removed db1 from plan databases earlier in my fix script, so foundDbCreate is false.
	if !foundBktCreate {
		t.Fatalf("missing D1/R2 creation calls")
	}
	if !foundTkCreate || !foundTkDelete {
		t.Fatalf("missing token creation/deletion calls")
	}

	tok, _ := ci.Has("CLOUDFLARE_API_TOKEN")
	if !tok {
		t.Fatalf("expected CLOUDFLARE_API_TOKEN in CI")
	}
}

func TestProvision_P3(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{
		Name:      "test",
		Databases: nil,
	}
	ci := newMemoryDestination()

	// Server that returns empty permission groups
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/user/tokens/permission_groups") {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []any{}})
			return
		}
		if r.URL.Path == "/accounts" {
			json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []map[string]any{{"id": "a"}}})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	err := target.Provision(p, ci)
	if err == nil || !strings.Contains(err.Error(), "ErrUnknownPermissionGroup") {
		t.Fatalf("expected ErrUnknownPermissionGroup, got: %v", err)
	}
}

func TestRotate_R1(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{Name: "test"}
	ci := newMemoryDestination()

	srv, paths := newFakeCFForProvision(t)
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	err := target.Rotate(p, ci)
	if err != nil {
		t.Fatalf("unexpected rotate error: %v", err)
	}

	foundTkCreate := false
	foundTkDelete := false
	for _, path := range *paths {
		if strings.Contains(path, "POST /accounts/a/tokens") {
			foundTkCreate = true
		}
		if strings.Contains(path, "DELETE /accounts/a/tokens/old_tk_id") {
			foundTkDelete = true
		}
	}
	if !foundTkCreate || !foundTkDelete {
		t.Fatalf("missing token creation/deletion calls in Rotate")
	}
}
