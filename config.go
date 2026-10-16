//go:build !wasm

package goflare

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"webtyp.com/deploy"
	"webtyp.com/router/routescan"
)

const (
	EnvKeyProjectName       = "PROJECT_NAME"
	EnvKeyAccountID         = "CLOUDFLARE_ACCOUNT_ID"
	EnvKeyWorkerName        = "WORKER_NAME"
	EnvKeyDomain            = "DOMAIN"
	EnvKeyRateLimitPer10s   = "RATE_LIMIT_PER_10S"
	EnvKeyCompilerMode      = "COMPILER_MODE"
	EnvKeyD1Binding         = "D1_BINDING"
	EnvKeyD1Name            = "D1_NAME"
	EnvKeyR2Binding         = "R2_BINDING"
	EnvKeyR2Bucket          = "R2_BUCKET"
	EnvKeyVars              = "VARS"
	EnvKeyCompatibilityDate = "COMPATIBILITY_DATE"
	EnvKeyNotFoundHandling  = "NOT_FOUND_HANDLING"

	DefaultCompatibilityDate = "2026-08-01"
	DefaultNotFoundHandling  = "single-page-application"
	HTMLHandlingDefault      = "auto-trailing-slash"
	DefaultRateLimitPer10s   = 100
)

const ErrMsgRateLimitInvalid = "RATE_LIMIT_PER_10S must be an integer >= 1: the edge rate limit cannot be disabled"

// DefaultRootDir is the project root Scan reads routescan.DefaultFile from when the
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
				case EnvKeyD1Binding:
					cfg.D1Binding = value
				case EnvKeyD1Name:
					cfg.D1Name = value
				case EnvKeyR2Binding:
					cfg.R2Binding = value
				case EnvKeyR2Bucket:
					cfg.R2Bucket = value
				case EnvKeyVars:
					for _, line := range strings.Split(value, "\n") {
						line = strings.TrimSpace(line)
						if line == "" {
							continue
						}
						parts := strings.SplitN(line, "=", 2)
						if len(parts) != 2 {
							return nil, fmt.Errorf("ErrInvalidVarLine: %s", line)
						}
						cfg.Vars = append(cfg.Vars, deploy.Var{
							Key:   strings.TrimSpace(parts[0]),
							Value: strings.TrimSpace(parts[1]),
						})
					}
				case EnvKeyRateLimitPer10s:
					val, err := strconv.Atoi(value)
					if err != nil || val < 1 {
						return nil, fmt.Errorf(ErrMsgRateLimitInvalid)
					}
					cfg.RateLimitPer10s = val
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
	if cfg.D1Binding == "" {
		cfg.D1Binding = os.Getenv(EnvKeyD1Binding)
	}
	if cfg.D1Name == "" {
		cfg.D1Name = os.Getenv(EnvKeyD1Name)
	}
	if cfg.R2Binding == "" {
		cfg.R2Binding = os.Getenv(EnvKeyR2Binding)
	}
	if cfg.R2Bucket == "" {
		cfg.R2Bucket = os.Getenv(EnvKeyR2Bucket)
	}
	if len(cfg.Vars) == 0 {
		envVars := os.Getenv(EnvKeyVars)
		if envVars != "" {
			for _, line := range strings.Split(envVars, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					return nil, fmt.Errorf("ErrInvalidVarLine: %s", line)
				}
				cfg.Vars = append(cfg.Vars, deploy.Var{
					Key:   strings.TrimSpace(parts[0]),
					Value: strings.TrimSpace(parts[1]),
				})
			}
		}
	}
	if cfg.RateLimitPer10s == 0 {
		envVal := os.Getenv(EnvKeyRateLimitPer10s)
		if envVal != "" {
			val, err := strconv.Atoi(envVal)
			if err != nil || val < 1 {
				return nil, fmt.Errorf(ErrMsgRateLimitInvalid)
			}
			cfg.RateLimitPer10s = val
		}
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

	if c.RateLimitPer10s == 0 {
		c.RateLimitPer10s = DefaultRateLimitPer10s
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
// in the routes file (routescan.DefaultFile) — never guessed.
//
// Each declaration becomes one entry: a Mount or PublicDir path already carries
// routescan.MountSuffix and is used as-is; a path with a "{param}" segment is
// truncated at that segment and suffixed with routescan.MountSuffix
// ("/api/orders/{id}" -> "/api/orders/*"); any other path is used verbatim.
// Duplicates are removed; order is source order.
//
// A project with no routes file declares no dynamic paths and gets an
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
