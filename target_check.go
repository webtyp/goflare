//go:build !wasm

package goflare

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"webtyp.com/deploy"
	"webtyp.com/lang"
)

// Permission group names goflare needs, resolved to IDs at run time from
// GET /accounts/{a}/tokens/permission_groups. IDs are never hard-coded.
const (
	permWorkersScriptsWrite = "Workers Scripts Write"    // workers/scripts/*, workers/subdomain, assets-upload-session, secrets
	permD1Write             = "D1 Write"                 // d1/database/* (resolve, cmd/migrate queries)
	permR2StorageWrite      = "Workers R2 Storage Write" // R2 binding
	permZoneRead            = "Zone Read"                // zones
	permWorkersRoutesWrite  = "Workers Routes Write"     // workers/domains
	permZoneWAFWrite        = "Zone WAF Write"           // zones/*/rulesets/phases/http_ratelimit/entrypoint
)

// bootstrapExpiryMargin is how close to expiry the bootstrap token is
// reported as expiring (K2).
const bootstrapExpiryMargin = 7 * 24 * time.Hour

const (
	resourceAccountPrefix = "com.cloudflare.api.account."
	resourceZonePrefix    = "com.cloudflare.api.account.zone."
)

// permissionNeed is one row of requiredPermissions: a group and whether it
// is scoped to the zone of Domain (true) or to the account (false).
type permissionNeed struct {
	name string
	zone bool
}

func neededPermissions(p deploy.Plan) []permissionNeed {
	needs := []permissionNeed{{permWorkersScriptsWrite, false}}
	if len(p.Databases) > 0 {
		needs = append(needs, permissionNeed{permD1Write, false})
	}
	if len(p.Buckets) > 0 {
		needs = append(needs, permissionNeed{permR2StorageWrite, false})
	}
	if p.Domain != "" {
		needs = append(needs,
			permissionNeed{permZoneRead, true},
			permissionNeed{permWorkersRoutesWrite, true},
			permissionNeed{permZoneWAFWrite, true},
		)
	}
	return needs
}

// requiredPermissions returns the token policies p needs: account groups
// scoped to accountID, zone groups scoped to zoneID (the zone of Domain).
func requiredPermissions(p deploy.Plan, client *CfClient, accountID, zoneID string) ([]map[string]any, error) {
	res, err := client.get(fmt.Sprintf("/accounts/%s/tokens/permission_groups", accountID))
	if err != nil {
		return nil, fmt.Errorf("fetch permission groups: %w", err)
	}
	var groups []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res, &groups); err != nil {
		return nil, fmt.Errorf("parse permission groups: %w", err)
	}

	var policies []map[string]any
	for _, need := range neededPermissions(p) {
		id := ""
		for _, g := range groups {
			if g.Name == need.name {
				id = g.ID
				break
			}
		}
		if id == "" {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPermissionGroup, need.name)
		}
		resource := resourceAccountPrefix + accountID
		if need.zone {
			resource = resourceZonePrefix + zoneID
		}
		policies = append(policies, map[string]any{
			"effect":            "allow",
			"permission_groups": []map[string]any{{"id": id}},
			"resources":         map[string]any{resource: "*"},
		})
	}
	return policies, nil
}

// policyGrants flattens policies into sorted "groupID resource" pairs, the
// form in which two policy lists are compared.
func policyGrants(policies []map[string]any) []string {
	raw, _ := json.Marshal(policies)
	var parsed []struct {
		Groups []struct {
			ID string `json:"id"`
		} `json:"permission_groups"`
		Resources map[string]any `json:"resources"`
	}
	json.Unmarshal(raw, &parsed)
	var grants []string
	for _, pol := range parsed {
		for _, g := range pol.Groups {
			for r := range pol.Resources {
				grants = append(grants, g.ID+" "+r)
			}
		}
	}
	sort.Strings(grants)
	return grants
}

func policiesMatch(have, want []map[string]any) bool {
	h, w := policyGrants(have), policyGrants(want)
	if len(h) != len(w) {
		return false
	}
	for i := range h {
		if h[i] != w[i] {
			return false
		}
	}
	return true
}

func (t *Target) bootstrapMissing(item lang.Text) deploy.Missing {
	return deploy.Missing{
		Item:   item,
		Why:    textBootstrapWhy,
		Steps:  []lang.Text{textBootstrapStep1, textBootstrapStep2, textBootstrapStep3},
		Fix:    deploy.FixInput,
		Secret: true,
		Accept: func(val string) error {
			active, _, err := verifyBootstrap(t.client(val))
			if err != nil {
				return err
			}
			if !active {
				return ErrBootstrapTokenInactive
			}
			return t.cfg.Personal.Set(KeyBootstrapToken, val)
		},
	}
}

