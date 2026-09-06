package goflare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEntry(t *testing.T) {
	tests := []struct {
		name    string
		mainGo  string
		wantErr string
	}{
		{
			name: "EdgeImport",
			mainGo: `package main
import _ "webtyp.com/goflare/edge"
func main() {}`,
		},
		{
			name: "WorkersImport",
			mainGo: `package main
import _ "webtyp.com/goflare/workers"
func main() {}`,
		},
		{
			name: "BothImportsAllowed",
			mainGo: `package main
import (
	_ "webtyp.com/goflare/edge"
	_ "webtyp.com/goflare/workers"
)
func main() {}`,
		},
		{
			name: "NoKnownImport",
			mainGo: `package main
import "fmt"
func main() { fmt.Println("hello") }`,
			wantErr: ErrNoKnownImport,
		},
		{
			name: "CommentedImport",
			mainGo: `package main
// import "webtyp.com/goflare/edge"
func main() {}`,
			wantErr: ErrNoKnownImport,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			entry := filepath.Join(tmp, "edge")

			if tt.mainGo != "" {
				os.MkdirAll(entry, 0755)
				os.WriteFile(filepath.Join(entry, "main.go"), []byte(tt.mainGo), 0644)
			}

			err := validateEntry(entry)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
