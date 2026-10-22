---
PLAN: "feat!: the Worker main is web/edge/main.go and builds into web/edge/"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 15823158798197945357
PR: https://github.com/webtyp/goflare/pull/33
REVIEWED_COMMIT: df9d8bee949991a5c3657d1c035515a1854b641a
---

> Read AGENTS.md at the repo root before any change.

# Worker main: `web/edge/main.go` → `web/edge/edge.wasm` + `web/edge/edge.js`

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

The ecosystem moves every binary main into its own folder under `web/` (the folder is the
binary): `web/browser/main.go` → `web/public/browser.wasm`, `web/server/main.go` →
`web/server/server`, `web/edge/main.go` → `web/edge/edge.wasm` + `web/edge/edge.js`. A main
carries no destination tag: its folder separates it. The Worker's static assets stay
`web/public` (served by Cloudflare). No backwards compatibility: `web/edge.go` stops being
detected. The `-tags browser` guard (`goflagsHaveTag`) does not change.

## Stage 1 — defaults

- `config.go` `applyDefaults` (or wherever `OutputDir`/`Entry` defaults live, lines ~248–262):
  `c.OutputDir = filepath.Join("web", "edge")`; entry auto-detection checks
  `filepath.Join("web", "edge", "main.go")` and sets `c.Entry` to it.
- `goflare.go`: the `sitec.NewWasmBuilder(... Entry: filepath.Join("web", "edge.go") ...)` →
  `Entry: filepath.Join("web", "edge", "main.go")`; the `OutputDir` field comment
  `// default: ".build/"` → `// default: "web/edge"`; the `Entry` field comment (if it names a
  path) → `web/edge/main.go`.
- `mode.go`: `ErrNoKnownImport` text `web/edge.go imports neither` →
  `web/edge/main.go imports neither`; the `validateEntry` comment likewise.

## Stage 2 — `events.go` (wrong today, made concrete by the new layout)

- `MainInputFileRelativePath` returns `filepath.Join(h.Config.Entry, "main.go")`, but `Entry` is
  already the main file. Return `h.Config.Entry`; delete the `// Simplified for now` comment.
- `UnobservedFiles` returns `_worker.js`, which goflare no longer writes. Return the two real
  outputs: `filepath.Join(h.Config.OutputDir, WasmArtifactName)` and
  `filepath.Join(h.Config.OutputDir, "edge.js")`; delete the commented-out line.
- Test (`tests/`, find the file that already covers events or add `tests/events_test.go`):
  `TestEventsPaths` — `cfg := &goflare.Config{Entry: "web/edge/main.go", OutputDir: "web/edge"}`,
  build the handler the way other tests do; expected `MainInputFileRelativePath()` ==
  `filepath.Join("web", "edge", "main.go")` and `UnobservedFiles()` == the two paths above.

## Stage 3 — tests

`mode_internal_test.go`, `tests/target_misc_test.go`, `tests/deploy_verify_test.go`,
`tests/build_test.go`, `tests/build_worker_assets_test.go`: every fixture `web/edge.go` →
`web/edge/main.go` (drop `//go:build wasm && !browser` from fixture mains: no destination tag);
every expected output `web/edge.wasm`/`web/edge.js` → `web/edge/edge.wasm`/`web/edge/edge.js`;
`Entry: "web/edge.go"` → `Entry: "web/edge/main.go"`. No assertion changes besides paths.

## Stage 4 — docs

`README.md`, `docs/QUICK_REFERENCE.md`, `docs/ARCHITECTURE.md`, `docs/BUILD_WORKERS.md`,
`docs/BUILD_WORKER_ASSETS.md`, `docs/diagrams/goflare-generic.md`: `web/edge.go` →
`web/edge/main.go`, `web/edge.wasm` → `web/edge/edge.wasm`, `web/edge.js` → `web/edge/edge.js`,
and remove any `//go:build wasm && !browser` shown for the Worker main. `AGENTS.md`: never edit
its `<!-- webtyp:base -->` block; outside it, same replacements.

## Delivery check

```bash
gotest
grep -rn 'web/edge\.go\|web/edge\.wasm\|web/edge\.js\|_worker\.js\|"\.build/"' --include=*.go --include=*.md . | grep -v docs/PLAN.md   # empty
```

A PR whose tests could not run is a failed delivery. Environment:
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.

Never pause to ask permission or report progress: when the delivery check passes, commit, push
and open the PR. Stuck for 30 minutes → open the PR anyway listing what fails.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `config.go`, `goflare.go`, `mode.go` | defaults `web/edge/main.go` → `web/edge/` |
| 2 | `events.go`, a test in `tests/` | watcher paths correct |
| 3 | tests listed above | fixtures follow the layout |
| 4 | docs listed above, `AGENTS.md` | docs |
