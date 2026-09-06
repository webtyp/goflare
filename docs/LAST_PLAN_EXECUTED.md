---
PLAN: "fix: deploy edge.js/edge.wasm multipart parts as ES module, not octet-stream"
EXECUTOR: jules
REVIEWER: none
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

# Plan — `webtyp/goflare`: fix "Main module must be an ES module" on `goflare deploy`

## The problem, in one line

`goflare deploy` uploads `edge.js` (the Worker's `main_module`) with
`Content-Type: application/octet-stream`. Cloudflare cannot tell it's an ES
module and rejects the whole deploy.

## Production evidence

`veltylabs/iam`'s CI (`.github/workflows/deploy.yml`, step "Deploy with
goflare") failed with:

```
Error: deploy failed: CF API PUT /accounts/***/workers/scripts/iam → HTTP 400: Uncaught TypeError: Main module must be an ES module.
--- Deployment Summary ---
[-] Worker: Failed - CF API PUT /accounts/***/workers/scripts/iam → HTTP 400: Uncaught TypeError: Main module must be an ES module.
 (code: 10021)
```

This is not iam-specific. `veltylabs/misitio` deploys the same shape of
Worker (script + static assets) through the same `goflare deploy` code path,
and every one of its `Deploy` runs on `main` has also failed (see
`gh run list --workflow=deploy.yml` in that repo) — though for an unrelated
reason (a stale local `replace` directive in its own `go.mod` that doesn't
resolve in CI; out of scope for this plan, tracked separately). No deploy of
a *script*-type Worker (one with `edge/main.go`) has ever actually
succeeded against the real Cloudflare API through `goflare deploy` — this
bug has been present since the function that causes it was first introduced
(commit `9e3234a`, Feb 2026) and was never exercised end-to-end until now.

## Root cause

Confirmed against Cloudflare's own OpenAPI spec for
`PUT /accounts/{account_id}/workers/scripts/{script_name}` and
<https://developers.cloudflare.com/workers/configuration/multipart-upload-metadata/>:
the `files` parts of the multipart body accept exactly these Content-Types:

```
application/javascript+module, text/javascript+module, application/javascript,
text/javascript, text/x-python, text/x-python-requirement, application/wasm,
text/plain, application/octet-stream, application/source-map
```

`application/javascript+module` (or `text/javascript+module`) is what tells
Cloudflare a part is an **ES module** — required for any part referenced by
`metadata.main_module`. `application/javascript` (no `+module`) is the
legacy Service Worker syntax, used with `body_part` instead. Cloudflare's
own curl examples upload it like this:

```bash
--form 'index.js=@-;filename=index.js;type=application/javascript+module'
```

`addFilePart` in [`cloudflare.go`](cloudflare.go) builds every multipart
part with `multipart.Writer.CreateFormFile`:

```go
func addFilePart(mw *multipart.Writer, fieldName, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()
	part, err := mw.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}
```

`multipart.Writer.CreateFormFile` is Go standard library — it **always**
sets `Content-Type: application/octet-stream` on the part, with no way to
override it. Both `edge.js` (the ES module main_module) and `edge.wasm` are
uploaded this way, so Cloudflare sees an opaque blob where it expects an ES
module and returns error 10021.

`assets.go` already solves the identical problem correctly for asset parts
— it builds an explicit `textproto.MIMEHeader` and calls `mw.CreatePart(h)`
directly instead of `CreateFormFile` (see `uploadAssets`, around line
150). `addFilePart` never received the same treatment. This plan applies
the same, already-proven pattern to it.

## Stage 1 — fix `addFilePart` (`cloudflare.go`)

Add two unexported constants near the top of `cloudflare.go`, next to the
existing `cfAPIBase` constant:

```go
const (
	contentTypeESModule = "application/javascript+module"
	contentTypeWasm     = "application/wasm"
)
```

Add `"net/textproto"` to the import block (it is not yet imported in this
file).

Replace `addFilePart` with a version that takes an explicit content type and
builds the part header manually — mirroring `assets.go`'s existing
`uploadAssets` pattern exactly (same header-construction shape, same
`mw.CreatePart` call):

```go
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
```

Note: this does not escape quotes in `fieldName`/`filename` (neither does
`assets.go`'s identical pattern for its own `Content-Disposition`). Do not
add escaping here — it would be inconsistent with `assets.go`, and both
call sites only ever pass fixed literals (`"edge.js"`, `"edge.wasm"`) or
paths goflare itself generated, never attacker- or user-supplied strings.

Update the two call sites inside `Deploy()`:

```go
	if hasScript {
		if err := addFilePart(mw, "edge.js", edgeJsPath, contentTypeESModule); err != nil {
			return err
		}
		if err := addFilePart(mw, "edge.wasm", edgeWasmPath, contentTypeWasm); err != nil {
			return err
		}
	}
```

## Stage 2 — test (already written, currently RED)

[`tests/deploy_test.go`](../tests/deploy_test.go) already contains
`TestDeploy_ScriptPart_UsesESModuleContentType`, plus a small extension to
the shared `capturedMetadata`/`captureDeployPUT` helpers (used by every
other test in that file) so they also expose the Content-Type header of the
`edge.js`/`edge.wasm` multipart parts. Confirmed failing today:

```
$ go test ./tests/... -run TestDeploy_ScriptPart_UsesESModuleContentType -v
    deploy_test.go:213: edge.js part Content-Type = "application/octet-stream", want "application/javascript+module" — Cloudflare rejects anything else on main_module with: Main module must be an ES module
    deploy_test.go:217: edge.wasm part Content-Type = "application/octet-stream", want "application/wasm"
--- FAIL: TestDeploy_ScriptPart_UsesESModuleContentType (0.00s)
```

Do not modify this test. Stage 1 must make it pass with no other test in
the package regressing.

## Stage 3 — documentation

Update [`docs/BUILD_WORKER_ASSETS.md`](../docs/BUILD_WORKER_ASSETS.md),
"Fase 3 — Despliegue del Worker" section: after the existing `metadata`
JSON example, add a short subsection documenting the per-part Content-Type
contract, so this never regresses silently again:

```markdown
Cada parte del multipart lleva su propio `Content-Type` — Cloudflare lo usa
para distinguir un módulo ES de un blob opaco, independientemente del
nombre de campo:

| Parte | Content-Type |
|---|---|
| `edge.js` (`main_module`) | `application/javascript+module` |
| `edge.wasm` | `application/wasm` |

> *Nunca uses `multipart.Writer.CreateFormFile` para estas partes — la
> librería estándar de Go fija `application/octet-stream` sin posibilidad
> de override, y Cloudflare responde `Main module must be an ES module.`
> (code 10021). Construye la cabecera a mano con `textproto.MIMEHeader` +
> `mw.CreatePart`, como ya hace `uploadAssets` en `assets.go`.*
```

## Acceptance criteria

- [ ] `go test ./tests/... -run TestDeploy -v` — all pass, including
      `TestDeploy_ScriptPart_UsesESModuleContentType`.
- [ ] `go test ./...` — full suite green, no regressions.
- [ ] `grep -n "CreateFormFile" cloudflare.go` → empty (no remaining
      hardcoded-content-type part uploads in this file).
- [ ] `docs/BUILD_WORKER_ASSETS.md` documents the per-part Content-Type
      table above.
- [ ] `go vet ./...` clean.

| Stage | File(s) | Done when |
|---|---|---|
| 1 | `cloudflare.go` | `addFilePart` takes an explicit content type; both call sites pass `contentTypeESModule`/`contentTypeWasm` |
| 2 | `tests/deploy_test.go` | Already written — `TestDeploy_ScriptPart_UsesESModuleContentType` passes |
| 3 | `docs/BUILD_WORKER_ASSETS.md` | Content-Type table added under "Fase 3" |
