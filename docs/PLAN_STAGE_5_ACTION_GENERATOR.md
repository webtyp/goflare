← [Etapa 4](PLAN_STAGE_4_VERSION_SKEW.md) | Etapa 5 de 7 | Siguiente → [Etapa 6](PLAN_STAGE_6_ACTION_AND_RELEASE.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 5 — El generador de `action.yml`

## Por qué generarlo y no escribirlo

`action.yml` contiene valores que **caducan solos**:

- La versión de TinyGo que va en la key de la caché. Escrita a mano, se queda
  vieja en silencio.
- La versión de goflare que se descarga cuando a la action la invocan con un ref
  móvil (`@v1`).

No se puede publicar un `action.yml` desactualizado: nadie lo nota hasta que
falla un despliegue ajeno. Así que el archivo lo produce código, y un test lo
mantiene sincronizado.

**Precedente concreto de por qué importa.** La action de
[`webtyp/tinygo`](https://github.com/webtyp/tinygo/blob/main/action.yml)
tiene hoy esta key de caché:

```yaml
key: tinygo-${{ runner.os }}-${{ runner.arch }}-${{ inputs.version || 'default' }}
```

Ese literal `'default'` **nunca cambia cuando `DefaultVersion` sube de 0.41.1 a
0.42.0**. Las keys de `actions/cache` son inmutables —un acierto nunca vuelve a
guardar—, así que la caché queda clavada en un árbol viejo para siempre: cada
corrida paga el restore *y además* la descarga completa, porque el instalador
detecta la versión equivocada y reinstala. Es exactamente lo que la generación
resuelve.

## Estructura — dos piezas, una portable

La lógica de "modelar una composite action y renderizarla a YAML" no tiene nada
de goflare y va a salir de aquí a su propio repo. Así que vive separada desde el
día uno.

```
actiongen/            ← paquete portable: SOLO stdlib. Sale de aqui tal cual.
    action.go             el modelo
    render.go             modelo → bytes de YAML
    sync.go               escribir-si-difiere
actiongen/tests/      ← package actiongen_test
action_data.go        ← especifico de goflare: construye el valor y lee el tag
```

> ⚠️ **Regla dura para `actiongen/`: solo biblioteca estándar.** Ni
> `webtyp/git`, ni `webtyp/fmt`, ni nada de goflare. El día que se extraiga
> a `webtyp.com/actiongen`, el paquete tiene que moverse sin tocar una
> línea. Todo lo que dependa de goflare vive en `action_data.go`, fuera del
> paquete.

## El modelo — `actiongen/action.go`

```go
// Package actiongen modela una composite action de GitHub y la renderiza a
// YAML de forma determinista. Solo stdlib: esta pensado para vivir en su propio
// repo.
package actiongen

// KeyValue es un par ordenado. El orden es parte del contrato: dos
// generaciones del mismo valor tienen que producir bytes identicos, y un mapa
// de Go no lo garantiza.
type KeyValue struct {
	Key   string
	Value string
}

type Input struct {
	Name        string
	Description string
	Default     string
	Required    bool
}

type Output struct {
	Name        string
	Description string
	Value       string
}

type Step struct {
	Name    string
	ID      string
	If      string
	Uses    string
	With    []KeyValue
	Shell   string
	Run     string
	Env     []KeyValue
	Comment string // comentario que precede al paso, sin el "#"
}

type Branding struct {
	Icon  string
	Color string
}

type Action struct {
	Name        string
	Description string
	Author      string
	Branding    Branding
	Header      string // comentario de cabecera del archivo, sin los "#"
	Inputs      []Input
	Outputs     []Output
	Steps       []Step
}
```

> ⚠️ **Nada de `map[K]V` en ningún archivo de este repo, tampoco en los que
> llevan `//go:build !wasm`.** Es una restricción del ecosistema webtyp. Para
> pares clave-valor usa `KeyValue` como arriba. Aquí, además, hay una razón
> funcional: el recorrido de un mapa de Go es aleatorio, así que un `action.yml`
> renderizado desde un mapa saldría distinto en cada corrida y el test de drift
> fallaría siempre.

## El renderizador — `actiongen/render.go`

```go
// Render produce el YAML de la action. Es determinista: la misma Action
// produce siempre los mismos bytes.
func (a Action) Render() []byte
```

Escríbelo a mano sobre un `strings.Builder`. **No metas
`gopkg.in/yaml.v3` ni ningún otro marshaller**, por dos motivos: añade una
dependencia a un paquete que debe ser solo-stdlib, y **descarta los
comentarios**. Los comentarios de un `action.yml` son la mitad de su valor —
explican por qué cada paso está donde está — y este generador tiene que
producirlos.

Reglas de renderizado:
- Sangría de 2 espacios, siempre.
- Un campo vacío se **omite**; no se emite `if:` con valor vacío.
- Un `Run` multilínea se emite como bloque `|` con la sangría correcta.
- `Comment` se emite como líneas `# …` justo antes de su paso.
- El archivo **termina** con exactamente un `\n`.
- El archivo **empieza** con una advertencia generada:
  ```yaml
  # ARCHIVO GENERADO — no lo edites a mano.
  # Lo produce actiongen y lo mantiene sincronizado un test.
  # Para cambiarlo, edita action_data.go y corre: gotest ./...
  ```
  Esa cabecera es una constante exportada, `HeaderGenerated`, para que el test
  pueda afirmar sobre ella.

## La sincronización — `actiongen/sync.go`

```go
// Sync escribe el YAML de a en path si difiere de lo que ya hay, y reporta si
// hubo cambio. Un archivo ausente cuenta como diferencia.
//
// El patron de uso es un test: llama a Sync y falla si changed es true. Asi,
// en local re-corres el test y queda verde con el archivo ya actualizado; en
// CI, un action.yml desincronizado rompe el PR que lo desincronizo, en vez de
// romperle el despliegue al primer consumidor que lo use.
func Sync(path string, a Action) (changed bool, err error)
```

Crea los directorios padre si faltan. Compara **bytes**, no cadenas normalizadas:
un cambio de espaciado también es un cambio.

## Los datos de goflare — `action_data.go` (`//go:build !wasm`)

```go
// GoflareAction construye la descripcion de la action de goflare.
// tinyGoVersion y goflareVersion se inyectan en vez de leerse aqui para que la
// funcion sea pura y el test pueda fijarlas.
func GoflareAction(tinyGoVersion, goflareVersion string) actiongen.Action

// LatestReleaseTag devuelve el tag semver mas alto del repositorio, que es la
// version que se hornea en action.yml para los refs moviles como @v1.
func LatestReleaseTag() (string, error)
```

`LatestReleaseTag` usa `webtyp.com/git`, que **ya resuelve esto
correctamente** y lo hace en un solo lugar del ecosistema:

```go
g, err := git.NewGit()
tag, err := g.GetLatestTag()   // "git tag -l --sort=-version:refname", primera linea
```

> ⚠️ **No escribas tu propio `git describe --tags --abbrev=0`.** Devuelve el tag
> alcanzable más cercano desde HEAD, no el semver más alto — es un bug sutil que
> `webtyp/git` ya documenta y evita. Reusa la función.

La versión de TinyGo sale de la constante de la etapa 3, `goflare.TinyGoVersion`,
que a su vez viene de `tinygo.DefaultVersion`. **Ninguna cifra escrita a mano.**

### Constantes obligatorias en `action_data.go`

```go
const (
	// ActionFilePath es donde vive el action.yml generado, relativo a la raiz
	// del modulo. GitHub exige ese nombre en la raiz del repo.
	ActionFilePath = "action.yml"

	// ReleaseAssetURLFmt arma la URL de descarga de un binario publicado.
	// Argumentos: version, nombre del asset.
	ReleaseAssetURLFmt = "https://github.com/webtyp/goflare/releases/download/%s/%s"

	// TinyGoCacheKeyFmt es la key de actions/cache del arbol de TinyGo. La
	// version va DENTRO de la key a proposito: las keys de actions/cache son
	// inmutables, asi que una key sin version se queda clavada en un arbol
	// viejo cuando DefaultVersion sube.
	TinyGoCacheKeyFmt = "tinygo-${{ runner.os }}-${{ runner.arch }}-%s"
)
```

## El contenido exacto de la action

Está especificado en la [etapa 6](PLAN_STAGE_6_ACTION_AND_RELEASE.md), que es
donde se commitea el archivo y se verifica. Esta etapa construye la maquinaria;
la siguiente define qué produce.

## Criterios de aceptación

- `grep -rn "map\[" actiongen/` → vacío.
- `grep -rn "gopkg.in/yaml" .` → vacío.
- `ls actiongen/*.go | xargs grep -l "github.com/webtyp"` → vacío (el paquete
  portable no importa nada del ecosistema).
- `grep -rn "git describe" .` → vacío.
- Llamar a `Render()` dos veces sobre el mismo `Action` produce bytes
  idénticos.
- `gotest ./...` en verde.

## Tests

### `actiongen/tests/render_test.go` (`package actiongen_test`)

1. `TestRenderIsDeterministic` — construye un `Action` con dos inputs, dos
   outputs y tres pasos; renderiza dos veces; `bytes.Equal`.
2. `TestRenderOmitsEmptyFields` — un `Step` con solo `Name` y `Run` no produce
   ninguna línea `if:`, `uses:` ni `id:`.
3. `TestRenderMultilineRun` — un `Run` de tres líneas sale como bloque `|` y
   cada línea queda con la sangría correcta.
4. `TestRenderEmitsComments` — un `Step` con `Comment` produce líneas `# …`
   antes del paso.
5. `TestRenderStartsWithGeneratedHeader` — la salida empieza con
   `actiongen.HeaderGenerated`.
6. `TestRenderEndsWithSingleNewline`.

### `tests/action_sync_test.go` (`package goflare_test`) — **el test que mantiene el archivo**

```go
func TestActionYmlIsInSync(t *testing.T) {
	tag, err := goflare.LatestReleaseTag()
	if err != nil {
		t.Fatal(err)
	}
	a := goflare.GoflareAction(goflare.TinyGoVersion, tag)
	changed, err := actiongen.Sync(goflare.ActionFilePath, a)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("%s estaba desincronizado y se acaba de regenerar. "+
			"Revisa el diff y commitealo; despues este test queda verde.",
			goflare.ActionFilePath)
	}
}
```

Ese cuerpo es el contrato completo de la etapa: **escribe, después falla si
había diferencia.** En local se arregla re-corriendo; en CI rompe el PR que
introdujo la desincronización.

> ⚠️ **Anti-footgun.** El test escribe en la raíz del repo, no en un directorio
> temporal — es a propósito, es el archivo que se publica. Pero eso significa
> que `goflare.ActionFilePath` se resuelve relativo al directorio de trabajo del
> test. Como los tests viven en `tests/`, el test tiene que resolver la ruta
> contra la raíz del módulo. Usa `filepath.Join("..", goflare.ActionFilePath)`
> y déjalo comentado, o el test escribirá `tests/action.yml` y nunca detectará
> nada.

> ⚠️ **Anti-footgun.** En un checkout sin tags (`actions/checkout` con
> `fetch-depth: 1` no los trae), `LatestReleaseTag()` devuelve cadena vacía. En
> ese caso el test debe hacer `t.Skip` con un motivo claro, no fallar: si no,
> CI regenera `action.yml` con una URL vacía y rompe a todos los consumidores.
> El workflow de la etapa 6 hace checkout con `fetch-tags: true` justamente por
> esto, pero el skip es la red de seguridad.
