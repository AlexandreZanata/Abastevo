// Command portalrelease promotes a verified portal candidate into a private
// release root with an atomic current symlink, rollback and withdrawal.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/portalgen"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: portalrelease <promote|rollback|withdraw|verify> [flags]")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "promote":
		promote(os.Args[2:])
	case "rollback":
		rollback(os.Args[2:])
	case "withdraw":
		withdraw(os.Args[2:])
	case "retain":
		retain(os.Args[2:])
	case "verify":
		verify(os.Args[2:])
	default:
		usage()
	}
}

func promote(args []string) {
	fs := flag.NewFlagSet("promote", flag.ExitOnError)
	var artifact, root, siteURL, expected string
	var fence, officialOnly bool
	var retainKeep int
	var retainMaxBytes int64
	fs.StringVar(&artifact, "artifact", "", "verified portal candidate directory")
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&fence, "fence", false, "require the current release to equal --expect-current")
	fs.StringVar(&expected, "expect-current", "", "expected current content hash")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only candidate")
	fs.IntVar(&retainKeep, "retain-keep", 2, "non-current releases whose assets stay retained")
	fs.Int64Var(&retainMaxBytes, "retain-max-bytes", 100<<20, "retained asset byte budget")
	fs.Parse(args)
	state, err := portalgen.Promote(portalgen.PromoteOptions{
		ArtifactDir: artifact, RootDir: root, SiteURL: siteURL,
		Fence: fence, ExpectedCurrent: expected, OfficialOnly: officialOnly,
		RetainKeep: retainKeep, RetainMaxBytes: retainMaxBytes,
	})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("promoted current=%s previous=%s\n", state.Current, state.Previous)
}

func rollback(args []string) {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	var root, target, siteURL string
	var officialOnly bool
	var retainKeep int
	var retainMaxBytes int64
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&target, "target", "", "release content hash")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only release")
	fs.IntVar(&retainKeep, "retain-keep", 2, "non-current releases whose assets stay retained")
	fs.Int64Var(&retainMaxBytes, "retain-max-bytes", 100<<20, "retained asset byte budget")
	fs.Parse(args)
	state, err := portalgen.Rollback(portalgen.RollbackOptions{RootDir: root, Target: target, SiteURL: siteURL, OfficialOnly: officialOnly, RetainKeep: retainKeep, RetainMaxBytes: retainMaxBytes})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("rolled back current=%s previous=%s\n", state.Current, state.Previous)
}

func withdraw(args []string) {
	fs := flag.NewFlagSet("withdraw", flag.ExitOnError)
	var root, fallback, siteURL, reason string
	var officialOnly bool
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&fallback, "fallback", "", "optional safe release content hash")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only release")
	fs.StringVar(&reason, "reason", "", "operator-pause (default) or revoked-source")
	fs.Parse(args)
	state, err := portalgen.Withdraw(portalgen.WithdrawOptions{RootDir: root, Fallback: fallback, SiteURL: siteURL, OfficialOnly: officialOnly, Reason: reason})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("withdrawn current=%s previous=%s\n", state.Current, state.Previous)
}

func verify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	var root, siteURL string
	var officialOnly bool
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only release")
	fs.Parse(args)
	release, _, err := portalgen.CurrentRelease(root, siteURL, officialOnly)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("current=%s pages=%d files=%d bytes=%d promoted_at=%s\n",
		release.ContentSHA256, release.Pages, release.Files, release.Bytes, release.PromotedAt)
}

func retain(args []string) {
	fs := flag.NewFlagSet("retain", flag.ExitOnError)
	var root string
	var keep int
	var maxBytes int64
	fs.StringVar(&root, "root", "", "private release root")
	fs.IntVar(&keep, "keep", 2, "newest non-withdrawn releases whose assets are retained")
	fs.Int64Var(&maxBytes, "max-bytes", 100<<20, "retained asset byte budget")
	fs.Parse(args)
	summary, err := portalgen.RetainAssets(root, keep, maxBytes)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("retained releases=%d files=%d bytes=%d\n", summary.Releases, summary.Files, summary.Bytes)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "portalrelease:", err)
	os.Exit(1)
}
