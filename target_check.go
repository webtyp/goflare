//go:build !wasm

package goflare

import (
	"encoding/json"
	"fmt"
	"net/http"

	"webtyp.com/deploy"
	"webtyp.com/lang"
)

var requiredPermissionsNames = []string{
	"Workers Scripts Write",
	"D1 Write",
	"Workers R2 Storage Write",
	"Zone Read",
	"Workers Routes Write",
	"Zone WAF Write",
}

// targetCheck contains the resolved requirements for the deploy Target.
type targetCheck struct {
	requiredPolicies []map[string]any
}

func (t *Target) ID() deploy.TargetID {
	return Worker
}

func requiredPermissions(p deploy.Plan, client *CfClient) ([]map[string]any, error) {
	res, err := client.get("/user/tokens/permission_groups")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch permission groups: %w", err)
	}

	var result []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	if err := json.Unmarshal(res, &result); err != nil {
		return nil, fmt.Errorf("failed to parse permission groups: %w", err)
	}

	byName := make(map[string]string)
	for _, g := range result {
		byName[g.Name] = g.ID
	}

	var needs []string
	needs = append(needs, "Workers Scripts Write")
	if len(p.Databases) > 0 {
		needs = append(needs, "D1 Write")
	}
	if len(p.Buckets) > 0 {
		needs = append(needs, "Workers R2 Storage Write")
	}
	if p.Domain != "" {
		needs = append(needs, "Zone Read", "Workers Routes Write", "Zone WAF Write")
	}

	var policies []map[string]any
	for _, n := range needs {
		id, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownPermissionGroup, n)
		}
		var resources map[string]any
		if n == "Zone Read" || n == "Workers Routes Write" || n == "Zone WAF Write" {
			resources = map[string]any{
				"com.cloudflare.api.account.zone.*": "*",
			}
		} else {
			resources = map[string]any{
				"com.cloudflare.api.account.*": "*",
			}
		}

		policies = append(policies, map[string]any{
			"effect": "allow",
			"permission_groups": []map[string]any{
				{"id": id},
			},
			"resources": resources,
		})
	}
	return policies, nil
}

// Check probes Cloudflare for missing resources and configuration.
func (t *Target) Check(p deploy.Plan, ci deploy.Destination) ([]deploy.Missing, error) {
	var msgs []deploy.Missing

	tokenStr, err := t.cfg.Personal.Get(KeyBootstrapToken)
	if err != nil || tokenStr == "" {
		msgs = append(msgs, deploy.Missing{
			Item:   StepCreateBootstrapToken,
			Fix:    deploy.FixInput,
			Secret: true,
			Accept: func(val string) error {
				client := &CfClient{Token: val, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}
				res, err := client.get("/user/tokens/verify")
				if err != nil {
					return fmt.Errorf("verify failed: %w", err)
				}
				var result struct {
					Status string `json:"status"`
				}
				if err := json.Unmarshal(res, &result); err != nil {
					return err
				}
				if result.Status != "active" {
					return fmt.Errorf("invalid token status")
				}
				return t.cfg.Personal.Set(KeyBootstrapToken, val)
			},
		})
		return msgs, nil
	}

	client := &CfClient{Token: tokenStr, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}

	// K2
	res, err := client.get("/user/tokens/verify")
	if err != nil {
		msgs = append(msgs, deploy.Missing{
			Item:   ItemBootstrapTokenInvalid,
			Fix:    deploy.FixInput,
			Secret: true,
		})
		return msgs, nil
	}
	var verifyResult struct {
		Status    string `json:"status"`
		ExpiresOn string `json:"expires_on"`
	}
	if err := json.Unmarshal(res, &verifyResult); err != nil {
		msgs = append(msgs, deploy.Missing{
			Item:   ItemBootstrapTokenInvalid,
			Fix:    deploy.FixInput,
			Secret: true,
		})
		return msgs, nil
	}
	if verifyResult.Status != "active" { // Simplified expires_on check
		msgs = append(msgs, deploy.Missing{
			Item:   ItemBootstrapTokenInvalid,
			Fix:    deploy.FixInput,
			Secret: true,
		})
		return msgs, nil
	}

	// K3
	resAcct, _ := client.get("/accounts")
	var acctResult []struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resAcct, &acctResult)
	if len(acctResult) != 1 {
		msgs = append(msgs, deploy.Missing{
			Item: StepLimitAccount,
			Fix: deploy.FixManual,
		})
		return msgs, nil
	}
	accountID := acctResult[0].ID

	// K4
	if len(p.Databases) > 1 || len(p.Buckets) > 1 {
		return nil, ErrOneDatabaseOneBucket
	}

	// K5
	for _, db := range p.Databases {
		res, _ := client.get(fmt.Sprintf("/accounts/%s/d1/database?name=%s", accountID, db.Name))
		var dbResult []struct {
			Name string `json:"name"`
		}
		json.Unmarshal(res, &dbResult)
		found := false
		for _, r := range dbResult {
			if r.Name == db.Name {
				found = true
				break
			}
		}
		if !found {
			msgs = append(msgs, deploy.Missing{
				Item: lang.Text(fmt.Sprintf("create database %s", db.Name)),
				Fix: deploy.FixAutomatic,
			})
		}
	}

	// K6
	for _, bkt := range p.Buckets {
		_, err := client.get(fmt.Sprintf("/accounts/%s/r2/buckets/%s", accountID, bkt.Name))
		if err != nil {
			msgs = append(msgs, deploy.Missing{
				Item: lang.Text(fmt.Sprintf("create bucket %s", bkt.Name)),
				Fix: deploy.FixAutomatic,
			})
		}
	}

	// K7
	if p.Domain != "" {
		res, _ := client.get(fmt.Sprintf("/zones?name=%s&account.id=%s", p.Domain, accountID)) // Needs proper registrable extraction, simplify for now
		var zResult []struct {
			ID string `json:"id"`
		}
		json.Unmarshal(res, &zResult)
		if len(zResult) == 0 {
			msgs = append(msgs, deploy.Missing{
				Item: StepAddDomain,
				Fix: deploy.FixManual,
			})
		}
	}

	// K8
	policies, err := requiredPermissions(p, client)
	if err != nil {
		return nil, err
	}
	tokenName := "webtyp-deploy-" + p.Name
	res, _ = client.get(fmt.Sprintf("/accounts/%s/tokens", accountID))
	var tkResult []struct {
		Name string `json:"name"`
		Status string `json:"status"`
		Policies []map[string]any `json:"policies"`
	}
	json.Unmarshal(res, &tkResult)
	foundTk := false
	for _, tk := range tkResult {
		if tk.Name == tokenName {
			foundTk = true
			if tk.Status != "active" {
				foundTk = false
			} else {
				// Very rough heuristic for differ check.
				if len(tk.Policies) != len(policies) {
					foundTk = false
				}
			}
			break
		}
	}
	if !foundTk {
		msgs = append(msgs, deploy.Missing{
			Item: lang.Text("create account token"),
			Fix: deploy.FixAutomatic,
		})
	}

	// K9
	hasToken, _ := ci.Has("CLOUDFLARE_API_TOKEN")
	hasAccount, _ := ci.Has("CLOUDFLARE_ACCOUNT_ID")
	if !hasToken || !hasAccount {
		msgs = append(msgs, deploy.Missing{
			Item: lang.Text("set CI secrets"),
			Fix: deploy.FixAutomatic,
		})
	}

	return msgs, nil
}
