---
PLAN: "feat!: goflare.Worker — deploy.Target for Cloudflare Workers: bootstrap token, per-project account token, D1/R2 by name, Worker secrets; rename the D1/R2 config"
EXECUTOR: jules
REVIEWER: none
STATUS: running
SESSION: 16051603333022940540
---

> Read AGENTS.md at the repo root before any change.

# Plan — goflare implements `deploy.Target`

Part of the "Despliegue guiado" wave (`https://github.com/webtyp/docs/blob/main/DEPLOY_WIZARD_MASTER_PLAN.md`,
decisions D9, D10, D18, D27, D29, D30, D34–D38, D40, and P26: non-secret runtime variables). Today a person deploying with goflare creates
a Cloudflare token by hand, creates D1 and R2 by hand, copies their IDs into GitHub secrets, and
loads Worker secrets in the dashboard. A hand-made token without `Workers Scripts:Edit` made every
`veltylabs/misitio` deploy fail with
`CF API PUT /accounts/***/workers/scripts/misitio → HTTP 403: No access to the specified resource.`
After this plan goflare does all of that itself, from each person's **bootstrap token**.

**Prerequisite:** `go get webtyp.com/deploy@latest` must provide `deploy.Target`, `deploy.Plan` (with `Vars`), `deploy.Var`,
`deploy.Destination`, `deploy.Missing`, `deploy.Job`, `deploy.TargetID`. If it does not, **stop**
and say so in the PR.

