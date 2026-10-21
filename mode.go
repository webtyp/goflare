//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

const (
	ImportEdge    = "webtyp.com/cloudflare/edge"
	ImportWorkers = "webtyp.com/cloudflare/workers"

	// LegacyImportEdge/Workers kept for backwards compat during migration.
	LegacyImportEdge    = "webtyp.com/goflare/edge"
	LegacyImportWorkers = "webtyp.com/goflare/workers"

	ErrNoKnownImport = "cannot infer mode: web/edge.go imports neither " + ImportEdge + " nor " + ImportWorkers
)

// validateEntry confirms that web/edge.go imports at least one of the known runtime packages.
func validateEntry(entry string) error {
	if _, err := os.Stat(entry); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("entry go file does not exist: %s", entry)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, entry, nil, parser.ImportsOnly)
	if err != nil {
		return err
	}

	for _, spec := range f.Imports {
		path, _ := strconv.Unquote(spec.Path.Value)
		if path == ImportEdge || path == ImportWorkers || path == LegacyImportEdge || path == LegacyImportWorkers {
			return nil
		}
	}

	return errors.New(ErrNoKnownImport)
}
