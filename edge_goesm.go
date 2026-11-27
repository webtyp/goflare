//go:build !wasm

package goflare

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func edgeJSEnabled() bool {
	return os.Getenv("WEBTYP_EDGE") == "js"
}

// buildWorkerJS compiles the Worker entry package (e.g. web/edge/main.go) directly to a
// single minified ES Module (edge.js) via goesm — zero edge.wasm, zero wasm_exec.js.
func (g *Goflare) buildWorkerJS() error {
	pkgDir := filepath.Dir(g.Config.Entry)
	tmpDir, err := os.MkdirTemp("", "goflare-goesm-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("goesm", "build", "-minify", "-o", tmpDir, ".")
	cmd.Dir = pkgDir
	env := os.Environ()
	if go127 := findGoflareGo127(); go127 != "" {
		env = goflareEnvOverride(env, "GOROOT", go127)
		env = goflareEnvOverride(env, "PATH", filepath.Join(go127, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("goesm edge build failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			data, err := os.ReadFile(filepath.Join(tmpDir, e.Name()))
			if err != nil {
				return err
			}
			dst := filepath.Join(g.Config.OutputDir, "edge.js")
			return os.WriteFile(dst, data, 0644)
		}
	}
	return fmt.Errorf("goesm produced no .js output for %s", g.Config.Entry)
}

func findGoflareGo127() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			gopath = filepath.Join(home, "go")
		}
	}
	if gopath == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(gopath, "pkg", "mod", "golang.org", "toolchain*1.27*"))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1]
}

func goflareEnvOverride(env []string, key, val string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return append(out, prefix+val)
}
