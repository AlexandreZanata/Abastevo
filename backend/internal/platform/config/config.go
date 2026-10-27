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
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
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
	// Optional signed map distribution. Both variables are required together;
	// an unset pair leaves the map distribution routes unmounted.
	keyMapDistributionDir  = "ANPFUEL_MAP_DIST_DIR"
	keyMapDistributionKeys = "ANPFUEL_MAP_DIST_PUBLIC_KEYS"
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
	keyQualityCompanyReadsEnabled = "ANPFUEL_QUALITY_COMPANY_READS_ENABLED"
	keyQualityReadsEnabled        = "ANPFUEL_QUALITY_READS_ENABLED"
	keyQualityReportsEnabled      = "ANPFUEL_QUALITY_REPORTS_ENABLED"
	// Anonymous ANP weekly ranking. The cache is in-process and bounded;
	// zero entries is the documented uncached recovery.
	keyPriceRankingEnabled         = "ANPFUEL_PRICE_RANKING_ENABLED"
	keyPriceRankingCacheEntries    = "ANPFUEL_PRICE_RANKING_CACHE_ENTRIES"
	keyPriceRankingCacheTTLSeconds = "ANPFUEL_PRICE_RANKING_CACHE_TTL_SECONDS"
	// Source discovery/revalidation schedules are independent and default
	// off. Installing a schedule never enables native imports.
	keyQualityEnforcementImportsEnabled   = "ANPFUEL_QUALITY_ENFORCEMENT_IMPORTS_ENABLED"
	keyChargingImportsEnabled             = "ANPFUEL_CHARGING_IMPORTS_ENABLED"
	keyChargingReadsEnabled               = "ANPFUEL_CHARGING_READS_ENABLED"
	keyQualityEnforcementDiscoveryEnabled = "ANPFUEL_QUALITY_ENFORCEMENT_DISCOVERY_ENABLED"
	keyQualitySanctionsDiscoveryEnabled   = "ANPFUEL_QUALITY_SANCTIONS_DISCOVERY_ENABLED"
	keyQualityDiscoveryEnabled            = "ANPFUEL_QUALITY_DISCOVERY_ENABLED"
	keyQualityRevalidationEnabled         = "ANPFUEL_QUALITY_REVALIDATION_ENABLED"
	// Administrative surface (ADR-023). The raw key never reaches the
	// process: only its lowercase SHA-256 is configured. No hash means no
	// /v1/admin/* route is registered.
	keyAdminKeySHA256     = "ANPFUEL_ADMIN_KEY_SHA256"
	keyAdminKeySHA256Next = "ANPFUEL_ADMIN_KEY_SHA256_NEXT"
	keyAdminOrigin        = "ANPFUEL_ADMIN_ORIGIN"
	keyAdminAllowedCIDRs  = "ANPFUEL_ADMIN_ALLOWED_CIDRS"
	// Contributor surface (ADR-023). The GitHub repository is owner/repo;
	// the token is optional and read-only. Public web origins are the only
	// origins allowed to read GET /v1/contributors with CORS.
	keyGitHubRepo       = "ANPFUEL_GITHUB_REPO"
	keyGitHubToken      = "ANPFUEL_GITHUB_TOKEN"
	keyPublicWebOrigins = "ANPFUEL_PUBLIC_WEB_ORIGINS"
	keyDriverEnabled    = "ANPFUEL_DRIVER_ENABLED"
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
	MetricsAddr string
	// MapDistributionDir/Keys configure the optional signed native-map
	// distribution. Keys use keyid:base64url,keyid:base64url.
	MapDistributionDir    string
	MapDistributionKeys   string
	ANPDiscoveryEnabled   bool
	QualityImportsEnabled bool
	// Bounded source schedules are independent and default off. Handlers
	// still require the native imports switch; a schedule alone only queues.
	QualityEnforcementImportsEnabled   bool
	ChargingImportsEnabled             bool
	ChargingReadsEnabled               bool
	QualityEnforcementDiscoveryEnabled bool
	QualitySanctionsDiscoveryEnabled   bool
	QualityDiscoveryEnabled            bool
	QualityRevalidationEnabled         bool
	// DriverEnabled gates private driver report/command routes and worker cleanup.
	DriverEnabled bool
	// QualityReadsEnabled gates /v1/quality/* (FQ-08 recovery switch).
	QualityReadsEnabled        bool
	QualityCompanyReadsEnabled bool
	QualityReportsEnabled      bool
	// PriceRankingEnabled gates /v1/prices/ranking. The in-process cache is
	// bounded; PriceRankingCacheEntries=0 runs the exact uncached query.
	PriceRankingEnabled         bool
	PriceRankingCacheEntries    int
	PriceRankingCacheTTLSeconds int
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
	// Enterprise export is a private, bounded candidate builder. It is off by
	// default and requires an absolute private workspace root when enabled.
	EnterpriseExportEnabled     bool
	EnterpriseExportRoot        string
	EnterpriseExportWindowWeeks int
	// Enterprise portal refresh builds and promotes a verified static release
	// from the export candidate. It is off by default and requires the export
	// builder plus an absolute private root and a public origin.
	EnterprisePortalRefreshEnabled bool
	EnterprisePortalRoot           string
	EnterprisePortalSiteURL        string
	EnterprisePortalIndexable      bool
	// EnterprisePortalOfficialOnly restricts refresh generation,
	// verification, promotion and withdrawal to the admitted
	// official-records slice. It can never widen to the full portal.
	EnterprisePortalOfficialOnly bool
	EnterprisePortalSearchScript string
	// R2 is nil unless private storage is configured; upload issuance
	// refuses explicitly while nil instead of misbehaving.
	R2                           *R2Config
	DevelopmentPhotoPreviewUntil time.Time
	// Payments holds provider switches and credentials; LogValue/Preflight
	// expose readiness only, never key material.
	Payments PaymentsConfig
	// AdminKeyHashes are the accepted lowercase SHA-256 digests (current
	// plus optional rotation value). Empty keeps the admin routes unmounted.
	// AdminOrigin must exactly match the browser Origin for non-GET admin
	// requests. AdminAllowedCIDRs is the optional network fence. None of
	// these values is a raw key and none is logged.
	AdminKeyHashes    []string
	AdminOrigin       string
	AdminAllowedCIDRs []netip.Prefix
	// GitHubRepo is owner/repo for contributor snapshots. GitHubToken is
	// optional and never logged. PublicWebOrigins is the CORS allowlist for
	// GET /v1/contributors only.
	GitHubRepo       string
	GitHubToken      string
	PublicWebOrigins []string
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
		slog.Bool("admin_configured", len(c.AdminKeyHashes) > 0),
		slog.String("github_repo", c.GitHubRepo),
		slog.Bool("github_token_configured", c.GitHubToken != ""),
		slog.Int("public_web_origins", len(c.PublicWebOrigins)),
		slog.Any("r2", c.R2.r2LogValue()),
		slog.Any("payments", c.Payments.logValue()),
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
	for key, target := range map[string]*bool{
		keyQualityEnforcementImportsEnabled:   &cfg.QualityEnforcementImportsEnabled,
		keyChargingImportsEnabled:             &cfg.ChargingImportsEnabled,
		keyChargingReadsEnabled:               &cfg.ChargingReadsEnabled,
		keyQualityEnforcementDiscoveryEnabled: &cfg.QualityEnforcementDiscoveryEnabled,
		keyQualitySanctionsDiscoveryEnabled:   &cfg.QualitySanctionsDiscoveryEnabled,
		keyQualityDiscoveryEnabled:            &cfg.QualityDiscoveryEnabled,
		keyQualityRevalidationEnabled:         &cfg.QualityRevalidationEnabled,
	} {
		if raw, present := getenv(key); present {
			enabled, err := strconv.ParseBool(raw)
			if err != nil {
				return Config{}, fmt.Errorf("%s must be boolean", key)
			}
			*target = enabled
		}
	}
	cfg.QualityReportsEnabled = true
	if raw, present := getenv(keyQualityReportsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_REPORTS_ENABLED must be boolean")
		}
		cfg.QualityReportsEnabled = enabled
	}
	if raw, present := getenv("ANPFUEL_ENTERPRISE_EXPORT_ENABLED"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_EXPORT_ENABLED must be boolean")
		}
		cfg.EnterpriseExportEnabled = enabled
	}
	cfg.EnterpriseExportWindowWeeks = 5
	if raw, present := getenv("ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS"); present {
		weeks, err := strconv.Atoi(raw)
		if err != nil || weeks < 1 || weeks > 12 {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_EXPORT_WINDOW_WEEKS must be 1..12")
		}
		cfg.EnterpriseExportWindowWeeks = weeks
	}
	cfg.EnterpriseExportRoot, _ = getenv("ANPFUEL_ENTERPRISE_EXPORT_ROOT")
	if cfg.EnterpriseExportEnabled {
		if cfg.EnterpriseExportRoot == "" || !filepath.IsAbs(cfg.EnterpriseExportRoot) {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_EXPORT_ROOT must be an absolute path when enterprise export is enabled")
		}
	}
	if raw, present := getenv("ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED must be boolean")
		}
		cfg.EnterprisePortalRefreshEnabled = enabled
	}
	if raw, present := getenv("ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_INDEXABLE must be boolean")
		}
		cfg.EnterprisePortalIndexable = enabled
	}
	if raw, present := getenv("ANPFUEL_ENTERPRISE_PORTAL_OFFICIAL_ONLY"); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_OFFICIAL_ONLY must be boolean")
		}
		cfg.EnterprisePortalOfficialOnly = enabled
	}
	cfg.EnterprisePortalRoot, _ = getenv("ANPFUEL_ENTERPRISE_PORTAL_ROOT")
	cfg.EnterprisePortalSiteURL, _ = getenv("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL")
	cfg.EnterprisePortalSearchScript, _ = getenv("ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS")
	if cfg.EnterprisePortalRefreshEnabled {
		if !cfg.EnterpriseExportEnabled {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_REFRESH_ENABLED requires ANPFUEL_ENTERPRISE_EXPORT_ENABLED")
		}
		if cfg.EnterprisePortalRoot == "" || !filepath.IsAbs(cfg.EnterprisePortalRoot) {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_ROOT must be an absolute path when portal refresh is enabled")
		}
		if cfg.EnterprisePortalSiteURL == "" {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL is required when portal refresh is enabled")
		}
		parsed, err := url.Parse(cfg.EnterprisePortalSiteURL)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_SITE_URL must be an http(s) origin")
		}
		if cfg.EnterprisePortalRoot == cfg.EnterpriseExportRoot {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_ROOT must differ from the export root")
		}
		if cfg.EnterprisePortalSearchScript != "" && !filepath.IsAbs(cfg.EnterprisePortalSearchScript) {
			return Config{}, fmt.Errorf("ANPFUEL_ENTERPRISE_PORTAL_SEARCH_JS must be an absolute path")
		}
	}
	if raw, present := getenv(keyQualityCompanyReadsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_COMPANY_READS_ENABLED must be boolean")
		}
		cfg.QualityCompanyReadsEnabled = enabled
	}
	cfg.QualityReadsEnabled = true
	if raw, present := getenv(keyQualityReadsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_QUALITY_READS_ENABLED must be boolean")
		}
		cfg.QualityReadsEnabled = enabled
	}
	cfg.ChargingReadsEnabled = true
	if raw, present := getenv(keyChargingReadsEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_CHARGING_READS_ENABLED must be boolean")
		}
		cfg.ChargingReadsEnabled = enabled
	}

	if raw, present := getenv(keyDriverEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_DRIVER_ENABLED must be boolean")
		}
		cfg.DriverEnabled = enabled
	}

	cfg.PriceRankingEnabled = true
	if raw, present := getenv(keyPriceRankingEnabled); present {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("ANPFUEL_PRICE_RANKING_ENABLED must be boolean")
		}
		cfg.PriceRankingEnabled = enabled
	}
	cfg.PriceRankingCacheEntries = 128
	if raw, present := getenv(keyPriceRankingCacheEntries); present {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 4096 {
			return Config{}, fmt.Errorf("ANPFUEL_PRICE_RANKING_CACHE_ENTRIES must be 0..4096")
		}
		cfg.PriceRankingCacheEntries = n
	}
	cfg.PriceRankingCacheTTLSeconds = 60
	if raw, present := getenv(keyPriceRankingCacheTTLSeconds); present {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 3600 {
			return Config{}, fmt.Errorf("ANPFUEL_PRICE_RANKING_CACHE_TTL_SECONDS must be 1..3600")
		}
		cfg.PriceRankingCacheTTLSeconds = n
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

	mapDir, _ := getenv(keyMapDistributionDir)
	mapKeys, _ := getenv(keyMapDistributionKeys)
	if (strings.TrimSpace(mapDir) == "") != (strings.TrimSpace(mapKeys) == "") {
		return Config{}, fmt.Errorf("%s and %s must be set together", keyMapDistributionDir, keyMapDistributionKeys)
	}
	cfg.MapDistributionDir = strings.TrimSpace(mapDir)
	cfg.MapDistributionKeys = strings.TrimSpace(mapKeys)

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
	cfg.Payments = loadPayments(getenv, cfg.Env)
	adminHashes, adminOrigin, adminCIDRs, err := loadAdmin(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.AdminKeyHashes = adminHashes
	cfg.AdminOrigin = adminOrigin
	cfg.AdminAllowedCIDRs = adminCIDRs
	githubRepo, githubToken, publicOrigins, err := loadContributor(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.GitHubRepo = githubRepo
	cfg.GitHubToken = githubToken
	cfg.PublicWebOrigins = publicOrigins
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

// loadAdmin validates the optional administrative surface. A malformed hash,
// origin or CIDR fails startup by variable name only; no value is echoed. No
// hash keeps the routes unmounted (ADM-01).
func loadAdmin(getenv func(string) (string, bool)) ([]string, string, []netip.Prefix, error) {
	var hashes []string
	seen := map[string]bool{}
	for _, name := range []string{keyAdminKeySHA256, keyAdminKeySHA256Next} {
		raw, _ := getenv(name)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !validAdminHash(raw) {
			return nil, "", nil, fmt.Errorf("%s must be 64 lowercase hex characters", name)
		}
		if !seen[raw] {
			seen[raw] = true
			hashes = append(hashes, raw)
		}
	}

	origin, _ := getenv(keyAdminOrigin)
	origin = strings.TrimSpace(origin)
	if origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
			parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, "", nil, fmt.Errorf("%s must be an http(s) origin", keyAdminOrigin)
		}
		origin = parsed.Scheme + "://" + parsed.Host
	}
	if len(hashes) > 0 && origin == "" {
		return nil, "", nil, fmt.Errorf("%s is required when an admin key hash is configured", keyAdminOrigin)
	}

	var cidrs []netip.Prefix
	rawCIDRs, _ := getenv(keyAdminAllowedCIDRs)
	for _, part := range strings.Split(rawCIDRs, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, "", nil, fmt.Errorf("%s must be comma-separated CIDRs", keyAdminAllowedCIDRs)
		}
		cidrs = append(cidrs, prefix.Masked())
	}
	return hashes, origin, cidrs, nil
}

