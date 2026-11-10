---
PLAN: "refactor: goflare takes its retry delay from webtyp.com/backoff and deletes its private doubling"
EXECUTOR: jules
REVIEWER: none
STATUS: review
SESSION: 9165095695805900288
PR: https://github.com/webtyp/goflare/pull/35
REVIEWED_COMMIT: 486be2bee09dd0d8da42c7e527ee0cfe8313fc6b
---

> Read AGENTS.md at the repo root before any change.
> **Finish by yourself:** when the delivery check passes, commit, push and open the pull request. Never ask whether to
> proceed or finalize — nobody answers, and a session that waits ends without your work. Commit after every stage.
> Never run `git reset`, `git restore`, `git checkout -- <file>` or `git stash` on your own work.
> A PR that does not compile, or whose tests could not run, is a failed delivery. Stuck for 30 minutes: open the PR anyway and list what fails.

# Plan: `goflare` uses `backoff`

`cloudflare.go` (`//go:build !wasm`) has `func (g *Goflare) retry(n int, base time.Duration, fn func() error) error` that doubles the wait with `base << i`. The ecosystem now has one library for that: `webtyp.com/backoff` (decision D59). This plan makes `retry` use it. **Behaviour does not change**: the same waits between the same attempts.

## Ecosystem search

`webtyp.com/backoff` `v0.1.0` computes «delay after N failures». **Reuse**. No exported symbol of `goflare` changes (`RetryBackoff time.Duration` stays).

## Design gate

Not required: no exported symbol is added, renamed or removed. Ledger: ways to compute a retry delay in this repo `−1`.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
export PATH="$PATH:$(go env GOPATH)/bin"
go get webtyp.com/backoff@v0.1.0 webtyp.com/time@v0.6.4
```

Environment: `GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`; `go mod tidy` after `go get`.

## APIs used

| symbol | exact signature | tag | file |
|---|---|---|---|
| `backoff.New` | `func New(first, longest time.Nano) (Policy, error)` (`time` is `webtyp.com/time`; `time.Nano` is `type Nano int64`, nanoseconds, the same unit as `time.Duration`) | `webtyp.com/backoff v0.1.0` | `backoff.go` |
| `backoff.Policy.DelayAfter` | `func (p Policy) DelayAfter(failures int64) (time.Nano, error)`: 1 failure or fewer → first delay; doubles after each further failure; never above `longest` | `webtyp.com/backoff v0.1.0` | `backoff.go` |
| `backoff.ErrFirstDelayNotPositive`, `backoff.ErrLongestBelowFirst` | errors of `New` when `first <= 0` or `longest < first` | `webtyp.com/backoff v0.1.0` | `backoff.go` |

If a signature in the published tag differs from this table, stop and name the symbol in the PR; never adapt it by guessing.

## Stage 1 — `cloudflare.go`

This file is backend tooling (`!wasm`): stdlib `time` is legitimate here, do NOT «fix» it. The new import needs an alias because the file already imports stdlib `time`: `webtime "webtyp.com/time"` (alias allowed in this `!wasm` file only).

1. Imports: add `"webtyp.com/backoff"` and `webtime "webtyp.com/time"` (keep the file's import order).
2. Replace the whole function `retry` (the comment line above it included) with exactly:

```go
// retry runs fn up to n times. Between two failed attempts it waits what webtyp.com/backoff gives: base after the first
// failure, doubling after each further one. The longest delay is base<<n, above every delay this loop asks for, so
// the cap never applies and the waits are the same as the old base<<i.
func (g *Goflare) retry(n int, base time.Duration, fn func() error) error {
	if n <= 0 {
		return nil
	}
	policy, err := backoff.New(webtime.Nano(base), webtime.Nano(base)<<uint(n))
	if err != nil {
		return err
	}
	for attempt := 1; attempt <= n; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if attempt < n {
			delay, delayErr := policy.DelayAfter(int64(attempt))
			if delayErr != nil {
				return delayErr
			}
			time.Sleep(time.Duration(delay))
		}
	}
	return err
}
```

Do not change the call site `g.retry(5, g.RetryBackoff, …)` (`cloudflare.go:526`). Behaviour note: a `RetryBackoff` of zero or less now fails at once with `backoff.ErrFirstDelayNotPositive` instead of retrying with no wait; `New` sets it to `time.Second` and the tests set `time.Millisecond`, so nothing in the repo does that.

3. Run `go mod tidy` (`go.mod` requires `webtyp.com/backoff v0.1.0` and `webtyp.com/time`). Run `gotest`: every test passes, including `tests/deploy_verify_test.go` (it sets `RetryBackoff = time.Millisecond`). No new test: the delays are proven by `backoff`'s own tests and the repo's tests cover the retry loop. Commit.

Deletions, each verified: `grep -n "base << i\|base<<i" cloudflare.go` → empty.

## Stage 2 — documentation

`docs/ARCHITECTURE.md`: add one sentence where the deploy verification probe is described (find it with `grep -n -i "probe\|retry" docs/ARCHITECTURE.md docs/DEPLOY_TARGET.md`): «The probe retries up to 5 times; the wait between attempts comes from `webtyp.com/backoff` (`RetryBackoff`, doubling).» If neither file mentions the probe, add the sentence at the end of the «Deploy» section of `docs/ARCHITECTURE.md`. Permanent docs never link `docs/PLAN.md`. English. Commit.

## Delivery check

1. `gotest` — builds, vets and runs every test; all pass.
2. `grep -n "base << i\|base<<i" cloudflare.go` → empty; `grep -n "webtyp.com/backoff" go.mod cloudflare.go` → one line in each.
3. `git diff --stat main` lists exactly: `cloudflare.go`, `go.mod`, `go.sum`, `docs/ARCHITECTURE.md` (or `docs/DEPLOY_TARGET.md`), and badge files `gotest` rewrites.

## Stages

| # | Stage | Files |
|---|---|---|
| 1 | use `backoff`, delete the doubling | `cloudflare.go`, `go.mod`, `go.sum` |
| 2 | documentation | `docs/ARCHITECTURE.md` |
