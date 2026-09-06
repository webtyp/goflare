//go:build !wasm

package goflare_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// webtypRoot walks up from this file until it finds the tinywasm monorepo dir
// (the one holding fmt/go.mod → "module webtyp.com/fmt"). Returns "" if not
// found (e.g. this repo was cloned standalone).
func webtypRoot() string {
	_, self, _, _ := runtime.Caller(0)
	dir := filepath.Dir(self)
	for i := 0; i < 8; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "fmt", "go.mod")); err == nil &&
			strings.HasPrefix(string(data), "module webtyp.com/fmt") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// webtypReplaces returns `replace webtyp.com/<mod> => <abs local path>` lines for
// every webtyp.com/* module checked out as a sibling in the monorepo. Fixtures
// append it to their synthetic go.mod so `go mod tidy` resolves the ecosystem
// locally, offline, independent of published tags. Skips the test if the
// sibling layout is absent.
func webtypReplaces(t *testing.T) string {
	t.Helper()
	root := webtypRoot()
	if root == "" {
		t.Skip("tinywasm monorepo sibling layout not found; fixture needs local webtyp.com checkouts")
	}
	ents, err := os.ReadDir(root)
	if err != nil {
		t.Skipf("cannot read monorepo root %s: %v", root, err)
	}
	var b strings.Builder
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "go.mod"))
		if err != nil {
			continue
		}
		first := string(data)
		if i := strings.IndexByte(first, '\n'); i >= 0 {
			first = first[:i]
		}
		const pfx = "module webtyp.com/"
		if strings.HasPrefix(first, pfx) {
			fmt.Fprintf(&b, "replace webtyp.com/%s => %s\n",
				strings.TrimSpace(first[len(pfx):]), filepath.Join(root, e.Name()))
		}
	}
	if b.Len() == 0 {
		t.Skip("no webtyp.com sibling modules found; fixture needs local checkouts")
	}
	return b.String()
}
