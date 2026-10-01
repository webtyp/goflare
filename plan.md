1. *Modify `assets.go` to implement `headersRules` and refuse artifacts.*
   - Define a new unexported function `headersRules(manifest map[string]assetEntry) (string, error)`. It will iterate over the manifest, count how many paths return `true` for `pwa.IsHashedName(path)`. If this count > 100, return `fmt.Errorf("goflare: %d content-hashed files exceed the 100 rules of _headers", count)`. Otherwise, format a rule `"<path>\n  Cache-Control: " + pwa.CacheImmutable + "\n"` for each hashed path. Sort the paths before building the string.
   - In `buildAssetManifest`, check if `strings.HasPrefix(key, pwa.ArtifactsDir)`. If so, increment a counter. At the end, if > 0, return `fmt.Errorf("goflare: %d files under %s cannot be Workers static assets (25 MiB per file); serving artifacts from R2 is not implemented yet", count, pwa.ArtifactsDir)`.
   - Add a hook function `ExportHeadersRules(manifest map[string]assetEntry) (string, error)` that delegates to `headersRules`.
   - Verify the edits by reading `assets.go`.
2. *Modify `cloudflare.go` to inject `_headers`.*
   - In `Deploy()`, inside the block where `hasAssets` is true, call `rules, err := headersRules(manifest)`. If `err != nil`, return it.
   - If `rules != ""`, assign `config["_headers"] = rules` (inside `metadata["assets"]["config"]`).
   - Verify the edits by reading `cloudflare.go`.
3. *Modify `tests/devserver_test.go` to add `PlainHTTP` flag.*
   - Inside `TestDevServer`, update the `devserver.New(httpd.Config{...})` call to include `TLS: httpd.TLSConfig{PlainHTTP: true}`. Add a comment `// devserver now uses HTTPS by default, so tests must ask for plain HTTP`.
   - Verify the edits by reading `tests/devserver_test.go`.
4. *Write tests in `tests/assets_test.go`.*
   - Use `replace_with_git_merge_diff` to add the following block:
     ```go
     func TestHeadersRules_OnlyHashedNamesImmutable(t *testing.T) {
         manifest := map[string]goflare.ExportAssetEntry{
             "/": {}, "/sw.js": {}, "/style.3f9a1c2b.css": {}, "/client.9a8b7c6d.wasm": {},
         }
         rules, err := goflare.ExportHeadersRules(manifest)
         if err != nil {
             t.Fatal(err)
         }
         want := "/client.9a8b7c6d.wasm\n  Cache-Control: public, max-age=31536000, immutable\n/style.3f9a1c2b.css\n  Cache-Control: public, max-age=31536000, immutable\n"
         if rules != want {
             t.Errorf("got %q, want %q", rules, want)
         }
     }
     func TestHeadersRules_NoneHashed(t *testing.T) {
         manifest := map[string]goflare.ExportAssetEntry{"/": {}, "/sw.js": {}}
         rules, err := goflare.ExportHeadersRules(manifest)
         if err != nil { t.Fatal(err) }
         if rules != "" { t.Errorf("expected empty string, got %q", rules) }
     }
     func TestHeadersRules_TooMany(t *testing.T) {
         manifest := make(map[string]goflare.ExportAssetEntry)
         for i := 0; i < 101; i++ { manifest[fmt.Sprintf("/style.3f9a1c2b.css-%d", i)] = goflare.ExportAssetEntry{} }
         _, err := goflare.ExportHeadersRules(manifest)
         if err == nil { t.Fatal("expected error, got nil") }
     }
     func TestBuildAssetManifest_RefusesArtifacts(t *testing.T) {
         env := newTestEnv(t)
         env.writePublic("artifacts/x.bin", "data")
         g := goflare.New(&goflare.Config{PublicDir: env.PublicDir, OutputDir: env.OutputDir})
         _, _, err := g.ExportBuildAssetManifest(env.PublicDir)
         if err == nil || !strings.Contains(err.Error(), "/artifacts/") {
             t.Errorf("expected artifacts error, got %v", err)
         }
     }
     ```
   - *Note: the actual test structure will be adapted to how `ExportAssetEntry` (or similar interface) is implemented in `assets.go` during execution.*
   - Verify the edits by reading `tests/assets_test.go`.
5. *Write tests in `tests/deploy_test.go`.*
   - Use `replace_with_git_merge_diff` to add the following test block:
     ```go
     func TestDeploy_SendsHeadersInAssetsConfig(t *testing.T) {
         withCloudflareToken(t)
         env := newTestEnv(t)
         env.writeOutput("edge.js", "console.log('edge')")
         env.writeOutput("edge.wasm", "wasm-bytes")
         env.writePublic("style.3f9a1c2b.css", "body{}")

         var captured capturedMetadata
         server := MockHTTPServer(func(w http.ResponseWriter, r *http.Request) {
             w.Header().Set("Content-Type", "application/json")
             if strings.Contains(r.URL.Path, "/assets-upload-session") {
                 w.WriteHeader(http.StatusOK)
                 w.Write([]byte(`{"success":true,"result":{"jwt":"session","buckets":[]}}`))
             } else if strings.Contains(r.URL.Path, "/workers/assets/upload") {
                 w.WriteHeader(http.StatusOK)
                 w.Write([]byte(`{"success":true,"result":{"jwt":"jwt"}}`))
             } else if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/workers/scripts/") {
                 captured = captureDeployPUT(t, r)
                 w.WriteHeader(http.StatusOK)
                 w.Write([]byte(`{"success":true,"result":{}}`))
             } else {
                 w.WriteHeader(http.StatusNotFound)
             }
         })
         defer server.Close()

         g := goflare.New(&goflare.Config{AccountID: "acc-123", WorkerName: "my-worker", PublicDir: env.PublicDir, OutputDir: env.OutputDir})
         g.BaseURL = server.URL
         if err := g.Deploy(); err != nil { t.Fatalf("Deploy failed: %v", err) }

         assets := captured.metadata["assets"].(map[string]any)
         config := assets["config"].(map[string]any)
         headers, ok := config["_headers"].(string)
         if !ok || !strings.Contains(headers, "/style.3f9a1c2b.css") || !strings.Contains(headers, "immutable") {
             t.Errorf("expected _headers to contain style and immutable, got %v", headers)
         }
     }
     ```
   - Verify the edits by reading `tests/deploy_test.go`.
6. *Run tests.*
   - Execute the test runner `gotest` in bash.
7. *Update documentation.*
   - Append to the `README.md` deploy section: `"Static assets with a content-hashed name automatically receive an \`immutable\` Cache-Control header via Cloudflare's \`_headers\` policy. Large files under \`/artifacts/\` are currently refused as static assets."` using `replace_with_git_merge_diff`.
   - Append to `docs/ARCHITECTURE.md` deploy section: `"Static assets with a content-hashed name automatically receive an \`immutable\` Cache-Control header via Cloudflare's \`_headers\` policy generated during deployment. Large files under \`/artifacts/\` are currently refused as static assets."` using `replace_with_git_merge_diff`.
   - Verify the edits by reading the documentation files.
8. *Complete pre commit steps.*
   - Complete pre commit steps to ensure proper testing, verification, review, and reflection are done.
9. *Submit.*
   - Commit the changes and submit the PR.
