---
PLAN: "refactor!: Exec reports rows affected and a duplicate key (storage.Result, storage.Duplicate) — goflare"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit after every stage, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.

## Goal

`webtyp/storage` v0.2.0 changed the executor contract (decision D48 of the architecture master, so that `crud` can do a conditional update and report a duplicate key):

| Before | After |
|---|---|
| `Exec(query string, args ...any) error` | `Exec(query string, args ...any) (storage.Result, error)` |
| a unique violation = the driver's own error | a unique violation = `storage.Duplicate(driverErr)`, tested with `storage.IsDuplicate(err)` |

`storage.Result` has one method, `RowsAffected() int64`; `storage.Affected(n)` is the value every implementation returns. Nothing else changes in this repo: no new feature, no rename.
Wave 4 of the migration; the repos it depends on are published (ddl, sqlt, cloudflare).

## Ecosystem search

Searched `Exec(` over `webtyp/*` and `veltylabs/*`: the implementers and callers of the old signature are the ones listed here; the compiler reports any site missed. No new library.

## Design gate

1. **Prior art**: `database/sql.Result` (`RowsAffected`), `sql.DB.Exec`.
2. **Novice-name test**: `result.RowsAffected()` = how many rows changed; `storage.IsDuplicate(err)` = the key already exists.
3. **Ledger**: concepts 0 here (the contract is storage's); ways to learn that a write changed nothing: 0 → 1.
4. **Where**: the contract is `webtyp/storage`; this repo adapts to it.
5. **Deletes**: the old single-value `Exec` shape.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/storage@v0.2.1
go get webtyp.com/ddl@v0.0.49
go get webtyp.com/sqlt@v0.0.17
go get webtyp.com/cloudflare@v0.0.73
go mod tidy
```

## What to change

`d1_migrate.go`: `d1Migrator` implements `ddl.Execer` (its method changed in the `ddl` plan): `Exec` returns `(storage.Result, error)`. `m.client.post` returns the D1 response body: parse `result[0].meta.changes` (read `post` and the response shape first) and return `storage.Affected(changes)`; a DDL statement reports 0. Any other `Exec` the compiler reports follows the same rule.

Rules for every call site (the compiler lists them: follow its errors, then `grep -rn "\.Exec(" --include=*.go .`):

| Old | New |
|---|---|
| `err := x.Exec(…)` | `_, err := x.Exec(…)` |
| `if err := x.Exec(…); err != nil {` | `if _, err := x.Exec(…); err != nil {` |
| `return x.Exec(…)` in a function that returns `error` | `_, err := x.Exec(…)` then `return err` |
| a type with `Exec(query string, args ...any) error` | `Exec(query string, args ...any) (storage.Result, error)`; return `storage.Affected(n)` with the real count, or `storage.Affected(0)` when it executes nothing (a test double) |

Never ignore the count where the caller can use it, and never invent a count: a driver that cannot know it fails the `storage/conformance` suite and the delivery.

## Delivery check

1. `gotest` passes (builds, vets, runs the tests; a PR that does not compile is a failed delivery).
2. `gotest` is green in every package of this repo (there is no driver conformance test here).
3. `grep -rnE "\) Exec\(([a-z]+) string, ([a-z]+) \.\.\.any\) error" --include=*.go .` → empty (no implementer with the old shape).
4. `git diff --stat main` lists only the files named above, `go.mod`, `go.sum` and the docs that showed `Exec`.
