# Servidores MCP de Cloudflare (Claude Code)

Guía breve de los servidores MCP remotos de Cloudflare: qué hace cada uno, cómo
configurarlos en Claude Code y cómo autorizarlos **sin exponer la cuenta principal**.

> Fuente: `developers.cloudflare.com/agents/model-context-protocol/cloudflare/servers-for-cloudflare/`
> (última verificación: 2026-08-22).

---

## 1. Qué es cada servidor

Todos son **remotos** (los ejecuta Cloudflare, no tu máquina), hablan *Streamable HTTP*
en `/mcp` y se autentican por **OAuth** contra tu cuenta de Cloudflare. Las URLs `/sse`
antiguas siguen vivas como alias, pero para conexiones nuevas se usa `/mcp`.

### Configurados actualmente

| Servidor | URL | Auth | Para qué sirve |
|---|---|---|---|
| `cloudflare-docs` | `https://docs.mcp.cloudflare.com/mcp` | **ninguna** | Búsqueda semántica en la documentación oficial actualizada (`search_cloudflare_documentation`) + guía de migración Pages→Workers. Cero acceso a tu cuenta: es solo lectura de docs públicas. |
| `cloudflare-bindings` | `https://bindings.mcp.cloudflare.com/mcp` | OAuth | Crear y administrar recursos de Workers: KV, D1, R2, Durable Objects, Hyperdrive, AI. Es el único de los tres con capacidad de **escritura sobre la cuenta**. |
| `cloudflare-observability` | `https://observability.mcp.cloudflare.com/mcp` | OAuth | Depuración: logs de Workers, errores, analytics de invocaciones. Lectura. |
| `cloudflare-api` | `https://mcp.cloudflare.com/mcp` | token `cfat_` | Toda la API de Cloudflare en dos tools (`search`/`execute`). Ver §2 y [MCP_TOKENS.md](MCP_TOKENS.md). |

### Catálogo completo (por si se necesita otro)

| Servidor | URL |
|---|---|
| Cloudflare API (unificado, ver §2) | `https://mcp.cloudflare.com/mcp` |
| Workers Builds | `https://builds.mcp.cloudflare.com/mcp` |
| Radar (tráfico global, escaneo de URLs) | `https://radar.mcp.cloudflare.com/mcp` |
| Containers (sandbox de desarrollo) | `https://containers.mcp.cloudflare.com/mcp` |
| Browser Run (fetch, markdown, screenshots) | `https://browser.mcp.cloudflare.com/mcp` |
| Logpush | `https://logs.mcp.cloudflare.com/mcp` |
| AI Gateway | `https://ai-gateway.mcp.cloudflare.com/mcp` |
| AI Search (AutoRAG) | `https://autorag.mcp.cloudflare.com/mcp` |
| Audit Logs | `https://auditlogs.mcp.cloudflare.com/mcp` |
| DNS Analytics | `https://dns-analytics.mcp.cloudflare.com/mcp` |
| Digital Experience Monitoring | `https://dex.mcp.cloudflare.com/mcp` |
| Cloudflare One CASB | `https://casb.mcp.cloudflare.com/mcp` |
| GraphQL Analytics | `https://graphql.mcp.cloudflare.com/mcp` |
| Agents SDK Docs | `https://agents.cloudflare.com/mcp` |

---

## 2. El servidor unificado `mcp.cloudflare.com`

Cloudflare publicó un servidor que expone **toda la API (~2.500 endpoints)** con solo
dos herramientas: `search()` y `execute()` (patrón *Code Mode*: el modelo escribe JS
tipado contra la spec OpenAPI y se ejecuta en un Dynamic Worker aislado). Cuesta
~1.000 tokens de contexto en vez de ~1.000.000 si cada endpoint fuera un tool.

```json
{ "mcpServers": { "cloudflare-api": { "type": "http", "url": "https://mcp.cloudflare.com/mcp" } } }
```

**Dato clave para seguridad:** es el único que documenta autenticación por
**bearer token** además de OAuth — se le puede pasar un token de API con permisos
recortados en vez de delegar la sesión completa del usuario (ver §4.C).

---

## 3. Configuración en Claude Code

Ámbito **user** (global, aplica a todos los proyectos) en `~/.claude.json`:

```json
{
  "mcpServers": {
    "cloudflare-docs":          { "type": "http", "url": "https://docs.mcp.cloudflare.com/mcp" },
    "cloudflare-bindings":      { "type": "http", "url": "https://bindings.mcp.cloudflare.com/mcp" },
    "cloudflare-observability": { "type": "http", "url": "https://observability.mcp.cloudflare.com/mcp" },
    "cloudflare-api": {
      "type": "http",
      "url": "https://mcp.cloudflare.com/mcp",
      "headersHelper": "printf '{\"Authorization\": \"Bearer %s\"}' \"$(cat /home/cesar/.config/cloudflare/mcp-token)\""
    }
  }
}
```

Equivalente por CLI:

```bash
claude mcp add --scope user --transport http cloudflare-docs https://docs.mcp.cloudflare.com/mcp
```

Autorización (obligatoria para todos menos `docs`):

1. Reiniciar Claude Code y ejecutar `/mcp` en una sesión **interactiva**.
2. Elegir el servidor en estado `Needs Auth` → abre el navegador con el consent de Cloudflare.
3. En el consent: **seleccionar la cuenta** y luego revisar los permisos.
4. Desde 2026-08-22 el diálogo trae **Edit Permissions**: los scopes opcionales se pueden
   apagar uno por uno, o usar el preset **Read only**. Los scopes requeridos no se pueden quitar.
