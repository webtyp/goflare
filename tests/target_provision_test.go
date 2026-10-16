//go:build !wasm

package goflare_test

import (
	"encoding/json"
	"io"
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
	events  *[]string
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
	if m.events != nil {
		*m.events = append(*m.events, "CI_PUT "+key)
	}
	return nil
}

func newFakeCFForProvision(t *testing.T, existing bool) (*httptest.Server, *[]string, *[]byte) {
	var requestedPaths []string
	var createdTokenBody []byte
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
			var res []map[string]any
			if existing {
				res = append(res, map[string]any{"name": r.URL.Query().Get("name")})
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  res,
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
			if !existing {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]any{"name": "bkt1"},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/r2/buckets") && r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/tokens/permission_groups") {
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
				createdTokenBody, _ = io.ReadAll(r.Body)
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
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  map[string]any{"id": "old_tk_id"},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	return srv, &requestedPaths, &createdTokenBody
}

func TestProvision_P1_P2(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{
		Name:      "test",
		Databases: []deploy.Database{{Name: "db1", Binding: "DB"}},
		Buckets:   []deploy.Bucket{{Name: "bkt1", Binding: "ASSETS"}},
	}

	srv, paths, tokenBody := newFakeCFForProvision(t, false)
	defer srv.Close()

	ci := newMemoryDestination()
	ci.events = paths

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	if err := target.Provision(p, ci); err != nil {
		t.Fatalf("unexpected provision error: %v", err)
	}

	foundDbCreate := false
	foundBktCreate := false
	foundTkCreate := false
	putIdx := -1
	delIdx := -1
	for i, path := range *paths {
		if path == "POST /accounts/a/d1/database" {
			foundDbCreate = true
		}
		if path == "POST /accounts/a/r2/buckets" {
			foundBktCreate = true
		}
		if path == "POST /accounts/a/tokens" {
			foundTkCreate = true
		}
		if path == "CI_PUT CLOUDFLARE_API_TOKEN" {
			putIdx = i
		}
		if path == "DELETE /accounts/a/tokens/old_tk_id" {
			delIdx = i
		}
	}

	if !foundDbCreate || !foundBktCreate {
		t.Fatalf("expected both D1 and R2 creation calls, got paths: %v", *paths)
	}
	if !foundTkCreate || putIdx == -1 || delIdx == -1 || delIdx < putIdx {
		t.Fatalf("expected token create -> CI_PUT -> DELETE order, got paths: %v", *paths)
	}

	// Assert permission groups in request body (ids "1", "2", "3")
	bodyStr := string(*tokenBody)
	for _, id := range []string{`"1"`, `"2"`, `"3"`} {
		if !strings.Contains(bodyStr, id) {
			t.Fatalf("expected permission group %s in token request body %s", id, bodyStr)
		}
	}

	// P1 idempotence: when D1 and R2 exist, Provision does not POST them
	srvExist, pathsExist, _ := newFakeCFForProvision(t, true)
	defer srvExist.Close()
	targetExist, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srvExist.URL})
	if err := targetExist.Provision(p, ci); err != nil {
		t.Fatalf("unexpected provision error on existing resources: %v", err)
	}
	for _, path := range *pathsExist {
		if path == "POST /accounts/a/d1/database" || path == "POST /accounts/a/r2/buckets" {
			t.Fatalf("expected no resource creation when resources exist, got %s", path)
		}
	}
}

func TestProvision_P3(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{
		Name: "test",
	}
	ci := newMemoryDestination()

	// Server that returns empty permission groups
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/accounts/a/tokens/permission_groups") {
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

	srv, paths, _ := newFakeCFForProvision(t, true)
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
