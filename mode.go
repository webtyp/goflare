//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
)

const (
	ImportEdge    = "webtyp.com/cloudflare/edge"
	ImportWorkers = "webtyp.com/cloudflare/workers"

	// LegacyImportEdge/Workers kept for backwards compat during migration.
	LegacyImportEdge    = "webtyp.com/goflare/edge"
	LegacyImportWorkers = "webtyp.com/goflare/workers"

	ErrNoKnownImport = "cannot infer mode: edge/main.go imports neither " + ImportEdge + " nor " + ImportWorkers
)

// validateEntry confirms that edge/main.go imports at least one of the known runtime packages.
func validateEntry(entry string) error {
	mainGo := filepath.Join(entry, "main.go")
	if _, err := os.Stat(mainGo); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("entry main.go does not exist: %s", mainGo)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, mainGo, nil, parser.ImportsOnly)
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
