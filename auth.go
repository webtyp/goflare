//go:build !wasm

package goflare

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

var errNoToken = fmt.Errorf("CLOUDFLARE_API_TOKEN is not defined.\n" +
	"Deployment is executed in CI. The deploy wizard (webtyp app, target goflare.Worker)\n" +
	"creates the project's Cloudflare token and stores it as the CLOUDFLARE_API_TOKEN\n" +
	"secret of the repository; run it to provision the project.")

func (g *Goflare) token() (string, error) {
	t := os.Getenv("CLOUDFLARE_API_TOKEN")
	if t == "" {
		return "", errNoToken
	}
	return t, nil
}

// Auth implements token validation.
func (g *Goflare) Auth() error {
	t, err := g.token()
	if err != nil {
		return err
	}
	return g.validateToken(t)
}

// accountTokenPrefix marks a Cloudflare account-owned token, the kind
// Provision creates. Those are verified on the account endpoint; user
// tokens (any other value) on /user/tokens/verify.
const accountTokenPrefix = "cfat_"

func (g *Goflare) verifyPath(token string) (string, error) {
	if !strings.HasPrefix(token, accountTokenPrefix) {
		return "/user/tokens/verify", nil
	}
	if g.Config == nil || g.Config.AccountID == "" {
		return "", fmt.Errorf("an account token (%s…) needs %s to be verified", accountTokenPrefix, EnvKeyAccountID)
	}
	return fmt.Sprintf("/accounts/%s/tokens/verify", g.Config.AccountID), nil
}

func (g *Goflare) validateToken(token string) error {
	client := &CfClient{
		Token:      token,
		HttpClient: http.DefaultClient,
		BaseURL:    g.BaseURL,
	}

	path, err := g.verifyPath(token)
	if err != nil {
		return err
	}
	if _, err := client.get(path); err != nil {
		return fmt.Errorf(
			"%w\n\nToken validation failed. Check:\n"+
				"  - Token is not expired\n"+
				"  - Token is for the correct Cloudflare account\n\n"+
				"The deploy wizard (webtyp app) creates and rotates this token; run it again to replace it.",
			err,
		)
	}
	return nil
}
