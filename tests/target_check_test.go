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

type fakeCFState struct {
	verifyStatus  string
	verifyExpires string
	accountCount  int
	dbFound       bool
	bucketFound   bool
	zoneFound     bool
	tokenFound    bool
	tokenActive   bool
	tokenPolicies []map[string]any
}

func newFakeCF(state fakeCFState) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/user/tokens/verify") {
			if r.Header.Get("Authorization") == "Bearer bad" {
				json.NewEncoder(w).Encode(map[string]any{
					"success": false,
					"result": map[string]any{
						"status": "disabled",
					},
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result": map[string]any{
					"status":     state.verifyStatus,
					"expires_on": state.verifyExpires,
				},
			})
			return
		}

		if r.URL.Path == "/accounts" {
			var res []map[string]any
			for i := 0; i < state.accountCount; i++ {
				res = append(res, map[string]any{"id": "a"})
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  res,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/d1/database") {
			var res []map[string]any
			if state.dbFound {
				res = append(res, map[string]any{"name": r.URL.Query().Get("name")})
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  res,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/r2/buckets/") {
			if !state.bucketFound {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/zones") {
			var res []map[string]any
			if state.zoneFound {
				res = append(res, map[string]any{"id": "z"})
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  res,
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
					{"id": "4", "name": "Zone Read"},
					{"id": "5", "name": "Workers Routes Write"},
					{"id": "6", "name": "Zone WAF Write"},
				},
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/accounts/a/tokens") {
			var res []map[string]any
			if state.tokenFound {
				status := "active"
				if !state.tokenActive {
					status = "disabled"
				}
				res = append(res, map[string]any{
					"name":     "webtyp-deploy-test",
					"status":   status,
					"policies": state.tokenPolicies,
				})
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"result":  res,
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestCheck_K1_K3(t *testing.T) {
	p := deploy.Plan{Name: "test"}
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	var ci deploy.Destination

	srv := newFakeCF(fakeCFState{
		verifyStatus: "active",
		accountCount: 1,
	})
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{
		Personal: kr,
		RootDir:  ".",
		BaseURL:  srv.URL,
	})

	// K1: missing token
	missing, _ := target.Check(p, ci)
	if len(missing) != 1 || missing[0].Fix != deploy.FixInput {
		t.Fatalf("expected 1 missing input for K1, got %v", missing)
	}

	// Accept invalid token
	err := missing[0].Accept("bad")
	if err == nil {
		t.Fatalf("expected error on accepting bad token")
	}

	// Accept good token
	kr.Set(goflare.KeyBootstrapToken, "good")

	// K2: not active
	srvK2 := newFakeCF(fakeCFState{
		verifyStatus: "disabled",
	})
	targetK2, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srvK2.URL})
	missing, _ = targetK2.Check(p, ci)
	if len(missing) != 1 || missing[0].Fix != deploy.FixInput {
		t.Fatalf("expected K2 missing")
	}
	srvK2.Close()

	// K3: multiple accounts
	srvK3 := newFakeCF(fakeCFState{
		verifyStatus: "active",
		accountCount: 2,
	})
	targetK3, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srvK3.URL})
	missing, _ = targetK3.Check(p, ci)
	if len(missing) != 1 || missing[0].Fix != deploy.FixManual {
		t.Fatalf("expected K3 missing")
	}
	srvK3.Close()
}

func TestCheck_K4_K9(t *testing.T) {
	mp := krtests.NewMemProvider()
	kr, _ := keyring.NewKeyring("goflare-test", mp)
	kr.Set(goflare.KeyBootstrapToken, "good")
	p := deploy.Plan{
		Name:      "test",
		Domain:    "example.com",
		Databases: []deploy.Database{{Name: "db1"}},
		Buckets:   []deploy.Bucket{{Name: "bkt1"}},
	}

	state := fakeCFState{
		verifyStatus:  "active",
		accountCount:  1,
		dbFound:       true,
		bucketFound:   true,
		zoneFound:     true,
		tokenFound:    true,
		tokenActive:   true,
		tokenPolicies: []map[string]any{
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "1"}}, "resources": map[string]any{"com.cloudflare.api.account.*": "*"}},
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "2"}}, "resources": map[string]any{"com.cloudflare.api.account.*": "*"}},
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "3"}}, "resources": map[string]any{"com.cloudflare.api.account.*": "*"}},
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "4"}}, "resources": map[string]any{"com.cloudflare.api.account.zone.*": "*"}},
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "5"}}, "resources": map[string]any{"com.cloudflare.api.account.zone.*": "*"}},
			{"effect": "allow", "permission_groups": []map[string]any{{"id": "6"}}, "resources": map[string]any{"com.cloudflare.api.account.zone.*": "*"}},
		},
	}

	srv := newFakeCF(state)
	defer srv.Close()

	target, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srv.URL})
	ci := newMemoryDestination()

	// Missing K9 (CI secrets)
	missing, err := target.Check(p, ci)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(missing) != 1 { // Should just be K9 now that K8 policies match
		t.Fatalf("expected 1 missing (K9), got %v", missing)
	}

	// Supply CI secrets to clear K9
	ci.Put("CLOUDFLARE_API_TOKEN", "ci-tok")
	ci.Put("CLOUDFLARE_ACCOUNT_ID", "a")
	missing, err = target.Check(p, ci)
	if len(missing) != 0 {
		t.Fatalf("expected 0 missing, got %v", missing)
	}

	// K4: shape error
	pBad := deploy.Plan{
		Name:      "test",
		Databases: []deploy.Database{{Name: "db1"}, {Name: "db2"}},
	}
	missing, err = target.Check(pBad, ci)
	if err != goflare.ErrOneDatabaseOneBucket {
		t.Fatalf("expected ErrOneDatabaseOneBucket, got %v (missing: %v)", err, missing)
	}

	// K5, K6: missing resources
	stateMissing := fakeCFState{verifyStatus: "active", accountCount: 1}
	srvMissing := newFakeCF(stateMissing)
	targetMissing, _ := goflare.NewTarget(goflare.TargetConfig{Personal: kr, RootDir: ".", BaseURL: srvMissing.URL})
	missing, _ = targetMissing.Check(p, ci)
	if len(missing) < 2 {
		t.Fatalf("expected at least K5 K6 missing")
	}
	srvMissing.Close()
}
