package goflare

import _ "embed"

//go:embed lang.json
var dict []byte

func Dictionary() []byte {
	return dict
}