5. Verificar en `/mcp` que quede `Connected`.

Revocar en cualquier momento: dashboard → **Manage OAuth authorizations**
(`https://dash.cloudflare.com/?to=/profile/access-management/authorization`).

Alternativa de instalación: el plugin oficial `cloudflare/skills`, que empaqueta estos
servidores más skills y slash commands (`/plugin marketplace add cloudflare/skills`).

---

## 4. Mínimo privilegio: cómo no exponer la cuenta principal

### A. Cuenta separada — **no hace falta un correo nuevo**

Un mismo usuario de Cloudflare puede ser dueño de **varias cuentas**, y desde 2026-08-04
cualquier usuario puede crear cuentas Free adicionales directamente desde el dashboard.
La cuenta es el contenedor de zonas y recursos, así que es el límite de aislamiento real.

Receta recomendada:

1. Crear una cuenta nueva (ej. `webtyp-sandbox`) con el mismo login.
2. Poner ahí los Workers/D1/R2/KV con los que el agente va a experimentar.
3. Al autorizar el MCP, **seleccionar únicamente esa cuenta** en el consent.
   Los servidores no ven las cuentas que no elegiste.

Solo se justifica un correo distinto si además se quiere separar la **facturación** o
tener un login que ni siquiera pueda iniciar sesión en producción.

### B. Bloquear OAuth en la cuenta de producción

Dashboard → seleccionar la cuenta → **Manage account → Members → pestaña `Settings`**
(la tercera, junto a *All members* y *Groups*) → **Public OAuth App access** → desactivar.

No confundir con **OAuth clients** de la barra lateral: eso sirve para registrar
aplicaciones OAuth propias, no para bloquear las de terceros.

Orden correcto (el ajuste solo afecta autorizaciones futuras):

1. **Primero autorizar** en `/mcp` lo que se vaya a usar, marcando en el consent solo la
   cuenta que corresponda.
2. **Después apagar** el toggle en la cuenta de producción. Queda como candado: lo ya
   concedido sigue funcionando, pero no se pueden crear autorizaciones nuevas sin
   volver a encenderlo.

Si se apaga primero, esa cuenta deja de aparecer en el selector del consent y no habrá
forma de conectar el servidor contra ella.

Advertencias:

* Impide autorizaciones **nuevas**; no revoca las existentes (esas se quitan en el perfil, §3).
* El ajuste se llama *Public* OAuth App y las apps de Cloudflare (Wrangler, sus propios
  servidores MCP) son de primera parte — la documentación no aclara si quedan exentas.
  Verificar empíricamente: tras desactivarlo, iniciar la autorización de un servidor MCP
  y comprobar si la cuenta bloqueada **desaparece del selector de cuentas** del consent.
  Si sigue apareciendo, el control efectivo es no marcarla ahí.
* `wrangler login` también usa OAuth: confirmar que los despliegues locales contra esa
  cuenta siguen funcionando, o pasarlos a token de API (§4.C).

### C. Token de API con permisos justos (en vez de OAuth)

Una autorización OAuth delega tu sesión de usuario con scopes gruesos; un token de API se
recorta permiso por permiso, se limita a una cuenta o zona, y admite expiración y filtro
por IP. Es la vía para el servidor unificado (`mcp.cloudflare.com`, el único que acepta
**bearer token**) y para CI.

Receta paso a paso —crear el token, guardarlo sin dejarlo en texto plano, configurar el
servidor y verificar qué cuentas alcanza— en **[MCP_TOKENS.md](MCP_TOKENS.md)**.

### D. Por qué NO alcanza con "invitar un miembro con rol limitado"

Los roles de cuenta de Cloudflare son gruesos y por producto (Administrator, R2 Admin,
Cloudflare Images, Secrets Store Admin, Minimal Account Access…). No existe un rol que
diga "solo este Worker": para desplegar Workers se termina cayendo en Administrator, que
es casi todo. Por eso el aislamiento correcto es **cuenta aparte** (§4.A), y el ajuste
fino se hace con **scopes OAuth opcionales** (§3.4) o **tokens de API** (§4.C).

### E. Reglas prácticas

* `cloudflare-docs` puede quedar siempre activo: no toca la cuenta.
* `cloudflare-observability` y demás servidores de diagnóstico: autorizar en **Read only**.
* `cloudflare-bindings` (escribe) solo apuntando a la cuenta sandbox.
* Producción se toca por CI con `CLOUDFLARE_API_TOKEN` (ver [CI_D1_SECRETS.md](CI_D1_SECRETS.md)),
  no por el agente.
* Revisar periódicamente **Manage OAuth authorizations** y revocar lo que no se use.

---

## 5. Verificación rápida

```bash
# 200 = servidor vivo y sin auth; 401 = vivo, esperando OAuth/token
curl -s -o /dev/null -w "%{http_code}\n" -X POST https://docs.mcp.cloudflare.com/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}'
```

| Síntoma | Causa probable |
|---|---|
| `Needs Auth` permanente | Falta correr `/mcp` en sesión interactiva; el flujo OAuth no puede hacerse en modo no interactivo. |
| `Failed` | URL mal escrita, transporte forzado a SSE (usar `/mcp`), o red/proxy bloqueando. |
| Un tool responde "permission denied" | Se declinó un scope opcional en el consent → reautorizar y concederlo. |
| El servidor no ve tus recursos | Se autorizó con otra cuenta seleccionada en el consent. |
