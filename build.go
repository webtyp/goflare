//go:build !wasm

package goflare

import (
	"errors"
	"fmt"
	"os"

	"webtyp.com/sitec"
)

// Build orchestrates the build pipeline.
func (g *Goflare) Build() error {
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
	// moduleRoot is the directory goflare runs from. Every project source path
	// — edge/main.go, web/client.go — resolves from here.
	moduleRoot = "."
)

func (g *Goflare) buildWorker() error {
	// 1. Verify Entry file exists
	if _, err := os.Stat(g.Config.Entry); os.IsNotExist(err) {
		return fmt.Errorf("entry path does not exist: %s", g.Config.Entry)
	}

	// 2. Ensure OutputDir exists
	if err := os.MkdirAll(g.Config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 3. The edge is JavaScript only: goesm output (app.js) + the module Worker (edge.js).
	return g.buildWorkerJS()
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
		Mode:      sitec.ModeRelease,
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
