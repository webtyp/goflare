//go:build !wasm

package goflare

import (
	"os"
	"runtime/debug"
	"time"

	"webtyp.com/sitec"
)

// HeaderIdentity is the header a goflare-deployed Worker identifies itself
// with. Its presence proves the response came from the Worker and not from the
// static file layer.
const HeaderIdentity = "x-goflare"

// identityValue returns the version of the running goflare module, or "dev"
// when there is no build information (a local checkout).
func identityValue() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

// SiteOutput is the already-built site, ready to be flushed to disk.
type SiteOutput interface {
	WriteTo(fs sitec.FS) error
}

// SiteBuilder builds the project's static site.
//
// It is a deliberate seam: the real sitec pipeline demands a valid Go module on
// disk and an installed compiler, and this repo's tests have neither. The real
// implementation is buildSite.
type SiteBuilder func(cfg sitec.BuildConfig) (SiteOutput, error)

// buildSite is the real implementation: the full sitec pipeline.
func buildSite(cfg sitec.BuildConfig) (SiteOutput, error) {
	out, err := sitec.Build(cfg)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type Config struct {
	// Project identity
	ProjectName string // PROJECT_NAME
	AccountID   string // CLOUDFLARE_ACCOUNT_ID
	WorkerName  string // WORKER_NAME  (default: ProjectName + "-worker")

	// Routing
	Domain string // DOMAIN (optional — custom domain)

	// Build inputs (conventions, not configurable via .env)
	Entry     string // ENTRY      (path to main Go file, empty = Pages only)
	PublicDir string // PUBLIC_DIR (path to static assets, empty = Worker only)
	RootDir   string // project root used to locate routes/routes.go; default "."

	// Build output (not in .env — always .build/)
	OutputDir string // default: ".build/"

	// Compiler
	CompilerMode string // "S" | "M" | "L"  default: "S"

	D1DatabaseID   string // D1_DATABASE_ID
	D1DatabaseName string // D1_DATABASE_NAME — optional, default: ProjectName
	R2BucketID     string // R2_BUCKET_ID
	R2BucketName   string // R2_BUCKET_NAME
}

type Goflare struct {
	edgeBuilder  sitec.WasmBuilder // Worker compiler (Entry)
	siteBuilder  SiteBuilder       // Static pages compiler
	Config       *Config           // exported so CLI can read it after LoadConfigFromEnv
	log          func(message ...any)
	BaseURL      string
	SiteURL      string        // override public URL for testing probe
	stagingDir   string        // temporary directory for build artifacts
	RetryBackoff time.Duration // base duration for retries (defaults to 1s)
}

// SetSiteBuilder sustituye el compilador de sitio. Pensado para tests; en
// producción nadie lo llama y se usa buildSite.
func (g *Goflare) SetSiteBuilder(b SiteBuilder) {
	if b != nil {
		g.siteBuilder = b
	}
}

// New creates a new Goflare instance with the provided configuration
func New(cfg *Config) *Goflare {
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.applyDefaults()

	staging, err := os.MkdirTemp("", "goflare-*")
	if err != nil {
		// Fallback to configured OutputDir if MkdirTemp fails
		staging = cfg.OutputDir
	}

	// goflare ALWAYS compiles the edge with TinyGo in production mode.
	// Cloudflare Workers/Pages enforce a 1 MiB wasm limit on Free plans,
	// which standard Go (2-10 MB) cannot meet. The typed wrapper insulates
	// goflare from any future change to the internal mode letter and
	// persists the choice to disk storage so a stale previous mode does
	// not silently override it.
	edgeBuilder := sitec.NewWasmBuilder(false, sitec.WasmBuildOptions{
		Entry:      "main.go",
		OutputName: "edge",
	})

	g := &Goflare{
		edgeBuilder:  edgeBuilder,
		siteBuilder:  buildSite,
		Config:       cfg,
		BaseURL:      cfAPIBase,
		stagingDir:   staging,
		RetryBackoff: time.Second,
	}

	return g
}

// StagingDir returns the temporary directory used for intermediate build artifacts.
// Exposed for testing — verifies that staging is outside the project tree.
func (g *Goflare) StagingDir() string { return g.stagingDir }

func (g *Goflare) SetLog(f func(message ...any)) {
	g.log = f
}

func (g *Goflare) Logger(messages ...any) {
	if g.log != nil {
		g.log(messages...)
	}
}

// SetCompilerMode changes the compiler mode
// mode: "L" (Large fast/Go), "M" (Medium TinyGo debug), "S" (Small TinyGo production)
func (g *Goflare) SetCompilerMode(newValue string) {
	g.Config.CompilerMode = newValue
}
