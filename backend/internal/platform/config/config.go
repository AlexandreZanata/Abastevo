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
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	legal "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/legalterms"
)

// Environment is the deployment mode. Production enables no permissive
// default that development relies on.
type Environment string

var defaultTrainingTermsDigest = legal.Digest

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
	// keyMailMode selects code delivery: "off" (default — codes
	// never issue) or "memory" (bounded local outbox for
	// development/tests). SMTP lands with deployment
	// configuration, never silently.
	keyMailMode = "ANPFUEL_MAIL_MODE"
	// keyTrainingTermsDigest carries the digest of the currently
	// published training-terms text that fresh consent grants must
	// present. Rotating it invalidates old grants until owners
	// re-grant explicitly (CS09); the verified terms text itself
	// lands with the Settings wiring (CS11).
	keyTrainingTermsDigest = "ANPFUEL_TRAINING_TERMS_DIGEST"
	// keyTrustedProxies carries the comma-separated CIDR allowlist
	// whose forwarding headers the identity quota resolver may bill
	// (SEC02-B). Empty trusts nothing and preserves peer-only
	// billing; staging/production set their edge network explicitly.
	keyTrustedProxies = "ANPFUEL_TRUSTED_PROXIES"
	// keyGlobalQuotaChallenge/Register/Write carry the deliberate
	// hourly global ceilings for identity operations (SEC02-C),
	// separate from per-client/key fairness. Empty or zero disables
	// that operation (no global cap; fairness still enforced).
	keyGlobalQuotaChallenge = "ANPFUEL_GLOBAL_QUOTA_CHALLENGE"
	keyGlobalQuotaRegister  = "ANPFUEL_GLOBAL_QUOTA_REGISTER"
	keyGlobalQuotaWrite     = "ANPFUEL_GLOBAL_QUOTA_WRITE"
	keyEvidenceMaxPending   = "ANPFUEL_EVIDENCE_MAX_PENDING_PHOTOS"
	// keyQualityReadsEnabled gates the anonymous quality ledger reads
	// (FQ-08). Enabled by default; setting it false answers 404 without
	// touching storage, which is the documented read recovery.
	keyQualityReadsEnabled   = "ANPFUEL_QUALITY_READS_ENABLED"
	keyQualityReportsEnabled = "ANPFUEL_QUALITY_REPORTS_ENABLED"
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
	MetricsAddr           string
	ANPDiscoveryEnabled   bool
	QualityImportsEnabled bool
	// QualityReadsEnabled gates /v1/quality/* (FQ-08 recovery switch).
	QualityReadsEnabled   bool
	QualityReportsEnabled bool
	// TrainingTermsDigest is the published digest fresh training
	// grants must present; served to owners on the status read, never
	// logged as anything but configured/unset.
	TrainingTermsDigest string
	// MailMode is "off" or "memory" (see keyMailMode). Production
	// stays off until delivery is deployed and configured.
	MailMode string
	// TrustedProxies is the raw comma-separated CIDR allowlist for
	// the identity quota resolver (see keyTrustedProxies). Empty
	// trusts nothing. The value carries network ranges, never
	// secrets, and stays out of logs with the rest of the config.
	TrustedProxies string
	// GlobalQuotaChallenge/Register/Write are the deliberate hourly
	// global ceilings for identity operations (see keyGlobalQuota*).
	// Zero disables that operation (no global cap).
	GlobalQuotaChallenge int
	GlobalQuotaRegister  int
	GlobalQuotaWrite     int
	// Zero keeps photo intake closed pending a measured resource budget.
	EvidenceMaxPendingPhotos int
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
	if raw, present := getenv("ANPFUEL_QUALITY_IMPORTS_ENABLED"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_IMPORTS_ENABLED must be boolean")
		}
		cfg.QualityImportsEnabled = enabled
	}
	cfg.QualityReportsEnabled = true
	if raw, present := getenv(keyQualityReportsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_REPORTS_ENABLED must be boolean")
		}
		cfg.QualityReportsEnabled = enabled
	}
	cfg.QualityReadsEnabled = true
	if raw, present := getenv(keyQualityReadsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_READS_ENABLED must be boolean")
		}
		cfg.QualityReadsEnabled = enabled
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

	digest, _ := getenv(keyTrainingTermsDigest)
	if digest == "" {
		digest = defaultTrainingTermsDigest
	}
	if !validTermsDigest(digest) || digest != legal.Digest {
		return Config{}, fmt.Errorf("%s must match the SHA-256 of the bundled published training terms", keyTrainingTermsDigest)
	}
	cfg.TrainingTermsDigest = digest

	mode, _ := getenv(keyMailMode)
	if mode == "" {
		mode = "off"
	}
	if mode != "off" && mode != "memory" {
		return Config{}, fmt.Errorf("%s must be off or memory", keyMailMode)
	}
	cfg.MailMode = mode

	proxies, _ := getenv(keyTrustedProxies)
	if strings.TrimSpace(proxies) != "" {
		for _, part := range strings.Split(proxies, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, err := netip.ParsePrefix(part); err != nil {
				return Config{}, fmt.Errorf("%s must be comma-separated CIDRs", keyTrustedProxies)
			}
		}
		cfg.TrustedProxies = proxies
	}

	for _, tc := range []struct {
		key string
		dst *int
	}{
		{keyGlobalQuotaChallenge, &cfg.GlobalQuotaChallenge},
		{keyGlobalQuotaRegister, &cfg.GlobalQuotaRegister},
		{keyGlobalQuotaWrite, &cfg.GlobalQuotaWrite},
	} {
		raw, _ := getenv(tc.key)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("%s must be a non-negative hourly ceiling", tc.key)
		}
		*tc.dst = n
	}

	r2, err := loadR2(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.R2 = r2
	if raw, _ := getenv(keyEvidenceMaxPending); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 100 {
			return Config{}, fmt.Errorf("%s must be 0..100 pending photos", keyEvidenceMaxPending)
		}
		cfg.EvidenceMaxPendingPhotos = n
	}

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

// validTermsDigest bounds the configured training-terms digest before
// it becomes part of the grant-equality check: non-empty,
// length-capped, lowercase hex only.
func validTermsDigest(digest string) bool {
	if len(digest) == 0 || len(digest) > 128 {
		return false
	}
	for _, c := range digest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
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
