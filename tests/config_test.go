//go:build !wasm

package goflare_test

import (
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
