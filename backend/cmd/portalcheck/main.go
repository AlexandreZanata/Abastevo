// Command portalcheck verifies a complete static portal candidate against its
// manifest, public-field allowlists, compression/file budgets and sitemap.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
)

func main() {
	var artifact, compare, siteURL string
	var maxFiles int
	var maxBytes, maxHTML, maxScript, maxShard int64
	var officialOnly bool
	flag.StringVar(&artifact, "artifact", "", "portal candidate directory")
	flag.StringVar(&compare, "compare", "", "second candidate for reproducible comparison")
	flag.StringVar(&siteURL, "site-url", "", "expected public origin (optional)")
	flag.IntVar(&maxFiles, "max-files", 0, "maximum files (default pages + 1000)")
	flag.Int64Var(&maxBytes, "max-bytes", 0, "maximum total bytes (default 2 GiB)")
	flag.Int64Var(&maxHTML, "max-html-gzip", 0, "maximum gzip bytes per HTML page (default 100 KiB)")
	flag.Int64Var(&maxScript, "max-script-gzip", 0, "maximum gzip bytes per script (default 100 KiB)")
	flag.Int64Var(&maxShard, "max-shard-gzip", 0, "maximum gzip bytes per search shard (default 250 KiB)")
	flag.BoolVar(&officialOnly, "official-only", false, "require an official-only artifact (no price/station/search/download routes)")
	flag.Parse()
	if artifact == "" {
		fmt.Fprintln(os.Stderr, "portalcheck: --artifact is required")
		os.Exit(2)
	}
	result, err := portalgen.Verify(artifact, portalgen.VerifyOptions{
		SiteURL: siteURL, CompareDir: compare, MaxFiles: maxFiles, MaxBytes: maxBytes,
		MaxHTMLGzipBytes: maxHTML, MaxScriptGzipBytes: maxScript, MaxShardGzipBytes: maxShard,
		OfficialOnly: officialOnly,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "portalcheck:", err)
		os.Exit(1)
	}
	fmt.Printf("portalcheck: %d pages, %d files, %d bytes, %s, sitemap=%d, gzip(html=%d js=%d css=%d shard=%d initial=%d)\n",
		result.Pages, result.Files, result.Bytes, result.ContentSHA256,
		result.SitemapURLs, result.LargestHTMLGzipBytes, result.LargestScriptGzipBytes,
		result.LargestCSSGzipBytes, result.LargestShardGzipBytes, result.InitialViewGzipBytes)
}
