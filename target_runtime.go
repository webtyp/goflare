//go:build !wasm

package goflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/textproto"

	"webtyp.com/deploy"
)

// placeholderWorker is uploaded, as Wrangler does, when a secret is put
// before the first deploy. The first real deploy replaces the script and
// Cloudflare keeps its secret_text bindings.
const placeholderWorker = `export default { fetch() { return new Response("", {status: 503}) } }`

// runtimeDestination puts Worker secrets with the person's bootstrap token,
// never the CI token. err holds a session failure, returned by every call.
type runtimeDestination struct {
	client    *CfClient
	accountID string
	script    string
	err       error
}

func (r *runtimeDestination) secretsPath() string {
	return fmt.Sprintf("/accounts/%s/workers/scripts/%s/secrets", r.accountID, r.script)
}

// secretNames lists the Worker's secret names; exists is false when the
// script does not exist yet.
func (r *runtimeDestination) secretNames() (names []string, exists bool, err error) {
	if r.err != nil {
		return nil, false, r.err
	}
	res, err := r.client.get(r.secretsPath())
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("list Worker secrets: %w", err)
	}
	var secrets []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(res, &secrets); err != nil {
		return nil, false, fmt.Errorf("parse Worker secrets: %w", err)
	}
	for _, s := range secrets {
		names = append(names, s.Name)
	}
	return names, true, nil
}

func (r *runtimeDestination) Has(key string) (bool, error) {
	names, _, err := r.secretNames()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == key {
			return true, nil
		}
	}
	return false, nil
}

func (r *runtimeDestination) Put(key, value string) error {
	_, exists, err := r.secretNames()
	if err != nil {
		return err
	}
	if !exists {
		if err := r.uploadPlaceholder(); err != nil {
			return err
		}
	}
	body, err := json.Marshal(map[string]string{
		"name": key,
		"text": value,
		"type": "secret_text",
	})
	if err != nil {
		return err
	}
	if _, err := r.client.put(r.secretsPath(), body); err != nil {
		return fmt.Errorf("put Worker secret %s: %w", key, err)
	}
	return nil
}

func (r *runtimeDestination) uploadPlaceholder() error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	metadata, err := json.Marshal(map[string]any{
		"main_module":        "index.js",
		"compatibility_date": DefaultCompatibilityDate,
	})
	if err != nil {
		return err
	}
	if err := mw.WriteField("metadata", string(metadata)); err != nil {
		return err
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="index.js"; filename="index.js"`)
	h.Set("Content-Type", contentTypeESModule)
	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := part.Write([]byte(placeholderWorker)); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", r.accountID, r.script)
	if _, err := r.client.putMultipart(path, &buf, mw.FormDataContentType()); err != nil {
		return fmt.Errorf("upload placeholder Worker: %w", err)
	}
	return nil
}

// Runtime is where ToRuntime secrets go: the Worker's secrets.
func (t *Target) Runtime(p deploy.Plan) deploy.Destination {
	client, accountID, err := t.session()
	return &runtimeDestination{client: client, accountID: accountID, script: p.Name, err: err}
}
