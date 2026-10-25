// Command map-load publishes one validated public map-overlay envelope into
// PostGIS. It runs no migrations and never publishes a partial generation.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/mapdata"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "map-load:", err)
		os.Exit(1)
	}
}

func run() error {
	envelopePath := flag.String("envelope", "", "prepared overlay envelope JSON")
	assertionsPath := flag.String("assertions", "", "validated assertion corpus JSON")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	timeout := flag.Duration("timeout", 2*time.Minute, "overall deadline")
	flag.Parse()
	if *envelopePath == "" || *assertionsPath == "" || *dsn == "" {
		return fmt.Errorf("usage: map-load --envelope FILE --assertions FILE [--dsn URL]")
	}
	if *timeout <= 0 || *timeout > 10*time.Minute {
		return fmt.Errorf("timeout must be positive and at most 10 minutes")
	}
	envelope, err := readBounded(*envelopePath, 1<<20)
	if err != nil {
		return err
	}
	assertions, err := readBounded(*assertionsPath, 64<<20)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	config, err := pgxpool.ParseConfig(*dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
	generation, err := mapdata.NewStore(pool).Publish(ctx, envelope, assertions)
	if err != nil {
		return err
	}
	fmt.Printf("generation=%s status=%s source_revision=%s\n", generation.ID, generation.Status, generation.SourceRevision)
	return nil
}

func readBounded(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, max)
	}
	return data, nil
}
