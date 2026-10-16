//go:build !wasm

package goflare

import (
	"encoding/json"
	"fmt"
	"net/http"

	"webtyp.com/deploy"
)

// Provision creates what is missing and delivers the target's own CI
// secrets to ci. Idempotent: never replaces a secret that exists.
func (t *Target) Provision(p deploy.Plan, ci deploy.Destination) error {
	tokenStr, err := t.cfg.Personal.Get(KeyBootstrapToken)
	if err != nil || tokenStr == "" {
		return fmt.Errorf("missing bootstrap token")
	}

	client := &CfClient{Token: tokenStr, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}

	resAcct, err := client.get("/accounts")
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	var acctResult []struct{
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resAcct, &acctResult); err != nil {
		return err
	}
	if len(acctResult) != 1 {
		return fmt.Errorf("multiple accounts found, cannot provision automatically")
	}
	accountID := acctResult[0].ID

	// Create databases
	for _, db := range p.Databases {
		res, _ := client.get(fmt.Sprintf("/accounts/%s/d1/database?name=%s", accountID, db.Name))
		var dbPayload []struct {
			Name string `json:"name"`
		}
		json.Unmarshal(res, &dbPayload)
		found := false
		for _, v := range dbPayload {
			if v.Name == db.Name {
				found = true
				break
			}
		}
		if !found {
			body, _ := json.Marshal(map[string]string{"name": db.Name})
			_, err := client.post(fmt.Sprintf("/accounts/%s/d1/database", accountID), body)
			if err != nil {
				return fmt.Errorf("failed to create database %s: %w", db.Name, err)
			}
		}
	}

	// Create buckets
	for _, bkt := range p.Buckets {
		res, err := client.get(fmt.Sprintf("/accounts/%s/r2/buckets/%s", accountID, bkt.Name))
		if err != nil || len(res) == 0 {
			body, _ := json.Marshal(map[string]string{"name": bkt.Name})
			_, err := client.post(fmt.Sprintf("/accounts/%s/r2/buckets", accountID), body)
			if err != nil {
				return fmt.Errorf("failed to create bucket %s: %w", bkt.Name, err)
			}
		}
	}

	// Create account token
	tokenName := "webtyp-deploy-" + p.Name
	policies, err := requiredPermissions(p, client)
	if err != nil {
		return err
	}

	// Retrieve existing tokens to delete the old one *after* success
	resTk, _ := client.get(fmt.Sprintf("/accounts/%s/tokens", accountID))
	var tkResult []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(resTk, &tkResult)

	body, _ := json.Marshal(map[string]any{
		"name": tokenName,
		"policies": policies,
	})
	resNewTk, err := client.post(fmt.Sprintf("/accounts/%s/tokens", accountID), body)
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}

	var newTkResult struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(resNewTk, &newTkResult); err != nil {
		return fmt.Errorf("failed to parse new token response")
	}

	// Put in CI
	if err := ci.Put("CLOUDFLARE_API_TOKEN", newTkResult.Value); err != nil {
		return fmt.Errorf("failed to put token to CI: %w", err)
	}

	// Delete old tokens of the same name
	for _, tk := range tkResult {
		if tk.Name == tokenName {
			client.delete(fmt.Sprintf("/accounts/%s/tokens/%s", accountID, tk.ID))
		}
	}

	hasAccount, _ := ci.Has("CLOUDFLARE_ACCOUNT_ID")
	if !hasAccount {
		if err := ci.Put("CLOUDFLARE_ACCOUNT_ID", accountID); err != nil {
			return fmt.Errorf("failed to put account ID to CI: %w", err)
		}
	}

	return nil
}

// Rotate replaces every production secret the target owns.
func (t *Target) Rotate(p deploy.Plan, ci deploy.Destination) error {
	tokenStr, err := t.cfg.Personal.Get(KeyBootstrapToken)
	if err != nil || tokenStr == "" {
		return fmt.Errorf("missing bootstrap token")
	}

	client := &CfClient{Token: tokenStr, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}

	resAcct, err := client.get("/accounts")
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	var acctResult []struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resAcct, &acctResult)
	if len(acctResult) != 1 {
		return fmt.Errorf("multiple accounts found, cannot provision automatically")
	}
	accountID := acctResult[0].ID

	tokenName := "webtyp-deploy-" + p.Name
	policies, err := requiredPermissions(p, client)
	if err != nil {
		return err
	}

	resTk, _ := client.get(fmt.Sprintf("/accounts/%s/tokens", accountID))
	var tkResult []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(resTk, &tkResult)

	body, _ := json.Marshal(map[string]any{
		"name": tokenName,
		"policies": policies,
	})
	resNewTk, err := client.post(fmt.Sprintf("/accounts/%s/tokens", accountID), body)
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}

	var newTkResult struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(resNewTk, &newTkResult); err != nil {
		return fmt.Errorf("failed to parse new token response")
	}

	if err := ci.Put("CLOUDFLARE_API_TOKEN", newTkResult.Value); err != nil {
		return fmt.Errorf("failed to put token to CI: %w", err)
	}

	for _, tk := range tkResult {
		if tk.Name == tokenName {
			client.delete(fmt.Sprintf("/accounts/%s/tokens/%s", accountID, tk.ID))
		}
	}

	return nil
}
