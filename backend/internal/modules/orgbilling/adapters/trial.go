package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTrialIneligible  = errors.New("orgbilling: trial is not eligible")
	ErrTrialConflict    = errors.New("orgbilling: trial or subscription already exists")
	ErrTrialUnavailable = errors.New("orgbilling: trial processing unavailable")
)

func TrialAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "billing.trial.started", TargetType: "subscription"}}
}

type TrialStore struct {
	Pool   *pgxpool.Pool
	Ledger SubscriptionLedger
	Audit  auditing.TransactionRunner
}

type TrialResult struct {
	SubscriptionID string
	Period         domain.Period
	Replayed       bool
}

// Start opens an already-reviewed grant through the restricted operator path.
// The grant's identifier also names the subscription, making the committed
// opening term permanent for both authorization and replay.
func (store TrialStore) Start(ctx context.Context, scope kernel.OrgScope, grantID, operator, operationID string) (TrialResult, error) {
	if store.Pool == nil || store.Audit == nil {
		return TrialResult{}, ErrTrialUnavailable
	}
	grant, err := kernel.ParseOrgID(grantID)
	if err != nil || grant.String() != grantID || scope.Validate() != nil ||
		!audit.ValidID(operator) || !audit.ValidID(operationID) {
		return TrialResult{}, ErrTrialIneligible
	}

	digest := sha256.Sum256([]byte(scope.OrgID().String() + ":" + grantID))
	event := audit.Event{
		OperationID: operationID, ActorID: operator, Action: "billing.trial.started",
		TargetType: "subscription", TargetID: grantID, RequestHash: hex.EncodeToString(digest[:]),
	}
	var result TrialResult
	err = database.WithOrgTx(ctx, store.Pool, scope, func(tx pgx.Tx) error {
		queries := db.New(tx)
		key := db.GetTrialGrantParams{OrgID: scope.OrgID().String(), GrantID: grantID}
		offer, err := queries.GetTrialGrant(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrialIneligible
		}
		if err != nil {
			return err
		}
		replayed, err := store.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			return store.startTx(ctx, tx, scope, key, offer)
		})
		if err != nil {
			return err
		}
		receipt, err := queries.GetTrialStart(ctx, db.GetTrialStartParams(key))
		if err != nil || receipt.EvidenceReference != offer.ApprovalRef {
			return ErrTrialUnavailable
		}
		result = TrialResult{SubscriptionID: grantID, Period: domain.Period{
			Start: receipt.TermStart.Time.UTC(), End: receipt.TermEnd.Time.UTC(),
		}, Replayed: replayed}
		return nil
	})
	if err != nil {
		return TrialResult{}, err
	}
	return result, nil
}

func (store TrialStore) startTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, grant db.GetTrialGrantParams, offer db.GetTrialGrantRow) error {
	if err := qcap.New(tx).LockEntitlementWrite(ctx, grant.OrgID); err != nil {
		return err
	}
	queries := db.New(tx)
	state, err := queries.LockTrialOrganization(ctx, grant.OrgID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && state != string(organization.Active) {
		return ErrTrialIneligible
	}
	if err != nil {
		return err
	}
	if _, err := queries.GetTrialStart(ctx, db.GetTrialStartParams(grant)); err == nil {
		return ErrTrialConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	alreadyStarted, err := queries.NonterminalSubscriptionForProduct(ctx, db.NonterminalSubscriptionForProductParams{
		OrgID: grant.OrgID, ProductCode: offer.ProductCode,
	})
	if err != nil {
		return err
	}
	if alreadyStarted {
		return ErrTrialConflict
	}
	instant, err := queries.OrganizationBillingClock(ctx)
	if err != nil {
		return err
	}
	now := instant.Time.UTC()
	if !instant.Valid || now.Before(offer.EligibleFrom.Time) || !now.Before(offer.EligibleUntil.Time) {
		return ErrTrialIneligible
	}

	// Provider billing periods are whole seconds; a paid plan change must start
	// exactly at the trial end, so the trial term uses the same precision.
	start := now.Truncate(time.Second)
	_, err = store.Ledger.ApplyTx(ctx, tx, domain.PlanKey{Product: offer.ProductCode, Plan: offer.PlanCode, Revision: offer.PlanRevision},
		domain.LifecycleEvent{Scope: scope, SubscriptionID: grant.GrantID, Kind: domain.StartTrial,
			Source: domain.TrialPolicy, Reference: offer.ApprovalRef, At: now,
			Period: domain.Period{Start: start, End: start.Add(time.Duration(offer.DurationSeconds) * time.Second)},
		})
	return err
}
