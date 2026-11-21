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
)

var errAdjustmentUsage = errors.New("usage: ops enterprise adjustment post --org <uuid> --approval-id <uuid> --operation-id <uuid> [--operator <uuid>]")

type reviewedBillingArgs struct {
	scope                         kernel.OrgScope
	approval, operation, operator string
}

func parseAdjustmentArgs(args []string, getenv func(string) string) (reviewedBillingArgs, error) {
	return parseReviewedBillingArgs(args, getenv, "enterprise adjustment post", "post", errAdjustmentUsage)
}

func parseReviewedBillingArgs(args []string, getenv func(string) string, command, verb string, usage error) (reviewedBillingArgs, error) {
	if len(args) == 0 || args[0] != verb {
		return reviewedBillingArgs{}, usage
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	org := flags.String("org", "", "organization UUID")
	approval := flags.String("approval-id", "", "reviewed billing approval UUID")
	operation := flags.String("operation-id", "", "operation UUID, reused for a matching retry")
	operator := flags.String("operator", "", "accountable operator UUID")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return reviewedBillingArgs{}, usage
	}
	who, err := resolveOperatorID(*operator, getenv)
	if err != nil {
		return reviewedBillingArgs{}, err
	}
	id, ok := canonicalUUID(*org)
	if !ok || !audit.ValidID(*approval) || !audit.ValidID(*operation) || !audit.ValidID(who) {
		return reviewedBillingArgs{}, usage
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return reviewedBillingArgs{}, usage
	}
	return reviewedBillingArgs{scope: scope, approval: *approval, operation: *operation, operator: who}, nil
}

func runEnterpriseAdjustment(args []string, getenv func(string) string, out io.Writer) error {
	in, err := parseAdjustmentArgs(args, getenv)
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
	catalog, err := audit.NewCatalog(billing.AdjustmentAuditDefinitions())
	if err != nil {
		return err
	}
	raw := pool.Underlying()
	store := billing.AdjustmentStore{Pool: raw, Audit: auditpg.NewStore(raw, catalog)}
	receipt, err := store.Apply(ctx, in.scope, in.approval, in.operator, in.operation)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errUnsafeLogin
	case errors.Is(err, billing.ErrAdjustmentUnavailable), errors.Is(err, billing.ErrAdjustmentPosted), errors.Is(err, audit.ErrReused):
		return err
	case err != nil:
		return errors.New("enterprise billing adjustment unavailable")
	}
	_, err = fmt.Fprintf(out, "organization %s subscription %s revision=%d adjustment=%s delta_centavos=%d posted=%s replayed=%t\n",
		in.scope.OrgID().String(), receipt.SubscriptionID, receipt.SubscriptionRevision, receipt.Kind, receipt.DeltaCentavos,
		receipt.PostedAt.Format(time.RFC3339Nano), receipt.Replayed)
	return err
}
