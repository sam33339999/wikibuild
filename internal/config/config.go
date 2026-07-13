// Package config loads application settings from environment variables.
//
// It is environment-agnostic: Load takes a LookupFunc so tests inject a
// fake environment without touching os.Environ, keeping it a pure function.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors. Missing-env errors are wrapped per-var and joined, so
// errors.Is(err, ErrMissingEnv) works while the joined error still reports
// every missing key.
var (
	ErrMissingEnv    = errors.New("missing required environment variable")
	ErrInvalidConfig = errors.New("invalid configuration")
)

const (
	defaultHost       = "127.0.0.1" // 本機 only; set WIKIBUILD_HOST=0.0.0.0 to expose
	defaultPort       = 8080
	defaultContentDir = "./content/uploads"
	defaultSiteTitle  = "WikiBuild"

	// minSessionSecretBytes is the minimum secret length for safe HMAC
	// session signing. 16 bytes (128 bits) is the conservative floor.
	minSessionSecretBytes = 16
)

// LookupFunc returns the value of a setting and whether it was explicitly
// set, mirroring os.LookupEnv. Accepting this in Load keeps config pure
// and injectable.
type LookupFunc func(key string) (string, bool)

// Config holds all settings the application needs to run. Values are
// validated by Load; callers can trust the result without re-checking.
type Config struct {
	DatabaseURL          string
	Host                 string
	Port                 int
	AdminUser            string
	AdminPass            string
	SessionSecret        string
	ContentDir           string
	DefaultProtectedPass string
	BaseURL              string // absolute origin for feeds/sitemap/SEO
	SiteTitle            string // feed channel title
	// Optional OpenAI-compatible LLM (S2). All three required for LLMEnabled.
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
	// MCPToken gates the `wikibuild mcp` stdio server (S4). Empty → refuse to start.
	MCPToken string
}

// LLMEnabled is true when base URL, API key, and model are all non-empty.
func (c Config) LLMEnabled() bool {
	return strings.TrimSpace(c.LLMAPIKey) != "" &&
		strings.TrimSpace(c.LLMBaseURL) != "" &&
		strings.TrimSpace(c.LLMModel) != ""
}

// Load reads settings via lookup, applies defaults, and validates required
// fields. It returns a fully usable Config or an error that names every
// problem it found.
func Load(lookup LookupFunc) (Config, error) {
	cfg := Config{
		Host:      defaultHost,
		Port:      defaultPort,
		ContentDir: defaultContentDir,
		SiteTitle: defaultSiteTitle,
	}

	cfg.DatabaseURL, _ = lookup("DATABASE_URL")
	cfg.AdminUser, _ = lookup("WIKIBUILD_ADMIN_USER")
	cfg.AdminPass, _ = lookup("WIKIBUILD_ADMIN_PASS")
	cfg.SessionSecret, _ = lookup("WIKIBUILD_SESSION_SECRET")
	cfg.DefaultProtectedPass, _ = lookup("WIKIBUILD_DEFAULT_PROTECTED_PASS")
	cfg.BaseURL, _ = lookup("WIKIBUILD_BASE_URL")

	if v, ok := lookup("WIKIBUILD_HOST"); ok && v != "" {
		cfg.Host = v
	}
	if v, ok := lookup("WIKIBUILD_PORT"); ok && v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("%w: WIKIBUILD_PORT %q is not an integer", ErrInvalidConfig, v)
		}
		cfg.Port = port
	}
	if v, ok := lookup("WIKIBUILD_CONTENT_DIR"); ok && v != "" {
		cfg.ContentDir = v
	}
	if v, ok := lookup("WIKIBUILD_SITE_TITLE"); ok && v != "" {
		cfg.SiteTitle = v
	}

	// Optional LLM (OpenAI-compatible). Empty key → feature disabled.
	if v, ok := lookup("WIKIBUILD_LLM_BASE_URL"); ok {
		cfg.LLMBaseURL = strings.TrimSpace(v)
	}
	if v, ok := lookup("WIKIBUILD_LLM_API_KEY"); ok {
		cfg.LLMAPIKey = strings.TrimSpace(v)
	}
	if v, ok := lookup("WIKIBUILD_LLM_MODEL"); ok {
		cfg.LLMModel = strings.TrimSpace(v)
	}
	if v, ok := lookup("WIKIBUILD_MCP_TOKEN"); ok {
		cfg.MCPToken = strings.TrimSpace(v)
	}

	var errs []error

	required := map[string]string{
		"DATABASE_URL":             cfg.DatabaseURL,
		"WIKIBUILD_ADMIN_USER":     cfg.AdminUser,
		"WIKIBUILD_ADMIN_PASS":     cfg.AdminPass,
		"WIKIBUILD_SESSION_SECRET": cfg.SessionSecret,
	}
	for key, val := range required {
		if val == "" {
			errs = append(errs, fmt.Errorf("%w: %s", ErrMissingEnv, key))
		}
	}

	if cfg.SessionSecret != "" && len(cfg.SessionSecret) < minSessionSecretBytes {
		errs = append(errs, fmt.Errorf("%w: WIKIBUILD_SESSION_SECRET must be at least %d bytes",
			ErrInvalidConfig, minSessionSecretBytes))
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("%w: WIKIBUILD_PORT %d out of range 1-65535", ErrInvalidConfig, cfg.Port))
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}
