# Deploy target — `goflare.Worker`

goflare implements `deploy.Target` for a Cloudflare Worker with static assets. A project
declares it in `config/deploy.go`:

```go
Target: goflare.Worker,
```

The person deploying provides **one** credential, their *bootstrap token*. goflare creates
everything else.

## Bootstrap token

It lives in the person's own keyring (`goflare.KeyBootstrapToken`, `cloudflare_bootstrap_token`),
never in the repository or in CI. To create it:

1. Open https://dash.cloudflare.com/profile/api-tokens.
2. Create Token → template **Create additional tokens**.
3. Limit it to **one** account.
4. Set the TTL to 90 days.
5. Copy it into the webtyp wizard. goflare verifies it (`GET /user/tokens/verify`) before storing it.

## What goflare checks and creates

| Probe | When missing |
|---|---|
| Bootstrap token present, `active`, not expiring within 7 days | asks for a new one |
| Exactly one account visible to the token | asks to limit the token to one account |
| One D1 database and one R2 bucket at most | `ErrOneDatabaseOneBucket` |
| D1 database named in the plan | created |
| R2 bucket named in the plan | created |
| Zone of the plan's domain in the account | asks to add the domain to the account |
| Account token `webtyp-deploy-<name>` with the permissions the plan needs | created, stored in CI as `CLOUDFLARE_API_TOKEN` |
| CI secrets `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID` | uploaded |

The permission groups are resolved by name at run time; an unknown name fails with
`ErrUnknownPermissionGroup`. Rotation always creates the new token, stores it in CI, and only
then deletes the old one. The token value is never stored locally or logged.

## Worker secrets and variables

- Secrets (`deploy.Destination` returned by `Runtime`) are written with the bootstrap token to the
  Worker. If the script does not exist yet, a placeholder Worker is uploaded first, as Wrangler does.
- Non-secret variables (`deploy.Plan.Vars`) travel to CI as the `vars` input, one `KEY=VALUE` per
  line, and become `plain_text` bindings on every deploy.

## CI job

`Job` emits `webtyp/goflare@v1` with the inputs `worker`, `domain`, `d1-binding`, `d1-name`,
`r2-binding`, `r2-bucket`, `vars`, `test` and, when `cmd/migrate/main.go` exists, `pre-deploy`.
Inputs are described in [CI_GITHUB_ACTIONS.md](CI_GITHUB_ACTIONS.md).
