//go:build !wasm

package goflare

import (
	"fmt"
	"runtime/debug"

	"webtyp.com/modfind"
)

const (
	// CloudflareModulePath is the edge runtime module. goflare embeds its JS
	// assets; the project compiles its Go code. Both halves have to come from
	// the same version.
	CloudflareModulePath = "webtyp.com/cloudflare"

	skewErrFmt = "webtyp/cloudflare version skew: your go.mod resolves %s while this goflare binary embeds the JS assets of %s. The JavaScript glue and the Worker's Go runtime share an ABI; when they diverge, the Worker traps during package initialization without logging anything. Fix with: go get webtyp.com/cloudflare@%s"
)

// EmbeddedCloudflareVersion returns the webtyp.com/cloudflare version
// THIS binary was built against.
func EmbeddedCloudflareVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep != nil && dep.Path == CloudflareModulePath {
			return dep.Version
		}
	}
	return ""
}

// ProjectCloudflareVersion returns the webtyp.com/cloudflare version
// the project go.mod in moduleRoot resolves to.
func ProjectCloudflareVersion(moduleRoot string) (string, error) {
	f := modfind.New()
	mods, err := f.Discover(moduleRoot)
	if err != nil {
		return "", fmt.Errorf("discover project modules: %w", err)
	}
	for _, mod := range mods {
		if mod.Path == CloudflareModulePath {
			return mod.Version, nil
		}
	}
	return "", nil
}

// CompareVersions implements the version-skew decision table.
func CompareVersions(project, embedded string) error {
	if project == "" || embedded == "" {
		return nil
	}
	if project != embedded {
		return fmt.Errorf(skewErrFmt, project, embedded, embedded)
	}
	return nil
}

// CheckVersionSkew fails when the project and this binary resolve different
// versions of webtyp/cloudflare.
func CheckVersionSkew(moduleRoot string) error {
	projVer, err := ProjectCloudflareVersion(moduleRoot)
	if err != nil {
		return err
	}
	embVer := EmbeddedCloudflareVersion()
	return CompareVersions(projVer, embVer)
}
