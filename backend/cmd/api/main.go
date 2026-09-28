// Command api is the public HTTP process role.
//
// Routes land in later tasks (health in P01-T06, domain reads in P02+);
// this task wires the bounded server lifecycle with signal-aware drain.
// No business code lives here.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/health"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := telemetry.New(os.Stdout, cfg.LogLevel, telemetry.RoleAPI)
	if err != nil {
		return err
	}
	router := httpserver.NewRouter(logger)
	// No readiness checks yet: the DB/schema probe lands with the pool in
	// P01-T09. Until then readiness reports process admission only.
	healthHandler, err := health.New(logger, 0)
	if err != nil {
		return err
	}
	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)
	server, err := httpserver.New(httpserver.Options{
		Addr:              cfg.HTTPAddr,
		ReadTimeout:       httpserver.DefaultReadTimeout,
		ReadHeaderTimeout: httpserver.DefaultReadHeaderTimeout,
		WriteTimeout:      httpserver.DefaultWriteTimeout,
		IdleTimeout:       httpserver.DefaultIdleTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		MaxBodyBytes:      cfg.MaxBodyBytes,
		Logger:            logger,
	}, router)
	if err != nil {
		return err
	}
	logger.Info(context.Background(), "api.startup", "api serving")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Run(ctx)
}
