# CI and D1 Secrets

Secret management in GoFlare is based on the CI platform (GitHub Actions).

## Configuration

1. **Get the bootstrap token**:
   - Open https://dash.cloudflare.com/profile/api-tokens → Create Token → template 'Create additional tokens' → limit it to one account → TTL 90 days → copy it.
   - This bootstrap token will be used by goflare locally to provision resources and the deployment token automatically.
2. **Register secrets in GitHub** (done by the goflare Provision step; see [DEPLOY_TARGET.md](DEPLOY_TARGET.md)):
   - Go to your repo → Settings → Secrets and variables → Actions.
   - **Secrets**: `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`.

<img src="img/github_env_managment.png">

## Behavior by environment

| Environment | Token | Behavior |
|---|---|---|
| Local | `os.Getenv` | Uses the environment variable if it exists. `goflare auth --check` to validate. |
| CI (GitHub Actions) | `secrets.*` | Automatically injected by the workflow. |
| PR from Forks | Empty | Integration tests are automatically skipped (`t.Skip`). |
