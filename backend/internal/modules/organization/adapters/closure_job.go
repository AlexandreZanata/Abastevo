package adapters

import (
	"context"
	"encoding/json"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const organizationClosureKind = "org.lifecycle.close"

var errOrganizationClosureJob = errors.New("organization closure: scheduled operation unavailable")

type organizationClosureJob struct {
	Version     int    `json:"version"`
	OrgID       string `json:"org_id"`
	OperationID string `json:"operation_id"`
}

type OrganizationClosureWorker struct{ Pool *pgxpool.Pool }

func (OrganizationClosureWorker) Kind() string { return organizationClosureKind }
func (OrganizationClosureWorker) Version() int { return 1 }

func (w OrganizationClosureWorker) Handle(ctx context.Context, job jobs.Job) error {
	var payload organizationClosureJob
	if w.Pool == nil || job.Kind != w.Kind() || job.LeaseToken < 1 || !audit.ValidID(job.ID) ||
		json.Unmarshal(job.Payload, &payload) != nil || payload.Version != w.Version() || !audit.ValidID(payload.OperationID) {
		return errOrganizationClosureJob
	}
	org, err := kernel.ParseOrgID(payload.OrgID)
	if err != nil || org.String() != payload.OrgID {
		return errOrganizationClosureJob
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return errOrganizationClosureJob
	}
	roots := NewStore(w.Pool)
	err = database.WithOrgTx(ctx, w.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		// Take account locks before the organization. Archived organizations
		// refuse new memberships while their closure request is pending.
		if _, err := queries.LockOrganizationClosureAccounts(ctx, payload.OrgID); err != nil {
			return err
		}
		if _, err := queries.LockLegalOrganization(ctx, q.LockLegalOrganizationParams{OrgID: payload.OrgID,
			TermsVersion: d.SyntheticTermsVersion, TermsDigest: d.SyntheticTermsDigest}); err != nil {
			return err
		}
		request, err := queries.FindScheduledOrganizationClosure(ctx, q.FindScheduledOrganizationClosureParams{OrgID: payload.OrgID,
			OperationID: payload.OperationID, JobID: job.ID})
		if err != nil || request.State != "PENDING" {
			return err
		}
		now, err := queries.CurrentOnboardingTime(ctx)
		if err != nil || !now.Valid || !request.ClosesAt.Valid || !d.ClosureDue(now.Time, request.ClosesAt.Time) {
			return errOrganizationClosureJob
		}
		root, err := read(ctx, tx, scope)
		if err != nil {
			return err
		}
		current, err := root.Snapshot(scope)
		if err != nil {
			return err
		}
		next, err := root.Transition(scope, current.Revision, d.Closed, d.Conditions{Closure: d.ClosureElapsed})
		if err != nil {
			return err
		}
		items, err := organizationOpenItems(ctx, tx, payload.OrgID)
		if err != nil || items.Trips > 0 || items.Agreements > 0 {
			return errOrganizationClosureJob
		}
		var holds int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_retention_and_holds WHERE org_id=$1 AND record_kind='LEGAL_HOLD' AND hold_active`,
			payload.OrgID).Scan(&holds); err != nil {
			return err
		}
		count, err := queries.CompleteOrganizationClosure(ctx, q.CompleteOrganizationClosureParams{OrgID: payload.OrgID,
			OperationID: payload.OperationID, JobID: job.ID, LeaseToken: job.LeaseToken, HoldCount: holds})
		if err != nil || count != 1 {
			return errOrganizationClosureJob
		}
		if changed, err := roots.SaveTx(ctx, tx, scope, current.Revision, next); err != nil || !changed {
			return errOrganizationClosureJob
		}
		if err := queries.EndOrganizationMemberships(ctx, payload.OrgID); err != nil {
			return err
		}
		return endOrganizationConnectorAccess(ctx, tx, payload.OrgID)
	})
	if err != nil {
		return errOrganizationClosureJob
	}
	return nil
}

func endOrganizationConnectorAccess(ctx context.Context, tx pgx.Tx, org string) error {
	// Membership reductions already end role assignments, revoke enrolled
	// devices and cancel pending invitations and ownership transfers.
	if _, err := tx.Exec(ctx, `UPDATE org_consolidation_grants SET state='ENDED',revision=revision+1,updated_at=clock_timestamp()
WHERE org_id=$1 AND state IN ('PROPOSED','ACTIVE')`, org); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE org_data_api_keys SET state='REVOKED',revision=revision+1,revoked_at=clock_timestamp()
WHERE org_id=$1 AND state IN ('ACTIVE','ROTATING')`, org); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE org_connections SET state='DISABLED',revision=revision+1,changed_at=clock_timestamp()
WHERE org_id=$1 AND state<>'DISABLED'`, org)
	return err
}
