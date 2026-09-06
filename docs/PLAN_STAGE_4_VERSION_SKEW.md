← [Etapa 3](PLAN_STAGE_3_TINYGO_SUBCOMMAND.md) | Etapa 4 de 7 | Siguiente → [Etapa 5](PLAN_STAGE_5_ACTION_GENERATOR.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 4 — Fallar cuando el runtime del proyecto y el del binario divergen

## El riesgo que esta etapa cierra

goflare empaqueta el pegamento JavaScript del Worker: `wasm_exec_worker.js`,
`worker.mjs` y `runtime.mjs`, que desde el split vienen embebidos de
`webtyp.com/cloudflare/assets` y llegan al binario **en la versión con
que se compiló goflare**. El proyecto, en cambio, importa
`webtyp.com/cloudflare/edge` en la versión que declare **su propio
`go.mod`**.

Mientras goflare se ejecutaba con `go run` desde el `go.mod` del proyecto, las
dos versiones eran forzosamente la misma. **Con un binario descargado dejan de
serlo**, y nada lo detecta.

Esto no es hipotético. El pegamento JS y el runtime Go comparten una ABI: el
episodio de `runtime.ticks` —que devolvía `float64` en milisegundos cuando
TinyGo espera `int64` en nanosegundos como `BigInt`— hacía que el Worker
abortara durante la inicialización de paquetes, antes de que `main()` corriera,
sin un solo mensaje de error. Costó horas de diagnóstico. Una divergencia de
versiones entre el JS embebido y el Go compilado reproduce exactamente esa clase
de fallo, y el binario descargado la vuelve fácil de provocar.

La divergencia **ya existe**: `veltylabs/iam` clava `webtyp.com/cloudflare v0.0.4`
mientras la última publicada es `v0.0.9`.

## Lo que hay que construir

Un archivo nuevo **`skew.go`** (`//go:build !wasm`).

### 1. La versión que lleva el binario

`runtime/debug` la publica sin necesidad de generar código:

```go
// EmbeddedCloudflareVersion devuelve la version de webtyp.com/cloudflare
// con la que se compilo ESTE binario, que es la de los assets JS que lleva
// embebidos. Cadena vacia si la informacion de build no esta disponible (por
// ejemplo bajo `go test`, donde el modulo principal no es goflare).
func EmbeddedCloudflareVersion() string
```

Implementación: `debug.ReadBuildInfo()`, recorrer `info.Deps` buscando
`dep.Path == CloudflareModulePath`, devolver `dep.Version`. Constante:

```go
// CloudflareModulePath es el modulo del runtime del edge. goflare embebe sus
// assets JS; el proyecto compila su codigo Go. Las dos mitades tienen que venir
// de la misma version.
const CloudflareModulePath = "webtyp.com/cloudflare"
```

### 2. La versión que usa el proyecto

**No parsees `go.mod` a mano y no escribas otro bucle de `go list -m`.**
`webtyp.com/modfind` centraliza justo eso, ya está en el `go.mod` de
este repo (hoy como `// indirect`; pásalo a directo) y cachea el resultado:

```go
f := modfind.New()
mods, err := f.Discover(moduleRoot)   // []modfind.Module
// modfind.Module tiene los campos Path y Version
```

```go
// ProjectCloudflareVersion devuelve la version de webtyp.com/cloudflare
// que resuelve el go.mod del proyecto en moduleRoot. Cadena vacia si el
// proyecto no depende del runtime del edge, que es legitimo: un sitio de solo
// assets estaticos no lo necesita.
func ProjectCloudflareVersion(moduleRoot string) (string, error)
```

### 3. La comprobación

```go
// CheckVersionSkew falla si el proyecto y este binario resuelven versiones
// distintas de webtyp/cloudflare. No hacerlo deja que el pegamento JS
// embebido y el runtime Go compilado se desincronicen, y esa clase de fallo se
// manifiesta como un Worker que aborta durante la inicializacion de paquetes,
// sin mensaje.
func CheckVersionSkew(moduleRoot string) error
```

Tabla de decisión — impleméntala tal cual:

| Versión del proyecto | Versión embebida | Resultado |
|---|---|---|
| vacía | cualquiera | **ok**, sin mensaje. El proyecto no usa el runtime del edge. |
| cualquiera | vacía | **ok**, con aviso al log. Pasa bajo `go test` y con `go run`; no hay nada que comparar. |
| iguales | iguales | **ok**, sin mensaje. |
| distintas | distintas | **error** |

El error, textualmente:

```
desajuste de versiones de webtyp/cloudflare: tu go.mod resuelve v0.0.4 y este binario de goflare lleva embebidos los assets JS de v0.0.9. El pegamento JavaScript y el runtime Go del Worker comparten una ABI; si divergen, el Worker aborta al inicializar paquetes sin dejar mensaje. Corrige con: go get webtyp.com/cloudflare@v0.0.9
```

Las dos versiones y el `go get` sugerido salen de variables, no de literales:
usa una constante de formato `const skewErrFmt = ...`.

## Dónde se llama

En `RunDeploy` ([run.go](../run.go)), **después** de `cfg.ValidateDeploy()` y
**antes** de `g.Auth()`. Ahí es donde el fallo sale más barato: antes de pedir
un token y antes de gastar una subida.

**No** la llames en `RunBuild`. Compilar con versiones distintas es útil durante
el desarrollo (por ejemplo, probando un `replace` local del runtime), y romper
ahí estorbaría sin proteger nada: el artefacto no sale de la máquina.

## Criterios de aceptación

- `grep -rn "go list -m" .` → vacío (nada de bucles propios; todo va por
  `modfind`).
- `grep -n "webtyp.com/modfind" go.mod` → aparece como dependencia
  **directa**, sin el comentario `// indirect`.
- `grep -rn "\"webtyp.com/cloudflare\"" . | grep -v _test` → solo la
  constante `CloudflareModulePath`.
- Un `goflare deploy` en un proyecto sin desajuste no imprime nada nuevo.
- `gotest ./...` en verde.

## Tests — en `tests/skew_test.go`

`CheckVersionSkew` no es testeable directamente porque `EmbeddedCloudflareVersion`
depende de cómo se compiló el binario. Extrae la decisión a una función pura y
testea **esa**:

```go
// compareVersions implementa la tabla de decision. Separada de la lectura del
// entorno justo para poder testear las cuatro filas sin compilar nada.
func compareVersions(project, embedded string) error
```

1. `TestCompareVersions` — tabla con las cuatro filas: `("", "v0.0.9")` → nil;
   `("v0.0.4", "")` → nil; `("v0.0.9", "v0.0.9")` → nil; `("v0.0.4", "v0.0.9")`
   → error cuyo texto contiene ambas versiones y la cadena `go get`.
2. `TestProjectCloudflareVersionAbsent` — un directorio temporal con un `go.mod`
   mínimo que no requiere `webtyp/cloudflare`; devuelve cadena vacía y `nil`,
   **no** un error.

> ⚠️ **Anti-footgun.** No hagas que la comprobación exija igualdad exacta de
> tags para el caso `replace`. Si el proyecto tiene un `replace` local hacia
> `webtyp/cloudflare`, `modfind` reporta `Version` vacía y la primera fila de
> la tabla ya lo deja pasar. Un desarrollador con un `replace` local está
> probando a propósito y no debe ser bloqueado.
