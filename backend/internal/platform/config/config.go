// Package config validates process configuration once at startup.
//
// All values come from ANPFUEL_* environment variables. Errors name the
// variable and the constraint, never the supplied value, so DSN secrets can
// never leak through startup failures or logs. Owning tasks extend Config
// with their own fields (R2 in P05, worker concurrency in P03, retention in
// P07); this task covers environment, DSN and limits only.
package config

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment is the deployment mode. Production enables no permissive
// default that development relies on.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// Environment variable names.
const (
	keyEnv             = "ANPFUEL_ENV"
	keyLogLevel        = "ANPFUEL_LOG_LEVEL"
	keyHTTPAddr        = "ANPFUEL_HTTP_ADDR"
	keyDatabaseURL     = "ANPFUEL_DATABASE_URL"
	keyMaxBodyBytes    = "ANPFUEL_MAX_BODY_BYTES"
	keyShutdownTimeout = "ANPFUEL_SHUTDOWN_TIMEOUT"
	keyCursorSecret    = "ANPFUEL_CURSOR_SECRET"
	keyCanonicalHost   = "ANPFUEL_CANONICAL_HOST"
	keyMetricsAddr     = "ANPFUEL_METRICS_ADDR"
)

// Defaults. devDatabaseURL is loopback-only and valid for local development;
// staging and production must provide an explicit DSN.
const (
	defaultLogLevel        = "info"
	defaultHTTPAddr        = ":8080"
	defaultMaxBodyBytes    = 1 << 20
	defaultShutdownTimeout = 10 * time.Second
	// defaultMetricsAddr binds the metrics listener to loopback only:
	// metrics stay on the private listener and are never published by
	// the staging/production topologies. Empty disables the listener.
	defaultMetricsAddr = "127.0.0.1:9090"

	devDatabaseURL = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
)

// Config is the validated process configuration. CursorSecret signs
// pagination cursors and is never logged; an unset value generates one
// per-boot key (single-instance default: cursors invalidate on restart,
// multi-instance production must set a shared secret). CanonicalHost is the
// authority proofs cover; proxy hosts are never trusted.
type Config struct {
	Env             Environment
	LogLevel        string
	HTTPAddr        string
	DatabaseURL     string
	MaxBodyBytes    int64
	ShutdownTimeout time.Duration
	CursorSecret    []byte
	CanonicalHost   string
	// MetricsAddr is the private metrics listener; empty disables it.
	MetricsAddr         string
	ANPDiscoveryEnabled bool
	// R2 is nil unless private storage is configured; upload issuance
	// refuses explicitly while nil instead of misbehaving.
	R2                           *R2Config
	DevelopmentPhotoPreviewUntil time.Time
}

// LogValue renders Config for slog without secrets: the DSN never appears,
// only whether one is configured.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", string(c.Env)),
		slog.String("log_level", c.LogLevel),
		slog.String("http_addr", c.HTTPAddr),
		slog.Int64("max_body_bytes", c.MaxBodyBytes),
		slog.Duration("shutdown_timeout", c.ShutdownTimeout),
		slog.Bool("db_configured", c.DatabaseURL != ""),
		slog.Any("r2", c.R2.r2LogValue()),
	)
}

