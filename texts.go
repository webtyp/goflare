//go:build !wasm

package goflare

import "webtyp.com/lang"

const (
	StepCreateBootstrapToken lang.Text = "open https://dash.cloudflare.com/profile/api-tokens → Create Token → template 'Create additional tokens' → limit it to one account → TTL 90 days → copy it here"
	ItemBootstrapTokenExpiring lang.Text = "Cloudflare bootstrap token expires soon"
	ItemBootstrapTokenInvalid lang.Text = "Cloudflare bootstrap token is not valid"
	StepLimitAccount lang.Text = "create the bootstrap token limited to one account (D40)"
	StepAddDomain lang.Text = "add the domain to this Cloudflare account"
)
