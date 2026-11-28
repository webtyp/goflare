//go:build !wasm

package goflare

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"webtyp.com/tinygo"
)

const (
	// TinyGoVersion is the TinyGo version this goflare binary will install.
	TinyGoVersion = tinygo.DefaultVersion

	// TinyGoBinDirPrefix marks the stdout line carrying the directory.
	TinyGoBinDirPrefix = "TINYGO_BINDIR="

	// TinyGoVersionPrefix marks the stdout line carrying the version.
	TinyGoVersionPrefix = "TINYGO_VERSION="
)

// TinyGo is not used by the edge (JavaScript only). The site build needs it for the Web Workers
// that declare //go:build wasm && tinygo, so CI installs it through `goflare tinygo`.

// TinyGoBinDir installs TinyGo when missing and returns the directory holding
// the binary, along with the version it reports.
func TinyGoBinDir() (dir, version string, err error) {
	installedPath, err := tinygo.EnsureInstalled()
	if err != nil {
		return "", "", fmt.Errorf("tinygo setup: %w", err)
	}

	binDir := filepath.Dir(installedPath)

	cmd := exec.Command(installedPath, "version")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("tinygo version check: %w", err)
	}

	return binDir, string(out), nil
}

// EnsureTinyGo installs TinyGo if absent and guarantees its bin dir is in PATH
// before any compilation attempt. Safe to call multiple times (idempotent).
func EnsureTinyGo(out io.Writer) error {
	binDir, _, err := TinyGoBinDir()
	if err != nil {
		return err
	}

	if _, lookErr := exec.LookPath("tinygo"); lookErr != nil {
		current := os.Getenv("PATH")
		if current != "" {
			os.Setenv("PATH", current+string(os.PathListSeparator)+binDir)
		} else {
			os.Setenv("PATH", binDir)
		}
		fmt.Fprintln(out, "TinyGo ready:", filepath.Join(binDir, "tinygo"))
	}
	return nil
}
