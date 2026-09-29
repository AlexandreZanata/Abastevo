// Command worker is the background-job process role.
//
// It composes the dispatcher, the recurring schedules and the durable
// handlers, then ticks schedules and drains the queue until signaled. Only
// composition lives here: schedules are versioned payloads, handlers own
// their logic, and the geocode schedule stays disabled until D05 selects a
// live provider.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/jobs"
	officialadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	officialjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := telemetry.New(os.Stdout, cfg.LogLevel, telemetry.RoleWorker)
	if err != nil {
		return err
	}
	_ = logger
	pool, err := database.Open(context.Background(), cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()

	directoryRepo := directoryadapters.NewRepository(raw)
	importer := officialadapters.NewImporter(raw)
	fetcher := source.NewFetcher(source.DefaultAllowlist())
	queue := jobs.NewQueue(raw)
	enqueueImport := func(ctx context.Context, payload officialjobs.ImportPayload, dedupe string) error {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = jobs.Enqueue(ctx, raw, "anp-import", encoded, dedupe, 3, time.Time{})
		return err
	}
	handlers := map[string]jobs.Handler{
		"anp-discovery": officialjobs.Discovery{
			Fetch:       &fetcher,
			ListingBase: "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/",
			Enqueue:     enqueueImport,
		},
		"anp-import": officialjobs.Import{
			Fetch:  &fetcher,
			Parser: anp.Parser{Limits: anp.DefaultLimits()},
			Import: importer,
			Resolve: func(ctx context.Context, cnpj, display string, addr map[string]string) (string, error) {
				st, err := directoryRepo.ResolveCNPJ(ctx, cnpj, display, addr)
				if err != nil {
					return "", err
				}
				return st.ID, nil
			},
		},
		"geocode-station": directoryjobs.Geocode{
			Pool:    raw,
			Service: nil,
			Batch:   25,
		},
	}
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: handlers,
		WorkerID: "worker-1", LeaseTTL: 5 * time.Minute, RetryDelay: time.Minute,
	}
	scheduler := jobs.NewScheduler(raw, []jobs.Schedule{
		{
			Name: "anp-discovery-daily", Kind: "anp-discovery", Version: 1,
			Interval: 24 * time.Hour, Enabled: true,
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "geocode-hourly", Kind: "geocode-station", Version: 1,
			Interval: time.Hour, Enabled: false, Reason: "D05 pending: no live provider",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dispatchErr := make(chan error, 1)
	go func() {
		if err := dispatch.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			dispatchErr <- err
			return
		}
		dispatchErr <- nil
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := scheduler.Tick(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return <-dispatchErr
		case err := <-dispatchErr:
			return err
		case <-ticker.C:
		}
	}
}
