//go:build !wasm

package goflare

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/gobuild"
)

// RunAuth runs the auth command.
func RunAuth(envPath string, out io.Writer, check bool) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}
	g := New(cfg)

	if check {
		if err := g.Auth(); err != nil {
			fmt.Fprintln(out, "Token invalid:", err)
			return err
		}
		fmt.Fprintln(out, "Token OK.")
		return nil
	}

	return g.Auth()
}

// RunBuild runs the build command.
func RunBuild(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}

	if err := cfg.ValidateBuild(); err != nil {
		return err
	}

	// Ensure TinyGo is installed and in PATH before creating compilers.
	// This prevents "exec: tinygo: not found in $PATH" errors when TinyGo
	// is installed at a non-PATH location (e.g. /usr/local/tinygo/bin in CI).
	if err := EnsureTinyGo(out); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Build(); err != nil {
		fmt.Fprintln(out, "Error:", err)
		return err
	}

	fmt.Fprintln(out, "Build complete. Artifacts are in", cfg.OutputDir)
	return nil
}

// FormatTinyGoOutput produces the standard output of the tinygo command.
func FormatTinyGoOutput(dir, version string) string {
	v := strings.TrimSpace(version)
	return fmt.Sprintf("%s%s\n%s%s\n", TinyGoBinDirPrefix, dir, TinyGoVersionPrefix, v)
}

// RunTinyGo installs TinyGo when missing and prints its bindir and version to stdout.
func RunTinyGo(out io.Writer) error {
	dir, version, err := TinyGoBinDir()
	if err != nil {
		return err
	}
	fmt.Fprint(out, FormatTinyGoOutput(dir, version))
	return nil
}

// RunSize runs the size diagnostic subcommand.
func RunSize(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}

	wasmPath := filepath.Join(cfg.OutputDir, WasmArtifactName)
	if _, err := os.Stat(wasmPath); err == nil {
		_ = CheckWasmSize(wasmPath, func(msgs ...any) {
			fmt.Fprintln(out, msgs...)
		})
	}

	entryDir := cfg.Entry
	if entryDir == "" {
		entryDir = "edge"
	}

	if info, err := os.Stat(entryDir); err == nil && info.IsDir() {
		bd, err := SizeBreakdown(entryDir)
		if err != nil {
			fmt.Fprintln(out, "Size breakdown error:", err)
		} else {
			fmt.Fprintln(out, bd)
		}
	}

	entryPkg := "./" + entryDir
	chains, err := ForbiddenImports(".", entryPkg)
	if err != nil {
		fmt.Fprintln(out, "Check forbidden imports error:", err)
	} else if len(chains) == 0 {
		fmt.Fprintln(out, "sin imports de stdlib prohibidos alcanzados directamente")
	} else {
		fmt.Fprintln(out, gobuild.FormatChains(chains))
	}

	return nil
}

// RunDeploy runs the deploy command.
func RunDeploy(envPath string, out io.Writer) error {
	cfg, err := LoadConfigFromEnv(envPath)
	if err != nil {
		return err
	}

	if err := cfg.ValidateDeploy(); err != nil {
		return err
	}

	if err := CheckVersionSkew("."); err != nil {
		return err
	}

	g := New(cfg)
	g.SetLog(func(msgs ...any) {
		fmt.Fprintln(out, msgs...)
	})

	if err := g.Auth(); err != nil {
		return err
	}

	token, err := g.token()
	if err != nil {
		return err
	}
	client := &CfClient{
		Token:      token,
		BaseURL:    g.BaseURL,
		HttpClient: http.DefaultClient,
	}

	subdomain := g.getWorkerSubdomain(client)
	url := fmt.Sprintf("https://%s.%s.workers.dev", cfg.WorkerName, subdomain)
	if cfg.Domain != "" {
		url = "https://" + cfg.Domain
	}

	err = g.Deploy()

	results := []DeployResult{
		{
			Target: "Worker",
			URL:    url,
			Err:    err,
		},
	}

	g.WriteSummary(out, results)

	if err != nil {
		return fmt.Errorf("deploy failed: %w", err)
	}

	return nil
}

// Usage returns the usage string.
func Usage() string {
	return `Usage: goflare <command> [flags]

Commands:
  auth      Validate CLOUDFLARE_API_TOKEN from environment
  build     Build the project (compiles WASM and/or copies assets)
  deploy    Deploy the project to Cloudflare (requires CLOUDFLARE_API_TOKEN env var)
  size      Break down the edge wasm size per package and list forbidden imports
  tinygo    Install TinyGo when missing and print its bin directory and version

Flags:
  -env string
	path to .env file (default ".env")

Auth Flags:
  -check    Verify token from environment
`
}

// DeployResult represents the result of a deployment to a target.
type DeployResult struct {
	Target string
	URL    string
	Err    error
}

// WriteSummary formats and writes the deploy summary to out.
func (g *Goflare) WriteSummary(out io.Writer, results []DeployResult) {
	fmt.Fprintln(out, "\n--- Deployment Summary ---")
	for _, res := range results {
		if res.Err != nil {
			fmt.Fprintf(out, "[-] %s: Failed - %v\n", res.Target, res.Err)
		} else {
			fmt.Fprintf(out, "[+] %s: Success - %s\n", res.Target, res.URL)
		}
	}
}
