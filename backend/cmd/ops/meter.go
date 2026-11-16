package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// meterArgs carries one parsed projection rebuild (EMP-METER-006, EDR-317).
// Parsing stays pure (no I/O) so validation is unit-testable.
type meterArgs struct {
	operator string
	org      kernel.OrgScope
}

func parseMeterArgs(args []string, getenv func(string) string) (meterArgs, error) {
	if len(args) == 0 || args[0] != "rebuild-projection" {
		return meterArgs{}, errors.New("usage: ops meter rebuild-projection --org <uuid> [--operator <id>]")
	}
	fs := flag.NewFlagSet("meter rebuild-projection", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var operator, org string
	fs.StringVar(&operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&org, "org", "", "organization UUID")
	if err := fs.Parse(args[1:]); err != nil {
		return meterArgs{}, err
	}
	if fs.NArg() != 0 {
		return meterArgs{}, errors.New("meter rebuild-projection takes no positional arguments")
	}
	id, err := kernel.ParseOrgID(strings.TrimSpace(org))
	if err != nil || id.String() != org {
		return meterArgs{}, errors.New("meter rebuild-projection requires a canonical --org UUID")
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return meterArgs{}, err
	}
	who, err := resolveOperatorID(operator, getenv)
	if err != nil {
		return meterArgs{}, err
	}
	return meterArgs{operator: who, org: scope}, nil
}

// runMeter rebuilds one organization's current-value projection; it changes
// no business fact and prints only the meter count.
func runMeter(args []string, getenv func(string) string) error {
	parsed, err := parseMeterArgs(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	n, err := fleetpg.RebuildMeterProjection(ctx, pool.Underlying(), parsed.org)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "meter projection rebuilt: organization %s, %d meters\n", parsed.org.OrgID(), n)
	return err
}
