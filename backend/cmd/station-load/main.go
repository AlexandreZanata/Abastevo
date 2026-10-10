// Command station-load stages one prepared station batch (manifest
// plus assertions/candidates/quarantine streams from station-prep)
// into a directory database through the owned loader. It runs no
// migrations and changes no schema: the target database must already
// be migrated. Exit 0 only when every input report reads complete;
// any other outcome exits 1 with the failing input named.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "station-load:", err)
		os.Exit(1)
	}
}

func run() error {
	emitDir := flag.String("emit-dir", "", "bench_stages out dir with manifest.json and the three .jsonl streams")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall deadline")
	flag.Parse()
	if *emitDir == "" || *dsn == "" {
		return fmt.Errorf("usage: station-load --emit-dir DIR [--dsn URL] [--timeout DURATION]")
	}
	read := func(name string) ([]byte, error) {
		raw, err := os.ReadFile(filepath.Join(*emitDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		return raw, nil
	}
	manifest, err := read("manifest.json")
	if err != nil {
		return err
	}
	streams := registry.BatchStreams{}
	for _, file := range []struct {
		name string
		into *[]byte
	}{
		{"assertions.jsonl", &streams.Assertions},
		{"candidates.jsonl", &streams.Candidates},
		{"quarantine.jsonl", &streams.Quarantine},
	} {
		raw, err := read(file.name)
		if err != nil {
			return err
		}
		*file.into = raw
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
	reports, err := registry.LoadBatch(ctx, registry.NewPGStore(pool), manifest, streams)
	if err != nil {
		return err
	}
	failed := false
	for key, report := range reports {
		fmt.Printf("input=%s state=%s accepted=%d duplicates=%d rejected=%d run=%s err=%s\n",
			key, report.State, report.Accepted, report.Duplicates, report.Rejected, report.RunID, report.ErrorCode)
		if report.State != "complete" {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("batch did not complete on every input")
	}
	return nil
}
