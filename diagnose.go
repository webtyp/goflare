//go:build !wasm

package goflare

import (
	"strings"

	"webtyp.com/gobuild"
)

const (
	goosJS     = "js"
	goarchWASM = "wasm"
)

// IsStdlib reports whether an import path belongs to the Go standard library.
func IsStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	firstSegment := strings.SplitN(importPath, "/", 2)[0]
	return !strings.Contains(firstSegment, ".")
}

// FilterActionable drops the chains where the forbidden package is reached
// through another stdlib package. Our code neither causes nor can fix those;
// reporting them only trains the user to ignore the guard.
func FilterActionable(chains []gobuild.ImportChain) []gobuild.ImportChain {
	var actionable []gobuild.ImportChain
	for _, chain := range chains {
		if len(chain.Path) < 2 {
			continue
		}
		// Finding the first stdlib package in chain.Path
		var firstStdlibIdx = -1
		for i, pkg := range chain.Path {
			if IsStdlib(pkg) {
				firstStdlibIdx = i
				break
			}
		}

		// A chain is actionable iff the FIRST stdlib package reached IS the forbidden package
		if firstStdlibIdx != -1 && chain.Path[firstStdlibIdx] == chain.Forbidden {
			actionable = append(actionable, chain)
		}
	}
	return actionable
}

// ForbiddenImports returns only the ACTIONABLE chains into forbidden stdlib
// within the edge graph.
func ForbiddenImports(moduleRoot, entryPkg string) ([]gobuild.ImportChain, error) {
	chains, err := gobuild.CheckForbiddenImports(moduleRoot, entryPkg, goosJS, goarchWASM, gobuild.ForbiddenWASMImports)
	if err != nil {
		return nil, err
	}
	return FilterActionable(chains), nil
}