// Load validates configuration from the process environment.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(getenv func(string) (string, bool)) (Config, error) {
	var cfg Config
	cfg.ANPDiscoveryEnabled = true
	if raw, present := getenv("ANPFUEL_ANP_DISCOVERY_ENABLED"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_ANP_DISCOVERY_ENABLED must be boolean")
		}
		cfg.ANPDiscoveryEnabled = enabled
	}

	env, _ := getenv(keyEnv)
	if env == "" {
		env = string(EnvDevelopment)
	}
	switch Environment(env) {
	case EnvDevelopment, EnvStaging, EnvProduction:
		cfg.Env = Environment(env)
	default:
		return Config{}, fmt.Errorf("%s must be development, staging or production", keyEnv)
	}

	level, _ := getenv(keyLogLevel)
	if level == "" {
		level = defaultLogLevel
	}
	switch level {
	case "debug", "info", "warn", "error":
		cfg.LogLevel = level
	default:
		return Config{}, fmt.Errorf("%s must be debug, info, warn or error", keyLogLevel)
	}

	addr, _ := getenv(keyHTTPAddr)
	if addr == "" {
		addr = defaultHTTPAddr
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return Config{}, fmt.Errorf("%s must be a valid host:port", keyHTTPAddr)
	}
	cfg.HTTPAddr = addr

	dsn, dsnSet := getenv(keyDatabaseURL)
	if !dsnSet || dsn == "" {
		if cfg.Env != EnvDevelopment {
			return Config{}, fmt.Errorf("%s is required outside development", keyDatabaseURL)
		}
		dsn = devDatabaseURL
	}
	if (cfg.Env == EnvStaging || cfg.Env == EnvProduction) && dsn == devDatabaseURL {
		return Config{}, fmt.Errorf("%s must be explicit outside development", keyDatabaseURL)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return Config{}, fmt.Errorf("%s must be a valid postgres DSN", keyDatabaseURL)
	}
	cfg.DatabaseURL = dsn

	rawBody, _ := getenv(keyMaxBodyBytes)
	if rawBody == "" {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	} else {
		n, err := strconv.ParseInt(rawBody, 10, 64)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive byte count", keyMaxBodyBytes)
		}
		cfg.MaxBodyBytes = n
	}

	rawTimeout, _ := getenv(keyShutdownTimeout)
	if rawTimeout == "" {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	} else {
		d, err := time.ParseDuration(rawTimeout)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive duration", keyShutdownTimeout)
		}
		cfg.ShutdownTimeout = d
	}

	secret, secretSet := getenv(keyCursorSecret)
	if !secretSet || secret == "" {
		if cfg.Env == EnvProduction {
			return Config{}, fmt.Errorf("%s is required in production", keyCursorSecret)
		}
		ephemeral := make([]byte, 32)
		if _, err := rand.Read(ephemeral); err != nil {
			return Config{}, fmt.Errorf("%s fallback key generation failed", keyCursorSecret)
		}
		cfg.CursorSecret = ephemeral
	} else {
		if len(secret) < 32 {
			return Config{}, fmt.Errorf("%s must be at least 32 bytes", keyCursorSecret)
		}
		cfg.CursorSecret = []byte(secret)
	}

	host, _ := getenv(keyCanonicalHost)
	if host == "" {
		if cfg.Env == EnvProduction {
			return Config{}, fmt.Errorf("%s is required in production", keyCanonicalHost)
		}
		host = "api.example.invalid"
	}
	if !validHostname(host) || (cfg.Env == EnvProduction && strings.HasSuffix(strings.ToLower(host), ".invalid")) {
		return Config{}, fmt.Errorf("%s must be a bare hostname", keyCanonicalHost)
	}
	cfg.CanonicalHost = strings.ToLower(host)

	metrics, metricsSet := getenv(keyMetricsAddr)
	if !metricsSet {
		metrics = defaultMetricsAddr
	}
	// Explicit empty disables the listener; otherwise a valid host:port.
	if metrics != "" {
		if _, _, err := net.SplitHostPort(metrics); err != nil {
			return Config{}, fmt.Errorf("%s must be a valid host:port or empty", keyMetricsAddr)
		}
		cfg.MetricsAddr = metrics
	}

	r2, err := loadR2(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.R2 = r2

	if raw, present := getenv("ANPFUEL_DEV_PHOTO_PREVIEW_UNTIL"); present && raw != "" {
		deadline, err := time.Parse(time.RFC3339, raw)
		if err != nil || cfg.Env == EnvProduction ||
			(cfg.Env == EnvStaging && cfg.CanonicalHost != "teste.abastevo.com.br") ||
			deadline.After(time.Now().Add(7*24*time.Hour)) {
			return Config{}, fmt.Errorf("ANPFUEL_DEV_PHOTO_PREVIEW_UNTIL requires a valid bounded deadline in development or the owned staging host")
		}
		cfg.DevelopmentPhotoPreviewUntil = deadline
	}
	return cfg, nil
}

// validHostname rejects controls, URL syntax and invalid DNS labels before
// the configured authority becomes part of a cryptographic signature base.
func validHostname(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}