// verifyBootstrap calls GET /user/tokens/verify. A rejected token is
// reported as inactive, not as an error.
func verifyBootstrap(client *CfClient) (active bool, expiresOn time.Time, err error) {
	res, err := client.get("/user/tokens/verify")
	if err != nil {
		if ce, ok := err.(*cfError); ok && ce.Status < http.StatusInternalServerError {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, fmt.Errorf("verify bootstrap token: %w", err)
	}
	var v struct {
		Status    string `json:"status"`
		ExpiresOn string `json:"expires_on"`
	}
	if err := json.Unmarshal(res, &v); err != nil {
		return false, time.Time{}, fmt.Errorf("parse token verify: %w", err)
	}
	if v.ExpiresOn != "" {
		if expiresOn, err = time.Parse(time.RFC3339, v.ExpiresOn); err != nil {
			return false, time.Time{}, fmt.Errorf("parse token expires_on %q: %w", v.ExpiresOn, err)
		}
	}
	return v.Status == "active", expiresOn, nil
}

// Check probes Cloudflare for missing resources and configuration.
func (t *Target) Check(p deploy.Plan, ci deploy.Destination) ([]deploy.Missing, error) {
	// K1
	token, err := t.bootstrapToken()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return []deploy.Missing{t.bootstrapMissing(textBootstrapMissing)}, nil
	}
	client := t.client(token)

	// K2
	active, expiresOn, err := verifyBootstrap(client)
	if err != nil {
		return nil, err
	}
	if !active {
		return []deploy.Missing{t.bootstrapMissing(textBootstrapInvalid)}, nil
	}
	if !expiresOn.IsZero() && time.Until(expiresOn) <= bootstrapExpiryMargin {
		return []deploy.Missing{t.bootstrapMissing(textBootstrapExpiring)}, nil
	}

	// K3
	accounts, err := listAccounts(client)
	if err != nil {
		return nil, err
	}
	if len(accounts) != 1 {
		return []deploy.Missing{{
			Item:  textAccountLimit,
			Why:   textAccountLimitWhy,
			Steps: []lang.Text{textAccountLimitStep},
			Fix:   deploy.FixManual,
		}}, nil
	}
	accountID := accounts[0]

	// K4
	if len(p.Databases) > 1 || len(p.Buckets) > 1 {
		return nil, ErrOneDatabaseOneBucket
	}

	var missing []deploy.Missing

	// K5
	for _, db := range p.Databases {
		ok, err := d1Exists(client, accountID, db.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, deploy.Missing{
				Item:  textD1Missing,
				Why:   textD1MissingWhy,
				Steps: []lang.Text{textD1MissingStep},
				Fix:   deploy.FixAutomatic,
			})
		}
	}

	// K6
	for _, bkt := range p.Buckets {
		ok, err := r2Exists(client, accountID, bkt.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, deploy.Missing{
				Item:  textR2Missing,
				Why:   textR2MissingWhy,
				Steps: []lang.Text{textR2MissingStep},
				Fix:   deploy.FixAutomatic,
			})
		}
	}

	// K7
	zoneID := ""
	if p.Domain != "" {
		if zoneID, err = findZone(client, accountID, p.Domain); err != nil {
			return nil, err
		}
		if zoneID == "" {
			missing = append(missing, deploy.Missing{
				Item:  textDomainMissing,
				Why:   textDomainMissingWhy,
				Steps: []lang.Text{textDomainMissingStep},
				Fix:   deploy.FixManual,
			})
		}
	}

	// K8
	policies, err := requiredPermissions(p, client, accountID, zoneID)
	if err != nil {
		return nil, err
	}
	tokens, err := tokensNamed(client, accountID, accountTokenName(p))
	if err != nil {
		return nil, err
	}
	if !accountTokenValid(tokens, policies) {
		missing = append(missing, deploy.Missing{
			Item:  textAccountTokenMissing,
			Why:   textAccountTokenWhy,
			Steps: []lang.Text{textAccountTokenStep},
			Fix:   deploy.FixAutomatic,
		})
	}

	// K9
	hasToken, err := ci.Has("CLOUDFLARE_API_TOKEN")
	if err != nil {
		return nil, err
	}
	hasAccount, err := ci.Has("CLOUDFLARE_ACCOUNT_ID")
	if err != nil {
		return nil, err
	}
	if !hasToken || !hasAccount {
		missing = append(missing, deploy.Missing{
			Item:  textCISecretsMissing,
			Why:   textCISecretsMissingWhy,
			Steps: []lang.Text{textCISecretsMissingStep},
			Fix:   deploy.FixAutomatic,
		})
	}

	return missing, nil
}

// accountTokenValid reports whether exactly one token of the name exists, is
// active and grants exactly policies.
func accountTokenValid(tokens []accountToken, policies []map[string]any) bool {
	return len(tokens) == 1 && tokens[0].Status == "active" && policiesMatch(tokens[0].Policies, policies)
}
