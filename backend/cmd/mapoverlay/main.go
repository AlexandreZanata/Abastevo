// Command mapoverlay builds the immutable per-UF static map overlay from the
// active map generation and the latest published official week. It is
// read-only against PostgreSQL and writes files under --output.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/mapdata"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	output := flag.String("output", "", "static overlay root (required)")
	dsn := flag.String("dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "mapoverlay: --output is required")
		os.Exit(2)
	}
	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "mapoverlay: database DSN is required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mapoverlay: database pool")
		os.Exit(1)
	}
	defer pool.Close()
	result, err := mapdata.BuildStaticOverlay(ctx, pool, mapdata.StaticOverlayOptions{OutputRoot: *output})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mapoverlay:", err)
		os.Exit(1)
	}
	fmt.Printf("release=%s generation=%s assertions=%d ufs=%d with_prices=%d noop=%t\n",
		result.Release, result.Generation, result.Counts.Assertions, result.Counts.UFs,
		result.Counts.WithPrices, result.Noop)
}
