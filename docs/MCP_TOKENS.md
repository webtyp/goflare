# Conectar Claude Code a Cloudflare con un token

Receta para dejar al agente operando sobre una cuenta de Cloudflare, sin que el token
pase por el chat ni quede en texto plano en la configuración.

Aplica al servidor unificado `https://mcp.cloudflare.com/mcp`, el único que acepta
**bearer token**. Los servidores por producto (`bindings`, `observability`) solo hacen
OAuth — ver [MCP_CLOUDFLARE.md](MCP_CLOUDFLARE.md).

---

## 1. Crear el token

**Manage Account → Account API tokens → Create Token** (token de cuenta, prefijo `cfat_`;
es un *service principal* de la cuenta y aparece como identidad propia en los audit logs).
Ponerle **expiración**.

Permisos según lo que deba hacer el agente:

| Tarea | Permiso |
|---|---|
| Leer configuración / diagnosticar | `Account Settings: Read`, `Workers Scripts: Read` |
| Logs de Workers | `Workers Tail: Read`, `Account Analytics: Read` |
| Desplegar Workers | `Workers Scripts: Edit` |
| Pages | `Cloudflare Pages: Edit` |
| D1 / KV / R2 | `D1: Edit` · `Workers KV Storage: Edit` · `Workers R2 Storage: Edit` |
| Rutas en dominio propio | `Workers Routes: Edit` (**Zone**, solo esa zona) |

Regla: si el agente solo diagnostica, token de puros `Read`. El token con `Edit` se
reserva para el pipeline de despliegue.

## 2. Guardar el token

En la terminal. `read -rs` no hace eco ni deja el token en el historial:

```bash
mkdir -p ~/.config/cloudflare
read -rs -p "Pega el token y Enter: " T \
  && printf '%s' "$T" > ~/.config/cloudflare/mcp-token \
  && chmod 600 ~/.config/cloudflare/mcp-token \
  && unset T && echo " guardado"
```

## 3. Configurar el servidor MCP

En `~/.claude.json` (ámbito user, aplica a todos los proyectos):

```json
{
  "mcpServers": {
    "cloudflare-api": {
      "type": "http",
      "url": "https://mcp.cloudflare.com/mcp",
      "headersHelper": "printf '{\"Authorization\": \"Bearer %s\"}' \"$(cat /home/cesar/.config/cloudflare/mcp-token)\""
    }
  }
}
```

`headersHelper` es un **comando**: Claude Code lo ejecuta al conectarse, su stdout debe
ser un objeto JSON de cabeceras, y eso se manda a Cloudflare. El token nunca queda
guardado en la configuración ni pasa por la conversación.

Usar **ruta absoluta** al archivo (`~` no se expande de forma fiable en el helper).

## 4. Reiniciar y verificar

Los servidores MCP se cargan al iniciar sesión: **reiniciar Claude Code**. Luego, pedirle
al agente que ejecute contra la API:

```js
async () => {
  const acc = await cloudflare.request({ method: "GET", path: "/accounts" });
  const scripts = await cloudflare.request({ method: "GET", path: `/accounts/${accountId}/workers/scripts` });
  return { accounts: acc.result?.map(a => a.name), workers: scripts.result?.map(s => s.id) };
}
```

Debe devolver **solo** las cuentas que el token alcanza. Si aparece una cuenta que no
esperabas, el token está de más y hay que rehacerlo.

## 5. Cuando falla

| Síntoma | Causa |
|---|---|
| `Needs Auth` en `/mcp` | El helper no devolvió JSON válido, o el archivo del token no existe. Probar el comando a mano. |
| `401` en cada llamada | Token mal pegado (revisar saltos de línea al final) o expirado. |
| `403` en una operación puntual | Falta ese permiso en el token → recrearlo con el permiso que corresponda. |
| Ve cuentas de más | Token de usuario en vez de token de cuenta. |

Rotar o revocar: misma pantalla donde se creó. Revocar es instantáneo y no toca nada más
de la cuenta. Al rotar, repetir solo el paso 2.

## 6. ¿Token u OAuth?

* **OAuth** (`/mcp` en sesión interactiva): más simple y sin secreto de larga duración en
  tu máquina. Es lo indicado para uso interactivo con los servidores por producto.
* **Token**: obligatorio para CI (no hay navegador) y para el servidor unificado. Ventaja
  real: se recorta permiso por permiso y expira.

## 7. Pendiente: hacerlo un comando

Los pasos 2 y 3 deberían ser `goflare auth login` y `goflare auth headers`, usando
[`webtyp/keyring`](https://github.com/webtyp/keyring) (`auto` elige Secret Service en
Linux, Keychain en macOS y Credential Manager en Windows). Ventajas sobre el archivo:

* Sin archivo de token en disco; el secreto queda cifrado por el sistema operativo.
* La misma línea `"headersHelper": "goflare auth headers"` sirve en los tres sistemas —
  el script `printf` de arriba es solo Unix.
* goflare ya valida tokens contra `/user/tokens/verify`, así que `login` puede rechazar
  un token malo en el momento de pegarlo.

Frontera a respetar: ese token local es para **diagnóstico y MCP**. `goflare deploy` sigue
exigiendo `CLOUDFLARE_API_TOKEN` por entorno y corriendo en CI
(ver [CI_D1_SECRETS.md](CI_D1_SECRETS.md)).
