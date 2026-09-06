← [Etapa 2](PLAN_STAGE_2_SIZE_DIAGNOSTIC.md) | Etapa 3 de 7 | Siguiente → [Etapa 4](PLAN_STAGE_4_VERSION_SKEW.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 3 — `goflare tinygo`: el binario instala su propia cadena de herramientas

## Por qué

La action de la etapa 6 no puede correr `go run
webtyp.com/tinygo/cmd/tinygoinstall` como hacen hoy los workflows: eso
volvería a exigir compilar desde fuente en el runner, que es justo lo que este
plan elimina. Y tampoco debe usar `uses: webtyp/tinygo@v1`, por un motivo que
ya está documentado en
[el workflow de misitio](https://github.com/veltylabs/misitio/blob/main/.github/workflows/deploy.yml):
esa action instala la versión del ref con que se la invoca, mientras que `sitec`
la vuelve a resolver por su cuenta durante el build. Son dos fuentes, y el día
que dejen de coincidir, `sitec` desinstala lo que puso la action y descarga lo
suyo.

Con el binario de goflare como único instalador hay **una sola fuente**: la
versión del módulo `webtyp.com/tinygo` con la que se compiló ese
binario. Un binario que descargar, y nada que pueda divergir.

Hay además una razón de orden: los tests de algunos proyectos invocan el binario
`tinygo` pelado desde el `PATH` (por ejemplo
`veltylabs/misitio/tests/client_size_test.go`), y los tests corren **antes** del
build. Así que TinyGo tiene que estar en el `PATH` antes de que la action llegue
a `goflare build`, y hace falta un comando explícito que lo ponga ahí.

## Lo que ya existe — reutilízalo, no lo dupliques

[wasm.go](../wasm.go) ya tiene la mitad del trabajo:

```go
// EnsureTinyGo installs TinyGo if absent and guarantees its bin dir is in PATH
// before any compilation attempt. Safe to call multiple times (idempotent).
func EnsureTinyGo(out io.Writer) error {
	installedPath, err := tinygo.EnsureInstalled()
	...
}
```

El problema para la action es que `EnsureTinyGo` **no devuelve el bindir**: lo
mete en el `PATH` del proceso, que muere al terminar el comando. La action
necesita el directorio impreso para escribirlo en `$GITHUB_PATH`.

## Lo que hay que construir

### 1. Exponer el bindir — en `wasm.go`

```go
// TinyGoBinDir instala TinyGo si falta y devuelve el directorio que contiene el
// binario, junto con la version que reporta. Es la forma consultable de
// EnsureTinyGo: el proceso que la llama puede morir, asi que devuelve el
// directorio en vez de solo mutar su propio PATH.
func TinyGoBinDir() (dir, version string, err error)
```

Implementación: `tinygo.EnsureInstalled()` devuelve la ruta del **binario**; el
bindir es `filepath.Dir(...)` de eso. Para la versión, ejecuta `<dir>/tinygo
version` y devuelve su salida con `strings.TrimSpace`.

Reescribe `EnsureTinyGo` para que llame a `TinyGoBinDir` y no duplique la
lógica de instalación. Después del cambio, `tinygo.EnsureInstalled()` debe
aparecer **una sola vez** en todo el repo.

### 2. Constantes de salida — compartidas entre productor y consumidor

La action va a parsear esta salida, así que el formato es un contrato:

```go
// TinyGoBinDirPrefix marca la linea de stdout que lleva el directorio. La
// action de GitHub la corta por este prefijo; cambiarla rompe el despliegue de
// todos los consumidores.
const TinyGoBinDirPrefix = "TINYGO_BINDIR="

// TinyGoVersionPrefix marca la linea de stdout que lleva la version.
const TinyGoVersionPrefix = "TINYGO_VERSION="
```

### 3. El subcomando

En [cmd/goflare/main.go](../cmd/goflare/main.go), un `case "tinygo"` que llama a
`goflare.RunTinyGo(out io.Writer) error` — sin lógica en `main.go`.

`RunTinyGo` escribe a **stdout**, exactamente dos líneas y nada más:

```
TINYGO_BINDIR=/home/runner/.local/tinygo/bin
TINYGO_VERSION=tinygo version 0.41.1 linux/amd64 (using go version go1.25.2 and LLVM version 20.1.1)
```

Todo lo demás — el progreso de descarga, la extracción — va a **stderr**. Esto
es el contrato de CLI consumible: stdout son datos, stderr son diagnósticos. Si
el log de descarga se cuela en stdout, la action escribe basura en
`$GITHUB_PATH` y el fallo aparece mucho después, en un `tinygo: not found`
incomprensible.

Sale **0** en éxito, **1** si la instalación falla, con el motivo en stderr.

Añade a `Usage()`:

```
  tinygo    Instala TinyGo si falta e imprime su directorio bin y su versión
```

### 4. La versión por defecto, consultable sin instalar nada

La etapa 5 necesita saber qué versión de TinyGo se instalará **sin ejecutar la
instalación**, para hornearla en la key de la caché. `webtyp/tinygo` ya
publica la constante:

```go
tinygo.DefaultVersion  // "0.41.1"
```

Re-expórtala para que el generador no tenga que importar `webtyp/tinygo` por
su cuenta:

```go
// TinyGoVersion es la version de TinyGo que este binario de goflare instalara.
// Sale de la version del modulo webtyp.com/tinygo clavada en go.mod,
// asi que no hay una cifra escrita a mano que pueda quedar desactualizada.
const TinyGoVersion = tinygo.DefaultVersion
```

## Criterios de aceptación

- `goflare tinygo` imprime exactamente dos líneas en stdout, ambas con su
  prefijo, y sale con 0.
- `goflare tinygo 2>/dev/null | wc -l` → `2`. Este es el criterio que prueba que
  no se filtró ruido a stdout.
- `grep -rn "tinygo.EnsureInstalled()" .` → **una sola** aparición.
- `goflare` sin argumentos sigue imprimiendo la ayuda y saliendo con 0.
- `gotest ./...` en verde.

## Tests — en `tests/tinygo_cmd_test.go`

1. `TestTinyGoVersionIsNotEmpty` — `goflare.TinyGoVersion != ""` y parsea como
   `major.minor.patch`. Barato y atrapa el día que la constante aguas arriba
   cambie de forma.
2. `TestRunTinyGoOutputFormat` — **no** instales TinyGo de verdad. Extrae el
   formateo a una función pura y testéala:
   ```go
   func formatTinyGoOutput(dir, version string) string
   ```
   Comprueba que produce dos líneas, que cada una empieza con su constante de
   prefijo, y que no hay una tercera.

> ⚠️ **Anti-footgun.** No escribas un test que llame a `TinyGoBinDir()` de
> verdad: descargaría ~350 MB en cada corrida de CI. La parte que se testea es
> el formato; la instalación la cubre la etapa 6, donde la action se consume a
> sí misma con `uses: ./`.
