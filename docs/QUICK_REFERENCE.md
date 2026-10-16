# Quick Reference

## Commands

| Command | Description |
|---|---|
| `goflare auth --check` | Validate `CLOUDFLARE_API_TOKEN` from environment. |
| `goflare build` | Build the project (wasm + assets). |
| `goflare deploy` | Deploy to Cloudflare. Requires env vars. CI only. |

## Configuration (.env)

| Key | Description |
|---|---|
| `PROJECT_NAME` | Identity of the project in Cloudflare. |
| `CLOUDFLARE_ACCOUNT_ID` | Cloudflare Account ID. |
| `WORKER_NAME` | Name of the Worker script (default: `<PROJECT_NAME>-worker`). |
| `DOMAIN` | Custom domain (optional). |
| `COMPILER_MODE` | `S` (Small), `M` (Medium), `L` (Large). Default: `S`. |
| `COMPATIBILITY_DATE` | Workers compatibility date (default: `2026-08-01`). |
| `NOT_FOUND_HANDLING` | Asset 404 handling (default: `single-page-application`). |

## GitHub Secrets / Variables

| Name | Type | Description |
|---|---|---|
| `CLOUDFLARE_API_TOKEN` | Secret | dash.cloudflare.com → Profile → API Tokens → Create Token (Edit Workers) |
| `CLOUDFLARE_ACCOUNT_ID` | Secret | Cloudflare Dashboard → Right sidebar |

## Conventions (Not configurable)

| Item | Path |
|---|---|
| Go Entry | `edge/main.go` |
| Public Assets | `web/public/` |
| Build Artifacts | `.build/` |
