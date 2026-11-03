package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	"io"
	"os"
	"strings"
	"time"
)

type directoryFactArgs struct{ campaign, operator, reason string }

func parseDirectoryFactArgs(args []string, mutation bool) (directoryFactArgs, error) {
	fs := flag.NewFlagSet("directory facts", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var a directoryFactArgs
	fs.StringVar(&a.campaign, "campaign", "", "immutable campaign UUID")
	fs.StringVar(&a.operator, "operator", "", "accountable restricted operator")
	fs.StringVar(&a.reason, "reason", "", "reason for an explicit re-projection request")
	if err := fs.Parse(args); err != nil {
		return a, err
	}
	if fs.NArg() != 0 || strings.TrimSpace(a.campaign) == "" {
		return a, errors.New("directory facts requires --campaign UUID and no positional arguments")
	}
	if mutation && (strings.TrimSpace(a.reason) == "" || len(a.reason) > 500) {
		return a, errors.New("directory reproject-facts requires a bounded --reason")
	}
	return a, nil
}
func runDirectory(args []string, getenv func(string) string) error {
	if len(args) == 0 || (args[0] != "reproject-facts" && args[0] != "facts-status") {
		return errors.New("usage: ops directory reproject-facts|facts-status --campaign UUID [--reason TEXT] [--operator ID]")
	}
	mutation := args[0] == "reproject-facts"
	a, err := parseDirectoryFactArgs(args[1:], mutation)
	if err != nil {
		return err
	}
	operator, err := resolveOperatorID(a.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	if mutation {
		result, err := directory.StartStationFactReprojection(ctx, pool.Underlying(), a.campaign, operator, a.reason)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "campaign=%s job=%s created=%t\n", a.campaign, result.ID, result.Created)
		return err
	}
	status, err := directory.ReadStationFactReprojectionStatus(ctx, pool.Underlying(), a.campaign)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}
