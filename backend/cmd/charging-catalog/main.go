// Command charging-catalog verifies local national artifacts and, with
// explicit owner approval, activates one complete batch in PostGIS.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "charging-catalog:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("charging-catalog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	bundle := flags.String("bundle", "", "owner-reviewed rights bundle directory")
	geography := flags.String("geography", "", "IBGE site classification sidecar")
	input := prepared.NationalImport{}
	flags.StringVar(&input.RightsSHA256, "rights-sha256", "", "trusted rights receipt SHA-256")
	flags.StringVar(&input.ManifestSHA256, "manifest-sha256", "", "trusted conflation manifest SHA-256")
	flags.StringVar(&input.GeographySHA256, "geography-sha256", "", "trusted geography SHA-256")
	flags.StringVar(&input.BoundarySHA256, "boundary-sha256", "", "trusted IBGE boundary SHA-256")
	activate := flags.Bool("activate", false, "activate a verified national batch")
	approved := flags.String("approve-batch-sha256", "", "owner-approved candidate digest from verification")
	previous := flags.String("override-previous-sha256", "", "owner-approved previous digest for a UF loss")
	actor := flags.String("override-actor", "", "owner identity for the exception audit")
	reason := flags.String("override-reason", "", "owner reason for the exception audit")
	timeout := flags.Duration("timeout", 5*time.Minute, "activation deadline")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *bundle == "" || *geography == "" {
		return errors.New("expected --bundle DIR --geography FILE and the four trusted SHA-256 pins")
	}
	hasOverride := *previous != "" || *actor != "" || *reason != ""
	if (!*activate && (*approved != "" || hasOverride)) ||
		(hasOverride && (*previous == "" || *actor == "" || *reason == "")) {
		return errors.New("activation approval and exception flags must be complete")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return errors.New("activation timeout must be positive and at most 10 minutes")
	}
	root, err := os.OpenRoot(*bundle)
	if err != nil {
		return errors.New("rights bundle unavailable")
	}
	defer root.Close()
	input.Files = root.FS()
	input.Geography, err = readGeography(*geography)
	if err != nil {
		return err
	}
	plan, err := prepared.PlanNationalImport(input)
	if err != nil {
		return err
	}
	if !*activate {
		return json.NewEncoder(output).Encode(plan)
	}
	if *approved != plan.BatchSHA256 {
		return errors.New("activation requires the exact verified batch digest")
	}
	env := getenv("ANPFUEL_ENV")
	switch env {
	case "development", "test":
	case "staging":
		if getenv("ANPFUEL_CANONICAL_HOST") != "teste.abastevo.com.br" {
			return errors.New("staging activation requires the staging canonical host")
		}
	default:
		return errors.New("activation requires an explicit local or staging environment")
	}
	dsn := getenv("ANPFUEL_DATABASE_URL")
	if dsn == "" {
		return errors.New("database configuration required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !allowedDatabase(env, cfg) {
		return errors.New("database target refused")
	}
	if hasOverride {
		input.Override = &prepared.CatalogCoverageOverride{
			PreviousSHA256: *previous, CandidateSHA256: *approved, Actor: *actor, Reason: *reason,
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("database pool unavailable")
	}
	defer pool.Close()
	id, err := prepared.ImportNationalCatalog(ctx, pool, input)
	if err != nil {
		return errors.New("national activation refused; no successful transaction was reported")
	}
	return json.NewEncoder(output).Encode(struct {
		prepared.NationalImportPlan
		BatchID string `json:"batch_id"`
	}{plan, id})
}

func allowedDatabase(env string, cfg *pgxpool.Config) bool {
	allowedHost := func(host string) bool {
		if env == "staging" {
			return host == "abastevo-temp-db" || host == "abastevo-temp-db.abastevo-temp.svc" ||
				host == "abastevo-temp-db.abastevo-temp.svc.cluster.local"
		}
		return host == "localhost" || net.ParseIP(host).IsLoopback()
	}
	if !allowedHost(cfg.ConnConfig.Host) {
		return false
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if !allowedHost(fallback.Host) {
			return false
		}
	}
	return true
}

func readGeography(path string) ([]byte, error) {
	const limit = 64 << 20
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("geography file unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("geography file refused")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(raw) > limit {
		return nil, errors.New("geography file refused")
	}
	return raw, nil
}
