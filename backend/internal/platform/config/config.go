// Package config validates process configuration once at startup.
//
// All values come from ANPFUEL_* environment variables. Errors name the
// variable and the constraint, never the supplied value, so DSN secrets can
// never leak through startup failures or logs. Owning tasks extend Config
// with their own fields (R2 in P05, worker concurrency in P03, retention in
// P07); this task covers environment, DSN and limits only.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
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
)

// Defaults. devDatabaseURL is loopback-only and valid for local development;
// staging and production must provide an explicit DSN.
const (
	defaultLogLevel        = "info"
	defaultHTTPAddr        = ":8080"
	defaultMaxBodyBytes    = 1 << 20
	defaultShutdownTimeout = 10 * time.Second

	devDatabaseURL = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
)

// Config is the validated process configuration.
type Config struct {
	Env             Environment
	LogLevel        string
	HTTPAddr        string
	DatabaseURL     string
	MaxBodyBytes    int64
	ShutdownTimeout time.Duration
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
	)
}

// Load validates configuration from the process environment.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(getenv func(string) (string, bool)) (Config, error) {
	var cfg Config

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

	return cfg, nil
}
