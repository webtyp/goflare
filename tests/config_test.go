//go:build !wasm

package goflare_test

import (
	"os"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestConfig_ValidateBuild(t *testing.T) {
	tests := []struct {
		name    string
		cfg     goflare.Config
		wantErr bool
	}{
		{
			name:    "empty config",
			cfg:     goflare.Config{},
			wantErr: true,
		},
		{
			name: "only entry",
			cfg: goflare.Config{
				Entry: "edge",
			},
			wantErr: false,
		},
		{
			name: "only public dir",
			cfg: goflare.Config{
				PublicDir: "web/public",
			},
			wantErr: false,
		},
		{
			name: "both entry and public dir",
			cfg: goflare.Config{
				Entry:     "edge",
				PublicDir: "web/public",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateBuild()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBuild() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ValidateDeploy(t *testing.T) {
	tests := []struct {
		name    string
		cfg     goflare.Config
		wantErr bool
	}{
		{
			name:    "empty config",
			cfg:     goflare.Config{},
			wantErr: true,
		},
		{
			name: "missing account ID",
			cfg: goflare.Config{
				ProjectName: "my-project",
				Entry:       "edge",
			},
			wantErr: true,
		},
		{
			name: "missing project name",
			cfg: goflare.Config{
				AccountID: "my-account",
				Entry:     "edge",
			},
			wantErr: true,
		},
		{
			name: "missing entry and public dir",
			cfg: goflare.Config{
				ProjectName: "my-project",
				AccountID:   "my-account",
			},
			wantErr: true,
		},
		{
			name: "valid worker deploy config",
			cfg: goflare.Config{
				ProjectName: "my-project",
				AccountID:   "my-account",
				Entry:       "edge",
			},
			wantErr: false,
		},
		{
			name: "valid pages deploy config",
			cfg: goflare.Config{
				ProjectName: "my-project",
				AccountID:   "my-account",
				PublicDir:   "web/public",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateDeploy()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeploy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfigFromEnv_RateLimit(t *testing.T) {
	// G7: default value 100
	cfg, err := goflare.LoadConfigFromEnv("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RateLimitPer10s != 100 {
		t.Errorf("expected default 100, got %d", cfg.RateLimitPer10s)
	}

	// Helper to set env, run, and check
	checkEnv := func(val string, expectedErr bool, expectedVal int) {
		os.Setenv(goflare.EnvKeyRateLimitPer10s, val)
		defer os.Unsetenv(goflare.EnvKeyRateLimitPer10s)
		c, err := goflare.LoadConfigFromEnv("")
		if expectedErr {
			if err == nil {
				t.Errorf("expected error for val %q", val)
			} else if !strings.Contains(err.Error(), "must be an integer >= 1") {
				t.Errorf("expected invalid rate limit error for val %q, got: %v", val, err)
			}
		} else {
			if err != nil {
				t.Errorf("unexpected error for val %q: %v", val, err)
			}
			if c.RateLimitPer10s != expectedVal {
				t.Errorf("expected %d for val %q, got %d", expectedVal, val, c.RateLimitPer10s)
			}
		}
	}

	// G8: invalid values
	checkEnv("0", true, 0)
	checkEnv("-5", true, 0)
	checkEnv("abc", true, 0)

	// Valid value
	checkEnv("40", false, 40)
}
