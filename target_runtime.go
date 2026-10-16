//go:build !wasm

package goflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"webtyp.com/deploy"
)

type runtimeDestination struct {
	client    *CfClient
	accountID string
	script    string
}

func (r *runtimeDestination) Has(key string) (bool, error) {
	res, err := r.client.get(fmt.Sprintf("/accounts/%s/workers/scripts/%s/secrets", r.accountID, r.script))
	if err != nil {
		return false, nil // Script not found
	}

	var result []struct{
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res, &result); err != nil {
		return false, nil
	}

	for _, sec := range result {
		if sec.Name == key {
			return true, nil
		}
	}
	return false, nil
}

func (r *runtimeDestination) Put(key, value string) error {
	// Check if script exists
	_, err := r.client.get(fmt.Sprintf("/accounts/%s/workers/scripts/%s/secrets", r.accountID, r.script))
	if err != nil {
		// Upload placeholder worker
		const placeholder = `export default { fetch() { return new Response("", {status: 503}) } }`

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)

		metadata := map[string]any{
			"main_module": "index.js",
		}
		metadataJSON, _ := json.Marshal(metadata)

		// Metadata part
		metaHeader := make(textproto.MIMEHeader)
		metaHeader.Set("Content-Disposition", `form-data; name="metadata"`)
		metaHeader.Set("Content-Type", "application/json")
		metaPart, _ := mw.CreatePart(metaHeader)
		metaPart.Write(metadataJSON)

		// Script part
		scriptHeader := make(textproto.MIMEHeader)
		scriptHeader.Set("Content-Disposition", `form-data; name="index.js"; filename="index.js"`)
		scriptHeader.Set("Content-Type", "application/javascript+module")
		scriptPart, _ := mw.CreatePart(scriptHeader)
		scriptPart.Write([]byte(placeholder))

		mw.Close()

		_, err := r.client.putMultipart(fmt.Sprintf("/accounts/%s/workers/scripts/%s", r.accountID, r.script), &buf, mw.FormDataContentType())
		if err != nil {
			return fmt.Errorf("failed to upload placeholder script: %w", err)
		}
	}

	// Put the secret
	secBody, _ := json.Marshal(map[string]string{
		"name": key,
		"text": value,
		"type": "secret_text",
	})
	_, err = r.client.put(fmt.Sprintf("/accounts/%s/workers/scripts/%s/secrets", r.accountID, r.script), secBody)
	if err != nil {
		return fmt.Errorf("failed to put secret: %w", err)
	}

	return nil
}

// Runtime is where ToRuntime secrets go.
func (t *Target) Runtime(p deploy.Plan) deploy.Destination {
	tokenStr, _ := t.cfg.Personal.Get(KeyBootstrapToken)
	client := &CfClient{Token: tokenStr, BaseURL: t.cfg.BaseURL, HttpClient: http.DefaultClient}

	// Need accountID. Best effort via /accounts if not cached
	resAcct, _ := client.get("/accounts")
	var acctResult []struct{
		ID string `json:"id"`
	}
	json.Unmarshal(resAcct, &acctResult)
	var accountID string
	if len(acctResult) == 1 {
		accountID = acctResult[0].ID
	}

	return &runtimeDestination{
		client: client,
		accountID: accountID,
		script: p.Name,
	}
}
