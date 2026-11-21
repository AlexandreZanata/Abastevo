package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const lifecycleTarget = "organization.lifecycle"

var lifecycleActions = map[app.LifecycleAction]string{
	app.ArchiveOrganization: "org.lifecycle.archived",
	app.RestoreOrganization: "org.lifecycle.restored",
	app.ScheduleClosure:     "org.lifecycle.closure_requested",
	app.CancelClosure:       "org.lifecycle.closure_cancelled",
}

func OrganizationLifecycleAuditDefinitions() []audit.Definition {
	out := make([]audit.Definition, 0, len(lifecycleActions))
	for _, action := range lifecycleActions {
		out = append(out, audit.Definition{Action: action, TargetType: lifecycleTarget})
	}
	return out
}

type LifecycleStore struct {
	Pool         *pgxpool.Pool
	Roots        *Store
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Capabilities setupCapability
	Audit        auditing.TransactionRunner
	Notices      closureNotices
}

var _ app.LifecycleRepository = LifecycleStore{}

func (s LifecycleStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member string, write bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope,
		actor, member, scope.OrgID().String(), app.LifecyclePolicy(write))
}

func lifecycleError(ctx context.Context, err error) error {
	if errors.Is(err, app.ErrOrganizationOpenWork) || errors.Is(err, d.ErrTransition) || errors.Is(err, d.ErrConditions) || errors.Is(err, audit.ErrReused) {
		return err
	}
	return legalError(ctx, err)
}

func (s LifecycleStore) LifecycleReport(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string) (app.LifecycleStatus, error) {
	if s.Pool == nil || s.Roots == nil || s.Capabilities == nil || s.Roles == nil || s.Memberships == nil {
		return app.LifecycleStatus{}, app.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return app.LifecycleStatus{}, app.ErrUnauthenticated
	}
	var status app.LifecycleStatus
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, false); err != nil {
			return err
		}
		root, err := read(ctx, tx, scope)
		if err != nil {
			return err
		}
		current, err := root.Snapshot(scope)
		if err != nil {
			return err
		}
		status, err = readLifecycleStatus(ctx, tx, scope, current)
		return err
	})
	return status, lifecycleError(ctx, err)
}

func readLifecycleStatus(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, current d.Snapshot) (app.LifecycleStatus, error) {
	org := scope.OrgID().String()
	status := app.LifecycleStatus{State: current.State, Revision: current.Revision}
	pending, err := q.New(tx).ReadOrganizationClosure(ctx, org)
	if err == nil {
		at := pending.ClosesAt.Time.UTC()
		status.ClosesAt = &at
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.LifecycleStatus{}, err
	}
	status.Items, err = organizationOpenItems(ctx, tx, org)
	return status, err
}

