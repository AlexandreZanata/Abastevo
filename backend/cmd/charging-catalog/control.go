package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runControl(args []string, getenv func(string) string, output io.Writer) error {
	flags := flag.NewFlagSet("charging-catalog control", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var control prepared.CatalogControl
	flags.StringVar(&control.Action, "action", "", "hold, resume or rollback")
	flags.StringVar(&control.ExpectedSHA256, "expected-sha256", "", "active digest or none")
	flags.StringVar(&control.RestoreSHA256, "restore-sha256", "", "approved predecessor digest or none for rollback")
	flags.StringVar(&control.Actor, "actor", "", "owner identity for the audit")
	flags.StringVar(&control.Reason, "reason", "", "owner reason for the audit")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("invalid catalog control flags")
	}
	if err := control.Validate(); err != nil {
		return err
	}
	cfg, err := ownerDatabaseConfig(getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("database pool unavailable")
	}
	defer pool.Close()
	if err := prepared.ControlCatalog(ctx, pool, control); err != nil {
		return errors.New("catalog control refused or unconfirmed; verify the active digest and control audit")
	}
	return json.NewEncoder(output).Encode(map[string]string{
		"action": control.Action, "expected_sha256": control.ExpectedSHA256, "restore_sha256": control.RestoreSHA256,
	})
}
