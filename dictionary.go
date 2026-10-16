//go:build !wasm

package goflare

func Dictionary() []byte {
	return []byte(`{"keys":["es","zh","hi","ar","pt","fr","de","ru"],"texts":{"Cloudflare bootstrap token expires soon":["El token de inicio de Cloudflare caduca pronto","","","","","","",""],"Cloudflare bootstrap token is not valid":["El token de inicio de Cloudflare no es válido","","","","","","",""],"add the domain to this Cloudflare account":["añade el dominio a esta cuenta de Cloudflare","","","","","","",""],"create the bootstrap token limited to one account (D40)":["crea el token de inicio limitado a una cuenta (D40)","","","","","","",""],"open https://dash.cloudflare.com/profile/api-tokens → Create Token → template 'Create additional tokens' → limit it to one account → TTL 90 days → copy it here":["abre https://dash.cloudflare.com/profile/api-tokens → Create Token → plantilla 'Create additional tokens' → limítalo a una cuenta → TTL 90 días → cópialo aquí","","","","","","",""]}}`)
}
