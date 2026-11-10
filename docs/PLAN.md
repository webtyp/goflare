---
PLAN: "refactor: backoff v0.2.0 (time.Duration delays)"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 6419781049252910824
PR: https://github.com/webtyp/goflare/pull/36
REVIEWED_COMMIT: d3d2483c89214121efac666c9a383448f6434d04
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> **Publishing:** you publish only by OPENING THE PULL REQUEST (a `git push` from your sandbox does not reach GitHub, and work not submitted is lost when the session ends).
> **Never ask a yes/no question about progress**; the answer is always yes. Ask only when a signature in this plan is wrong, and name the symbol.
> Never write `go.mod`/`go.sum` lines by hand: only `go get` and `go mod tidy` do. No scratch files and no scripts over the source.

# Plan: `goflare` on `backoff` v0.2.0

## Why (decided: P47 B of the architecture master)

`webtyp.com/time` v0.6.5 has a type for a wait, `time.Duration` (nanoseconds, never an instant), with `time.Nanosecond`, `time.Microsecond`, `time.Millisecond`, `time.Second`, `(Duration).Milliseconds() int64` and `(Nano).Add(Duration) Nano`. `webtyp.com/backoff` v0.2.0 takes and answers `time.Duration` instead of `time.Nano` (an instant): `New(first, longest time.Duration) (Policy, error)` and `(Policy).DelayAfter(failures int64) (time.Duration, error)`. This repo called the v0.1.0 API with `time.Nano`; this plan moves it to the new one. Behaviour does not change.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go get webtyp.com/backoff@v0.2.0 webtyp.com/time@v0.6.5
go mod tidy
```
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`; no `replace` in `go.mod`. If a module cannot be fetched, stop and say so in the PR; never invent a hash. If a signature in the published tags differs from this plan, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1 — `cloudflare.go`

In `(*Goflare).retry` (today `cloudflare.go:632`) the policy is built from two `time.Duration` values the function already receives. Replace that one line with exactly:

```go
	policy, err := backoff.New(webtime.Duration(base), webtime.Duration(base)<<uint(n))
```
(`webtime` is the existing alias of `webtyp.com/time`; `time` is the stdlib, which this file legitimately uses.) The line `time.Sleep(time.Duration(delay))` stays as it is: converting a `webtime.Duration` to the stdlib `time.Duration` is the same nanosecond count. Update the doc comment of `retry` only if it names `time.Nano`.

Deletions: `grep -n "webtime.Nano" cloudflare.go` → empty.

## Stage 2 — tests

The tests that set `g.RetryBackoff = time.Millisecond` (`tests/deploy_verify_test.go:63,118`) are untouched. Run `gotest`; if a test does not compile because of this change, fix only the type and say which one in the PR.

`gotest` must end green on every destination it runs; a tree that does not compile or whose tests did not run is a failed delivery.
`git diff --stat main` lists `cloudflare.go`, `go.mod`, `go.sum`; nothing is deleted.
