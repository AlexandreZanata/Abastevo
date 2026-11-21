package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errTrialUsage = errors.New("usage: ops enterprise trial start --org <uuid> --grant-id <uuid> --operation-id <uuid> [--operator <uuid>]")

type trialArgs struct {
	scope                        kernel.OrgScope
	grant, operationID, operator string
}

func parseTrialArgs(args []string, getenv func(string) string) (trialArgs, error) {
	if len(args) == 0 || args[0] != "start" {
		return trialArgs{}, errTrialUsage
	}
	flags := flag.NewFlagSet("enterprise trial start", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	rawOrg := flags.String("org", "", "organization UUID")
	grant := flags.String("grant-id", "", "reviewed trial grant UUID")
	operation := flags.String("operation-id", "", "operation UUID, reused only to retry")
	operator := flags.String("operator", "", "accountable operator UUID")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return trialArgs{}, errTrialUsage
	}
	who, err := resolveOperatorID(*operator, getenv)
	if err != nil {
		return trialArgs{}, err
	}
	id, ok := canonicalUUID(*rawOrg)
	if !ok || !audit.ValidID(*grant) || !audit.ValidID(*operation) || !audit.ValidID(who) {
		return trialArgs{}, errTrialUsage
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return trialArgs{}, errTrialUsage
	}
	return trialArgs{scope: scope, grant: *grant, operationID: *operation, operator: who}, nil
}

func executeTrial(ctx context.Context, pool *pgxpool.Pool, in trialArgs) (billing.TrialResult, error) {
	capabilities, err := capabilityCatalog()
	if err != nil {
		return billing.TrialResult{}, err
	}
	catalog, err := audit.NewCatalog(billing.TrialAuditDefinitions())
	if err != nil {
		return billing.TrialResult{}, err
	}
	store := billing.TrialStore{
		Pool: pool, Ledger: billing.SubscriptionLedger{Capabilities: capabilities},
		Audit: auditpg.NewStore(pool, catalog),
	}
	return store.Start(ctx, in.scope, in.grant, in.operator, in.operationID)
}

func runEnterpriseTrial(args []string, getenv func(string) string, out io.Writer) error {
	in, err := parseTrialArgs(args, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, cfg, err := openPool(ctx)
	if err != nil {
		return errors.New("enterprise database unavailable")
	}
	defer pool.Close()
	if !cfg.EnterpriseEnabled || !cfg.EnterpriseBillingEnabled {
		return errors.New("enterprise billing is disabled")
	}
	result, err := executeTrial(ctx, pool.Underlying(), in)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errUnsafeLogin
	case errors.Is(err, billing.ErrTrialIneligible), errors.Is(err, billing.ErrTrialConflict), errors.Is(err, audit.ErrReused):
		return err
	case err != nil:
		return errors.New("enterprise trial unavailable")
	}
	_, err = fmt.Fprintf(out, "organization %s trial %s starts=%s ends=%s replayed=%t\n",
		in.scope.OrgID().String(), result.SubscriptionID,
		result.Period.Start.Format(time.RFC3339Nano), result.Period.End.Format(time.RFC3339Nano), result.Replayed)
	return err
}
