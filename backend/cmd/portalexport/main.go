// Command portalexport builds a verified static portal candidate from the
// published FQ-07 official lease. It performs no database writes, source
// access, promotion or publication: promote the verified candidate with
// portalrelease under an explicit operational scope.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	var cfg Config
	var dsn string
	var indexable, officialOnly bool
	var siteURL, snapshot, out string
	flag.StringVar(&dsn, "dsn", os.Getenv("ANPFUEL_DATABASE_URL"), "postgres DSN (or ANPFUEL_DATABASE_URL)")
	flag.StringVar(&snapshot, "snapshot", "", "new private export snapshot directory")
	flag.StringVar(&out, "out", "", "new private portal candidate directory")
	flag.BoolVar(&officialOnly, "official-only", true, "emit only the admitted official-records slice")
	flag.BoolVar(&indexable, "indexable", false, "allow indexing (requires --site-url)")
	flag.StringVar(&siteURL, "site-url", "", "origin for sitemap/robots (required with --indexable)")
	flag.Parse()
	cfg = Config{
		DSN: dsn, SnapshotDir: snapshot, PortalOut: out,
		OfficialOnly: officialOnly, Indexable: indexable, SiteURL: siteURL,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()
	result, err := RunExport(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "portalexport:", err)
		os.Exit(1)
	}
	fmt.Printf("portalexport: official edition=%s rows=%d linked=%d unmatched=%d ambiguous=%d portal=%d pages/%d files/%d bytes %s\n",
		result.Edition, result.Rows, result.Linked, result.Unmatched, result.Ambiguous,
		result.Pages, result.Files, result.Bytes, result.ContentSHA256)
}
