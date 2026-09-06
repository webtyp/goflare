# Building for Cloudflare Workers

GoFlare enables you to run Go applications on Cloudflare Workers by compiling them to WebAssembly and providing a JavaScript bridge.

## Build Process

GoFlare will automatically detect the entry point if `edge/main.go` exists (Convention). `goflare build` will:

1. Compile your Go source code to WASM using `webtyp` (delegating to `tinygo`).
2. Generate `edge.js`, which bundles the entry point, runtime, and `wasm_exec.js`.
3. Output all artifacts to the `.build/` directory.

## Deployment

Deployment is done via a multipart upload to the Cloudflare Workers API.

### Upload Fields
- **`metadata`**: A JSON object specifying the `main_module` (`edge.js`).
- **`edge.js`**: The bundled JavaScript wrapper script.
- **`edge.wasm`**: The compiled WebAssembly binary.

## Binary Size & Gate Limits

Cloudflare Workers isolate instantiation cost is directly tied to the raw WASM binary size. `goflare` enforces budget checks on raw binary size before deployment to avoid slow isolate cold starts and budget exhaustion.

| Threshold / Limit | Value | Owner | Description |
|---|---|---|---|
| Warning Threshold | 256 KiB raw | `goflare` budget | Emits warning on stdout/stderr during build |
| Abort Threshold | 900 KiB raw | `goflare` budget | Aborts build/deploy before uploading |
| Cloudflare Free Limit | **3 MB gzip** (64 MB raw) | Cloudflare | Account platform limit |
| Cloudflare Paid Limit | 10 MB gzip | Cloudflare | Account platform limit |
| Instantiation Budget | 1 s | Cloudflare | Time budget allowed to parse & instantiate isolate |

Thresholds can be adjusted using environment variables `WASM_WARN_SIZE_KIB` and `WASM_MAX_SIZE_KIB`. Setting a threshold key to `0` disables that check.

## CI/CD Deployment

Deployment requires `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` environment variables. It is designed to run in CI (e.g., GitHub Actions).

## workers.dev Only

GoFlare currently targets deployment to the `*.workers.dev` subdomain. After deployment, your worker will be live at `https://<worker-name>.<your-subdomain>.workers.dev`.