**Anti-footgun:** goflare is backend tooling (it runs on the developer's machine and in CI).
stdlib (`net/http`, `encoding/json`, `os`) is correct here; do NOT port it to `webtyp/fmt`. Only
code under `devserver/` or anything built for `wasm` follows the WASM rules.

## Tools

```bash
go install webtyp.com/devflow/cmd/gotest@latest
go install webtyp.com/lang/cmd/langc@latest
```

## Design gate

1. **Prior art.** Wrangler (`wrangler login`, `d1 create`, `secret put`, which creates an empty
   Worker when the script does not exist yet), Fly (`fly launch` creates what is missing), SST
   (resources from code). Cloudflare offers no OAuth to third-party tools, so the documented
   alternative is a token created with the template "Create additional tokens" that mints scoped
   tokens through `POST /accounts/{id}/tokens`
   (`https://developers.cloudflare.com/fundamentals/api/how-to/create-via-api/`).
2. **Names.** `goflare.Worker` (the target id a project writes), `goflare.Target`,
   `goflare.NewTarget`, `goflare.TargetConfig`, `goflare.KeyBootstrapToken`. Config fields renamed
   to say what they are: `D1Binding`, `D1Name`, `R2Binding`, `R2Bucket`.
3. **Ledger.** Manual steps per project −7 (token, D1, R2, two ID secrets, Worker secrets, token
   scope). Config fields: `D1DatabaseName` (was the binding), `D1DatabaseID`, `R2BucketName` (was
   the binding), `R2BucketID` (was the bucket name) → `D1Binding`, `D1Name`, `R2Binding`,
   `R2Bucket`: same count, honest names, no ID copied by hand.
4. **Where.** Cloudflare knowledge lives here; `deploy` holds only the contract.
5. **Deletes.** `D1DatabaseID`, `R2BucketID`, `EnvKeyD1DatabaseID`, `EnvKeyR2BucketID`,
   `EnvKeyD1DatabaseName`, `EnvKeyR2BucketName` and their action inputs/env.

## Stage 1 — rename the D1/R2 configuration (CI side)

`goflare.go` `Config`: replace `D1DatabaseID`, `D1DatabaseName`, `R2BucketID`, `R2BucketName` with

```go
	D1Binding string // D1_BINDING — name the Worker code uses, e.g. "DB"
	D1Name    string // D1_NAME    — database name in the account; its ID is resolved at deploy
	R2Binding string // R2_BINDING — name the Worker code uses, e.g. "ASSETS"
	R2Bucket  string // R2_BUCKET  — bucket name in the account
```

`config.go`: env keys `EnvKeyD1Binding = "D1_BINDING"`, `EnvKeyD1Name = "D1_NAME"`,
`EnvKeyR2Binding = "R2_BINDING"`, `EnvKeyR2Bucket = "R2_BUCKET"`; delete the four old keys and their
`switch` cases (`config.go:88-95,126-133`).

`cloudflare.go:233-253`: bindings are built when `D1Name != ""` / `R2Bucket != ""`. The D1 ID is
resolved by name: `GET /accounts/{account}/d1/database?name=<D1Name>` → exact-name match →
`uuid`. Not found → error `ErrD1NotFound` naming the database (`Provision` creates it; CI never
creates resources). A `D1Name` without `D1Binding`, or `R2Bucket` without `R2Binding`, is a config
error (`ErrBindingWithoutName`), never a default binding name: delete the `"DB"`/`"FILES"` defaults.

Non-secret runtime variables (`deploy.Plan.Vars`): new `Config.Vars []deploy.Var` read from the
env key `EnvKeyVars = "VARS"`, one `KEY=VALUE` per line (split on the first `=`; a line without
`=` → `ErrInvalidVarLine` naming the line). Each becomes a binding
`{"type": "plain_text", "name": KEY, "text": VALUE}` in the script metadata of **every** deploy, so
a `PUT` of the script never drops it (Cloudflare drops undeclared `plain_text` bindings).

New CLI subcommand `goflare d1-id` (`cmd/goflare/main.go`): prints the resolved D1 ID on stdout
(exit 1 + stderr when not found).

`action_data.go` (the generator of `action.yml`): inputs `d1-binding` → `D1_BINDING`, new
`d1-name` → `D1_NAME`, `r2-binding` → `R2_BINDING`, new `r2-bucket` → `R2_BUCKET`, new `vars` → `VARS`. The pre-deploy
step runs `D1_DATABASE_ID="$(goflare d1-id)" ${{ inputs.pre-deploy }}` when `d1-name` is set, so
existing `cmd/migrate` programs (they read `D1_DATABASE_ID`) keep working. Regenerate `action.yml`
with the existing generator test (`gotest`), never by hand.

## Stage 2 — the target id and construction

New `target.go` (no build tag: the constant must compile everywhere `config/deploy.go` is vetted):

```go
// Worker is the target id a project declares in config/deploy.go.
const Worker deploy.TargetID = "webtyp.com/goflare.Worker"

// KeyBootstrapToken is the entry, in the person's own keyring, holding their
// Cloudflare bootstrap token (template "Create additional tokens").
const KeyBootstrapToken = "cloudflare_bootstrap_token"

// TargetConfig configures a Target.
type TargetConfig struct {
	Personal *keyring.Keyring // the person's keyring (not the project's)
	RootDir  string           // the project root
	BaseURL  string           // Cloudflare API base; "" = https://api.cloudflare.com/client/v4
}

// Target implements deploy.Target for a Cloudflare Worker with static assets.
type Target struct{ /* unexported */ }

func NewTarget(cfg TargetConfig) (*Target, error) // error when Personal is nil or RootDir is ""

var _ deploy.Target = (*Target)(nil)
```

Move the `target.go` part that needs `!wasm` (everything except the two constants) to
`target_native.go` with `//go:build !wasm`.

## Stage 3 — `Check(p, ci)`

Results in this order. Every text is a `lang.Text` constant (Stage 7). Steps are shown one per line.

| # | Probe | Missing when | Fix |
|---|---|---|---|
| K1 | `Personal.Get(KeyBootstrapToken)` | absent | `FixInput`, `Secret: true`, Steps: open `https://dash.cloudflare.com/profile/api-tokens` → Create Token → template "Create additional tokens" → limit it to **one** account → TTL 90 days → copy it here. `Accept`: verify (`GET /user/tokens/verify`, status `active`) then `Personal.Set`. Invalid → error, nothing stored. |
| K2 | `GET /user/tokens/verify` | not `active`, or `expires_on` within 7 days | `FixInput` as K1, Item "Cloudflare bootstrap token expires soon" / "is not valid" |
| K3 | `GET /accounts` | ≠ 1 account | `FixManual`: create the bootstrap token limited to one account (D40) |
| K4 | plan shape | more than one `Database` or `Bucket` | return an error `ErrOneDatabaseOneBucket` (goflare binds at most one of each) |
| K5 | each `Database`: `GET /accounts/{a}/d1/database?name=` | not found | `FixAutomatic` |
| K6 | each `Bucket`: `GET /accounts/{a}/r2/buckets/{name}` | 404 | `FixAutomatic` |
| K7 | `p.Domain != ""`: `GET /zones?name=<registrable zone of Domain>&account.id={a}` | none | `FixManual`: add the domain to this Cloudflare account |
| K8 | account token named `"webtyp-deploy-" + p.Name` (`GET /accounts/{a}/tokens`) | absent, not active, or its policies differ from `requiredPermissions(p)` | `FixAutomatic` |
| K9 | `ci.Has("CLOUDFLARE_API_TOKEN")`, `ci.Has("CLOUDFLARE_ACCOUNT_ID")` | absent | `FixAutomatic` |

If K1, K2 or K3 is missing, return only those (nothing else can be probed).

`requiredPermissions(p)` is a fixed table (named constants, one row per Cloudflare API family
goflare calls), resolved to IDs at run time from
`GET /accounts/{a}/tokens/permission_groups` by **name** — IDs are never hard-coded:

| Needed when | Endpoints goflare calls | Permission group name |
|---|---|---|
| always | `workers/scripts/*`, `workers/subdomain`, `assets-upload-session`, `workers/scripts/*/secrets` | `Workers Scripts Write` |
| `Databases` non-empty | `d1/database/*` (resolve, `cmd/migrate` queries) | `D1 Write` |
| `Buckets` non-empty | R2 binding | `Workers R2 Storage Write` |
| `Domain` non-empty | `zones`, `workers/domains` | `Zone Read`, `Workers Routes Write` |
| `Domain` non-empty | `zones/*/rulesets/phases/http_ratelimit/entrypoint` | `Zone WAF Write` |

A name absent from the API's list → error `ErrUnknownPermissionGroup` naming it (loud; never
skipped). Before writing the table, check every endpoint goflare calls (`grep -n "accounts/\|zones" *.go`)
is covered; if one is not, add its row and say so in the PR.

