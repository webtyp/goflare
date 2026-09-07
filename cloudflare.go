//go:build !wasm

package goflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const cfAPIBase = "https://api.cloudflare.com/client/v4"

const (
	contentTypeESModule = "application/javascript+module"
	contentTypeWasm     = "application/wasm"
)

type CfClient struct {
	Token      string
	BaseURL    string // default: cfAPIBase; overridden in tests
	HttpClient *http.Client
}

func (c *CfClient) get(path string) ([]byte, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *CfClient) post(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPost, path, bytes.NewReader(body))
}

func (c *CfClient) put(path string, body []byte) ([]byte, error) {
	return c.do(http.MethodPut, path, bytes.NewReader(body))
}

func (c *CfClient) putMultipart(path string, body io.Reader, contentType string) ([]byte, error) {
	return c.doMultipart(http.MethodPut, path, body, contentType)
}

func (c *CfClient) postMultipart(path string, body io.Reader, contentType string) ([]byte, error) {
	return c.doMultipart(http.MethodPost, path, body, contentType)
}

func (c *CfClient) doMultipart(method, path string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", contentType)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(method, path, resp)
}

func (c *CfClient) do(method, path string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseCFResponse(method, path, resp)
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".wasm":
		return "application/wasm"
	case ".txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func (g *Goflare) getWorkerSubdomain(client *CfClient) string {
	path := fmt.Sprintf("/accounts/%s/workers/subdomain", g.Config.AccountID)
	data, err := client.get(path)
	if err != nil {
		return "<your-subdomain>"
	}
	var result struct {
		Result struct {
			Subdomain string `json:"subdomain"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "<your-subdomain>"
	}
	return result.Result.Subdomain
}

func hasFilesInDir(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	hasFiles := false
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			hasFiles = true
		}
		return nil
	})
	return hasFiles
}

// Deploy uploads the site to Cloudflare as a Worker with static assets.
func (g *Goflare) Deploy() error {
	token, err := g.token()
	if err != nil {
		return err
	}

	client := &CfClient{
		Token:      token,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}

	hasAssets := hasFilesInDir(g.Config.PublicDir)
	edgeJsPath := filepath.Join(g.Config.OutputDir, "edge.js")
	edgeWasmPath := filepath.Join(g.Config.OutputDir, "edge.wasm")

	hasScript := false
	if _, err := os.Stat(edgeJsPath); err == nil {
		hasScript = true
		if _, err := os.Stat(edgeWasmPath); os.IsNotExist(err) {
			return fmt.Errorf("missing artifact: edge.wasm")
		}
	}

	if !hasAssets && !hasScript {
		return fmt.Errorf("nothing to deploy: neither %s nor %s/edge.js exist", g.Config.PublicDir, g.Config.OutputDir)
	}

	var completionToken string
	if hasAssets {
		manifest, byHash, err := g.buildAssetManifest(g.Config.PublicDir)
		if err != nil {
			return err
		}
		token, err := g.uploadAssets(client, manifest, byHash)
		if err != nil {
			return err
		}
		completionToken = token
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	metadata := map[string]any{
		"compatibility_date": g.compatibilityDate(),
	}

	if hasScript {
		metadata["main_module"] = "edge.js"
	}

	if hasAssets {
		workerFirst, err := g.workerFirstRoutes()
		if err != nil {
			return err
		}
		metadata["assets"] = map[string]any{
			"jwt": completionToken,
			"config": map[string]any{
				"html_handling":      HTMLHandlingDefault,
				"not_found_handling": g.notFoundHandling(),
				"run_worker_first":   workerFirst,
			},
		}
	}

	var bindings []any
	if g.Config.D1DatabaseID != "" {
		dbName := g.Config.D1DatabaseName
		if dbName == "" {
			dbName = "DB"
		}
		bindings = append(bindings, map[string]string{
			"type": "d1",
			"name": dbName,
			"id":   g.Config.D1DatabaseID,
		})
	}

	if g.Config.R2BucketID != "" {
		bucketName := g.Config.R2BucketName
		if bucketName == "" {
			bucketName = "FILES"
		}
		bindings = append(bindings, map[string]string{
			"type":        "r2_bucket",
			"name":        bucketName,
			"bucket_name": g.Config.R2BucketID,
		})
	}

	if len(bindings) > 0 {
		metadata["bindings"] = bindings
	}

	metadataJSON, _ := json.Marshal(metadata)
	if err := mw.WriteField("metadata", string(metadataJSON)); err != nil {
		return err
	}

	if hasScript {
		if err := addFilePart(mw, "edge.js", edgeJsPath, contentTypeESModule); err != nil {
			return err
		}
		if err := addFilePart(mw, "edge.wasm", edgeWasmPath, contentTypeWasm); err != nil {
			return err
		}
	}

	if err := mw.Close(); err != nil {
		return err
	}

	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", g.Config.AccountID, g.Config.WorkerName)
	if _, err := client.putMultipart(path, &buf, mw.FormDataContentType()); err != nil {
		return err
	}

	if g.Config.Domain != "" {
		if err := g.configureWorkerDomain(client); err != nil {
			g.Logger("Warning: failed to configure domain:", err)
		}
	}

	if hasScript {
		if err := g.verifyProbe(client); err != nil {
			return err
		}
	}

	return nil
}

func (g *Goflare) configureWorkerDomain(client *CfClient) error {
	zonesData, err := client.get("/zones")
	if err != nil {
		return fmt.Errorf("failed to fetch zones: %w", err)
	}

	var zones []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(zonesData, &zones); err != nil {
		return fmt.Errorf("failed to parse zones response: %w", err)
	}

	var matchedZoneID string
	var longestMatchLen int

	domain := g.Config.Domain
	for _, zone := range zones {
		if domain == zone.Name || strings.HasSuffix(domain, "."+zone.Name) {
			if len(zone.Name) > longestMatchLen {
				longestMatchLen = len(zone.Name)
				matchedZoneID = zone.ID
			}
		}
	}

	if matchedZoneID == "" {
		return fmt.Errorf("no zone in the account matches domain %s", domain)
	}

	domainPath := fmt.Sprintf("/accounts/%s/workers/domains", g.Config.AccountID)
	body, _ := json.Marshal(map[string]string{
		"environment": "production",
		"hostname":    domain,
		"service":     g.Config.WorkerName,
		"zone_id":     matchedZoneID,
	})

	if _, err := client.put(domainPath, body); err != nil {
		return err
	}

	return nil
}

func (g *Goflare) verifyProbe(client *CfClient) error {
	baseURL := g.SiteURL
	if baseURL == "" && g.Config.Domain != "" {
		baseURL = "https://" + g.Config.Domain
	}
	if baseURL == "" {
		subdomain := g.getWorkerSubdomain(client)
		if subdomain != "" && subdomain != "<your-subdomain>" {
			baseURL = fmt.Sprintf("https://%s.%s.workers.dev", g.Config.WorkerName, subdomain)
		}
	}

	if baseURL == "" {
		g.Logger("Skipping probe verification: public URL could not be determined")
		return nil
	}

	probeURL := strings.TrimSuffix(baseURL, "/") + "/api/__goflare_probe"

	hc := client.HttpClient
	if hc == nil {
		hc = http.DefaultClient
	}

	err := g.retry(5, g.RetryBackoff, func() error {
		req, err := http.NewRequest(http.MethodGet, probeURL, nil)
		if err != nil {
			return err
		}
		resp, err := hc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.Header.Get(HeaderIdentity) != "" {
			return nil
		}
		return fmt.Errorf("missing %s header", HeaderIdentity)
	})

	if err != nil {
		return fmt.Errorf(
			"deploy verification failed: %s responded without the %s header — "+
				"the files were uploaded but the Worker is not serving requests",
			probeURL, HeaderIdentity)
	}

	return nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfAPIError    `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfError struct {
	Status  int          // status HTTP
	Code    int          // primer errors[].code, si hay
	Message string       // resumen legible
	Errors  []cfAPIError // todos los errores del envelope
	Body    string       // cuerpo crudo truncado (fallback)
	Path    string       // método + ruta que falló
}

func (e *cfError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("CF API %s → HTTP %d: %s", e.Path, e.Status, e.Message)
	}
	return fmt.Sprintf("CF API %s → HTTP %d, success=false, body: %s", e.Path, e.Status, e.Body)
}

func parseCFResponse(method, path string, resp *http.Response) (json.RawMessage, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read CF response: %w", err)
	}
	var env cfEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, &cfError{
			Status: resp.StatusCode,
			Path:   method + " " + path,
			Body:   truncate(string(data), 500),
		}
	}
	if !env.Success || resp.StatusCode >= 400 {
		ce := &cfError{
			Status: resp.StatusCode,
			Errors: env.Errors,
			Path:   method + " " + path,
			Body:   truncate(string(data), 500),
		}
		if len(env.Errors) > 0 {
			ce.Code = env.Errors[0].Code
			var msgs []string
			for _, e := range env.Errors {
				msgs = append(msgs, fmt.Sprintf("%s (code: %d)", e.Message, e.Code))
			}
			ce.Message = strings.Join(msgs, ", ")
		}
		return nil, ce
	}
	return env.Result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// retry executes fn up to n times with exponential backoff.
func (g *Goflare) retry(n int, base time.Duration, fn func() error) error {
	var err error
	for i := 0; i < n; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < n-1 {
			time.Sleep(base << i)
		}
	}
	return err
}

func addFilePart(mw *multipart.Writer, fieldName, filePath, contentType string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filepath.Base(filePath)))
	h.Set("Content-Type", contentType)

	part, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
