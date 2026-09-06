//go:build !wasm

package goflare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tdewolff/minify/v2"
	minjs "github.com/tdewolff/minify/v2/js"
	cloudflareassets "webtyp.com/cloudflare/assets"
)

// generateWorkerFile bundles and minifies the three JS assets into a single edge.js.
func (g *Goflare) generateWorkerFile() error {
	dest := filepath.Join(g.stagingDir, "edge.js")
	return g.bundleJS(dest, "./edge.wasm")
}

// bundleJS produces the JS glue.
//
// Bundle order:
//  1. Static imports (top-level — required by Cloudflare module format)
//  2. wasm_exec.js  — TinyGo runtime IIFE (no imports)
//  3. runtime.mjs   — loadModule + createRuntimeContext (imports stripped, already at top)
//  4. worker.mjs    — fetch/scheduled/queue + export default
func (g *Goflare) bundleJS(dest, wasmImport string) error {
	wasmExecBody := stripIIFEWrapper(string(cloudflareassets.WasmExecJS))
	runtimeBody := stripExports(stripImports(string(cloudflareassets.RuntimeMJS)))
	workerBody := stripImports(string(cloudflareassets.WorkerMJS))

	workerBody = strings.ReplaceAll(workerBody, "__GOFLARE_VERSION__", identityValue())

	bundle := strings.Join([]string{
		`import mod from "` + wasmImport + `";`,
		`import { connect } from "cloudflare:sockets";`,
		wasmExecBody,
		runtimeBody,
		workerBody,
	}, "\n\n")

	m := minify.New()
	m.AddFunc("text/javascript", minjs.Minify)
	minified, err := m.String("text/javascript", bundle)
	if err != nil {
		return fmt.Errorf("failed to minify %s: %w", filepath.Base(dest), err)
	}

	return os.WriteFile(dest, []byte(minified), 0644)
}

// stripImports removes ES module import lines from a JS source string.
func stripImports(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "import ") && !strings.HasPrefix(trimmed, "import{") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// stripExports removes "export " from the beginning of lines,
// but keeps "export default".
func stripExports(src string) string {
	var lines []string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "export ") && !strings.HasPrefix(trimmed, "export default") {
			line = strings.Replace(line, "export ", "", 1)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// stripIIFEWrapper removes the outer (() => { ... })(); from wasm_exec.js,
// leaving the inner body for inline embedding.
func stripIIFEWrapper(src string) string {
	start := strings.Index(src, "(() => {")
	if start == -1 {
		return src
	}
	end := strings.LastIndex(src, "})();")
	if end == -1 || end <= start {
		return src
	}
	// Extract content between (() => { and })();
	return src[start+8 : end]
}