## Stage 4 — `Provision(p, ci)`, `Rotate(p, ci)`

- Create each missing database (`POST /accounts/{a}/d1/database` `{"name": ...}`) and bucket
  (`POST /accounts/{a}/r2/buckets` `{"name": ...}`).
- Account token (K8): if absent or wrong, create
  `POST /accounts/{a}/tokens` `{"name": "webtyp-deploy-<Name>", "policies": [...requiredPermissions...]}`
  (account resources for account groups, the zone of `Domain` for zone groups), then
  `ci.Put("CLOUDFLARE_API_TOKEN", value)` and, only after that succeeded, delete the previous token
  of that name if there was one. This is the one case where Provision replaces a CI secret: the
  old value no longer works.
- `ci.Put("CLOUDFLARE_ACCOUNT_ID", a)` if absent.
- `Rotate`: always create a new account token, `Put` it, then delete the old one.
- The token value is never stored locally and never logged.

## Stage 5 — `Runtime(p)`: Worker secrets

Returns a `deploy.Destination`:

- `Has(key)`: `GET /accounts/{a}/workers/scripts/{Name}/secrets` → names. Script not found → `false`.
- `Put(key, value)`: `PUT /accounts/{a}/workers/scripts/{Name}/secrets`
  `{"name": key, "text": value, "type": "secret_text"}`. If the script does not exist yet, first
  upload a minimal placeholder Worker (module `export default { fetch() { return new Response("", {status: 503}) } }`)
  exactly as Wrangler does, then put the secret. The first real deploy replaces the script and,
  per Cloudflare, keeps `secret_text` bindings.

