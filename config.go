//go:build !wasm

package goflare

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"webtyp.com/router/routescan"
)

const (
	EnvKeyProjectName       = "PROJECT_NAME"
	EnvKeyAccountID         = "CLOUDFLARE_ACCOUNT_ID"
	EnvKeyWorkerName        = "WORKER_NAME"
	EnvKeyDomain            = "DOMAIN"
	EnvKeyCompilerMode      = "COMPILER_MODE"
	EnvKeyD1DatabaseID      = "D1_DATABASE_ID"
	EnvKeyD1DatabaseName    = "D1_DATABASE_NAME"
	EnvKeyR2BucketID        = "R2_BUCKET_ID"
	EnvKeyR2BucketName      = "R2_BUCKET_NAME"
	EnvKeyCompatibilityDate = "COMPATIBILITY_DATE"
	EnvKeyNotFoundHandling  = "NOT_FOUND_HANDLING"

	DefaultCompatibilityDate = "2026-08-01"
	DefaultNotFoundHandling  = "single-page-application"
	HTMLHandlingDefault      = "auto-trailing-slash"
)

// DefaultRootDir is the project root Scan reads routes/routes.go from when the
// caller leaves Config.RootDir empty. It is a convention, not a .env key.
const DefaultRootDir = "."

// symbolRouteParam marks a parameterised segment in a declared route path
// ("/api/orders/{id}"). Everything from that segment on is variable, so the
// prefix Cloudflare is given stops before it.
const symbolRouteParam = "{"

// LoadConfigFromEnv reads a .env file and populates Config.
// Falls back to OS environment variables if .env path is empty or does not exist.
// Applies defaults after loading.
func LoadConfigFromEnv(path string) (*Config, error) {
	cfg := &Config{}

	if path != "" {
		file, err := os.Open(path)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}

				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Strip optional surrounding quotes
				if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
					value = value[1 : len(value)-1]
				}

				switch key {
				case EnvKeyProjectName:
					cfg.ProjectName = value
				case EnvKeyAccountID:
					cfg.AccountID = value
				case EnvKeyWorkerName:
					cfg.WorkerName = value
				case EnvKeyDomain:
					cfg.Domain = value
				case EnvKeyCompilerMode:
					cfg.CompilerMode = value
				case EnvKeyD1DatabaseID:
					cfg.D1DatabaseID = value
				case EnvKeyD1DatabaseName:
					cfg.D1DatabaseName = value
				case EnvKeyR2BucketID:
					cfg.R2BucketID = value
				case EnvKeyR2BucketName:
					cfg.R2BucketName = value
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading .env file: %w", err)
			}
		}
	}

	// Fallback to OS environment variables if still empty
	if cfg.ProjectName == "" {
		cfg.ProjectName = os.Getenv(EnvKeyProjectName)
	}
	if cfg.AccountID == "" {
		cfg.AccountID = os.Getenv(EnvKeyAccountID)
	}
	if cfg.WorkerName == "" {
		cfg.WorkerName = os.Getenv(EnvKeyWorkerName)
	}
	if cfg.Domain == "" {
		cfg.Domain = os.Getenv(EnvKeyDomain)
	}
	if cfg.CompilerMode == "" {
		cfg.CompilerMode = os.Getenv(EnvKeyCompilerMode)
	}
	if cfg.D1DatabaseID == "" {
		cfg.D1DatabaseID = os.Getenv(EnvKeyD1DatabaseID)
	}
	if cfg.D1DatabaseName == "" {
		cfg.D1DatabaseName = os.Getenv(EnvKeyD1DatabaseName)
	}
	if cfg.R2BucketID == "" {
		cfg.R2BucketID = os.Getenv(EnvKeyR2BucketID)
	}
	if cfg.R2BucketName == "" {
		cfg.R2BucketName = os.Getenv(EnvKeyR2BucketName)
	}

	cfg.applyDefaults()
	return cfg, nil
}

// ValidateBuild checks only what goflare build requires.
// ProjectName and AccountID are deploy-only — never referenced by build.go.
func (c *Config) ValidateBuild() error {
	if c.Entry == "" && c.PublicDir == "" {
		return fmt.Errorf("nothing to build: Entry and PublicDir are both empty")
	}
	return nil
}

// ValidateDeploy checks everything required for a Cloudflare API deploy.
func (c *Config) ValidateDeploy() error {
	if c.ProjectName == "" {
		return fmt.Errorf("ProjectName is required (set PROJECT_NAME env var)")
	}
	if c.AccountID == "" {
		return fmt.Errorf("AccountID is required (set CLOUDFLARE_ACCOUNT_ID env var)")
	}
	if c.Entry == "" && c.PublicDir == "" {
		return fmt.Errorf("Entry and PublicDir cannot both be empty")
	}
	return nil
}

func (c *Config) applyDefaults() {
	if c.WorkerName == "" && c.ProjectName != "" {
		c.WorkerName = c.ProjectName + "-worker"
	}
	if c.RootDir == "" {
		c.RootDir = DefaultRootDir
	}
	if c.OutputDir == "" {
		c.OutputDir = ".build/"
	}
	if c.CompilerMode == "" {
		c.CompilerMode = "S"
	}

	// Auto-detect edge function entry (convention).
	if c.Entry == "" {
		if _, err := os.Stat(filepath.Join("edge", "main.go")); err == nil {
			c.Entry = "edge"
		}
	}

	// Auto-detect public directory (convention).
	if c.PublicDir == "" {
		if _, err := os.Stat("web/public"); err == nil {
			c.PublicDir = "web/public"
		}
	}
}

func (g *Goflare) compatibilityDate() string {
	if v := os.Getenv(EnvKeyCompatibilityDate); v != "" {
		return v
	}
	return DefaultCompatibilityDate
}

func (g *Goflare) notFoundHandling() string {
	if v := os.Getenv(EnvKeyNotFoundHandling); v != "" {
		return v
	}
	return DefaultNotFoundHandling
}

// workerFirstRoutes returns the path prefixes Cloudflare must route to the
// Worker before the static assets, derived from the routes the project declares
// in routes/routes.go — never guessed.
//
// Each declaration becomes one entry: a Mount or PublicDir path already carries
// routescan.MountSuffix and is used as-is; a path with a "{param}" segment is
// truncated at that segment and suffixed with routescan.MountSuffix
// ("/api/orders/{id}" -> "/api/orders/*"); any other path is used verbatim.
// Duplicates are removed; order is source order.
//
// A project with no routes/routes.go declares no dynamic paths and gets an
// empty slice — not the two prefixes the old hardcoded default guessed, which
// is the silent-unreachable-route bug this replaced. A scan error is returned
// to the caller so the deploy fails; there is no fallback list.
func (g *Goflare) workerFirstRoutes() ([]string, error) {
	decls, err := routescan.Scan(g.Config.RootDir)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(decls))
	seen := make(map[string]bool, len(decls))
	for _, d := range decls {
		prefix := d.Path
		if i := strings.Index(prefix, symbolRouteParam); i >= 0 {
			prefix = prefix[:i] + routescan.MountSuffix
		}
		if prefix == "" || seen[prefix] {
			continue
		}
		seen[prefix] = true
		out = append(out, prefix)
	}
	return out, nil
}
