---
PLAN: "fix: exact -tags browser guard; action.yml no longer bakes a release tag"
EXECUTOR: jules
REVIEWER: none
---

> Read AGENTS.md at the repo root before any change.

# Exact browser-tag guard and a release-independent action.yml

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
```

## Why

1. `Config.ValidateBuild` (`config.go`) rejects the browser destination tag with
   `strings.Contains(os.Getenv("GOFLAGS"), "browser")`: it also rejects unrelated flags that
   merely contain the word (`-tags=nobrowser`, a module path with `browser` in it). The guard
   must match the tag exactly.
2. `GoflareAction` (`action_data.go`) bakes the latest git tag into `action.yml` as a last-resort
   download version. Every release changes the latest tag, so `tests/action_sync_test.go` fails
   on the first test run after each release (it regenerates the file and asks for a commit).
   The script already resolves the version from `inputs.version`, then `github.action_ref`, then
   the GitHub API `releases/latest`. When all three fail, the action must fail loudly instead of
   downloading a stale baked version (no silent fallback). `webtyp/gorelease` already works this
   way: its `GoreleaseAction(_ string)` ignores the version.

## Design gate (public API: `GoflareAction` signature, `LatestReleaseTag` removed)

1. **Prior art:** `webtyp/gorelease` `action_data.go` — its action resolves the version at run
   time and bakes nothing.
2. **Novice-name test:** `GoflareAction(tinyGoVersion)` — the only input is the one the action
   really bakes (the TinyGo cache key).
3. **Complexity ledger:** −1 parameter, −1 exported function (`LatestReleaseTag`), −1 fallback
   branch in the script, −1 doc rule ("one-version lag").
4. **Where it belongs:** stays in goflare.
5. **What it deletes:** the baked fallback, `LatestReleaseTag`, the `webtyp.com/git` import in
   `action_data.go` (if nothing else in the file uses it), the per-release `action.yml` diff.

## Stage 1 — exact tag guard

In `config.go` replace the `strings.Contains(os.Getenv("GOFLAGS"), "browser")` check with
`if goflagsHaveTag(os.Getenv("GOFLAGS"), browserBuildTag) {`. Add, in `config.go`:

```go
// browserBuildTag is the destination tag of the browser binary
// (web/browser.go). The Worker is defined by its absence.
const browserBuildTag = "browser"

// goflagsHaveTag reports whether goflags passes tag in a -tags flag, in any of
// the forms the go command accepts: -tags=a,b · -tags a,b · --tags=a,b.
func goflagsHaveTag(goflags, tag string) bool {
	fields := strings.Fields(goflags)
	for i, f := range fields {
		var list string
		switch {
		case strings.HasPrefix(f, "-tags="):
			list = strings.TrimPrefix(f, "-tags=")
		case strings.HasPrefix(f, "--tags="):
			list = strings.TrimPrefix(f, "--tags=")
		case (f == "-tags" || f == "--tags") && i+1 < len(fields):
			list = fields[i+1]
		default:
			continue
		}
		for _, t := range strings.Split(list, ",") {
			if t == tag {
				return true
			}
		}
	}
	return false
}
```

Test (`tests/build_test.go`): replace `TestBuild_BrowserTagForbidden` with a table test
`TestBuild_BrowserTagGuard` (use `t.Setenv`, not `os.Setenv`). Each row: `GOFLAGS` → error expected.

| GOFLAGS | error |
|---|---|
| `-tags=browser` | yes |
| `-tags=dev,browser` | yes |
| `-tags browser` | yes |
| `-mod=mod -tags=browser` | yes |
| `-tags=nobrowser` | no |
| `-tags=dev` | no |
| `-mod=mod` | no |
| (empty) | no |

When an error is expected, assert `errors.Is(err, goflare.ErrBrowserTagForbidden)`. Use the
same `Config` as today (`Entry: "web/edge.go"`, `ProjectName: "test"`, `AccountID: "123"`).

## Stage 2 — `action.yml` stops baking a release

In `action_data.go`:

1. Signature: `func GoflareAction(tinyGoVersion string) ghaction.Action`. Delete the
   `if goflareVersion == "" { goflareVersion = "v0.5.22" }` block.
2. In `downloadScript`, replace

```sh
# Last resort only. This literal is baked at generation time, so it can never
# name the tag being released right now.
if [ -z "$version" ]; then
  version="%s"
fi
```

with

```sh
if [ -z "$version" ]; then
  echo "goflare: cannot resolve which release to download (no 'version' input, the action ref is not a vX.Y.Z tag, and the GitHub API did not answer)." >&2
  echo "  Pin one with the 'version' input or use webtyp/goflare@vX.Y.Z." >&2
  exit 1
fi
```

   and turn the `fmt.Sprintf(...)` around `downloadScript` into a plain raw string (it has no
   other `%s`; verify with `grep -n '%s' action_data.go` that no verb is left in that script).
3. Delete `LatestReleaseTag` and the `webtyp.com/git` import if unused afterwards
   (`grep -rn "LatestReleaseTag" .` → empty).
4. `tests/action_sync_test.go`: drop the tag lookup and skip; call
   `goflare.GoflareAction(goflare.TinyGoVersion)`. Run the test once (it regenerates
   `action.yml`), commit the regenerated `action.yml`, run it again: green.
5. `docs/CI_GITHUB_ACTIONS.md`, section «Resolución de Versiones»: the list becomes
   1) `inputs.version`, 2) `github.action_ref` if it is a full semver tag, 3) the latest release
   from the GitHub API; if none resolves, the action fails with the message above. Delete the
   subsection «Desfase de una versión en `@v1`» (the baked version no longer exists).

## Delivery check

```bash
go build ./...
gotest
grep -rn "LatestReleaseTag\|v0.5.22" --include=*.go .     # empty
grep -n 'strings.Contains(os.Getenv("GOFLAGS")' config.go  # empty
grep -n "Desfase" docs/CI_GITHUB_ACTIONS.md                 # empty
```

`gotest` twice in a row must be green (the second run proves `action.yml` no longer changes).
A PR that does not compile, or whose tests could not run, is a failed delivery. Environment:
`GOPRIVATE=webtyp.com/*,github.com/veltylabs/*`, `GOFLAGS=-mod=mod`.

Never pause to ask permission or report progress: when the delivery check passes, commit, push
and open the PR. Stuck for 30 minutes → open the PR anyway listing what fails.

## Stages

| # | Files | Result |
|---|---|---|
| 1 | `config.go`, `tests/build_test.go` | exact `-tags browser` guard |
| 2 | `action_data.go`, `action.yml`, `tests/action_sync_test.go`, `docs/CI_GITHUB_ACTIONS.md` | no baked release; sync test stable across releases |
