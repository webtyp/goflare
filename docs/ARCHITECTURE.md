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
│  webtyp   │ (Compiles Go to WASM)
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

### 2. Storage (`store.go`)
- **Memory Store:** An exported `MemoryStore` is provided for testing and library consumers. Local keyring management has been removed in favor of platform-based secrets (CI/CD).

### 3. Build Pipeline (`build.go`, `mode.go`, `javascripts.go`, `wasm.go`)
- **Entry Validation (`mode.go`):** `validateEntry()` verifies `edge/main.go` imports `webtyp/cloudflare/edge` or `webtyp/cloudflare/workers` (legacy `webtyp/goflare/edge` aceptado durante migración).
- **Worker Build:** Produces `.build/edge.js` (bundled from `webtyp/cloudflare/assets` via `javascripts.go`) and `.build/edge.wasm`.
- **Static Site Build:** Delegates static site compilation to `sitec.Build` via a pluggable `SiteBuilder` seam. `sitec` scans project packages for declared producers, compiles frontend WASM if `web/client.go` exists, and emits static assets to `web/public/`.

### 4. Authentication (`auth.go`)
- **Environment-based:** Validates `CLOUDFLARE_API_TOKEN` environment variable via `GET /user/tokens/verify`.

### 5. Deployment (`cloudflare.go`, `assets.go`)
- **Internal HTTP Client:** `CfClient` handles direct interaction with Cloudflare API v4.
- **Worker + Assets Deploy:** Unified deployment via 3-phase Direct Upload (Asset Upload Session -> Chunked Uploads -> Worker Script PUT with metadata and asset JWT).

### 6. Edge Runtime (`webtyp/cloudflare/assets/worker.mjs`, `webtyp/cloudflare/assets/runtime.mjs`, `webtyp/cloudflare/workers/workers.go`)
> El runtime vive en **`webtyp/cloudflare`**, no en este repo — `goflare` sólo lo empaqueta vía `cloudflare/assets` embed.
- **Lifecycle:** Go/WASM instance is booted **once per isolate** (not per request). `main()` runs once during isolate initialization.
- **Request Dispatch:** Sequential and concurrent requests reuse the isolate's Go instance via `binding.handleRequest`.
- **Handshake:** Init signal passes via `context.binding.ready` (instance-scoped door) rather than shared globals.

## Project Structure

```
goflare/
├── goflare.go          # Core Goflare struct and entry points
├── config.go           # Configuration loading and validation
├── store.go            # Memory storage abstraction
├── mode.go             # Entry validation for edge/main.go imports (webtyp/cloudflare)
├── build.go            # Build orchestration
├── assets.go           # Asset hashing, manifest generation, and bucket upload
├── auth.go             # Cloudflare authentication logic
├── cloudflare.go       # Cloudflare API client and deployer
├── run.go              # CLI runner functions
├── javascripts.go      # JS bundling (reads from webtyp/cloudflare/assets)
├── wasm.go             # WASM compilation delegation
├── files/              # WASM file upload helper (uses webtyp/cloudflare/log)
├── tests/              # Comprehensive test suite
└── cmd/goflare/        # CLI entry point (main.go)

# Runtime lives in webtyp/cloudflare:
#   edge/, workers/, d1/, r2/, log/, assets/, env_*.go
```

## Un binario, no un módulo

`goflare` se consume como un **binario publicado**, no como una dependencia en el `go.mod` del proyecto:
- Compilar `goflare` desde fuentes en el runner de CI toma entre 18 s (máquina rápida caliente) y 40-70 s (runner de GitHub). La caché de `actions/setup-go` se invalida frecuentemente por cambios en `go.sum`. Descargar el binario publicado (10 MB) toma 1-2 s.
- El binario instala y gestiona TinyGo por su propia cuenta, garantizando **una sola fuente** para la versión del compilador WASM.
- **Riesgo de desajuste de versiones (ABI skew):** Al compilar con un binario independiente, los assets JS embebidos en el binario y el runtime `webtyp.com/cloudflare` importado por el proyecto Go podrían divergir. Para prevenirlo, `CheckVersionSkew` verifica en `goflare deploy` que ambas versiones coincidan exactamente y aborta con un mensaje claro si difieren.

## Design Principles

- **Single Deployment Path:** Every project deploys as a Cloudflare Worker with static assets.
- **Convention over Configuration:** Output directory is `.build/`. Entry points (`edge/main.go`) and public directories (`web/public/`) are auto-detected by convention.
- **Minimal binary in wasm code:** Files compiled to WASM (`edge/`, `workers/`, `cloudflare/env_wasm.go`) NEVER import heavy stdlib. Use `webtyp/*` equivalents instead.
- **Platform Secrets (never in `.env`):** `CLOUDFLARE_API_TOKEN` is provided via environment variables, ideally managed as GitHub Secrets.
- **Self-Contained:** No external tools like Node.js or Wrangler required.
