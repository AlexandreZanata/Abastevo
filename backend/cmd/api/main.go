// Command api is the public HTTP process role.
//
// Routes land in later tasks (health in P01-T06, domain reads in P02+);
// this task wires the bounded server lifecycle with signal-aware drain.
// No business code lives here.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	directoryhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/http"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	officialhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/http"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
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
	// Lazy pool: the process boots (live, not ready) while PostgreSQL is
	// down. Closed after the server drains, at shutdown.
	pool, err := database.Open(context.Background(), cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return err
	}
	defer pool.Close()
	healthHandler, err := health.New(logger, 0, health.Check{Name: "db", Fn: pool.Ping})
	if err != nil {
		return err
	}
	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)
	// Anonymous catalog reads (P02-T08). Only this composition root wires
	// modules together: the official handler gets station existence as a
	// closure so modules never cross-read.
	stations := directoryread.NewReader(pool.Underlying())
	prices := officialread.NewReader(pool.Underlying())
	directoryhttp.Handler{Stations: stations, Secrets: cfg.CursorSecret}.RegisterRoutes(router)
	officialhttp.Handler{
		Prices:  prices,
		Secrets: cfg.CursorSecret,
		StationsExist: func(ctx context.Context, id string) (bool, error) {
			_, err := stations.Detail(ctx, id)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, directoryapp.ErrUnknownStation) {
				return false, nil
			}
			return false, err
		},
	}.RegisterRoutes(router)
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