All calls use the bootstrap token (person's credential), never the CI token.

## Stage 6 — `Job(p)`

```go
deploy.Job{
	Uses: "webtyp/goflare@v1",
	With: [worker=<Name>, domain=<Domain if set>, d1-binding, d1-name, r2-binding, r2-bucket (if set),
	       vars=<one "KEY=VALUE" line per p.Vars entry, in order> (if any),
	       test="./tests/...", pre-deploy="go run ./cmd/migrate" only if RootDir/cmd/migrate/main.go exists],
	Secrets: ["CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID"],
}
```

Inputs in exactly that order; empty values omitted.

## Stage 7 — texts, dictionary

`texts.go`: one `lang.Text` constant per Missing item, reason and step above (English, nouns
before adjectives, no gendered forms). `lang.json` (8 columns `es zh hi ar pt fr de ru`; `es`
mandatory) and `dictionary.go` with `func Dictionary() []byte` like
`https://github.com/webtyp/devtui/blob/main/dictionary.go`. Run `langc sync`.

`errors.go`: `ErrD1NotFound`, `ErrBindingWithoutName`, `ErrOneDatabaseOneBucket`,
`ErrUnknownPermissionGroup`, `ErrNoPersonalKeyring`, `ErrNoRootDir`, `ErrInvalidVarLine` (English).

## Stage 8 — tests (`tests/`)

Fake Cloudflare API with `httptest` (extend `tests/helpers_test.go`), an in-memory keyring
(`keyring` in-memory provider, as `webtyp/keyring/tests` do), an in-memory `deploy.Destination`.

| Branch | Test |
|---|---|
| K1–K9 | one test per Check row; K1/K2/K3 missing → only those returned |
| K1a | `Accept` with an invalid token → error, keyring unchanged |
| P1 | Provision creates the missing D1 and R2 and does nothing when they exist |
| P2 | K8 absent → token created with exactly the permission groups of the plan (assert the request body), `Put` to ci, old token deleted only after `Put` |
| P3 | `ErrUnknownPermissionGroup` when the fake list lacks a name |
| R1 | Rotate always replaces the token |
| W1 | `Runtime.Put` on a missing script uploads the placeholder, then the secret |
| W2 | no request body or log line contains a secret value (record them and search) |
| J1 | `Job` with and without `cmd/migrate/main.go` |
| C1 | CI side: `D1Name` resolves to the ID in the deploy metadata; unknown name → `ErrD1NotFound` |
| C2 | `goflare d1-id` prints the ID |
| A1 | `action.yml` regenerated: inputs `d1-name`, `r2-bucket`, `vars`; no `D1_DATABASE_ID`/`R2_BUCKET_ID` |
| V1 | `VARS` with two lines → two `plain_text` bindings in the deploy metadata; a line without `=` → `ErrInvalidVarLine` |
| V2 | `Job` with `p.Vars` → `vars` input with one line per var, in order |

## Stage 9 — docs

`README.md` and `docs/`: the `config/deploy.go` line (`Target: goflare.Worker`), the bootstrap token
(how to create it: the K1 steps), what goflare creates, the renamed inputs. Delete every
instruction to create a token by hand or copy `D1_DATABASE_ID`/`R2_BUCKET_ID`.

| Stage | Files |
|---|---|
| 1 | `goflare.go`, `config.go`, `cloudflare.go`, `cmd/goflare/main.go`, `action_data.go`, `action.yml` |
| 2 | `target.go`, `target_native.go`, `go.mod`, `go.sum` |
| 3 | `target_check.go` |
| 4 | `target_provision.go` |
| 5 | `target_runtime.go` |
| 6 | `target_job.go` |
| 7 | `texts.go`, `errors.go`, `lang.json`, `dictionary.go` |
| 8 | `tests/*_test.go` |
| 9 | `README.md`, `docs/*` that mention the old keys |

## Acceptance

- `gotest` green.
- `grep -rn "D1DatabaseID\|R2BucketID\|D1DatabaseName\|R2BucketName\|D1_DATABASE_NAME\|R2_BUCKET_ID\|R2_BUCKET_NAME" --include=*.go --include=*.yml --include=*.md .` → empty.
- `grep -rn "D1_DATABASE_ID" --include=*.go --include=*.yml . | grep -v "d1-id\|pre-deploy"` → empty.
- `git diff --stat main` lists every file of the stages table.
