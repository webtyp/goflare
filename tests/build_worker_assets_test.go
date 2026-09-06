//go:build !wasm

package goflare_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestBuildWorkerAssets_SingleArtifact(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Dir(cwd)

	imports := []string{
		`webtyp.com/cloudflare/edge`,
		`webtyp.com/cloudflare/workers`,
	}

	for _, imp := range imports {
		t.Run("Import_"+imp, func(t *testing.T) {
			t.Setenv("GOWORK", "off")
			tmpDir := t.TempDir()
			entryDir := filepath.Join(tmpDir, "edge")
			outputDir := filepath.Join(tmpDir, ".build")
			if err := os.MkdirAll(entryDir, 0755); err != nil {
				t.Fatal(err)
			}

			// Copy go.mod and go.sum from repoRoot and adjust module name
			modBytes, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
			if err != nil {
				t.Fatal(err)
			}
			newMod := strings.Replace(string(modBytes), "module webtyp.com/goflare", "module testapp", 1)
			newMod = strings.ReplaceAll(newMod, "webtyp.com/cloudflare v0.0.2", "webtyp.com/cloudflare v0.0.0")
			{
				var keep []string
				for _, ln := range strings.Split(newMod, "\n") {
					if strings.HasPrefix(strings.TrimSpace(ln), "replace webtyp.com/") {
						continue
					}
					keep = append(keep, ln)
				}
				newMod = strings.Join(keep, "\n") + "\n" + webtypReplaces(t)
			}
			if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(newMod), 0644); err != nil {
				t.Fatal(err)
			}
			if sumBytes, err := os.ReadFile(filepath.Join(repoRoot, "go.sum")); err == nil {
				os.WriteFile(filepath.Join(tmpDir, "go.sum"), sumBytes, 0644)
			}
			cmd := exec.Command("go", "mod", "tidy")
			cmd.Dir = tmpDir
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("go mod tidy failed: %v\n%s", err, string(out))
			}

			mainContent := `package main
import _ "` + imp + `"
func main() {}
`
			if err := os.WriteFile(filepath.Join(entryDir, "main.go"), []byte(mainContent), 0644); err != nil {
				t.Fatal(err)
			}

			cfg := &goflare.Config{
				Entry:     entryDir,
				OutputDir: outputDir,
			}
			g := goflare.New(cfg)

			if err := g.Build(); err != nil {
				t.Fatalf("Build failed for import %s: %v", imp, err)
			}

			edgeJs := filepath.Join(outputDir, "edge.js")
			edgeWasm := filepath.Join(outputDir, "edge.wasm")

			if _, err := os.Stat(edgeJs); os.IsNotExist(err) {
				t.Errorf("Expected %s to exist", edgeJs)
			}
			if _, err := os.Stat(edgeWasm); os.IsNotExist(err) {
				t.Errorf("Expected %s to exist", edgeWasm)
			}

			// functions/ must NOT exist
			functionsDir := filepath.Join(tmpDir, "functions")
			if _, err := os.Stat(functionsDir); !os.IsNotExist(err) {
				t.Errorf("Expected functions dir %s NOT to exist", functionsDir)
			}

			jsContent, err := os.ReadFile(edgeJs)
			if err != nil {
				t.Fatal(err)
			}
			jsStr := string(jsContent)

			if !strings.Contains(jsStr, "export default") {
				t.Errorf("Expected edge.js to contain 'export default'")
			}
			if strings.Contains(jsStr, "export{onRequest}") || strings.Contains(jsStr, "export { onRequest }") {
				t.Errorf("Expected edge.js NOT to contain export onRequest re-export")
			}
		})
	}
}

func TestBuildWorkerAssets_UnknownImportFails(t *testing.T) {
	tmpDir := t.TempDir()
	entryDir := filepath.Join(tmpDir, "edge")
	outputDir := filepath.Join(tmpDir, ".build")
	os.MkdirAll(entryDir, 0755)

	mainContent := `package main
import "fmt"
func main() { fmt.Println("hi") }
`
	os.WriteFile(filepath.Join(entryDir, "main.go"), []byte(mainContent), 0644)

	cfg := &goflare.Config{
		Entry:     entryDir,
		OutputDir: outputDir,
	}
	g := goflare.New(cfg)

	err := g.Build()
	if err == nil {
		t.Fatal("Expected build error for unknown import")
	}
	if !strings.Contains(err.Error(), goflare.ErrNoKnownImport) {
		t.Errorf("Expected error containing %q, got %v", goflare.ErrNoKnownImport, err)
	}
}
