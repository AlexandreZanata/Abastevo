package database

import (
	"context"
	"testing"
	"time"
)

func TestOpenRejectsBadOptions(t *testing.T) {
	good := DefaultOptions()
	cases := map[string]func(*Options){
		"zero max conns":    func(o *Options) { o.MaxConns = 0 },
		"negative max":      func(o *Options) { o.MaxConns = -1 },
		"negative min":      func(o *Options) { o.MinConns = -1 },
		"min above max":     func(o *Options) { o.MinConns = o.MaxConns + 1 },
		"zero connect":      func(o *Options) { o.ConnectTimeout = 0 },
		"zero healthcheck":  func(o *Options) { o.HealthCheckPeriod = 0 },
		"negative idle":     func(o *Options) { o.MaxConnIdleTime = -time.Second },
		"negative lifetime": func(o *Options) { o.MaxConnLifetime = -time.Second },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			opts := good
			mutate(&opts)
			if _, err := Open(context.Background(), "postgres://u:p@127.0.0.1:1/db", opts); err == nil {
				t.Errorf("expected error for %s, got nil", name)
			}
		})
	}
}

func TestOpenRejectsMalformedDSN(t *testing.T) {
	if _, err := Open(context.Background(), "://missing-scheme", DefaultOptions()); err == nil {
		t.Error("expected error for malformed DSN, got nil")
	}
}

func TestDefaultOptionsSane(t *testing.T) {
	opts := DefaultOptions()
	if opts.MaxConns <= 0 || opts.MinConns < 0 || opts.MinConns > opts.MaxConns {
		t.Errorf("bad conn bounds: %+v", opts)
	}
	if opts.ConnectTimeout <= 0 {
		t.Errorf("ConnectTimeout must be positive: %+v", opts)
	}
}