func organizationOpenItems(ctx context.Context, tx pgx.Tx, org string) (app.OpenOrganizationItems, error) {
	var items app.OpenOrganizationItems
	err := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_trips WHERE org_id=$1 AND state IN ('IN_PROGRESS','PAUSED')),
 (SELECT count(*) FROM org_station_agreements WHERE org_id=$1 AND state='ACTIVE' AND valid_until>statement_timestamp()),
 (SELECT count(*) FROM org_fuelings f WHERE f.org_id=$1 AND
     (SELECT r.state FROM org_fueling_review_entries r WHERE (r.org_id,r.fueling_id)=(f.org_id,f.id) ORDER BY r.sequence DESC LIMIT 1) IN ('SUBMITTED','IN_REVIEW'))
 + (SELECT count(*) FROM org_trips t WHERE t.org_id=$1 AND t.state='COMPLETED' AND
     (SELECT r.state FROM org_trip_review_entries r WHERE (r.org_id,r.trip_id)=(t.org_id,t.id) ORDER BY r.sequence DESC LIMIT 1) IN ('SUBMITTED','IN_REVIEW')),
 (SELECT count(*) FROM org_sync_review_items i WHERE i.org_id=$1 AND NOT EXISTS
     (SELECT 1 FROM org_sync_review_resolutions r WHERE (r.org_id,r.review_item_id)=(i.org_id,i.id))),
 (SELECT count(*) FROM org_payable_statements WHERE org_id=$1 AND state='OPEN'),
 (SELECT count(*) FROM org_billing_station_plan_reviews_at($1,statement_timestamp()) r WHERE EXISTS
     (SELECT 1 FROM org_billing_station_plan_admissions s WHERE s.org_id=$1 AND (s.station_id,s.review_revision)=(r.station_id,r.revision))),
 (SELECT count(*) FROM org_consolidation_grants WHERE org_id=$1 AND state='ACTIVE' AND valid_until>statement_timestamp())`, org).
		Scan(&items.Trips, &items.Agreements, &items.Reviews, &items.HeldOperations, &items.OpenPayables, &items.StationAuthorities, &items.ActiveGrants)
	return items, err
}

func (s LifecycleStore) ChangeLifecycle(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, command app.LifecycleCommand) (app.LifecycleResult, error) {
	if s.Pool == nil || s.Roots == nil || s.Capabilities == nil || s.Roles == nil || s.Memberships == nil || s.Audit == nil {
		return app.LifecycleResult{}, app.ErrUnavailable
	}
	if actor.Validate() != nil || legalIdentity(scope, member) != nil {
		return app.LifecycleResult{}, app.ErrUnauthenticated
	}
	if err := command.Validate(); err != nil {
		return app.LifecycleResult{}, err
	}
	org := scope.OrgID().String()
	intent, err := json.Marshal(command)
	if err != nil {
		return app.LifecycleResult{}, app.ErrUnavailable
	}
	digest := sha256.Sum256(intent)
	event := audit.Event{OperationID: command.OperationID, TargetID: org, TargetType: lifecycleTarget,
		ActorID: actor.AccountID, Action: lifecycleActions[command.Action], RequestHash: hex.EncodeToString(digest[:])}
	var result app.LifecycleResult
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, true); err != nil {
			return err
		}
		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			status, err := s.change(ctx, tx, scope, command)
			if err != nil {
				return err
			}
			result.LifecycleStatus = status
			body, err := json.Marshal(status)
			if err != nil {
				return err
			}
			return q.New(tx).InsertOrganizationLifecycleReceipt(ctx, q.InsertOrganizationLifecycleReceiptParams{OrgID: org,
				OperationID: command.OperationID, Result: body})
		})
		if err != nil || !replayed {
			return err
		}
		body, err := q.New(tx).ReadOrganizationLifecycleReceipt(ctx, q.ReadOrganizationLifecycleReceiptParams{OrgID: org, OperationID: command.OperationID})
		if err != nil {
			return err
		}
		result.Replayed = true
		return json.Unmarshal(body, &result.LifecycleStatus)
	})
	return result, lifecycleError(ctx, err)
}

func (s LifecycleStore) change(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, command app.LifecycleCommand) (app.LifecycleStatus, error) {
	org := scope.OrgID().String()
	queries := q.New(tx)
	root, err := read(ctx, tx, scope)
	if err != nil {
		return app.LifecycleStatus{}, err
	}
	current, err := root.Snapshot(scope)
	if err != nil {
		return app.LifecycleStatus{}, err
	}
	if current.Revision != command.ExpectedRevision {
		return app.LifecycleStatus{}, d.ErrConflict
	}
	status, err := readLifecycleStatus(ctx, tx, scope, current)
	if err != nil {
		return app.LifecycleStatus{}, err
	}
	now, err := queries.CurrentOnboardingTime(ctx)
	if err != nil || !settingsInstant(now) {
		return app.LifecycleStatus{}, app.ErrUnavailable
	}
	var next d.Organization
	switch command.Action {
	case app.ArchiveOrganization, app.ScheduleClosure:
		if status.ClosesAt != nil {
			return app.LifecycleStatus{}, d.ErrConflict
		}
		if status.Items.Trips > 0 || status.Items.Agreements > 0 {
			return app.LifecycleStatus{}, app.OrganizationOpenWork{Trips: status.Items.Trips, Agreements: status.Items.Agreements}
		}
		if command.Action == app.ScheduleClosure && current.State == d.Archived {
			next, err = root.ReviseArchivedClosure(scope, command.ExpectedRevision)
		} else {
			next, err = root.Transition(scope, command.ExpectedRevision, d.Archived, d.Conditions{ArchiveClear: true})
		}
	case app.RestoreOrganization:
		if status.ClosesAt != nil {
			return app.LifecycleStatus{}, d.ErrConflict
		}
		facts := d.Conditions{Closure: d.ClosureNone, SuspensionResolved: current.SuspensionReason == "", SuspensionReason: current.SuspensionReason}
		target := d.Active
		if !facts.SuspensionResolved {
			target = d.Suspended
		}
		next, err = root.Transition(scope, command.ExpectedRevision, target, facts)
	case app.CancelClosure:
		if status.ClosesAt == nil || d.ClosureDue(now.Time, *status.ClosesAt) {
			return app.LifecycleStatus{}, d.ErrConflict
		}
		req, err := queries.ReadOrganizationClosure(ctx, org)
		if err != nil {
			return app.LifecycleStatus{}, err
		}
		count, err := queries.CancelOrganizationClosure(ctx, q.CancelOrganizationClosureParams{OrgID: org,
			OperationID: req.OperationID, CancelOperation: command.OperationID})
		if err != nil || count != 1 {
			return app.LifecycleStatus{}, d.ErrConflict
		}
		next, err = root.ReviseArchivedClosure(scope, command.ExpectedRevision)
		if err != nil {
			return app.LifecycleStatus{}, err
		}
	}
	if err != nil {
		return app.LifecycleStatus{}, err
	}
	changed, err := s.Roots.SaveTx(ctx, tx, scope, current.Revision, next)
	if err != nil || !changed {
		return app.LifecycleStatus{}, errors.Join(d.ErrConflict, err)
	}
	after, err := next.Snapshot(scope)
	if err != nil {
		return app.LifecycleStatus{}, err
	}
	if command.Action == app.ScheduleClosure {
		deadline, err := d.ClosureDeadline(current.Profile.Name, command.Confirmation, now.Time, command.ClosureDays)
		if err != nil {
			return app.LifecycleStatus{}, err
		}
		if err := scheduleOrganizationClosure(ctx, tx, org, command.OperationID, after.Revision, now, deadline); err != nil {
			return app.LifecycleStatus{}, err
		}
		if err := s.notifyClosureOwners(ctx, tx, scope, command.OperationID, after.Revision, deadline); err != nil {
			return app.LifecycleStatus{}, err
		}
	}
	return readLifecycleStatus(ctx, tx, scope, after)
}

func scheduleOrganizationClosure(ctx context.Context, tx pgx.Tx, org, operation string, revision int64, now pgtype.Timestamptz, deadline time.Time) error {
	body, err := json.Marshal(organizationClosureJob{Version: 1, OrgID: org, OperationID: operation})
	if err != nil {
		return err
	}
	job, err := jobs.Enqueue(ctx, tx, organizationClosureKind, body, organizationClosureKind+":"+org+":"+operation, 5, deadline)
	if err != nil || !job.Created {
		return app.ErrUnavailable
	}
	return q.New(tx).InsertOrganizationClosure(ctx, q.InsertOrganizationClosureParams{OrgID: org, OperationID: operation,
		JobID: job.ID, ArchiveRevision: revision, RequestedAt: now, ClosesAt: pgtype.Timestamptz{Time: deadline, Valid: true}})
}
