Etapa 1 de 7 | Siguiente → [Etapa 2](PLAN_STAGE_2_SIZE_DIAGNOSTIC.md) | Índice: [PLAN.md](PLAN.md)

# Etapa 1 — Reportar el tamaño siempre, avisar en 256 KiB, abortar en 900 KiB

## El problema exacto

[build.go](../build.go) contiene hoy:

```go
const (
	// maxWasmSize is the Cloudflare Workers Free limit for the WASM binary.
	// https://developers.cloudflare.com/workers/platform/limits/#worker-size
	maxWasmSize = 1 * 1024 * 1024 // 1 MiB
	...
)

func checkWasmSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("wasm size check: %w", err)
	}
	size := info.Size()
	if size > maxWasmSize {
		return fmt.Errorf(
			"edge.wasm exceeds Cloudflare Free limit: %d bytes (%.1f KiB) > 1 MiB — "+
				"reduce binary size or upgrade to a paid plan",
			size, float64(size)/1024,
		)
	}
	return nil
}
```

Tres defectos, todos reales:

1. **El dato es falso.** El límite Free de Cloudflare no es 1 MiB. Es **3 MB
   comprimido (gzip)** y 64 MB sin comprimir; en Paid, 10 MB comprimido. El
   mensaje le miente al usuario y le sugiere pagar un plan que no necesita.
2. **Mide la magnitud equivocada.** El límite de Cloudflare es sobre el tamaño
   comprimido. `veltylabs/iam` hoy: 568.097 B crudo → **202.745 B gzip**, o sea
   el 6,6 % del límite Free. Medir 568.097 contra 1.048.576 no dice nada útil.
3. **Es mudo.** No recibe `Logger`. Solo devuelve `error`, y ese error no se ha
   disparado nunca. Por eso los logs de CI jamás dijeron cuánto pesaba el
   binario que se estaba subiendo.

## Lo que hay que construir

Un archivo nuevo **`size.go`** (`//go:build !wasm`) con toda la lógica de
tamaño, y `build.go` reducido a llamarla.

### Constantes — en `size.go`, exportadas

```go
const (
	// WasmWarnSizeKiB es el umbral de aviso sobre el tamaño CRUDO. Por encima
	// de aqui cada KiB se paga en tiempo de arranque del isolate: Cloudflare
	// da 1 s para parsear e instanciar el modulo antes de atender la primera
	// peticion, y un wasm grande consume ese presupuesto.
	WasmWarnSizeKiB = 256

	// WasmMaxSizeKiB es el presupuesto PROPIO de goflare, no un limite de
	// Cloudflare. Cloudflare acepta hasta 3 MB comprimido en Free y 10 MB en
	// Paid; este corte existe para detener el despliegue antes de gastar una
	// subida cuando el binario se disparo.
	WasmMaxSizeKiB = 900

	// EnvKeyWasmWarnSizeKiB permite subir o bajar el aviso sin recompilar.
	EnvKeyWasmWarnSizeKiB = "WASM_WARN_SIZE_KIB"

	// EnvKeyWasmMaxSizeKiB permite subir o bajar el corte duro sin recompilar.
	EnvKeyWasmMaxSizeKiB = "WASM_MAX_SIZE_KIB"

	// bytesPerKiB evita el literal 1024 repartido por el archivo.
	bytesPerKiB = 1024
)
```

### La función principal

```go
// WasmSizes son las dos magnitudes que importan de un artefacto del edge: la
// cruda, que es la que el isolate compila e instancia, y la comprimida, que es
// la que Cloudflare pesa contra su limite.
type WasmSizes struct {
	Raw  int64
	Gzip int64
}

// MeasureWasm devuelve el tamaño crudo y el comprimido con gzip del archivo.
// Comprime en memoria y descarta el resultado: solo interesa el largo.
func MeasureWasm(path string) (WasmSizes, error)

// CheckWasmSize mide el artefacto, escribe SIEMPRE el reporte en log, avisa si
// supera el umbral de aviso, y devuelve error si supera el presupuesto.
// log nunca es nil: el llamador pasa g.Logger.
func CheckWasmSize(path string, log func(...any)) error
```

`MeasureWasm` usa `compress/gzip` con `gzip.BestCompression` sobre un
`io.Discard` envuelto en un contador, o sobre un `bytes.Buffer` — cualquiera de
las dos sirve; no leas el archivo entero dos veces.

### Salida — el formato es el producto

El reporte se emite **siempre**, en cada `goflare build`, tanto local como en CI:

```
edge.wasm: 568097 B crudo (554.8 KiB) | 202745 B gzip (198.0 KiB)
```

