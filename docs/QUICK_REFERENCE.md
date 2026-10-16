# Quick Reference

## Commands

| Command | Description |
|---|---|
| `goflare auth --check` | Validate `CLOUDFLARE_API_TOKEN` from environment. |
| `goflare build` | Build the project (wasm + assets). |
| `goflare deploy` | Deploy to Cloudflare. Requires env vars. CI only. |
| `goflare d1-id` | Print the ID of the D1 database named by `D1_NAME`. |

## Configuration (.env)

| Key | Description |
|---|---|
| `PROJECT_NAME` | Identity of the project in Cloudflare. |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare Account ID. |
| `WORKER_NAME` | Name of the Worker script (default: `<PROJECT_NAME>-worker`). |
| `DOMAIN` | Custom domain (optional). |
| `D1_BINDING` / `D1_NAME` | D1 binding name and database name (ID resolved at deploy). |
| `R2_BINDING` / `R2_BUCKET` | R2 binding name and bucket name. |
| `VARS` | Non-secret variables, one `KEY=VALUE` per line. |
| `COMPILER_MODE` | `S` (Small), `M` (Medium), `L` (Large). Default: `S`. |
| `COMPATIBILITY_DATE` | Workers compatibility date (default: `2026-08-01`). |
| `NOT_FOUND_HANDLING` | Asset 404 handling (default: `single-page-application`). |

## GitHub Secrets / Variables

| Name | Type | Description |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Secret | Created and stored by goflare from the bootstrap token ([DEPLOY_TARGET.md](DEPLOY_TARGET.md)) |
| `CLOUDFLARE_ACCOUNT_ID` | Secret | Uploaded by goflare |

## Conventions (Not configurable)

| Item | Path |
|---|---|
| Go Entry | `edge/main.go` |
| Public Assets | `web/public/` |
| Build Artifacts | `.build/` |
