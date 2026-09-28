// Command migrate is the restricted one-shot schema-migration tool.
//
// It applies the embedded append-only chain with lock/checksum ledger and
// exits non-zero on any failure. The API role must never run DDL.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	applied, err := migrate.Apply(ctx, cfg.DatabaseURL, dbmigrations.Files)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Fprintln(os.Stdout, "anpfuel migrate: schema already current")
		return nil
	}
	fmt.Fprintf(os.Stdout, "anpfuel migrate: applied %v\n", applied)
	return nil
}
