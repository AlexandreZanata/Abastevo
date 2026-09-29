// Command ops is the restricted operator tool (P07-T02, BUC-006).
//
// It invokes audited moderation application commands over controlled
// operator access: there is no public admin HTTP path, every mutation
// appends actor/reason/policy audit history, and raw business facts are
// never edited. Operator identity arrives from the ANPFUEL_OPERATOR_ID
// environment or an explicit --operator flag; anonymous invocations are
// denied before any work. See docs/operator/access.md (access runbook).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel ops:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "moderation":
		return runModeration(args[1:], getenv)
	case "privacy":
		return runPrivacy(args[1:], getenv)
	case "monitoring":
		return runMonitoring(args[1:], getenv)
	case "evidence-url":
		return runEvidenceURL(args[1:], getenv)
	case "-h", "-help", "--help", "help":
		return errors.New(usage)
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage)
	}
}

const usage = `usage:
  ops moderation act --case <id> --action REVIEW|INVALIDATE|BLOCK|RESOLVE|DISMISS --reason <text> [--operator <id>]
  ops moderation invalidate --case <id> --reason <text> [--operator <id>]
  ops moderation block --case <id> --reason <text> [--operator <id>]
  ops privacy erase --contributor <id> --reason <text> [--client-key <k>] [--operator <id>]
  ops privacy replay --contributor <id> [--operator <id>]
  ops monitoring eval [--webhook <url>] [--backup-manifest <path>]
  ops evidence-url --case <id> --evidence-id <id> [--operator <id>]

operator identity: --operator flag or ANPFUEL_OPERATOR_ID environment.
no public admin path exists; every command appends audit history`

// resolveOperatorID derives the accountable operator identity from
// restricted access context: explicit flag first, environment second.
// Empty identities are denied (no anonymous admin path).
func resolveOperatorID(flagVal string, getenv func(string) string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	if env := getenv("ANPFUEL_OPERATOR_ID"); env != "" {
		return env, nil
	}
	return "", errors.New("operator identity required: pass --operator or set ANPFUEL_OPERATOR_ID (restricted access)")
}

// openPool loads startup config once and opens the bounded pool shared
// by the command's stores. Secrets never print, even on failure.
func openPool(ctx context.Context) (*database.Pool, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, err
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return nil, config.Config{}, err
	}
	return pool, cfg, nil
}

// newUUID mints v4 identifiers for server-owned audit facts.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}
