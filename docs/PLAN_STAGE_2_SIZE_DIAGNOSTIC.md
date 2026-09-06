← [Etapa 1](PLAN_STAGE_1_SIZE_GATE.md) | Etapa 2 de 7 | Siguiente → [Etapa 3](PLAN_STAGE_3_TINYGO_SUBCOMMAND.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 2 — `goflare size`: el instrumento para saber qué pesa

## Para qué

La etapa 1 dice **cuánto** pesa. Esta dice **de qué**. Sin ella, cualquier
intento de adelgazar el binario es adivinar, y el ecosistema ya pagó una vez el
precio de refactorizar sobre una premisa equivocada.

Este subcomando es la herramienta que consumirá el plan de latencia de
`veltylabs/iam`. No se usa en el camino de despliegue.

## Descubrimiento clave — no lo re-descubras

TinyGo acepta `-size=full` para imprimir el desglose por paquete, **pero con
`-no-debug` la atribución se pierde entera**. Comprobado:

```
$ tinygo build -target wasm -no-debug -size=full -o out.wasm main.go
warning: data incomplete, remove the -no-debug flag for more detail
   code  rodata    data     bss |   flash     ram | package
 235047       0  121076   75532 |  356123  196608 | (unknown)
```

Todo cae en `(unknown)`. Con la información de depuración activada, el mismo
comando produce una tabla de ~80 filas con una línea por paquete.

**Consecuencia de diseño:** la compilación de diagnóstico es una compilación
**aparte, con símbolos**, cuyo `.wasm` se descarta. Nunca reemplaza ni altera la
compilación que se despliega. El binario que se sube sigue saliendo de `sitec`
con `-no-debug`, sin cambios.

## Lo que hay que construir

Un archivo nuevo **`diagnose.go`** (`//go:build !wasm`).

### 1. Desglose por paquete

```go
// SizeBreakdown compila entryDir con simbolos y devuelve la tabla por paquete
// que emite TinyGo. El artefacto se escribe en un directorio temporal y se
// borra: solo interesa el reporte. NUNCA sustituye a la compilacion que se
// despliega, que sigue saliendo de sitec sin simbolos.
func SizeBreakdown(entryDir string) (string, error)
```

Implementación: `EnsureTinyGo` primero (ya existe en [wasm.go](../wasm.go)),
después `exec.Command` con estos argumentos exactos, en este orden, y
`cmd.Dir = entryDir`:

```go
"tinygo", "build", "-target", "wasm", "-size=full", "-o", tmpPath, "main.go"
```

Sin `-no-debug` — es justo el flag que hay que omitir. `GOOS=js` y `GOARCH=wasm`
en el entorno. La tabla sale por **stderr** de TinyGo, así que captura
`CombinedOutput()` y devuélvelo tal cual: el formato de TinyGo es el producto,
no lo re-tabules.

Constantes para cada literal de flag, como en
[depguard.go de gobuild](https://github.com/webtyp/gobuild/blob/main/depguard.go).

### 2. Guarda de imports prohibidos

`webtyp.com/gobuild` **ya publica** exactamente lo que hace falta, y
**nadie lo llama**. Son funciones libres exportadas — no hace falta construir un
`GoBuild`:

```go
var  gobuild.ForbiddenWASMImports []string   // bytes, encoding/json, errors, fmt, io, reflect, strconv, strings, unicode
func gobuild.CheckForbiddenImports(dir, pkg, goos, goarch string, forbidden []string) ([]gobuild.ImportChain, error)
func gobuild.FormatChains(chains []gobuild.ImportChain) string

type gobuild.ImportChain struct {
	Forbidden string   // el paquete prohibido que se alcanzo
	Path      []string // raiz → … → Forbidden
}
```

Añade `webtyp.com/gobuild` a `go.mod` y expón:

```go
// ForbiddenImports devuelve solo las cadenas ACCIONABLES hacia stdlib
// prohibida en el grafo del edge. Ver filterActionable para que significa eso.
func ForbiddenImports(moduleRoot, entryPkg string) ([]gobuild.ImportChain, error)
```

llamando a `gobuild.CheckForbiddenImports(moduleRoot, entryPkg, "js", "wasm",
gobuild.ForbiddenWASMImports)`.

### 3. El filtro que hace útil a la guarda — léelo antes de implementar

Corriendo la guarda cruda contra `veltylabs/iam` hoy, reporta cadenas hacia
`bytes`, `errors`, `io`, `strconv`, `strings` y `unicode`. **Ninguna es
accionable**, y una guarda que grita sobre lo inarreglable se apaga y deja de
servir.

El motivo: los seis se alcanzan **solo a través de la biblioteca estándar**,
casi todos bajo `crypto/*`. Medido en el grafo real del edge de iam:

```
bytes  <-  crypto/cipher, crypto/internal/fips140/aes, .../drbg, .../hmac, .../sha256, .../sha3, .../sha512
strings <- crypto/internal/fips140
os      <- crypto/internal/sysrand
```

Y **cero** paquetes `github.com/webtyp/*` o `github.com/veltylabs/*` importan
directamente ninguno de los prohibidos. La disciplina del ecosistema está
intacta; lo que se ve es la consecuencia de una decisión ya tomada y
documentada: `webtyp/crypto` usa `crypto/*` de stdlib a propósito, porque
reimplementar primitivas criptográficas a mano sería más lento y menos seguro
(ver [AGENTS.md de webtyp/crypto](https://github.com/webtyp/crypto/blob/main/AGENTS.md),
sección "The stdlib rule — and its one carve-out").

Por eso la regla del filtro es:

> **Una cadena es accionable si y solo si el primer paquete de stdlib que
> aparece en ella ES el paquete prohibido.** Es decir: alguien fuera de stdlib lo
> importa directamente. Si el prohibido se alcanza *a través* de otro paquete de
> stdlib, la cadena se descarta: no hay nada que arreglar en nuestro código.

```go
// filterActionable descarta las cadenas donde el paquete prohibido se alcanza a
// traves de otro paquete de stdlib. Esas no las causa nuestro codigo y no las
// puede arreglar nuestro codigo; reportarlas solo entrena al usuario a ignorar
// la guarda.
func filterActionable(chains []gobuild.ImportChain) []gobuild.ImportChain
```

Para decidir si un import path es de stdlib basta con que su **primer segmento
no contenga un punto** (`"crypto/sha256"` sí es stdlib, `"github.com/x/y"` no).
Esa es la misma heurística que usa `go list`. Ponla en una función nombrada,
`isStdlib(importPath string) bool`, con su propio test.

## El subcomando

En [cmd/goflare/main.go](../cmd/goflare/main.go), un `case "size"` nuevo que
llama a una única función de librería `goflare.RunSize(envPath string, out
io.Writer) error`, siguiendo exactamente el patrón de `RunBuild`. **Ninguna
lógica en `main.go`.**

`RunSize` hace, en orden:
1. `LoadConfigFromEnv` → `cfg.Entry` (por convención, `"edge"`).
2. Imprime el reporte de tamaño de la etapa 1 si el artefacto ya existe.
3. Imprime el desglose de `SizeBreakdown`.
4. Imprime las cadenas accionables de `ForbiddenImports`, o la línea
   `sin imports de stdlib prohibidos alcanzados directamente` si no hay ninguna.

Sale **0** aunque haya cadenas accionables: es un diagnóstico, no un gate. El
gate es la etapa 1.

Añade la entrada a `Usage()`:

```
  size      Desglosa el tamaño del wasm del edge por paquete y lista imports prohibidos
```

## Criterios de aceptación

- `goflare size` sobre un proyecto con `edge/main.go` imprime una tabla con más
  de una fila de paquete (si imprime solo `(unknown)`, se coló `-no-debug`).
- `grep -n "no-debug" diagnose.go` → vacío.
- `grep -n "webtyp.com/gobuild" go.mod` → presente.
- `goflare size` sale con código **0**.
- `gotest ./...` en verde.

## Tests — en `tests/diagnose_test.go`

1. `TestIsStdlib` — tabla: `"bytes"`, `"crypto/sha256"`, `"internal/poll"` →
   `true`; `"webtyp.com/fmt"`, `"golang.org/x/net/html"` → `false`.
2. `TestFilterActionable` — construye a mano dos `ImportChain`: una donde el
   prohibido lo importa `webtyp.com/ejemplo` directamente (se
   conserva), y otra donde el camino pasa por `crypto/sha256` antes de llegar a
   `bytes` (se descarta). **No** invoques la cadena de herramientas real: este
   test debe correr sin TinyGo instalado.
3. `SizeBreakdown` **no** se testea con una compilación real (requiere TinyGo y
   tarda). Extrae el armado de argumentos a
   `func sizeBreakdownArgs(tmpPath string) []string` y testea eso: comprueba que
   contiene `-size=full` y que **no** contiene `-no-debug`.

## Lo que NO hay que hacer

- **No** conectes esta guarda al camino de `build` ni de `deploy`. Un import
  prohibido no debe romper un despliegue todavía; primero hay que ver qué
  reporta en los proyectos reales.
- **No** modifiques `gobuild`. Sus funciones exportadas bastan.
- **No** intentes reducir el árbol de `crypto/*`. Es una decisión de
  arquitectura ya tomada y documentada aguas arriba.
