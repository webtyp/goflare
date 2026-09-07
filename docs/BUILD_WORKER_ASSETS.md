# Build & Deploy — Worker con Assets Estáticos

`goflare` compila y despliega aplicaciones a Cloudflare como un **Worker con assets estáticos** en un solo comando (`goflare deploy`).

---

## Modos de proyecto y artefactos

Cualquier proyecto con `edge/main.go` genera exactamente el mismo par de artefactos en `.build/`:

- `.build/edge.js` — el pegamento JavaScript empaquetado y minificado.
- `.build/edge.wasm` — el binario compilado de Go (goflare valida el presupuesto de 900 KiB crudo).

Si el proyecto contiene `web/public/`, sus archivos se procesan como assets estáticos.

| Estructura del proyecto | Assets | Script Worker |
|---|---|---|
| Sitio estático (sin `edge/main.go`) | Sí | No |
| App con servidor (`edge/main.go` + `web/public/`) | Sí | Sí |
| Worker sin frontend (`edge/main.go` sin `web/public/`) | No | Sí |

---

## Proceso de Despliegue (API de 3 Fases)

El despliegue combina la API Direct Upload de Assets de Cloudflare Workers con el `PUT` del Worker:

1. **Fase 1 — Sesión de subida (`assets-upload-session`):**
   `goflare` genera un manifiesto de hashes sha256 truncados a 32 caracteres hexadecimales para todos los archivos estáticos y solicita una sesión de subida a `/accounts/{account_id}/workers/scripts/{script_name}/assets-upload-session`.

2. **Fase 2 — Subida de bloques (`/workers/assets/upload`):**
   Si Cloudflare solicita subir archivos (buckets no vacíos), `goflare` sube los datos codificados en base64 usando el JWT obtenido en la Fase 1 como cabecera de autorización. Devuelve un **token de finalización** (validado por 1 hora).

3. **Fase 3 — Despliegue del Worker (`PUT /accounts/.../workers/scripts/{script_name}`):**
   Envía la petición multipart/form-data con los artefactos `.build/edge.js` y `.build/edge.wasm`, junto al campo `metadata` configurado:

```json
{
  "main_module": "edge.js",
  "compatibility_date": "2026-08-01",
  "assets": {
    "jwt": "<COMPLETION_TOKEN>",
    "config": {
      "html_handling": "auto-trailing-slash",
      "not_found_handling": "single-page-application",
      "run_worker_first": ["/api/contacto", "/static*"]
    }
  },
  "bindings": []
}
```

Cada parte del multipart lleva su propio `Content-Type` — Cloudflare lo usa
para distinguir un módulo ES de un blob opaco, independientemente del
nombre de campo:

| Parte | Content-Type |
|---|---|
| `edge.js` (`main_module`) | `application/javascript+module` |
| `edge.wasm` | `application/wasm` |

> *Nunca uses `multipart.Writer.CreateFormFile` para estas partes — la
> librería estándar de Go fija `application/octet-stream` sin posibilidad
> de override, y Cloudflare responde `Main module must be an ES module.`
> (code 10021). Construye la cabecera a mano con `textproto.MIMEHeader` +
> `mw.CreatePart`, como ya hace `uploadAssets` en `assets.go`.*

---

## Enrutamiento y `run_worker_first`

Por defecto los assets estáticos tienen prioridad sobre el Worker. Con
`not_found_handling: "single-page-application"`, cualquier petición que no
coincida con un archivo estático devuelve `index.html` con estado **`200`** —
sin registrar nada. Si esa petición era una ruta dinámica, el Worker nunca se
ejecuta y el llamador recibe una página en vez de su respuesta.

Para evitarlo, `goflare` deriva `run_worker_first` de **las rutas que el
proyecto declara** en `routes/routes.go`, leídas en tiempo de build con
`webtyp.com/router/routescan`:

- Cada ruta declarada produce una entrada, en orden de aparición, sin
  duplicados.
