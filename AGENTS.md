# Agent Guide — `webtyp/goflare`

<!-- webtyp:base — copied from devskills skill agents-md; do not edit here -->
## Ecosystem rules — read before any change

`docs/PLAN.md` is the spec: no design judgment; anything unclear is a question in the PR, not a guess.

- Public API: typed (no `any`/`interface{}`), explicit, minimal; never export a symbol only a test uses.
- One intent, one path: the old path is deleted in the same change. No silent fallback, `TODO`, commented code or stub.
- Fail at compile time, else a loud diagnostic — never silently. Zero values deny.
- A defect in a dependency is reported, never wrapped or patched here.
- No string literals in logic: named constants. The same logic never lives in two repos: import the package the plan names; if you find an existing one the plan does not name that already does what you are about to write, say so in the PR instead of writing it.
- `cmd/*/main.go`: parse flags, inject, print/exit only. stdout = data, stderr = logs, exit codes are the contract.
- WASM code: `webtyp/fmt`, `webtyp/time`, `webtyp/json` instead of stdlib; no `map`; `dom.Element` embedded by value.
- `css.go`, `svg.go`, `js.go`, `html.go` carry `//go:build !wasm`; no `ssr.go`, no `front.go`.
- Build tags: `wasm`/`!wasm` only when the API does not exist on that platform; `browser`/`!browser` when the file belongs to another binary (`web/browser.go` → `browser.wasm`, `web/edge.go` `wasm && !browser` → `edge.wasm`, `web/server.go` `!wasm` → `server`).
- Code and keys in English; UI text is `lang.Text`, translations live in `lang.json`.
- Tests cover behaviour and domain rules, never the look (CSS, colours, markup order, class strings). In `tests/` (external package), real stack with fakes only at the edge; run `gotest`.
- Docs (`README.md`, `docs/*`) updated with the API; permanent docs never link `docs/PLAN.md`.
- Before the PR: `git diff --stat main` lists every file of the plan's stages, and nothing the plan did not order is deleted.
<!-- /webtyp:base -->

## This repo

Constraints for agents working on this library. Read this before any change.

---

## Scope — single responsibility

`goflare` is a **deployment runtime for Cloudflare**: it builds a Go project into the edge
artifacts Cloudflare expects, deploys them, and provides the **edge runtime** the deployed
code runs on.

- It **implements** the routing contract `webtyp.com/router` — it does **not** define
  it. Anything shaped like a `Context`, `Router` or `HandlerFunc` belongs in `webtyp/router`,
  never here.
- The native (`!wasm`) dev server is **not** ours: it is `webtyp.com/server/httpd`,
  the ecosystem's only native implementor. Do not write a second one.
- Business logic never lives here.

## Two build targets — know which one you are in

This is the single most important thing to get right in this repo.

| Target | Packages | Runs where | Rules |
|---|---|---|---|
| **`wasm`** | **`webtyp/cloudflare`** (`edge/`, `workers/`, `d1/`, `r2/`, `log/`, `env_wasm.go`) — see `webtyp/cloudflare/AGENTS.md` for isolate lifecycle, `runtime.ticks` ABI, and shared-global constraints | **Inside the Cloudflare Worker** | **No standard library.** Use `webtyp/fmt` instead of `errors`/`fmt`/`strconv`/`strings`. Talks to the runtime through `syscall/js`. |
| **`!wasm`** | `build.go`, `mode.go`, `config.go`, `cloudflare.go`, `devserver/`, `cmd/` | The developer's machine / CI | **The standard library is correct and expected here** — `net/http`, `go/parser`, `os`, `strings`. |
| **`actiongen`** | `actiongen/` | Generator tool | **Strictly standard library only.** No ecosystem dependencies (`webtyp/*`). Designed for extraction to its own repository. |

> ⚠️ `action.yml` in the root of the repository is **generated code** produced by `actiongen` and synchronized by `TestActionYmlIsInSync`. Never edit `action.yml` manually — modify `action_data.go` and run `go test ./tests/`.

> ⚠️ **Anti-footgun.** The ecosystem rule "no stdlib in WASM code" applies **only to the `wasm`
> column**. Do not "fix" stdlib imports in host tooling — that code never reaches a browser or
> a Worker, and purging it breaks the build for no reason.
>
> Runtime invariants (isolate lifecycle, `runtime.ticks` BigInt ABI, `context.binding` vs `globalThis`) are documented in **`webtyp/cloudflare/AGENTS.md`** — read it before touching `javascripts.go` or any bundling logic. Do not duplicate that guidance here.

## Testing

**Read [docs/TESTING.md](docs/TESTING.md) before writing a single test.** It defines the three
tiers and the rule for choosing one.

**Tests live in `tests/`**, as `package goflare_test` — black-box, through the public API. The
only exception is a test that must reach an **unexported** symbol: it goes next to the code as
`*_internal_test.go` (see `build_internal_test.go`).

The short version: our edge code talks to `js.Global()`, not to Cloudflare — so we inject a
fake `context.env` and test the real code path in a browser. **Deploying is not a test**, and
`wrangler` is reserved for a tiny smoke tier that proves the fake does not lie.

```bash
go install webtyp.com/devflow/cmd/gotest@latest
gotest        # never `go test` — dual WASM/stdlib, browser-driven
```

Stdlib assertions only. Publish with `gopush 'message'`.

## Fail loudly, never silently

A misconfiguration must break the build or the request — never degrade into a wrong result.
This repo has been bitten by exactly that: a body decoded as text corrupted binaries with no
error, and an unrecognized import silently produced the wrong deploy artifact. When in doubt,
return an error that **names what was missing**.

## Documentation First

Update docs **before** code and before `gopush`: keep `docs/PLAN.md` (the master index) and
`docs/ARCHITECTURE.md` in sync, and re-index `README.md` so every `docs/` file is linked.
Diagrams use `flowchart TD`, no `subgraph`, `<br/>` for line breaks.
