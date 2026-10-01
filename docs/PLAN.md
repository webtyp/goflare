---
PLAN: "feat: _headers from the pwa contract (immutable hashed names); refuse /artifacts/ as Workers assets"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `goflare`: cache headers for content-hashed names, and no large artifacts as assets

Master plan: [PWA_ARTIFACTS_MASTER_PLAN.md](https://github.com/webtyp/app/blob/main/docs/PWA_ARTIFACTS_MASTER_PLAN.md),
decisions D-PWA-5, D-PWA-14, D-PWA-16. Read [AGENTS.md](../AGENTS.md) first: everything in this plan
is **host code** (`!wasm`, `cloudflare.go`, `assets.go`) where the standard library is correct.

## Why

- `sitec` release builds name the stylesheet, the script and the page binary by their content
  hash (`style.3f9a1c2b.css`, `client.9a8b7c6d.wasm`). Such a file never changes, so the browser may
  keep it forever: `Cache-Control: public, max-age=31536000, immutable`. Cloudflare's default for
  static assets is `public, max-age=0, must-revalidate` (+ ETag), which is already right for the
  fixed names (`/`, `sw.js`, `manifest.webmanifest`) but makes every visit revalidate the hashed ones.
- The rule "which path gets which header" is written once, in `webtyp.com/pwa` (v0.1.1, published):
  `pwa.CacheControl(path)`, `pwa.IsHashedName(path)`, `pwa.CacheImmutable`. `server/httpd` already
  uses it for the LAN server; goflare must use the same function, never its own pattern.
- Large artifacts (model weights, hundreds of MB) are served under `pwa.ArtifactsDir`
  (`/artifacts/`). Workers static assets accept at most **25 MiB per file**, so they cannot go there;
  serving them from R2 is **not** implemented yet (D-PWA-16: no Cloudflare-hosted project needs it).
  Today such a file would make the upload fail with an opaque API error, or worse, a small one would
  go up as an asset with the wrong headers. goflare must refuse it with a clear message.

## Design gate

1. **Prior art.** Cloudflare Pages / Workers static assets and Netlify read a `_headers` file
   (`[path]` + indented `Name: value` lines); Wrangler, deploying Workers with assets, does not
   upload `_headers` as an asset but sends its text in the script-upload metadata as
   `assets.config._headers` (same for `_redirects`). Vite/Next leave the policy to that file.
   We generate the rules from the file list instead of asking the developer to write them.
2. **Novice-name test.** No new exported symbol. The behaviour reads as "deploy sends immutable
   headers for hashed files".
3. **Complexity ledger.** Concepts +0 (the rule lives in `pwa`), ways to set headers 1.
4. **Where it belongs.** Header *policy*: `pwa`. Header *transport* to Cloudflare: here.
5. **What it deletes.** Nothing.

## Stage 1 — `_headers` rules in the upload metadata

File `assets.go` (host code):

1. New unexported function:
   ```go
   // headersRules returns the _headers text Cloudflare applies to static assets: one rule per
   // content-hashed file (pwa.IsHashedName), with pwa.CacheImmutable. Fixed names keep
   // Cloudflare's default (revalidate with ETag), which is what pwa.CacheRevalidate asks for.
   // Paths are sorted so the text is deterministic.
   func headersRules(manifest map[string]assetEntry) string
   ```
   Each rule is exactly `"<path>\n  Cache-Control: " + pwa.CacheImmutable + "\n"`. Empty string when
   no path is hashed. Cloudflare allows at most 100 rules: if more than 100 hashed paths exist, return
   an error instead (`goflare: %d content-hashed files exceed the 100 rules of _headers`) — change
   the signature to `(string, error)`.
2. `cloudflare.go` `Deploy`: when `hasAssets`, compute `rules, err := headersRules(manifest)` and,
   when non-empty, add `"_headers": rules` to the `assets.config` map next to `html_handling`,
   `not_found_handling`, `run_worker_first`.
3. **Verify the field name against Wrangler's source before writing it**: in
   `cloudflare/workers-sdk`, search `packages/wrangler/src` for `_headers` in the code that builds
   the script-upload metadata (`assets.config`). If Wrangler uses a different key or shape, use
   Wrangler's, and write what you found (file and line) in the PR description. If you cannot reach
   that repository, keep `_headers` and say so in the PR description.

## Stage 2 — refuse large artifacts as assets

In `buildAssetManifest` (`assets.go`), any file whose key starts with `pwa.ArtifactsDir` makes the
function return this error (constant `errArtifactsAsAssets`):
`goflare: %d files under /artifacts/ cannot be Workers static assets (25 MiB per file); serving artifacts from R2 is not implemented yet` (count).
Use `pwa.ArtifactsDir` in the check, never the literal.

## Stage 3 — the dev server test

`tests/devserver_test.go` `TestDevServer` talks plain `http://` to `httpd`, which serves HTTPS by
default since `webtyp.com/server` v0.2.67. The test is about routing, not TLS: add
`TLS: httpd.TLSConfig{PlainHTTP: true}` to its `httpd.Config` with a one-line comment saying so.
Run `go get webtyp.com/server@latest webtyp.com/pwa@latest` first.

## Stage 4 — tests

Follow [docs/TESTING.md](TESTING.md) for where each test goes. Required cases:

| Test | Proves |
|---|---|
| `TestHeadersRules_OnlyHashedNamesImmutable` | manifest keys `/`, `/sw.js`, `/style.3f9a1c2b.css`, `/client.9a8b7c6d.wasm` → exactly two rules, sorted, each with `pwa.CacheImmutable` |
| `TestHeadersRules_NoneHashed` | → `""`, no error |
| `TestHeadersRules_TooMany` | 101 hashed keys → the error |
| `TestDeploy_SendsHeadersInAssetsConfig` | with the existing fake Cloudflare API of the deploy tests, a PublicDir holding `style.3f9a1c2b.css`: the metadata part's `assets.config._headers` contains `/style.3f9a1c2b.css` and `immutable` |
| `TestBuildAssetManifest_RefusesArtifacts` | a PublicDir with `artifacts/x.bin` → the error text, naming the count |
| `TestDevServer` | passes again with `PlainHTTP` |

## Stage 5 — docs

`README.md` (deploy section) and `docs/ARCHITECTURE.md`: one paragraph each — hashed names get
`immutable` through `_headers` generated from `pwa.CacheControl`'s rule; `/artifacts/` is refused
until R2 serving exists (D-PWA-16).

## Acceptance

- `gotest` green (`go install webtyp.com/devflow/cmd/gotest@latest`). Never run `gopush`/`codejob`.
- `grep -rn '"/artifacts/"\|immutable' --include=*.go . | grep -v _test` → no literal outside the
  rule text built from `pwa.CacheImmutable` (the literal `immutable` must not appear in goflare code).

| Stage | Files | Done when |
|---|---|---|
| 1 | `assets.go`, `cloudflare.go` | `_headers` in the metadata |
| 2 | `assets.go` | `/artifacts/` refused |
| 3 | `tests/devserver_test.go`, `go.mod` | dev server test green |
| 4 | tests | table green |
| 5 | `README.md`, `docs/ARCHITECTURE.md` | written |
