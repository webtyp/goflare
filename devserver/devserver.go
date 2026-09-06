//go:build !wasm

package devserver

import (
	"webtyp.com/server/httpd"
)

// New returns a new development server.
func New(c httpd.Config) *httpd.Server {
	// Dev server defaults
	c.NoCache = true
	c.RoutesEndpoint = true
	if c.Port == "" {
		c.Port = "8080"
	}
	return httpd.New(c)
}
