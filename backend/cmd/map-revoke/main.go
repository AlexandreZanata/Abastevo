// Command map-revoke records a source-record or whole-generation revocation.
// Revocations are append-only and never delete historical assertions.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/mapdata"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "map-revoke:", err)
		os.Exit(1)
	}
}

func run() error {
	assertion := flag.String("assertion", "", "assertion UUID to revoke")
	generation := flag.String("generation", "", "generation UUID to revoke")
	reason := flag.String("reason", "", "bounded operator reason")
	sourceRef := flag.String("source-ref", "", "source/decision reference")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	timeout := flag.Duration("timeout", 30*time.Second, "overall deadline")
	flag.Parse()
	if *dsn == "" || *reason == "" || *sourceRef == "" || (*assertion == "") == (*generation == "") {
		return fmt.Errorf("usage: map-revoke (--assertion UUID | --generation UUID) --reason TEXT --source-ref TEXT")
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	config, err := pgxpool.ParseConfig(*dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
	store := mapdata.NewStore(pool)
	if *assertion != "" {
		if err := store.RevokeAssertion(ctx, *assertion, *reason, *sourceRef); err != nil {
			return err
		}
		fmt.Printf("assertion=%s revoked\n", *assertion)
		return nil
	}
	if err := store.RevokeGeneration(ctx, *generation, *reason, *sourceRef); err != nil {
		return err
	}
	fmt.Printf("generation=%s revoked\n", *generation)
	return nil
}