- Una ruta con un segmento `{param}` se recorta en ese segmento y se le añade
  `*` (`/api/orders/{id}` → `/api/orders/*`).
- Un `Mount` o un `PublicDir` ya llegan como prefijo (`/api/auth*`,
  `/static*`) y se usan tal cual.
- Un proyecto **sin `routes/routes.go` no declara rutas dinámicas**: la lista va
  vacía. No se envían prefijos adivinados.
- Si `routes/routes.go` no se puede leer (una ruta que no es literal ni const
  del mismo archivo), el deploy **falla**: no hay lista por defecto.

> *Si una ruta de tu aplicación devuelve el HTML del sitio en vez de su
> respuesta, falta en `routes/routes.go` — declárala ahí y el deploy la
> incluirá en `run_worker_first`.*

---

## Secretos y Bindings

Los bindings de D1 y R2 se configuran automáticamente si se definen `D1_DATABASE_ID` o `R2_BUCKET_ID`.

> *Las variables de ejecución se cargan como Secret. Una variable de texto plano creada en el panel desaparece en el siguiente despliegue: el `metadata` del despliegue es la fuente de verdad para todo binding que no sea secreto.*

---

## Verificación posterior al despliegue

Tras completar el `PUT`, `goflare` realiza una sonda automática `GET /api/__goflare_probe` reintentando con backoff exponencial. Comprueba que la respuesta contenga la cabecera `x-goflare`. Si la cabecera está ausente, el despliegue falla y alerta al pipeline de CI.

---

## Ciclo de vida del Worker

> **Nota:** el JS runtime (`worker.mjs`, `runtime.mjs`, `wasm_exec_worker.js`) y el
> Go runtime (`edge/`, `workers/`, `d1/`, `r2/`, `log/`) viven en
> **`webtyp/cloudflare`** — `goflare` sólo los empaqueta desde
> `webtyp/cloudflare/assets` (`javascripts.go`).

La instancia de Go se crea **una vez por isolate**, no una vez por petición: la
primera petición paga la instanciación del WASM y el `main()` completo, y todas
las siguientes reutilizan esa instancia a través de `binding.handleRequest`.

> *`main()` se ejecuta una sola vez. Todo lo que haga — sincronizar el esquema,
> leer secretos, construir el router — es coste de arranque del isolate, no de
> cada petición.*

El handshake de arranque viaja por `context.binding.ready`, nunca por una global
compartida: `wasm_exec_worker.js` sólo aísla la propiedad `context` por
instancia, así que cualquier otro nombre global es un único objeto compartido por
todo el isolate.

### Si `main()` retorna sin registrar el handler

Como el arranque se cachea para toda la vida del isolate, un `main()` que retorna
antes de llamar a `Handle()` dejaría el arranque pendiente y colgaría **todas**
las peticiones siguientes sin registrar nada. `worker.mjs` observa la promesa de
`go.run` justamente para evitarlo y falla con:

```
goflare: Go main() returned without registering a request handler — check the
Worker's logs for the error it printed before returning
```

> *Si ves ese error, la causa está en lo que tu `main()` imprimió antes de
> retornar — típicamente un binding o un secreto ausente. Sin esta comprobación
> el síntoma sería un cuelgue mudo que Cloudflare reporta como `the Workers
> runtime canceled this request because it detected that your Worker's code had
> hung`, sin ninguna pista del origen.*

---

## Variables de Entorno

| Variable | Descripción | Valor por defecto |
|---|---|---|
| `PROJECT_NAME` | Nombre del proyecto | Requerido |
| `CLOUDFLARE_ACCOUNT_ID` | ID de la cuenta Cloudflare | Requerido |
| `WORKER_NAME` | Nombre del Worker | `<PROJECT_NAME>-worker` |
| `DOMAIN` | Dominio personalizado | Opcional |
| `COMPATIBILITY_DATE` | Fecha de compatibilidad de Workers | `2026-08-01` |
| `NOT_FOUND_HANDLING` | Manejo de 404 para assets | `single-page-application` |
