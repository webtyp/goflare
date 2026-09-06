//go:build !wasm

package goflare

import "webtyp.com/ghaction"

// ReleaseWorkflow returns the typed release workflow for goflare.
// Generated via ghaction, single source for checkout/setup-go versions (Node24).
func ReleaseWorkflow() ghaction.Workflow {
	buildRun := `set -euo pipefail
mkdir -p dist
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  os=${target%/*}
  arch=${target#*/}
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch \
    go build -trimpath -ldflags="-s -w" \
    -o "dist/goflare-${os}-${arch}" ./cmd/goflare
done
ls -l dist`

	publishRun := `set -euo pipefail
tag="${GITHUB_REF_NAME}"
if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" dist/* --clobber
else
  gh release create "$tag" dist/* \
    --title "$tag" \
    --generate-notes
fi`

	moveRun := `set -euo pipefail
git tag -f v1 "${GITHUB_SHA}"
git push origin -f v1`

	return ghaction.Workflow{
		Name:        "Release",
		On:          ghaction.On{Push: &ghaction.Push{Tags: []string{"v*"}}},
		Permissions: map[string]string{"contents": "write"},
		Jobs: map[string]ghaction.Job{
			"release": {
				RunsOn: ghaction.DefaultRunsOn,
				Steps: []ghaction.WorkflowStep{
					ghaction.CheckoutWithTags(),
					ghaction.SetupGoStep(),
					{Name: "Build the release binaries", Shell: "bash", Run: buildRun},
					{Name: "Publish the release", Shell: "bash", Env: []ghaction.KeyValue{{Key: "GH_TOKEN", Value: "${{ secrets.GITHUB_TOKEN }}"}} , Run: publishRun},
					{Name: "Move the v1 tag", Shell: "bash", Run: moveRun},
				},
			},
		},
	}
}
