package goflare

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"webtyp.com/router/routescan"
)

// writeRoutes writes the routes file routescan reads (routescan.DefaultFile)
// under a fresh temp root and returns it.
func writeRoutes(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, filepath.FromSlash(routescan.DefaultFile))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package config\n\nimport \"webtyp.com/router\"\n\nfunc Register(r router.Router) {\n" + body + "\n}\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWorkerFirstDerivation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			// Two calls on the same /api path (different verbs) collapse to one entry.
			name: "DuplicatesRemoved",
			body: "\tr.Post(\"/api/contacto\", h)\n\tr.Get(\"/api/contacto\", h)",
			want: []string{"/api/contacto"},
		},
		{
			// The regression case: a route outside /api/ must appear. The old
			// hardcoded default only covered /api/* and /oauth/*.
			name: "RouteOutsideApiIsPresent",
			body: "\tr.Get(\"/dom\", h)",
			want: []string{"/dom"},
		},
		{
			name: "ParamSegmentTruncatedAndGlobbed",
			body: "\tr.Get(\"/api/orders/{id}\", h)",
			want: []string{"/api/orders/*"},
		},
		{
			// routescan reports PublicDir as a prefix (path already carries "*").
			name: "PublicDirIsAPrefix",
			body: "\tr.PublicDir(\"/static\", \"web/public\")",
			want: []string{"/static*"},
		},
		{
			name: "SourceOrderPreserved",
			body: "\tr.Get(\"/b\", h)\n\tr.Get(\"/a\", h)\n\tr.Mount(\"/api/auth\", authRoutes)",
			want: []string{"/b", "/a", "/api/auth*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Goflare{Config: &Config{RootDir: writeRoutes(t, tt.body)}}
			got, err := g.workerFirstRoutes()
			if err != nil {
				t.Fatalf("workerFirstRoutes: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// A project with no routes file declares no dynamic paths: the result is
// empty and there is no error. This is the behaviour that replaced the guessed
// ["/api/*", "/oauth/*"] default.
func TestWorkerFirstDerivation_NoRoutesFile(t *testing.T) {
	g := &Goflare{Config: &Config{RootDir: t.TempDir()}}
	got, err := g.workerFirstRoutes()
	if err != nil {
		t.Fatalf("workerFirstRoutes: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

// A path routescan cannot read (a variable) is a scan error, surfaced to the
// caller — never swallowed with a fallback list.
func TestWorkerFirstDerivation_ScanError(t *testing.T) {
	root := writeRoutes(t, "\tp := \"/dynamic\"\n\tr.Get(p, h)")
	g := &Goflare{Config: &Config{RootDir: root}}
	if _, err := g.workerFirstRoutes(); err == nil {
		t.Fatal("expected a scan error for a non-literal route path, got nil")
	}
}