Si el crudo llega o supera el umbral de aviso, una línea más, a stderr vía el
logger:

```
warning: edge.wasm pesa 554.8 KiB crudo, sobre el umbral de aviso de 256.0 KiB — cada KiB se paga en tiempo de arranque del isolate (Cloudflare da 1 s). El limite duro de Cloudflare es sobre el gzip: 3 MB en Free, 10 MB en Paid.
```

Si el crudo supera el presupuesto, `CheckWasmSize` devuelve este error
**textualmente** (es la cadena que ve el usuario en CI):

```
edge.wasm pesa 950.0 KiB crudo, sobre el presupuesto de 900.0 KiB que impone goflare — despliegue detenido antes de gastar la subida. Este presupuesto es de goflare, no de Cloudflare (cuyo limite es 3 MB gzip en Free). Reduce el binario o sube WASM_MAX_SIZE_KIB.
```

Las tres cadenas son constantes con formato (`const wasmSizeReportFmt = ...`),
no literales dispersos.

### Lectura de los umbrales

Una función no exportada por umbral, con la misma forma que
`compatibilityDate()` en [config.go](../config.go):

```go
func wasmWarnSizeBytes() int64  // EnvKeyWasmWarnSizeKiB o WasmWarnSizeKiB
func wasmMaxSizeBytes() int64   // EnvKeyWasmMaxSizeKiB  o WasmMaxSizeKiB
```

Un valor de entorno que no parsea como entero positivo se **ignora** y se usa el
default; no es un error fatal. Un valor de `0` **desactiva** ese umbral (útil
para un proyecto que a propósito no quiere el corte).

## Cambios en `build.go`

1. **Borra** la constante `maxWasmSize` y **toda** la función `checkWasmSize`.
2. En `buildWorker()`, el paso 6 pasa de:
   ```go
   wasmPath := filepath.Join(g.Config.OutputDir, "edge.wasm")
   if err := checkWasmSize(wasmPath); err != nil {
       return err
   }
   ```
   a:
   ```go
   wasmPath := filepath.Join(g.Config.OutputDir, "edge.wasm")
   if err := CheckWasmSize(wasmPath, g.Logger); err != nil {
       return err
   }
   ```
3. El literal `"edge.wasm"` ya aparece tres veces en `buildWorker`. Extráelo a
   `const WasmArtifactName = "edge.wasm"` en `size.go` y úsalo en las tres.

## Criterios de aceptación

- `grep -rn "maxWasmSize" .` → vacío.
- `grep -rn "func checkWasmSize" .` → vacío.
- `grep -rn "Cloudflare Free limit" .` → vacío (el mensaje falso desapareció).
- `grep -c '"edge.wasm"' build.go` → `0`.
- `gotest ./...` en verde.
- Un `goflare build` sobre un proyecto real imprime la línea de reporte con
  ambas cifras.

## Tests — en `tests/size_test.go`, `package goflare_test`

1. `TestMeasureWasm` — escribe un archivo temporal con contenido conocido y
   altamente comprimible; comprueba `Raw` exacto y `Gzip < Raw`.
2. `TestCheckWasmSizeAlwaysReports` — un archivo de 10 KiB; captura el logger en
   un slice; comprueba que se emitió exactamente una línea y que contiene tanto
   `crudo` como `gzip`. **Este es el test que evita la regresión que motivó la
   etapa**: el reporte debe salir incluso cuando todo está bien.
3. `TestCheckWasmSizeWarns` — archivo de 300 KiB; `error` nil, pero el logger
   recibió una línea que contiene `warning:`.
4. `TestCheckWasmSizeAborts` — archivo de 1 MiB; `error` no nil y su texto
   contiene `presupuesto`.
5. `TestWasmSizeThresholdsFromEnv` — con `WASM_MAX_SIZE_KIB=1` un archivo de
   10 KiB falla; con `WASM_MAX_SIZE_KIB=0` el mismo archivo pasa. Usa
   `t.Setenv`.

Para generar archivos grandes en los tests, escribe bytes repetidos con
`bytes.Repeat`; no incluyas binarios en el repo.

## Lo que NO hay que hacer en esta etapa

- **No** toques `webtyp/sitec` ni cambies los flags con que se invoca TinyGo.
- **No** añadas el desglose por paquete aquí: es la etapa 2.
- **No** apliques el gate a `client.wasm` (el frontend). El límite y el costo de
  arranque de los que habla esta etapa son del Worker. `client.wasm` lo descarga
  un navegador y tiene otro presupuesto.
