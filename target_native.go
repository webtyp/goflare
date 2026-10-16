//go:build !wasm

package goflare

import (
	"fmt"

	"webtyp.com/deploy"
	"webtyp.com/keyring"
)

// TargetConfig configures a Target.
type TargetConfig struct {
	Personal *keyring.Keyring // the person's keyring (not the project's)
	RootDir  string           // the project root
	BaseURL  string           // Cloudflare API base; "" = https://api.cloudflare.com/client/v4
}

// Target implements deploy.Target for a Cloudflare Worker with static assets.
type Target struct {
	cfg TargetConfig
}

var _ deploy.Target = (*Target)(nil)

// NewTarget creates a new Target.
func NewTarget(cfg TargetConfig) (*Target, error) {
	if cfg.Personal == nil {
		return nil, fmt.Errorf("ErrNoPersonalKeyring")
	}
	return &Target{cfg: cfg}, nil
}
