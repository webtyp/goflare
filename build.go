//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"webtyp.com/sitec"
)

// Build orchestrates the build pipeline.
func (g *Goflare) Build() error {
	if g.stagingDir != g.Config.OutputDir {
		defer os.RemoveAll(g.stagingDir)
	}

	if g.Config.Entry == "" && g.Config.PublicDir == "" {
		return errors.New("nothing to build: both Entry and PublicDir are empty")
	}

	var buildErrors []error

	if g.Config.Entry != "" {
		if err := validateEntry(g.Config.Entry); err != nil {
			return fmt.Errorf("mode detection failed: %w", err)
		}
		if err := g.buildWorker(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("worker build failed: %w", err))
		}
	}

	if g.Config.PublicDir != "" {
		if err := g.buildPages(); err != nil {
			buildErrors = append(buildErrors, fmt.Errorf("pages build failed: %w", err))
		}
	}

	if len(buildErrors) > 0 {
		return errors.Join(buildErrors...)
	}

	return nil
}

const (
	// CompilerModeStdlib builds the frontend with standard Go instead of TinyGo:
	// large binary, fast build, no minification. This is the development mode.
	CompilerModeStdlib = "L"

	// moduleRoot is the directory goflare runs from. Every project source path
	// — edge/main.go, web/client.go — resolves from here.
	moduleRoot = "."
)

// siteMode translates goflare's compiler mode into sitec's.
func siteMode(compilerMode string) sitec.Mode {
	if compilerMode == CompilerModeStdlib {
		return sitec.ModeDev
	}
	return sitec.ModeRelease
}

// moveFile renames src to dst, falling back to copy+delete across filesystems.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return err
	}
	return os.Remove(src)
}

func (g *Goflare) buildWorker() error {
	// 1. Verify Entry file exists
	if _, err := os.Stat(g.Config.Entry); os.IsNotExist(err) {
		return fmt.Errorf("entry path does not exist: %s", g.Config.Entry)
	}

	// 2. Ensure OutputDir exists
	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 3. Call generateWasmFile()
	if err := g.generateWasmFile(); err != nil {
		return err
	}

	// 4. Call generateWorkerFile()
	if err := g.generateWorkerFile(); err != nil {
		return err
	}

	// 5. Move files from staging to OutputDir
	for _, name := range []string{WasmArtifactName, "edge.js"} {
		src := filepath.Join(g.stagingDir, name)
		dst := filepath.Join(g.Config.OutputDir, name)
		if err := moveFile(src, dst); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	// 6. Check WASM size
	wasmPath := filepath.Join(g.Config.OutputDir, WasmArtifactName)
	if err := CheckWasmSize(wasmPath, g.Logger); err != nil {
		return err
	}

	return nil
}

func (g *Goflare) buildPages() error {
	// 1. Verify that PUBLIC_DIR exists
	if _, err := os.Stat(g.Config.PublicDir); os.IsNotExist(err) {
		return fmt.Errorf("public dir does not exist: %s", g.Config.PublicDir)
	}

	// 2. sitec walks the module, collects what the project declares, builds the
	//    frontend WASM and assembles the whole site in memory.
	g.Logger("building site →", g.Config.PublicDir)
	site, err := g.siteBuilder(sitec.BuildConfig{
		RootDir:   moduleRoot,
		Mode:      siteMode(g.Config.CompilerMode),
		OutputDir: g.Config.PublicDir,
		AppName:   g.Config.ProjectName,
		Log:       g.Logger,
	})
	if err != nil {
		return fmt.Errorf("site build failed: %w", err)
	}

	// 3. Nothing exists until it is flushed: Build() works in memory.
	if err := site.WriteTo(sitec.NewOsFS()); err != nil {
		return fmt.Errorf("failed to write site artifacts: %w", err)
	}

	return nil
}
