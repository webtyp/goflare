# GoFlare Architecture

GoFlare is a Go library and CLI that bridges the gap between Go source code and Cloudflare Workers with static assets. It automates the compilation, JS glue generation, asset manifest upload, and deployment process.

## Component Overview

```
┌─────────────┐
│ Go Source   │
│ (main.go)   │
└──────┬──────┘
       │
       ▼ [Build]
┌─────────────┐
│  webtyp   │ (Compiles Go to JavaScript)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  goflare    │ (Generates JS glue)
└──────┬──────┘
       │
       ▼ [Deploy]
┌─────────────┐
│ Cloudflare  │ (Workers API v4)
└─────────────┘
```

## Core Modules

### 1. Configuration (`config.go`)
- **Flat Struct:** Simple `Config` struct that maps directly to `.env` keys.
- **Stdlib Parser:** Loads settings from `.env` using standard library scanners.
- **Single Source of Truth:** Library callers use the struct directly; CLI users use `.env` / environment variables.

### 2. Storage
- **None.** goflare keeps no state of its own: secrets come from the platform (CI/CD) and the
  environment. The former `Store`/`MemoryStore` (left over from local keyring management) had no
  caller and was removed; a key-value store, when one is needed, is `webtyp.com/kvdb`.

### 3. Build Pipeline (`build.go`, `mode.go`, `edge_goesm.go`)
- **Entry Validation (`mode.go`):** `validateEntry()` verifies `web/edge/main.go` imports `webtyp/cloudflare/edge` or `webtyp/cloudflare/workers` (legacy `webtyp/goflare/edge` aceptado durante migración).
- **Worker Build:** The edge is JavaScript only. `edge_goesm.go` compiles `web/edge` with goesm to `web/edge/app.js` and writes `web/edge/edge.js`, the main module (`webtyp/cloudflare/assets/worker.mjs`, with the `x-goflare` identity) that imports `app.js` on the first event. No WebAssembly.
- **Static Site Build:** Delegates static site compilation to `sitec.Build` via a pluggable `SiteBuilder` seam. `sitec` scans project packages for declared producers, compiles `web/browser` to an ES Module with goesm and every Web Worker with the compiler its `main.go` declares, and emits static assets to `web/public/`. Always release mode.

### 4. Authentication (`auth.go`)
- **Environment-based:** Validates `CLOUDFLARE_API_TOKEN` environment variable via `GET /user/tokens/verify`.

### 5. Deployment (`cloudflare.go`, `assets.go`)
- **Internal HTTP Client:** `CfClient` handles direct interaction with Cloudflare API v4.
- **Worker + Assets Deploy:** Unified deployment via 3-phase Direct Upload (Asset Upload Session -> Chunked Uploads -> Worker Script PUT with metadata and asset JWT).
- **Asset Headers:** Static assets with a content-hashed name automatically receive an `immutable` Cache-Control header via Cloudflare's `_headers` policy generated from `pwa.CacheControl`'s rule; `/artifacts/` is refused until R2 serving exists (D-PWA-16). The probe retries up to 5 times; the wait between attempts comes from `webtyp.com/backoff` (`RetryBackoff`, doubling).

### 6. Edge Runtime (`webtyp/cloudflare/assets/worker.mjs`, `webtyp/cloudflare/workers/workers.go`)
> El runtime vive en **`webtyp/cloudflare`**, no en este repo — `goflare` sólo lo sube vía `cloudflare/assets` embed.
- **Lifecycle:** the goesm module is imported **once per isolate** (not per request), on the first event, with `globalThis.context = {env, connect, binding}`; `main()` runs once. A `main()` that never registers its handler fails the first request after `startTimeoutMs` with a named error.
- **Request Dispatch:** Sequential and concurrent requests reuse the isolate's Go instance via `binding.handleRequest`.
- **Handshake:** Init signal passes via `context.binding.ready` (instance-scoped door) rather than shared globals.

## Project Structure

```
goflare/
├── goflare.go          # Core Goflare struct and entry points
├── config.go           # Configuration loading and validation
├── store.go            # Memory storage abstraction
├── mode.go             # Entry validation for web/edge/main.go imports (webtyp/cloudflare)
├── build.go            # Build orchestration
├── assets.go           # Asset hashing, manifest generation, and bucket upload
├── auth.go             # Cloudflare authentication logic
├── cloudflare.go       # Cloudflare API client and deployer
├── run.go              # CLI runner functions
├── edge_goesm.go       # edge → app.js (goesm) + edge.js (worker.mjs from webtyp/cloudflare/assets)
├── tinygo.go           # TinyGo install for CI (site Web Workers only; the edge never uses it)
├── files/              # file upload helper (uses webtyp/cloudflare/log)
├── tests/              # Comprehensive test suite
└── cmd/goflare/        # CLI entry point (main.go)

# Runtime lives in webtyp/cloudflare:
#   edge/, workers/, d1/, r2/, log/, assets/, env_*.go
```

## Un binario, no un módulo

`goflare` se consume como un **binario publicado**, no como una dependencia en el `go.mod` del proyecto:
- Compilar `goflare` desde fuentes en el runner de CI toma entre 18 s (máquina rápida caliente) y 40-70 s (runner de GitHub). La caché de `actions/setup-go` se invalida frecuentemente por cambios en `go.sum`. Descargar el binario publicado (10 MB) toma 1-2 s.
- El binario instala y gestiona TinyGo por su propia cuenta (`goflare tinygo`) para los Web Workers del sitio que lo declaran, garantizando **una sola fuente** para su versión; el edge no lo usa.
- **Riesgo de desajuste de versiones (ABI skew):** Al compilar con un binario independiente, los assets JS embebidos en el binario y el runtime `webtyp.com/cloudflare` importado por el proyecto Go podrían divergir. Para prevenirlo, `CheckVersionSkew` verifica en `goflare deploy` que ambas versiones coincidan exactamente y aborta con un mensaje claro si difieren.

## Design Principles

- **Single Deployment Path:** Every project deploys as a Cloudflare Worker with static assets.
- **Convention over Configuration:** Output directory is `web/`. Entry points (`web/edge/main.go`) and public directories (`web/public/`) are auto-detected by convention.
- **Minimal edge code:** Files of the edge (`edge/`, `workers/`, `cloudflare/env_wasm.go`) NEVER import heavy stdlib: each package is more JavaScript in `app.js`. Use `webtyp/*` equivalents instead.
- **Platform Secrets (never in `.env`):** `CLOUDFLARE_API_TOKEN` is provided via environment variables, ideally managed as GitHub Secrets.
- **Self-Contained:** No external tools like Node.js or Wrangler required.
