//go:build !wasm

package goflare

import (
	"runtime/debug"
	"time"

	"webtyp.com/deploy"
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
//
// BuildWithConfig, not Build: sitec v0.2.26 renamed the old in-memory
// Build(cfg BuildConfig) (*Output, error) to BuildWithConfig and gave Build a
// new, incompatible signature — a one-shot disk-writing entry point for
// release pipelines (Build(rootDir, outDir string, opts...) error). This repo
// wants the in-memory *Output SiteBuilder's own signature already returns, so
// it stays on the renamed function.
func buildSite(cfg sitec.BuildConfig) (SiteOutput, error) {
	out, err := sitec.BuildWithConfig(cfg)
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

	RateLimitPer10s int // RATE_LIMIT_PER_10S (default 100, must be >= 1)

	// Routing
	Domain string // DOMAIN (optional — custom domain)

	// Build inputs (conventions, not configurable via .env)
	Entry     string // ENTRY      (path to main Go file, empty = Pages only) web/edge/main.go
	PublicDir string // PUBLIC_DIR (path to static assets, empty = Worker only)
	RootDir   string // project root used to locate the routes file (routescan.DefaultFile); default "."

	// Build output (not in .env — always .build/)
	OutputDir string // default: "web/edge"

	D1Binding string // D1_BINDING
	D1Name    string // D1_NAME    — database name in the account; its ID is resolved at deploy
	R2Binding string // R2_BINDING
	R2Bucket  string // R2_BUCKET
	Vars      []deploy.Var
}

type Goflare struct {
	siteBuilder  SiteBuilder // Static pages compiler
	Config       *Config     // exported so CLI can read it after LoadConfigFromEnv
	log          func(message ...any)
	BaseURL      string
	SiteURL      string        // override public URL for testing probe
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
	if cfg.RateLimitPer10s == 0 {
		cfg.RateLimitPer10s = DefaultRateLimitPer10s
	}
	cfg.applyDefaults()

	g := &Goflare{
		siteBuilder:  buildSite,
		Config:       cfg,
		BaseURL:      cfAPIBase,
		RetryBackoff: time.Second,
	}

	return g
}

func (g *Goflare) SetLog(f func(message ...any)) {
	g.log = f
}

func (g *Goflare) Logger(messages ...any) {
	if g.log != nil {
		g.log(messages...)
	}
}
