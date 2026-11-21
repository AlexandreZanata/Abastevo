package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	sqlbilling "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	queries "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbillingops"
	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billingpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const subscriptionSuspendedAction = "subs.billing.suspended"

var (
	errBillingSuspendUsage          = errors.New("usage: ops enterprise billing suspend --org <uuid> --approval-id <uuid> --operation-id <uuid> [--operator <uuid>]")
	errBillingSuspensionUnavailable = errors.New("enterprise billing suspension review unavailable")
)

func runEnterpriseBillingSuspension(args []string, getenv func(string) string, out io.Writer) error {
	in, err := parseReviewedBillingArgs(args, getenv, "enterprise billing suspend", "suspend", errBillingSuspendUsage)
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
	revision, replayed, err := executeBillingSuspension(ctx, pool.Underlying(), in)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errUnsafeLogin
	case errors.Is(err, errBillingSuspensionUnavailable), errors.Is(err, audit.ErrReused):
		return err
	case err != nil:
		return errors.New("enterprise billing suspension unavailable")
	}
	_, err = fmt.Fprintf(out, "subscription suspended revision=%d replayed=%t\n", revision, replayed)
	return err
}

func executeBillingSuspension(ctx context.Context, pool *pgxpool.Pool, in reviewedBillingArgs) (int64, bool, error) {
	catalog, err := audit.NewCatalog([]audit.Definition{{Action: subscriptionSuspendedAction, TargetType: "subscription"}})
	if err != nil {
		return 0, false, err
	}
	store := auditpg.NewStore(pool, catalog)
	org := in.scope.OrgID().String()
	var revision int64
	var replayed bool
	err = database.WithOrgTx(ctx, pool, in.scope, func(tx pgx.Tx) error {
		q := queries.New(tx)
		approval, err := q.ReadSuspensionApproval(ctx, queries.ReadSuspensionApprovalParams{
			OrgID: org, ApprovalID: in.approval, OperatorID: in.operator,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errBillingSuspensionUnavailable
		}
		if err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(subscriptionSuspendedAction + "/v1/" + org + "/" + in.approval))
		event := audit.Event{OperationID: in.operation, ActorID: in.operator, Action: subscriptionSuspendedAction,
			TargetType: "subscription", TargetID: approval.SubscriptionID, RequestHash: hex.EncodeToString(hash[:])}
		replayed, err = store.RecordChangeTx(ctx, tx, in.scope, event, func(ctx context.Context, tx pgx.Tx) error {
			if err := qcap.New(tx).LockEntitlementWrite(ctx, org); err != nil {
				return err
			}
			b := sqlbilling.New(tx)
			current, err := b.GetSubscription(ctx, sqlbilling.GetSubscriptionParams{OrgID: org, SubscriptionID: approval.SubscriptionID})
			if err != nil {
				return err
			}
			now, err := b.OrganizationBillingClock(ctx)
			if err != nil {
				return err
			}
			if !now.Valid || current.Revision != approval.ExpectedRevision || current.State != string(billing.PastDue) ||
				now.Time.Before(approval.ValidFrom.Time) || !now.Time.Before(approval.ValidUntil.Time) {
				return errBillingSuspensionUnavailable
			}
			_, err = (billingpg.SubscriptionLedger{}).ApplyTx(ctx, tx, billing.PlanKey{
				Product: current.ProductCode, Plan: current.PlanCode, Revision: current.PlanRevision,
			}, billing.LifecycleEvent{Scope: in.scope, SubscriptionID: approval.SubscriptionID, ExpectedRevision: current.Revision,
				Kind: billing.SuspendAccess, Source: billing.ReviewedOperator, Reference: in.approval, At: now.Time.UTC()})
			return err
		})
		if err != nil {
			return err
		}
		revision = approval.ExpectedRevision + 1
		hasProof, err := q.HasReviewedSuspension(ctx, queries.HasReviewedSuspensionParams{
			OrgID: org, SubscriptionID: approval.SubscriptionID, Revision: revision, ApprovalID: in.approval,
		})
		if err != nil {
			return err
		}
		if !hasProof {
			return errBillingSuspensionUnavailable
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return revision, replayed, nil
}
