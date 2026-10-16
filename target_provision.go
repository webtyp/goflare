//go:build !wasm

package goflare

import (
	"encoding/json"
	"fmt"

	"webtyp.com/deploy"
)

// Provision creates what is missing and delivers the target's own CI
// secrets to ci. Idempotent: the only CI secret it replaces is
// CLOUDFLARE_API_TOKEN, and only when the account token is absent or wrong
// (the old value no longer works).
func (t *Target) Provision(p deploy.Plan, ci deploy.Destination) error {
	if len(p.Databases) > 1 || len(p.Buckets) > 1 {
		return ErrOneDatabaseOneBucket
	}
	client, accountID, err := t.session()
	if err != nil {
		return err
	}

	for _, db := range p.Databases {
		ok, err := d1Exists(client, accountID, db.Name)
		if err != nil {
			return err
		}
		if !ok {
			if err := createNamed(client, fmt.Sprintf("/accounts/%s/d1/database", accountID), db.Name); err != nil {
				return fmt.Errorf("create D1 database %s: %w", db.Name, err)
			}
		}
	}

	for _, bkt := range p.Buckets {
		ok, err := r2Exists(client, accountID, bkt.Name)
		if err != nil {
			return err
		}
		if !ok {
			if err := createNamed(client, fmt.Sprintf("/accounts/%s/r2/buckets", accountID), bkt.Name); err != nil {
				return fmt.Errorf("create R2 bucket %s: %w", bkt.Name, err)
			}
		}
	}

	policies, tokens, err := t.tokenState(client, accountID, p)
	if err != nil {
		return err
	}
	hasCIToken, err := ci.Has("CLOUDFLARE_API_TOKEN")
	if err != nil {
		return err
	}
	if !accountTokenValid(tokens, policies) || !hasCIToken {
		if err := replaceAccountToken(client, accountID, p, policies, tokens, ci); err != nil {
			return err
		}
	}

	hasAccount, err := ci.Has("CLOUDFLARE_ACCOUNT_ID")
	if err != nil {
		return err
	}
	if !hasAccount {
		if err := ci.Put("CLOUDFLARE_ACCOUNT_ID", accountID); err != nil {
			return fmt.Errorf("put CLOUDFLARE_ACCOUNT_ID to CI: %w", err)
		}
	}
	return nil
}

// Rotate replaces every production secret the target owns: it always
// creates a new account token, puts it in ci, then deletes the old one.
func (t *Target) Rotate(p deploy.Plan, ci deploy.Destination) error {
	client, accountID, err := t.session()
	if err != nil {
		return err
	}
	policies, tokens, err := t.tokenState(client, accountID, p)
	if err != nil {
		return err
	}
	return replaceAccountToken(client, accountID, p, policies, tokens, ci)
}

// tokenState returns the policies p requires and the existing account
// tokens of p's name.
func (t *Target) tokenState(client *CfClient, accountID string, p deploy.Plan) ([]map[string]any, []accountToken, error) {
	zoneID := ""
	if p.Domain != "" {
		var err error
		if zoneID, err = findZone(client, accountID, p.Domain); err != nil {
			return nil, nil, err
		}
		if zoneID == "" {
			return nil, nil, fmt.Errorf("%w: %s", ErrZoneNotFound, p.Domain)
		}
	}
	policies, err := requiredPermissions(p, client, accountID, zoneID)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := tokensNamed(client, accountID, accountTokenName(p))
	if err != nil {
		return nil, nil, err
	}
	return policies, tokens, nil
}

func createNamed(client *CfClient, path, name string) error {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return err
	}
	_, err = client.post(path, body)
	return err
}
