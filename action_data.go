//go:build !wasm

package goflare

import (
	"fmt"
	"strings"

	"webtyp.com/ghaction"
	"webtyp.com/git"
)

const (
	ActionFilePath     = "action.yml"
	ReleaseAssetURLFmt = "https://github.com/webtyp/goflare/releases/download/%s/%s"
	TinyGoCacheKeyFmt  = "tinygo-${{ runner.os }}-${{ runner.arch }}-%s"
)

// LatestReleaseTag returns the highest semver tag in the repository.
func LatestReleaseTag() (string, error) {
	g, err := git.NewGit()
	if err != nil {
		return "", fmt.Errorf("git init: %w", err)
	}
	tag, err := g.GetLatestTag()
	if err != nil {
		return "", fmt.Errorf("get latest tag: %w", err)
	}
	return strings.TrimSpace(tag), nil
}

// GoflareAction builds the description of the goflare action.
func GoflareAction(tinyGoVersion, goflareVersion string) ghaction.Action {
	if goflareVersion == "" {
		goflareVersion = "v0.5.22"
	}

	downloadScript := fmt.Sprintf(`set -euo pipefail

version="${{ inputs.version }}"
if [ -z "$version" ]; then
  ref="${{ github.action_ref }}"
  case "$ref" in
    v[0-9]*.[0-9]*.[0-9]*) version="$ref" ;;
    *)
      # A floating ref such as v1 carries no release number, so ask GitHub
      # which release is current.
      version="$(curl -fsSL https://api.github.com/repos/webtyp/goflare/releases/latest \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)"
      ;;
  esac
fi

# Last resort only. This literal is baked at generation time, so it can never
# name the tag being released right now.
if [ -z "$version" ]; then
  version="%s"
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64)  asset=goflare-linux-amd64 ;;
  Linux-aarch64) asset=goflare-linux-arm64 ;;
  Darwin-arm64)  asset=goflare-darwin-arm64 ;;
  Darwin-x86_64) asset=goflare-darwin-amd64 ;;
  *) echo "goflare: unsupported platform: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

url="https://github.com/webtyp/goflare/releases/download/${version}/${asset}"
dest="${RUNNER_TEMP}/goflare"

if ! curl -fsSL "$url" -o "$dest"; then
  echo "goflare: no ${asset} binary in release ${version}." >&2
  echo "  URL: ${url}" >&2
  echo "  The likely cause is that gorelease never ran for that tag." >&2
  echo "  Check https://github.com/webtyp/goflare/releases and pin a version that does have binaries via the 'version' input." >&2
  exit 1
fi

chmod +x "$dest"
echo "$(dirname "$dest")" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"`, goflareVersion)

	installTinyGoScript := `set -euo pipefail
out="$(goflare tinygo)"
bindir="${out#*TINYGO_BINDIR=}"
bindir="${bindir%%$'\n'*}"
version="$(printf '%s' "$out" | sed -n 's/^TINYGO_VERSION=//p')"

if [ -z "$bindir" ]; then
  echo "goflare tinygo returned no bin directory" >&2
  exit 1
fi

echo "$bindir" >> "$GITHUB_PATH"
echo "version=${version}" >> "$GITHUB_OUTPUT"`

	envMapping := []ghaction.KeyValue{
		{Key: "PROJECT_NAME", Value: "${{ inputs.project || inputs.worker }}"},
		{Key: "WORKER_NAME", Value: "${{ inputs.worker }}"},
		{Key: "DOMAIN", Value: "${{ inputs.domain }}"},
		{Key: "D1_DATABASE_NAME", Value: "${{ inputs.d1-binding }}"},
		{Key: "R2_BUCKET_NAME", Value: "${{ inputs.r2-binding }}"},
		{Key: "COMPATIBILITY_DATE", Value: "${{ inputs.compatibility-date }}"},
		{Key: "NOT_FOUND_HANDLING", Value: "${{ inputs.not-found-handling }}"},
	}

	return ghaction.Action{
		Name:        "Deploy with goflare",
		Description: "Builds a Go project to WASM and deploys it as a Cloudflare Worker",
		Author:      "webtyp",
		Branding: ghaction.Branding{
			Icon:  "upload-cloud",
			Color: "orange",
		},
		Inputs: []ghaction.Input{
			{Name: "version", Description: "which goflare release to download; empty = resolve automatically", Default: "", Required: false},
			{Name: "project", Description: "PROJECT_NAME; empty = the value of worker", Default: "", Required: false},
			{Name: "worker", Description: "WORKER_NAME", Required: true},
			{Name: "domain", Description: "DOMAIN", Default: "", Required: false},
			{Name: "d1-binding", Description: "D1_DATABASE_NAME", Default: "", Required: false},
			{Name: "r2-binding", Description: "R2_BUCKET_NAME", Default: "", Required: false},
			{Name: "compatibility-date", Description: "COMPATIBILITY_DATE", Default: "", Required: false},
			{Name: "not-found-handling", Description: "NOT_FOUND_HANDLING", Default: "", Required: false},
			{Name: "setup-go", Description: "run actions/setup-go with the project go.mod", Default: "true", Required: false},
			{Name: "vet", Description: "run go vet ./...", Default: "true", Required: false},
			{Name: "test", Description: "pattern for go test; empty = do not run tests", Default: "./tests/...", Required: false},
			{Name: "pre-deploy", Description: "command to run between build and deploy", Default: "", Required: false},
			{Name: "deploy", Description: "set to 'false' on pull requests to build without deploying", Default: "true", Required: false},
			{Name: "cache", Description: "cache the TinyGo tree", Default: "true", Required: false},
		},
		Outputs: []ghaction.Output{
			{Name: "goflare-version", Description: "the version that was resolved and downloaded", Value: "${{ steps.goflare.outputs.version }}"},
			{Name: "tinygo-version", Description: "whatever tinygo version reports", Value: "${{ steps.tinygo.outputs.version }}"},
		},
		Steps: []ghaction.Step{
			{
				Name:    "Setup Go",
				If:      "inputs.setup-go == 'true'",
				Uses:    ghaction.DefaultSetupGo,
				With:    []ghaction.KeyValue{{Key: "go-version-file", Value: "go.mod"}},
				Comment: "The version comes from the project go.mod, so the caller picks it, not us. setup-go also caches ~/go/pkg/mod and ~/.cache/go-build on its own, keyed off go.sum. Via ghaction preset (Node24).",
			},
			{
				Name:  "Resolve and download the goflare binary",
				ID:    "goflare",
				Shell: "bash",
				Run:   downloadScript,
			},
			{
				Name: "Restore the TinyGo cache",
				If:   "inputs.cache == 'true'",
				Uses: "actions/cache@v4",
				With: []ghaction.KeyValue{
					{Key: "path", Value: "/usr/local/tinygo\n~/.local/tinygo"},
					{Key: "key", Value: fmt.Sprintf(TinyGoCacheKeyFmt, tinyGoVersion)},
				},
				Comment: "The installed tree is cached, not the tarball: a hit costs neither a download nor an extraction. The version sits inside the key deliberately. actions/cache keys are immutable — a hit never saves again — so a key without the version stays pinned to a stale tree once the TinyGo version moves, and from then on every run pays the restore plus the full download anyway. The generator keeps this number current.",
			},
			{
				Name:    "Install TinyGo",
				ID:      "tinygo",
				Shell:   "bash",
				Run:     installTinyGoScript,
				Comment: "TinyGo is installed by the goflare binary itself, via webtyp/tinygo. Using uses: webtyp/tinygo@v1 is deliberately avoided: that action installs the version of the ref it is invoked with, while sitec resolves it again during the build. That would be two sources, and the day they stop agreeing, sitec uninstalls what the action put there and downloads its own. With the binary as the only installer there is a single source. This step runs before the tests because some projects invoke the bare tinygo binary from PATH during go test.",
			},
			{
				Name:  "go vet",
				If:    "inputs.vet == 'true'",
				Shell: "bash",
				Run:   "go vet ./...",
			},
			{
				Name:  "go test",
				If:    "inputs.test != ''",
				Shell: "bash",
				Run:   "go test ${{ inputs.test }}",
			},
			{
				Name:  "goflare build",
				Shell: "bash",
				Run:   "goflare build",
				Env:   envMapping,
			},
			{
				Name:    "Pre-deploy command",
				If:      "inputs.pre-deploy != '' && inputs.deploy == 'true'",
				Shell:   "bash",
				Run:     "${{ inputs.pre-deploy }}",
				Comment: "This is where schema migration goes: it runs once per deploy, not once per isolate start.",
			},
			{
				Name:  "goflare deploy",
				If:    "inputs.deploy == 'true'",
				Shell: "bash",
				Run:   "goflare deploy",
				Env:   envMapping,
			},
		},
	}
}
