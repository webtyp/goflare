//go:build !wasm

package goflare

import (
	"fmt"
	"os"
	"path/filepath"

	"webtyp.com/deploy"
)

// Job is the CI step that deploys p.
func (t *Target) Job(p deploy.Plan) deploy.Job {
	var with []deploy.Input

	with = append(with, deploy.Input{Name: "worker", Value: p.Name})
	if p.Domain != "" {
		with = append(with, deploy.Input{Name: "domain", Value: p.Domain})
	}

	if len(p.Databases) > 0 {
		with = append(with, deploy.Input{Name: "d1-binding", Value: p.Databases[0].Binding})
		with = append(with, deploy.Input{Name: "d1-name", Value: p.Databases[0].Name})
	}
	if len(p.Buckets) > 0 {
		with = append(with, deploy.Input{Name: "r2-binding", Value: p.Buckets[0].Binding})
		with = append(with, deploy.Input{Name: "r2-bucket", Value: p.Buckets[0].Name})
	}

	if len(p.Vars) > 0 {
		var varsStr string
		for i, v := range p.Vars {
			if i > 0 {
				varsStr += "\n"
			}
			varsStr += fmt.Sprintf("%s=%s", v.Key, v.Value)
		}
		with = append(with, deploy.Input{Name: "vars", Value: varsStr})
	}

	with = append(with, deploy.Input{Name: "test", Value: "./tests/..."})

	if _, err := os.Stat(filepath.Join(t.cfg.RootDir, "cmd", "migrate", "main.go")); err == nil {
		with = append(with, deploy.Input{Name: "pre-deploy", Value: "go run ./cmd/migrate"})
	}

	return deploy.Job{
		Uses: "webtyp/goflare@v1",
		With: with,
		Secrets: []string{"CLOUDFLARE_API_TOKEN", "CLOUDFLARE_ACCOUNT_ID"},
	}
}
