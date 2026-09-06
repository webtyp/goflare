//go:build !wasm

package goflare_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"webtyp.com/goflare"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		embedded string
		wantErr  bool
	}{
		{"empty project", "", "v0.0.9", false},
		{"empty embedded", "v0.0.4", "", false},
		{"both empty", "", "", false},
		{"matching versions", "v0.0.9", "v0.0.9", false},
		{"mismatching versions", "v0.0.4", "v0.0.9", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := goflare.CompareVersions(tt.project, tt.embedded)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersions(%q, %q) error = %v, wantErr %v", tt.project, tt.embedded, err, tt.wantErr)
				return
			}

			if tt.wantErr {
				msg := err.Error()
				if !strings.Contains(msg, tt.project) || !strings.Contains(msg, tt.embedded) || !strings.Contains(msg, "go get") {
					t.Errorf("expected error message to contain project ver (%s), embedded ver (%s), and 'go get', got: %s", tt.project, tt.embedded, msg)
				}
			}
		})
	}
}

func TestProjectCloudflareVersionAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	goModPath := filepath.Join(tmpDir, "go.mod")

	content := `module testproj

go 1.25.2
`
	if err := os.WriteFile(goModPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ver, err := goflare.ProjectCloudflareVersion(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ver != "" {
		t.Errorf("expected empty version for module without webtyp/cloudflare, got %q", ver)
	}
}
