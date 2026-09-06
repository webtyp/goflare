//go:build !wasm

package goflare_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

// TestBuild_StagingDirOutsideRepo verifies that New() uses os.MkdirTemp for the
// edge compiler staging dir — not .build/ inside the project tree.
func TestBuild_StagingDirOutsideRepo(t *testing.T) {
	tmpRepo, err := os.MkdirTemp("", "goflare-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpRepo)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpRepo)
	defer os.Chdir(oldWd)

	cfg := &goflare.Config{
		ProjectName: "test-staging",
		AccountID:   "123",
		Entry:       "edge/main.go",
		// OutputDir defaults to .build/ — staging must go to os.MkdirTemp instead
	}

	g := goflare.New(cfg)

	staging := g.StagingDir()

	// Staging must not be inside the repo
	if strings.HasPrefix(staging, tmpRepo) {
		t.Errorf("staging dir %q is inside the repo %q — must be in os.MkdirTemp", staging, tmpRepo)
	}

	// Staging must actually exist (MkdirTemp succeeded)
	if _, err := os.Stat(staging); os.IsNotExist(err) {
		t.Errorf("staging dir %q does not exist", staging)
	}

	// .build/ must NOT have been created in the repo
	dotBuild := filepath.Join(tmpRepo, ".build")
	if _, err := os.Stat(dotBuild); err == nil {
		t.Errorf(".build/ was created inside the repo at %q — must not happen", dotBuild)
	}

	// Cleanup: staging dir is removed when Goflare is done (defer in Build).
	// Here we verify New() alone does not leak it by cleaning up manually.
	os.RemoveAll(staging)
}
