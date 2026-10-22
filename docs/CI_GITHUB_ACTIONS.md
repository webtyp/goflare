# CI con GitHub Actions — Action `webtyp/goflare`

GoFlare se despliega en GitHub Actions usando la action oficial `webtyp/goflare@v1`. No requiere instalar Node.js, Wrangler ni compilar `goflare` desde fuente.

## Uso rápido

```yaml
- uses: actions/checkout@v4
- uses: webtyp/goflare@v1
  with:
    worker: mi-worker
    domain: mi-worker.ejemplo.cl
    d1-binding: DB
    d1-name: my-d1
  env:
    CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
    CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
```

## Inputs y Variables de Entorno

| Input | Requerido | Default | Variable de Entorno Asociada |
|---|---|---|---|
| `worker` | **sí** | — | `WORKER_NAME` |
| `project` | no | `''` | `PROJECT_NAME` (si está vacío usa `worker`) |
| `domain` | no | `''` | `DOMAIN` |
| `d1-binding` | no | `''` | `D1_BINDING` |
| `d1-name` | no | `''` | `D1_NAME` |
| `r2-binding` | no | `''` | `R2_BINDING` |
| `r2-bucket` | no | `''` | `R2_BUCKET` |
| `vars` | no | `''` | `VARS` |
| `compatibility-date` | no | `''` | `COMPATIBILITY_DATE` |
| `not-found-handling` | no | `''` | `NOT_FOUND_HANDLING` |
| `version` | no | `''` | Versión del binario de `goflare` a descargar |
| `setup-go` | no | `'true'` | Ejecuta `actions/setup-go@v5` con el `go.mod` |
| `vet` | no | `'true'` | Ejecuta `go vet ./...` |
| `test` | no | `'./tests/...'` | Patrón para `go test` (vacío = omitir) |
| `pre-deploy` | no | `''` | Comando a ejecutar entre build y deploy |
| `deploy` | no | `'true'` | Ejecutar el paso de despliegue (`'false'` en PRs) |
| `cache` | no | `'true'` | Cachear el árbol instalado de TinyGo |

### Inputs vs Secretos

- **Inputs:** Opciones de configuración del proyecto (`worker`, `domain`, `d1-binding`, `vars`, etc.).
- **Secretos:** Credenciales de autenticación (`CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`).

> ⚠️ **Los secretos NUNCA son inputs.** Pasar secretos como inputs de una composite action los expone en los logs de depuración de GitHub Actions. Pásalos siempre mediante `env:`.

## Resolución de Versiones

El binario de `goflare` se descarga automáticamente según las siguientes reglas:
1. `inputs.version` si está especificado.
2. `github.action_ref` si la action se invoca con un tag semver completo (ej: `uses: webtyp/goflare@v0.5.22`).
3. El último release de la GitHub API.

Si ninguna resuelve, la action falla.

## Compilar sin desplegar en Pull Requests

```yaml
name: Deploy
on:
  push:
    branches: [main]
  pull_request:

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: webtyp/goflare@v1
        with:
          worker: mi-worker
          deploy: ${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}
        env:
          CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
          CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
```

## Generación de `action.yml`

El archivo `action.yml` en la raíz del repositorio no se edita a mano:
- Es producido determinísticamente por el paquete `actiongen` en `action_data.go`.
- El test `TestActionYmlIsInSync` verifica en CI que `action.yml` coincide con el código de Go.
- Para modificar la action, edita `action_data.go` y ejecuta los tests de Go.
