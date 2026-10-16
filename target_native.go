//go:build !wasm

package goflare

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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
		return nil, ErrNoPersonalKeyring
	}
	if cfg.RootDir == "" {
		return nil, ErrNoRootDir
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = cfAPIBase
	}
	return &Target{cfg: cfg}, nil
}

// ID returns the target id a project declares in config/deploy.go.
func (t *Target) ID() deploy.TargetID {
	return Worker
}

// accountTokenName is the name of the per-project account token.
func accountTokenName(p deploy.Plan) string {
	return "webtyp-deploy-" + p.Name
}

// bootstrapToken reads the bootstrap token from the person's keyring. An
// absent entry is "" with no error; any other keyring failure is returned.
func (t *Target) bootstrapToken() (string, error) {
	token, err := t.cfg.Personal.Get(KeyBootstrapToken)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("read bootstrap token: %w", err)
	}
	return token, nil
}

// client builds a Cloudflare client authenticated with token.
func (t *Target) client(token string) *CfClient {
	return &CfClient{Token: token, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}
}

// session returns a client authenticated with the person's bootstrap token
// and the one account it is limited to.
func (t *Target) session() (*CfClient, string, error) {
	token, err := t.bootstrapToken()
	if err != nil {
		return nil, "", err
	}
	if token == "" {
		return nil, "", ErrNoBootstrapToken
	}
	client := t.client(token)
	ids, err := listAccounts(client)
	if err != nil {
		return nil, "", err
	}
	if len(ids) != 1 {
		return nil, "", fmt.Errorf("%w: %d", ErrNotOneAccount, len(ids))
	}
	return client, ids[0], nil
}

// isNotFound reports whether err is a Cloudflare API 404.
func isNotFound(err error) bool {
	var ce *cfError
	return errors.As(err, &ce) && ce.Status == http.StatusNotFound
}

func listAccounts(client *CfClient) ([]string, error) {
	res, err := client.get("/accounts")
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	var accounts []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res, &accounts); err != nil {
		return nil, fmt.Errorf("parse accounts: %w", err)
	}
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	return ids, nil
}

// d1Exists reports whether a D1 database with exactly that name exists.
func d1Exists(client *CfClient, accountID, name string) (bool, error) {
	res, err := client.get(fmt.Sprintf("/accounts/%s/d1/database?name=%s", accountID, url.QueryEscape(name)))
	if err != nil {
		return false, fmt.Errorf("look up D1 database %s: %w", name, err)
	}
	var dbs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res, &dbs); err != nil {
		return false, fmt.Errorf("parse D1 lookup: %w", err)
	}
	for _, db := range dbs {
		if db.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// r2Exists reports whether the R2 bucket exists (404 → false).
func r2Exists(client *CfClient, accountID, name string) (bool, error) {
	_, err := client.get(fmt.Sprintf("/accounts/%s/r2/buckets/%s", accountID, url.PathEscape(name)))
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("look up R2 bucket %s: %w", name, err)
}

// findZone returns the ID of the account zone that holds domain, trying the
// domain itself and then each parent ("a.b.example.co.uk" → … → "co.uk"), so
// multi-label public suffixes need no suffix list. "" when none matches.
func findZone(client *CfClient, accountID, domain string) (string, error) {
	labels := strings.Split(strings.TrimSuffix(domain, "."), ".")
	for i := 0; i+2 <= len(labels); i++ {
		name := strings.Join(labels[i:], ".")
		res, err := client.get(fmt.Sprintf("/zones?name=%s&account.id=%s", url.QueryEscape(name), url.QueryEscape(accountID)))
		if err != nil {
			return "", fmt.Errorf("look up zone %s: %w", name, err)
		}
		var zones []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(res, &zones); err != nil {
			return "", fmt.Errorf("parse zones: %w", err)
		}
		if len(zones) > 0 {
			return zones[0].ID, nil
		}
	}
	return "", nil
}

// accountToken is one entry of GET /accounts/{a}/tokens.
type accountToken struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Status   string           `json:"status"`
	Policies []map[string]any `json:"policies"`
}

// tokensNamed lists the account tokens called name.
func tokensNamed(client *CfClient, accountID, name string) ([]accountToken, error) {
	res, err := client.get(fmt.Sprintf("/accounts/%s/tokens", accountID))
	if err != nil {
		return nil, fmt.Errorf("list account tokens: %w", err)
	}
	var all []accountToken
	if err := json.Unmarshal(res, &all); err != nil {
		return nil, fmt.Errorf("parse account tokens: %w", err)
	}
	var named []accountToken
	for _, tk := range all {
		if tk.Name == name {
			named = append(named, tk)
		}
	}
	return named, nil
}

// replaceAccountToken creates a new account token for p, puts its value in ci
// and, only after that succeeded, deletes the previous tokens of that name.
// The value is never stored locally nor logged.
func replaceAccountToken(client *CfClient, accountID string, p deploy.Plan, policies []map[string]any, old []accountToken, ci deploy.Destination) error {
	body, err := json.Marshal(map[string]any{
		"name":     accountTokenName(p),
		"policies": policies,
	})
	if err != nil {
		return err
	}
	res, err := client.post(fmt.Sprintf("/accounts/%s/tokens", accountID), body)
	if err != nil {
		return fmt.Errorf("create account token: %w", err)
	}
	var created struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(res, &created); err != nil || created.Value == "" {
		return fmt.Errorf("create account token: response carries no token value")
	}
	if err := ci.Put("CLOUDFLARE_API_TOKEN", created.Value); err != nil {
		return fmt.Errorf("put CLOUDFLARE_API_TOKEN to CI: %w", err)
	}
	for _, tk := range old {
		if _, err := client.delete(fmt.Sprintf("/accounts/%s/tokens/%s", accountID, tk.ID)); err != nil {
			return fmt.Errorf("delete previous account token %s: %w", tk.ID, err)
		}
	}
	return nil
}
