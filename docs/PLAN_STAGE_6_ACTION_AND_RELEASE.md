← [Etapa 5](PLAN_STAGE_5_ACTION_GENERATOR.md) | Etapa 6 de 7 | Siguiente → [Etapa 7](PLAN_STAGE_7_DOCS.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 6 — La action publicada y los releases automáticos

Tres archivos: el `action.yml` que produce el generador, el workflow que la
consume a sí misma para probarla, y el workflow que publica los binarios.

---

## Parte A — El contenido de la action

Esto define qué devuelve `GoflareAction(tinyGoVersion, goflareVersion)` de la
etapa 5. El archivo `action.yml` **no se escribe a mano**: se escribe este valor
en Go, se corre `gotest ./...`, y el test lo materializa.

### Metadatos

```
name:        Deploy with goflare
description: Compila un proyecto Go a WASM y lo despliega como Cloudflare Worker
author:      webtyp
branding:    icon "upload-cloud", color "orange"
```

### Inputs

| Nombre | Requerido | Default | Va a |
|---|---|---|---|
| `version` | no | `''` | qué release de goflare descargar; vacío = resolución automática |
| `project` | no | `''` | `PROJECT_NAME`; vacío = el valor de `worker` |
| `worker` | **sí** | — | `WORKER_NAME` |
| `domain` | no | `''` | `DOMAIN` |
| `d1-binding` | no | `''` | `D1_DATABASE_NAME` |
| `r2-binding` | no | `''` | `R2_BUCKET_NAME` |
| `compatibility-date` | no | `''` | `COMPATIBILITY_DATE` |
| `not-found-handling` | no | `''` | `NOT_FOUND_HANDLING` |
| `setup-go` | no | `'true'` | correr `actions/setup-go` con el `go.mod` del proyecto |
| `vet` | no | `'true'` | correr `go vet ./...` |
| `test` | no | `'./tests/...'` | patrón para `go test`; vacío = no correr tests |
| `pre-deploy` | no | `''` | comando a correr entre build y deploy (típicamente la migración) |
| `deploy` | no | `'true'` | poner `'false'` en pull requests para compilar sin desplegar |
| `cache` | no | `'true'` | cachear el árbol de TinyGo |

> ⚠️ **`d1-binding` va a la variable `D1_DATABASE_NAME`.** El nombre de la
> variable en goflare es engañoso —es el nombre del *binding*, no el de la base—
> pero **no la renombres en este plan**: hay `.env` en uso que la traen así.
> Renombrarla es un cambio aparte.

Los **secretos no son inputs**. `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`,
`D1_DATABASE_ID` y `R2_BUCKET_ID` los pasa el llamador por `env:` y el binario
los lee del entorno, igual que hoy. Un secreto en un `input` acaba impreso en
los logs de depuración de la action.

### Outputs

| Nombre | Valor |
|---|---|
| `goflare-version` | la versión que se resolvió y descargó |
| `tinygo-version` | lo que reporta `tinygo version` |

### Pasos, en orden

**1. Setup Go** — `if: inputs.setup-go == 'true'`

```yaml
uses: actions/setup-go@v5
with:
  go-version-file: 'go.mod'
```

Comentario del paso: *La versión sale del `go.mod` del proyecto, así que la
elige el llamador, no nosotros. `setup-go` además cachea `~/go/pkg/mod` y
`~/.cache/go-build` por su cuenta, con key derivada de `go.sum`.*

**2. Resolver y descargar el binario de goflare** — `id: goflare`

```bash
set -euo pipefail

version="${{ inputs.version }}"
if [ -z "$version" ]; then
  ref="${{ github.action_ref }}"
  case "$ref" in
    v[0-9]*.[0-9]*.[0-9]*) version="$ref" ;;
    *) version="__GOFLARE_VERSION__" ;;
  esac
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  asset=goflare-linux-amd64 ;;
  Linux-aarch64) asset=goflare-linux-arm64 ;;
  Darwin-arm64)  asset=goflare-darwin-arm64 ;;
  Darwin-x86_64) asset=goflare-darwin-amd64 ;;
  *) echo "goflare: plataforma no soportada: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

url="https://github.com/webtyp/goflare/releases/download/${version}/${asset}"
dest="${RUNNER_TEMP}/goflare"

if ! curl -fsSL "$url" -o "$dest"; then
  echo "goflare: no existe el binario ${asset} en el release ${version}." >&2
  echo "  URL: ${url}" >&2
  echo "  Lo mas probable es que gorelease no haya corrido para ese tag." >&2
  echo "  Revisa https://github.com/webtyp/goflare/releases y fija una version que si tenga binarios con el input 'version'." >&2
  exit 1
fi

chmod +x "$dest"
echo "$(dirname "$dest")" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"
```

`__GOFLARE_VERSION__` es el marcador que sustituye el generador por el tag real.
En el `action.yml` publicado aparece la versión literal.

**Nada de fallback a `go run`.** Si el binario no existe, la action **falla en
el acto con el motivo escrito**. Un fallback silencioso a compilar desde fuente
esconde precisamente el problema que este plan arregla — y ese problema existe
hoy: los releases se detuvieron en `v0.5.13` con los tags en `v0.5.22`.

**3. Restaurar la caché de TinyGo** — `if: inputs.cache == 'true'`

```yaml
uses: actions/cache@v4
with:
  path: |
    /usr/local/tinygo
    ~/.local/tinygo
  key: tinygo-${{ runner.os }}-${{ runner.arch }}-__TINYGO_VERSION__
```

Comentario del paso: *Se cachea el árbol instalado, no el tarball: un acierto no
cuesta ni descarga ni extracción. La versión va **dentro** de la key a
propósito. Las keys de `actions/cache` son inmutables —un acierto nunca vuelve a
guardar—, así que una key sin versión se queda clavada en un árbol viejo cuando
la versión de TinyGo sube, y a partir de ahí cada corrida paga el restore y
además la descarga completa. El generador mantiene esta cifra al día.*

**4. Instalar TinyGo** — `id: tinygo`

```bash
set -euo pipefail
out="$(goflare tinygo)"
bindir="${out#*TINYGO_BINDIR=}"
bindir="${bindir%%$'\n'*}"
version="$(printf '%s' "$out" | sed -n 's/^TINYGO_VERSION=//p')"

if [ -z "$bindir" ]; then
  echo "goflare tinygo no devolvio un directorio bin" >&2
  exit 1
fi

echo "$bindir" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"
```

Comentario del paso: *TinyGo lo instala el propio binario de goflare, vía
`webtyp/tinygo`. A propósito **no** se usa `uses: webtyp/tinygo@v1`: esa
action instala la versión del ref con que se la invoca, mientras `sitec` la
vuelve a resolver durante el build. Serían dos fuentes, y el día que dejen de
coincidir, `sitec` desinstala lo que puso la action y descarga lo suyo. Con el
binario como único instalador hay una sola fuente. Este paso va antes de los
tests porque algunos proyectos invocan el binario `tinygo` pelado desde el PATH
durante `go test`.*

**5. `go vet`** — `if: inputs.vet == 'true'` → `run: go vet ./...`

**6. `go test`** — `if: inputs.test != ''` → `run: go test ${{ inputs.test }}`

**7. `goflare build`** — con todos los `env:` mapeados desde los inputs.
Este paso emite el reporte de tamaño de la etapa 1 y aborta si se pasa del
presupuesto, **antes** de gastar una subida.

**8. Comando pre-deploy** — `if: inputs.pre-deploy != '' && inputs.deploy == 'true'`
→ `run: ${{ inputs.pre-deploy }}`

Comentario del paso: *Aquí es donde va la migración del esquema, que corre una
vez por despliegue y no una vez por arranque de isolate.*

**9. `goflare deploy`** — `if: inputs.deploy == 'true'`, mismos `env:` que el
build.

---

## Parte B — El workflow que prueba la action

Archivo nuevo: **`.github/workflows/action.yml`**.

Copia el patrón de
[webtyp/tinygo](https://github.com/webtyp/tinygo/blob/main/.github/workflows/action.yml):
consumir la action con `uses: ./`, de modo que lo que corre es el árbol de
trabajo de ese commit y **un `action.yml` roto rompe el PR que lo introdujo**,
no el despliegue del primer usuario que la fije.

```yaml
name: Action

on:
  push:
    branches: [main]
  pull_request:

jobs:
  consume:
    runs-on: ubuntu-latest
    steps:
      # fetch-tags: true es obligatorio. Sin tags, LatestReleaseTag() devuelve
      # cadena vacia y el test de sincronizacion no puede verificar nada.
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          fetch-tags: true

      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      # El test que mantiene action.yml sincronizado. Falla si el archivo
      # commiteado difiere de lo que produce el generador.
      - name: action.yml esta sincronizado
        run: go test ./tests/ -run TestActionYmlIsInSync -v

      # Compila un proyecto de prueba con la action, sin desplegar. Prueba de
      # punta a punta que la descarga del binario, la instalacion de TinyGo y
      # el build funcionan.
      - name: consumir la action sin desplegar
        uses: ./
        with:
          worker: goflare-action-selftest
          deploy: 'false'
          test: ''
```

> ⚠️ **`deploy: 'false'` es obligatorio en este job.** El repo de goflare no
> tiene credenciales de Cloudflare y no debe tenerlas. Este job prueba la
> mecánica de la action, no un despliegue.

El proyecto de prueba: crea `tests/fixture/` con un `edge/main.go` mínimo que
importe `webtyp.com/cloudflare/edge` y sirva una ruta. Si montar el
fixture resulta ser más trabajo del previsto, **reduce el job a correr solo el
test de sincronización y déjalo anotado en el PR** — el test de sincronización
es el que protege contra el fallo silencioso; el fixture es un extra.

---

## Parte C — El workflow que publica los binarios

Archivo nuevo: **`.github/workflows/release.yml`**.

Éste es el que arregla el fallo en vivo: hoy `webtyp/goflare-demo` descarga
`…/download/v0.5.22/goflare-linux-amd64` y recibe **HTTP 404**, porque nadie
corrió `gorelease` desde `v0.5.13`.

```yaml
name: Release

# Se dispara con el tag que empuja gopush/codejob al publicar el modulo.
on:
  push:
    tags: ['v*']

permissions:
  contents: write   # gorelease crea el GitHub Release y sube los binarios

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          fetch-tags: true

      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      # gorelease compila cmd/ para linux, darwin y windows, genera
      # checksums.txt y sube todo como assets del release del tag.
      # Ver https://github.com/webtyp/devflow/blob/main/docs/GORELEASE.md
      - name: gorelease
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          set -euo pipefail
          go run webtyp.com/devflow/cmd/gorelease@latest "${GITHUB_REF_NAME}"

      # Mueve el tag movil v1 al commit recien publicado, para que
      # uses: webtyp/goflare@v1 siga a la ultima linea v0.x.
      - name: mover el tag v1
        run: |
          set -euo pipefail
          git tag -f v1 "${GITHUB_SHA}"
          git push origin -f v1
```

### El desfase de una versión — documéntalo, no intentes eliminarlo

En el commit del tag `vX.Y.Z`, `action.yml` tiene horneado `v(X.Y.Z-1)`: el
generador lee el tag más alto que existía **cuando corrió el test**, y ese tag
es el del release anterior.

Consecuencia: `uses: webtyp/goflare@v1` descarga el binario de la penúltima
versión. **Es deliberado y es la opción segura** — `@v1` nunca puede apuntar a
un release cuyos binarios todavía se estén subiendo. Quien necesite la última,
la fija: `uses: webtyp/goflare@vX.Y.Z`, y ahí `github.action_ref` gana y
descarga esa exacta.

**No** intentes cerrar el desfase haciendo que el workflow de release commitee
un `action.yml` regenerado a `main`: pelearía con el ciclo de vida de `codejob`
sobre la misma rama.

---

## Criterios de aceptación

- `action.yml` existe en la raíz del repo y su primera línea es
  `# ARCHIVO GENERADO — no lo edites a mano.`
- `grep -c "__GOFLARE_VERSION__\|__TINYGO_VERSION__" action.yml` → `0` (los
  marcadores están sustituidos por valores reales).
- `grep -n "go run webtyp.com/goflare" action.yml` → vacío (nada de
  compilar desde fuente en el runner).
- `grep -n "webtyp/tinygo@" action.yml` → vacío.
- El job `consume` pasa en un PR.
- `gotest ./...` en verde.

## Lo que NO hay que hacer

- **No** migres los workflows de `veltylabs/iam`, `veltylabs/misitio` ni
  `webtyp/goflare-demo`. Están en otros repos y tienen sus propios planes.
- **No** añadas soporte de Cloudflare Pages ni de `wrangler`. Pages se está
  deprecando y queremos una sola forma de desplegar.
- **No** publiques la action en el GitHub Marketplace. Eso es un paso manual con
  decisiones de nombre y licencia que no están tomadas.
