// Package database owns the pgx pool lifecycle and its limits.
//
// Open is lazy: it validates options and the DSN shape without dialing, so a
// process can boot (live, not ready) while PostgreSQL is down. Ping surfaces
// dependency state for readiness checks. Generated per-module query packages
// run against Underlying; no module imports another module's generated
// package. Close on shutdown; a closed pool fails operations loudly.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options bounds pool resources. Tune only with measured pressure (P08).
type Options struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	ConnectTimeout    time.Duration
	HealthCheckPeriod time.Duration
}

// DefaultOptions is the single-host starting point: small, bounded, fast to
// fail when the database is unreachable.
func DefaultOptions() Options {
	return Options{
		MaxConns:          10,
		MinConns:          1,
		MaxConnLifetime:   30 * time.Minute,
		MaxConnIdleTime:   5 * time.Minute,
		ConnectTimeout:    5 * time.Second,
		HealthCheckPeriod: time.Minute,
	}
}

// Pool wraps the pgx pool with validated limits.
type Pool struct {
	pool *pgxpool.Pool
}

// Open validates options and DSN shape, then creates the pool without
// dialing. A malformed DSN fails here; an unreachable database fails at Ping.
func Open(ctx context.Context, dsn string, opts Options) (*Pool, error) {
	if opts.MaxConns <= 0 {
		return nil, fmt.Errorf("database: max conns must be positive")
	}
	if opts.MinConns < 0 || opts.MinConns > opts.MaxConns {
		return nil, fmt.Errorf("database: min conns must be within [0, max conns]")
	}
	if opts.ConnectTimeout <= 0 {
		return nil, fmt.Errorf("database: connect timeout must be positive")
	}
	if opts.MaxConnLifetime < 0 || opts.MaxConnIdleTime < 0 {
		return nil, fmt.Errorf("database: lifetimes must not be negative")
	}
	if opts.HealthCheckPeriod <= 0 {
		return nil, fmt.Errorf("database: healthcheck period must be positive")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("database: parse dsn: %w", err)
	}
	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = opts.MinConns
	cfg.MaxConnLifetime = opts.MaxConnLifetime
	cfg.MaxConnIdleTime = opts.MaxConnIdleTime
	cfg.HealthCheckPeriod = opts.HealthCheckPeriod
	cfg.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("database: open pool: %w", err)
	}
	return &Pool{pool: pool}, nil
}

// Ping reports dependency state for readiness.
func (p *Pool) Ping(ctx context.Context) error {
	return p.pool.Ping(ctx)
}

// Underlying exposes the pool for generated queries and module adapters.
func (p *Pool) Underlying() *pgxpool.Pool {
	return p.pool
}

// Close drains the pool. Operations after Close fail; call once at shutdown.
func (p *Pool) Close() {
	p.pool.Close()
}
