// Command station-discover stages a bounded official ANP API scope. It creates
// no stations, profiles, reviewed points or schema changes. Reusing a snapshot
// verifies the stored terminal result without refetching the provider.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "station-discover:", err)
		os.Exit(1)
	}
}

func run() error {
	snapshot := flag.String("snapshot", "", "immutable scope/edition identity; repeat only for exact replay")
	uf := flag.String("uf", "", "ANP state scope")
	municipality := flag.String("municipio", "", "ANP municipality name (requires UF)")
	cnpj := flag.String("cnpj", "", "full establishment CNPJ (optional targeted lookup)")
	timeout := flag.Duration("timeout", time.Minute, "overall deadline, maximum five minutes")
	flag.Parse()
	if *snapshot == "" || (*cnpj == "" && (*uf == "" || *municipality == "")) || *timeout <= 0 || *timeout > 5*time.Minute {
		return fmt.Errorf("require --snapshot and either --cnpj or --uf/--municipio, with timeout in (0,5m]")
	}
	cfg := registry.ProductionAPIConfig()
	cfg.UF, cfg.Municipality, cfg.CNPJ = *uf, *municipality, *cnpj
	cfg.MaxPages, cfg.MaxRequests = 5, 10
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pc, err := pgxpool.ParseConfig(os.Getenv("ANPFUEL_DATABASE_URL"))
	if err != nil || os.Getenv("ANPFUEL_DATABASE_URL") == "" {
		return fmt.Errorf("invalid/missing database configuration")
	}
	pc.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	defer pool.Close()
	report, err := registry.Discover(ctx, registry.NewPGStore(pool), *snapshot, cfg)
	if err != nil {
		return fmt.Errorf("discovery failed")
	}
	if err = json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return err
	}
	if report.State != "complete" {
		return fmt.Errorf("scope did not complete: %s/%s", report.State, report.ErrorCode)
	}
	return nil
}
