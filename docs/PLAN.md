---
PLAN: "fix!: derive run_worker_first from the project's declared routes"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 1792881506691849070
PR: https://github.com/webtyp/goflare/pull/27
---

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase 2a of ROUTES_SINGLE_SOURCE_MASTER_PLAN.md.
> **Blocked until `webtyp.com/router/routescan` is published** (phase 1 gate).

## Prerequisite — install the test runner

External agents run in isolated environments where `gotest` is not installed.
Run this **before anything else**; the acceptance criteria depend on it:

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

Then use `gotest` for the whole suite and `gotest -run TestName` for one test.
Never call `go test` directly: `gotest` handles `-vet`, `-race`, `-cover`, the
WASM suite and the README badges.

# Plan — `run_worker_first` from declared routes

## The defect being fixed

`goflare` deploys a Cloudflare Worker with static assets. Cloudflare needs to
know which paths must reach the Worker **ahead of** the asset layer; `goflare`
sends that list as `run_worker_first` in the deploy metadata
(`cloudflare.go:210`). Today the list is a hardcoded package variable
(`config.go:35`):

```go
// WorkerFirstRoutes are the prefixes Cloudflare must send to the Worker ahead
// of the static assets. This is not project configuration: /api/ is the route
// convention of webtyp/router and /oauth/ is mounted by webtyp/user. A
// project using the ecosystem is correct without declaring anything.
var WorkerFirstRoutes = []string{"/api/*", "/oauth/*"}
```

The comment's assumption is false for any project that answers on a path
outside those two prefixes. Such a request is matched against the static assets
first, matches no file, and falls to `not_found_handling` — whose default is
`single-page-application`, so Cloudflare returns `index.html` with **HTTP 200**.

The Worker never runs. The caller receives a page instead of a response. Nothing
is written to any log. `webtyp.com`, whose Go vanity-import resolver answers on
`/<module>`, would ship broken and silent.

## The fix

Derive the list from the routes the project actually declares, read at build
time from `routes/routes.go` via `webtyp.com/router/routescan`.

### Stage 1 — dependency and root

`go get webtyp.com/router@latest`.

Add to `Config` (`goflare.go:48`), in the "Build inputs" block:

```go
    RootDir string // project root used to locate routes/routes.go; default "."
```

Default it to `"."` in `LoadConfigFromEnv` alongside the other defaults. It is a
**convention, not a `.env` key** — do not add an `EnvKey` constant for it. It
exists so tests can point at a fixture directory.

### Stage 2 — derive the list

Add to `config.go`:

```go
// workerFirstRoutes returns the path prefixes Cloudflare must route to the
// Worker before the static assets, derived from the project's declared routes.
//
// A route's path becomes a prefix pattern: "/api/contacto" → "/api/contacto",
// and a path containing a "{param}" segment is truncated at that segment and
// suffixed with "*": "/api/orders/{id}" → "/api/orders/*".
// Duplicates are removed; order is source order.
func (g *Goflare) workerFirstRoutes() ([]string, error)
```

Rules:

- `routescan.Scan(g.Config.RootDir)` provides the declarations.
- A `PublicDir` prefix becomes `<prefix>*`.
- **When the scan returns no routes, return an empty slice, not the old
  default.** A project with no `routes/routes.go` has no dynamic paths; sending
  the two guessed prefixes for it is exactly the behaviour being deleted.
- A scan error is returned to the caller; the deploy must fail. Do not fall back
  to a default list — a silent fallback reintroduces the defect.

Replace the use at `cloudflare.go:210` with the derived value. `Deploy()`
returns the scan error unchanged.

### Stage 3 — delete the variable

```go
var WorkerFirstRoutes = []string{"/api/*", "/oauth/*"}
```

is deleted, together with its comment. Acceptance criterion 1 below proves it.

### Stage 4 — the `not_found_handling` note

`DefaultNotFoundHandling = "single-page-application"` **stays as it is**. It is
correct for an application whose client does its own routing. Do not change it.

Add to `docs/BUILD_WORKER_ASSETS.md`, in the "Enrutamiento y `WorkerFirstRoutes`"
section, replacing that section's body: `run_worker_first` is now derived from
`routes/routes.go`; a project whose route is missing from that file will have its
request answered by the asset layer. State the failure mode explicitly — HTTP
200 with `index.html`, no log — because it is invisible otherwise.

## Constraints

- **No hardcoded strings.** The `"*"` suffix, the `"{"` parameter marker and
  every error message are named constants in the package.
- **No logic duplication.** `workerFirstRoutes()` is the single producer of the
  list; `cloudflare.go` calls it and never rebuilds it inline.
- This repository is backend tooling: the standard library is legitimate here.
  Do not "fix" stdlib imports.

## Tests

Extend the existing test file that covers deploy metadata. Table-driven, each
case writing a `routes/routes.go` into `t.TempDir()` and setting
`Config.RootDir` to it:

1. Two `/api/*` routes → `["/api/contacto"]`-shaped output, deduplicated.
2. A route outside `/api/` (`r.Get("/dom", h)`) → present in the output. This is
   the regression test for the defect; it must fail against `main` today.
3. `"/api/orders/{id}"` → `"/api/orders/*"`.
4. `PublicDir("/static", "web/public")` → `"/static*"`.
5. No `routes/routes.go` → empty slice, nil error.
6. A route path that is a variable → `Deploy` returns the `routescan` error and
   sends no request.

## Acceptance criteria

1. `grep -rn "WorkerFirstRoutes" --include='*.go' .` → empty.
2. `grep -rn '"/oauth/\*"\|"/api/\*"' --include='*.go' . | grep -v _test` → empty.
3. `go build ./... && go vet ./... && go test ./...` → clean.
4. Test case 2 passes.

## Stages

| # | Stage | File(s) | Gate |
|---|---|---|---|
| 1 | dep + `RootDir` | `goflare.go`, `config.go` | compiles |
| 2 | `workerFirstRoutes()` | `config.go`, `cloudflare.go` | tests 1–6 |
| 3 | delete the var | `config.go` | criteria 1, 2 |
| 4 | docs | `docs/BUILD_WORKER_ASSETS.md` | — |

Sequential.
