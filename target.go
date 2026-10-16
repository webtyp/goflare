package goflare

import (
	"webtyp.com/deploy"
)

// Worker is the target id a project declares in config/deploy.go.
const Worker deploy.TargetID = "webtyp.com/goflare.Worker"

// KeyBootstrapToken is the entry, in the person's own keyring, holding their
// Cloudflare bootstrap token (template "Create additional tokens").
const KeyBootstrapToken = "cloudflare_bootstrap_token"
