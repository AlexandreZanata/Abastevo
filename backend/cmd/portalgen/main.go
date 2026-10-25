// Command portalgen writes a private static portal candidate from one frozen
// enterprise export. It performs no database, API or source access.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
)

func main() {
	var snapshot, out string
	var maxPages int
	var maxBytes int64
	var indexable bool
	var searchJS, siteURL string
	var officialOnly bool
	flag.StringVar(&snapshot, "snapshot", "", "frozen export candidate directory")
	flag.StringVar(&out, "out", "", "new private portal output directory")
	flag.IntVar(&maxPages, "max-pages", 0, "maximum HTML pages (default 50000)")
	flag.Int64Var(&maxBytes, "max-bytes", 0, "maximum output bytes (default 2 GiB)")
	flag.BoolVar(&indexable, "indexable", false, "allow indexing (candidate builds default to noindex)")
	flag.StringVar(&siteURL, "site-url", "", "public origin for sitemap/robots (required with --indexable)")
	flag.StringVar(&searchJS, "search-js", "", "compiled portal search JavaScript (optional)")
	flag.BoolVar(&officialOnly, "official-only", false, "emit only the admitted official-records slice (no price/station/search/download routes)")
	flag.Parse()
	if snapshot == "" || out == "" {
		fmt.Fprintln(os.Stderr, "portalgen: --snapshot and --out are required")
		os.Exit(2)
	}
	result, err := portalgen.Generate(portalgen.Options{
		SnapshotDir: snapshot, OutputDir: out,
		MaxPages: maxPages, MaxBytes: maxBytes, Indexable: indexable,
		SiteURL: siteURL, SearchScriptPath: searchJS, OfficialOnly: officialOnly,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "portalgen:", err)
		os.Exit(1)
	}
	fmt.Printf("portal: %d pages, %d files, %d bytes, %s\n",
		result.Pages, result.Files, result.Bytes, result.ContentSHA256)
}
