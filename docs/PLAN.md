---
PLAN: "refactor!: build Worker from web/edge.go into web/ (OutputDir default web/), reject -tags browser"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 6212008180401272509
---

> Read AGENTS.md at the repo root before any change.

# goflare: build destination refactor (`web/edge.go` → `web/edge.wasm`, default `OutputDir = "web"`)

Part of [BUILD_DESTINATIONS_MASTER_PLAN.md](https://github.com/webtyp/docs/blob/main/BUILD_DESTINATIONS_MASTER_PLAN.md) (D1, D10, D12, D15):
- Every build destination in `web/` is named after its target: `web/edge.go` (`//go:build wasm && !browser`) → `web/edge.wasm` (+ `web/edge.js`), `web/browser.go` (`//go:build browser`) → `web/public/browser.wasm`, `web/server.go` (`//go:build !wasm`) → `web/server`.
- The `edge/` root directory no longer exists in any webtyp project: its `main.go` moved to `web/edge.go`.
- The `.build/` output directory no longer exists: all output stays in `web/`. Public assets go to `web/public/`, private worker artifacts stay next to their entry in `web/` (`web/edge.js` and `web/edge.wasm`). Default `OutputDir` becomes `"web"`.
- The Worker is defined by the **absence** of `browser`. `goflare` MUST reject `-tags browser` (D10): compiling a Worker with `browser` would cause symbol collisions or leak browser code into the Cloudflare Worker.

> **Anti-footgun:** `goflare` is build and deploy tooling. Standard library imports (`os`, `os/exec`, `path/filepath`, `strings`, `fmt`, etc.) are legitimate here — do **not** replace them.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Environment

```bash
export GOPRIVATE=webtyp.com/*,github.com/veltylabs/*
export GOFLAGS=-mod=mod
```

A PR that does not compile, or whose tests could not run, is a failed delivery. Never pause to ask permission or report progress: when the Delivery check passes, commit, push and open the PR. Stuck for 30 minutes: open the PR anyway listing what fails.

## Design gate

1. **Prior art:** WebTyp `BUILD_DESTINATIONS` D1, D10, D12 (`web/edge.go` → `web/edge.wasm`, default `OutputDir = "web"`, reject `-tags browser`).
2. **Novice-name test:** `c.OutputDir` defaults to `"web"`; `c.Entry` defaults to `filepath.Join("web", "edge.go")`.
3. **Complexity ledger:** −1 directory (`edge/` deleted), −1 build dir (`.build/` eliminated); +0 exported types.
4. **Where it belongs:** `goflare` owns Worker bundling and Cloudflare deployment.
5. **What it deletes:** `.build/` defaults and `edge/main.go` entry detection; all fallback checks to `edge/`.

---

## Stage 1 — Config defaults, entry auto-detection and D10 tag guard

Files: `config.go`, `goflare.go`, `errors.go`

1. In `errors.go`:
   - Add error constant:
     ```go
     ErrMsgBrowserTagForbidden = "goflare: the Cloudflare Worker cannot be compiled with -tags browser"
     ```
2. In `config.go`:
   - Update `c.OutputDir` default:
     Change line 217 from `c.OutputDir = ".build/"` to `c.OutputDir = "web"`.
   - Update `c.Entry` auto-detection:
     Replace the check for `filepath.Join("edge", "main.go")` with `filepath.Join("web", "edge.go")`:
     ```go
     if _, err := os.Stat(filepath.Join("web", "edge.go")); err == nil {
     	c.Entry = filepath.Join("web", "edge.go")
     }
     ```
   - In entry / build tag validation (e.g., in `Validate()` or `Prepare()`):
     If any build tag contains `browser` (check custom tags in config if passed): return an error wrapping `ErrMsgBrowserTagForbidden`.
3. In `goflare.go`:
   - In `New` / `NewProductionMode`:
     When setting up `sitec.WasmBuildOptions`:
     ```go
     edgeBuilder := sitec.NewWasmBuilder(false, sitec.WasmBuildOptions{
     	Entry:      filepath.Join("web", "edge.go"),
     	OutputName: "edge",
     })
     ```
   - Ensure the artifacts are written to `g.Config.OutputDir` (which is `"web"` by default): `filepath.Join(g.Config.OutputDir, "edge.js")` and `filepath.Join(g.Config.OutputDir, "edge.wasm")`.

---

## Stage 2 — Update tests and add rejection test

Files: `tests/`, `mode_internal_test.go`, `workerfirst_internal_test.go`

1. Replace all fixture directories named `edge` with `web/edge.go`:
   - Fixture file: `web/edge.go` with content `//go:build wasm && !browser\n\npackage main\n\nfunc main() {}\n` (or importing the runtime).
2. Update expected output artifact paths from `.build/edge.*` to `web/edge.*`.
3. Add a test asserting that if `-tags browser` is supplied, `goflare` returns `ErrMsgBrowserTagForbidden`.
4. Run `gotest ./...` and ensure all tests pass.

---

## Stage 3 — Update documentation

Files: `README.md`, `docs/ARCHITECTURE.md`, `docs/BUILD_WORKERS.md`

1. Replace `.build/` with `web/` for Worker output (`web/edge.js`, `web/edge.wasm`).
2. Replace `edge/main.go` and `edge/` directory references with `web/edge.go`.

---

## Deletions and acceptance criteria

- `grep -rn "\.build/" --include=*.go .` returns empty (except possibly comments documenting migration).
- `grep -rn "filepath.Join(\"edge\"" --include=*.go .` returns empty.
- `gotest ./...` passes with zero failures.

## Stages

| # | Files | Goal |
|---|---|---|
| 1 | `config.go`, `goflare.go`, `errors.go` | `OutputDir = "web"`, auto-detect `web/edge.go`, reject `-tags browser` |
| 2 | `tests/`, test files | Update fixtures to `web/edge.go` and verify D10 rejection |
| 3 | `README.md`, docs | Document `web/edge.go` and `web/` artifact locations |