// loadContributor validates the optional contributor surface. The token is
// never echoed; malformed values fail by variable name only.
func loadContributor(getenv func(string) (string, bool)) (string, string, []string, error) {
	repo := strings.TrimSpace(getenvValue(getenv, keyGitHubRepo))
	if repo != "" {
		parts := strings.Split(repo, "/")
		if len(parts) != 2 || !validRepoPart(parts[0]) || !validRepoPart(parts[1]) {
			return "", "", nil, fmt.Errorf("%s must be owner/repo", keyGitHubRepo)
		}
	}
	token := strings.TrimSpace(getenvValue(getenv, keyGitHubToken))
	var origins []string
	seen := map[string]bool{}
	for _, part := range strings.Split(getenvValue(getenv, keyPublicWebOrigins), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := url.Parse(part)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
			parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return "", "", nil, fmt.Errorf("%s must be comma-separated http(s) origins", keyPublicWebOrigins)
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if !seen[origin] {
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	return repo, token, origins, nil
}

func getenvValue(getenv func(string) (string, bool), key string) string {
	value, _ := getenv(key)
	return value
}

func validRepoPart(part string) bool {
	if part == "" || len(part) > 100 {
		return false
	}
	for _, c := range part {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func validAdminHash(raw string) bool {
	if len(raw) != 64 || strings.ToLower(raw) != raw {
		return false
	}
	_, err := hex.DecodeString(raw)
	return err == nil
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
