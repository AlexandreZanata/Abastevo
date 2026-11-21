package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

var errStationPlanUsage = errors.New("usage: ops enterprise station admit --org <uuid> --approval-id <uuid> --operation-id <uuid> [--operator <uuid>]")

func runEnterpriseStation(args []string, getenv func(string) string, out io.Writer) error {
	in, err := parseReviewedBillingArgs(args, getenv, "enterprise station admit", "admit", errStationPlanUsage)
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
	caps, err := capabilityCatalog()
	if err != nil {
		return err
	}
	catalog, err := audit.NewCatalog(billing.StationPlanAuditDefinitions())
	if err != nil {
		return err
	}
	raw := pool.Underlying()
	audits := auditpg.NewStore(raw, catalog)
	plans := billing.EntitlementStore{Pool: raw, Capabilities: caps, Now: time.Now}
	flags := cappg.NewStore(raw, caps, audits)
	flags.PlanWrites = plans
	store := billing.StationMeter{Pool: raw, Plans: plans, Flags: flags, Audit: audits}
	receipt, err := store.Admit(ctx, in.scope, in.approval, in.operator, in.operation)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errUnsafeLogin
	case errors.Is(err, billing.ErrStationApprovalUnavailable), errors.Is(err, billing.ErrStationAdmissionExists),
		errors.Is(err, access.ErrEntitlementRequired), errors.Is(err, access.ErrLimitReached), errors.Is(err, audit.ErrReused):
		return err
	case err != nil:
		return errors.New("enterprise station admission unavailable")
	}
	_, err = fmt.Fprintf(out, "organization %s station %s review=%d admitted=%s replayed=%t\n",
		in.scope.OrgID().String(), receipt.StationID, receipt.ReviewRevision, receipt.AdmittedAt.Format(time.RFC3339Nano), receipt.Replayed)
	return err
}
