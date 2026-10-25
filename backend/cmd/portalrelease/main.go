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
	fs.StringVar(&artifact, "artifact", "", "verified portal candidate directory")
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&fence, "fence", false, "require the current release to equal --expect-current")
	fs.StringVar(&expected, "expect-current", "", "expected current content hash")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only candidate")
	fs.Parse(args)
	state, err := portalgen.Promote(portalgen.PromoteOptions{
		ArtifactDir: artifact, RootDir: root, SiteURL: siteURL,
		Fence: fence, ExpectedCurrent: expected, OfficialOnly: officialOnly,
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
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&target, "target", "", "release content hash")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only release")
	fs.Parse(args)
	state, err := portalgen.Rollback(portalgen.RollbackOptions{RootDir: root, Target: target, SiteURL: siteURL, OfficialOnly: officialOnly})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("rolled back current=%s previous=%s\n", state.Current, state.Previous)
}

func withdraw(args []string) {
	fs := flag.NewFlagSet("withdraw", flag.ExitOnError)
	var root, fallback, siteURL string
	var officialOnly bool
	fs.StringVar(&root, "root", "", "private release root")
	fs.StringVar(&fallback, "fallback", "", "optional safe release content hash")
	fs.StringVar(&siteURL, "site-url", "", "origin used by the artifact manifest")
	fs.BoolVar(&officialOnly, "official-only", false, "require an official-only release")
	fs.Parse(args)
	state, err := portalgen.Withdraw(portalgen.WithdrawOptions{RootDir: root, Fallback: fallback, SiteURL: siteURL, OfficialOnly: officialOnly})
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

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "portalrelease:", err)
	os.Exit(1)
}
