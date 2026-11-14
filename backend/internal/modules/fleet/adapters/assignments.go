package adapters

import (
	"context"
	"errors"
	"slices"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var _ app.AssignmentRepository = (*AssetStore)(nil)
var _ app.AssignedAssetsPort = (*AssetStore)(nil)

// The fleet.md §9 assignment facts are the exact audit actions; the employee
// termination and asset disposal endings are part of their source command's
// audited change and carry its revision on the row (EDR-165).
const (
	assignmentCreatedAction = "fleet.assignment.created"
	assignmentEndedAction   = "fleet.assignment.ended"
)

func assignmentAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: assignmentCreatedAction, TargetType: "asset"}, {Action: assignmentEndedAction, TargetType: "asset"}}
}

// assignmentError maps the B-BR-EMP-244 exclusion refusal before the shared
// store mapping.
func assignmentError(ctx context.Context, err error) error {
	var pe *pgconn.PgError
	if ctx.Err() == nil && errors.As(err, &pe) && pe.Code == "23P01" && pe.ConstraintName == "org_asset_assignments_primary_no_overlap" {
		return app.ErrAssignmentOverlap
	}
	return storeError(ctx, err)
}

func timestamp(at time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: at.UTC(), Valid: true}
}
func optionalTimestamp(at *time.Time) pgtype.Timestamptz {
	if at == nil {
		return pgtype.Timestamptz{}
	}
	return timestamp(*at)
}

// receiptResult replays one stored receipt of the same actor, kind, asset and
// intent; anything else under the operation is OPERATION_ID_REUSED.
func receiptResult(row q.AssignmentCommandReceiptRow, actor, kind, digest, asset string, count int) (app.AssignmentResult, error) {
	if row.ActorID != actor || row.Kind != kind || row.RequestHash != digest || row.AssetID != asset {
		return app.AssignmentResult{}, app.ErrOperationReused
	}
	if len(row.AssignmentIds) != count {
		return app.AssignmentResult{}, app.ErrUnavailable
	}
	return app.AssignmentResult{AssetID: row.AssetID, AssignmentIDs: slices.Clone(row.AssignmentIds), ReplacedID: row.ReplacedID}, nil
}

// assignmentCommand is one write of B-BR-EMP-244 bound to its receipt.
type assignmentCommand struct {
	asset, operation, kind, action, digest string
	count                                  int
	effect                                 func(*q.Queries, time.Time) (app.AssignmentResult, error)
}

// assignmentTx runs one assignment command under the organization root lock:
// authority, receipt replay, exactly one audit event, the effect, the receipt
// and authority again, all in one transaction.
func (s *AssetStore) assignmentTx(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, c assignmentCommand) (app.AssignmentResult, bool, error) {
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil || c.digest == "" {
		return app.AssignmentResult{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var result app.AssignmentResult
	replayed := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetAssign); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		prior, err := queries.AssignmentCommandReceipt(ctx, q.AssignmentCommandReceiptParams{OrgID: org, OperationID: c.operation})
		if err == nil {
			if result, err = receiptResult(prior, actor.AccountID, c.kind, c.digest, c.asset, c.count); err != nil {
				return err
			}
			replayed = true
			return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetAssign)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		event := audit.Event{OperationID: c.operation, ActorID: actor.AccountID, Action: c.action, TargetType: "asset", TargetID: c.asset, RequestHash: c.digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: c.asset}); err != nil {
				return err
			}
			now, err := meterDBNow(ctx, queries)
			if err != nil {
				return err
			}
			if result, err = c.effect(queries, now); err != nil {
				return err
			}
			return queries.InsertAssignmentCommand(ctx, q.InsertAssignmentCommandParams{OrgID: org, OperationID: c.operation, ActorID: actor.AccountID, AssetID: c.asset, Kind: c.kind, RequestHash: c.digest, AssignmentIds: result.AssignmentIDs, ReplacedID: result.ReplacedID})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetAssign)
	})
	if err != nil {
		return app.AssignmentResult{}, false, assignmentError(ctx, err)
	}
	return result, replayed, nil
}

// CreateAssignments creates one assignment per employee or none (AC4). A
// replace first ends the primary containing the new start at that start in
// the same transaction (B-BR-EMP-244); the exclusion constraint decides any
// remaining overlap, also against a concurrent command.
func (s *AssetStore) CreateAssignments(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, ids []string, command app.AssignmentCreate) (app.AssignmentResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssignmentResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AssignmentResult{}, false, err
	}
	if len(ids) != len(v.EmployeeIDs) {
		return app.AssignmentResult{}, false, fleet.ErrInvalid
	}
	for i, id := range ids {
		if !validID(id) || slices.Contains(ids[:i], id) {
			return app.AssignmentResult{}, false, fleet.ErrInvalid
		}
	}
	org := scope.OrgID().String()
	return s.assignmentTx(ctx, scope, actor, member, assignmentCommand{asset: v.AssetID, operation: v.OperationID, kind: "CREATE", action: assignmentCreatedAction, digest: command.Digest(scope), count: len(ids), effect: func(queries *q.Queries, now time.Time) (app.AssignmentResult, error) {
		start := now
		if v.Start != nil {
			if !v.Start.After(now) {
				return app.AssignmentResult{}, fleet.ErrInvalid
			}
			start = *v.Start
		}
		period, err := fleet.NewAssignmentPeriod(start, v.End)
		if err != nil {
			return app.AssignmentResult{}, err
		}
		if err := fleet.ValidNewAssignment(v.Kind, period, now); err != nil {
			return app.AssignmentResult{}, err
		}
		result := app.AssignmentResult{AssetID: v.AssetID, AssignmentIDs: slices.Clone(ids)}
		if v.ReplacePrimary {
			covering, err := queries.CoveringPrimaryForUpdate(ctx, q.CoveringPrimaryForUpdateParams{OrgID: org, AssetID: v.AssetID, At: timestamp(start)})
			if err != nil {
				return app.AssignmentResult{}, err
			}
			if len(covering) > 1 {
				return app.AssignmentResult{}, app.ErrUnavailable
			}
			for _, previous := range covering {
				n, err := queries.EndAssignmentByCommand(ctx, q.EndAssignmentByCommandParams{ValidTo: timestamp(start), EndCause: string(app.EndedByReplacement), EndOperationID: v.OperationID, EndedBy: actor.AccountID, OrgID: org, ID: previous.ID})
				if err != nil {
					return app.AssignmentResult{}, err
				}
				if n != 1 {
					return app.AssignmentResult{}, app.ErrConflict
				}
				result.ReplacedID = previous.ID
			}
		}
		for i, employee := range v.EmployeeIDs {
			if err := queries.InsertAssignment(ctx, q.InsertAssignmentParams{OrgID: org, ID: ids[i], AssetID: v.AssetID, EmployeeID: employee, Kind: string(v.Kind), ValidFrom: timestamp(period.From), ValidTo: optionalTimestamp(period.To), Reason: v.Reason, OperationID: v.OperationID, AssignedBy: actor.AccountID}); err != nil {
				return app.AssignmentResult{}, err
			}
		}
		return result, nil
	}})
}

// EndAssignment ends one assignment of the asset that is not ENDED at the
// database instant (at its start when it has not started); an ENDED one
// refuses with ErrAssignmentEnded.
func (s *AssetStore) EndAssignment(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.AssignmentEnd) (app.AssignmentResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssignmentResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AssignmentResult{}, false, err
	}
	org := scope.OrgID().String()
	return s.assignmentTx(ctx, scope, actor, member, assignmentCommand{asset: v.AssetID, operation: v.OperationID, kind: "END", action: assignmentEndedAction, digest: command.Digest(scope), count: 1, effect: func(queries *q.Queries, now time.Time) (app.AssignmentResult, error) {
		row, err := queries.AssignmentForEnd(ctx, q.AssignmentForEndParams{OrgID: org, AssetID: v.AssetID, ID: v.AssignmentID})
		if err != nil {
			return app.AssignmentResult{}, err
		}
		period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
		if err != nil {
			return app.AssignmentResult{}, err
		}
		ended, err := period.EndAt(now)
		if err != nil {
			return app.AssignmentResult{}, err
		}
		n, err := queries.EndAssignmentByCommand(ctx, q.EndAssignmentByCommandParams{ValidTo: timestamp(*ended.To), EndCause: string(app.EndedByCommand), EndOperationID: v.OperationID, EndedBy: actor.AccountID, EndReason: v.Reason, OrgID: org, ID: v.AssignmentID})
		if err != nil {
			return app.AssignmentResult{}, err
		}
		if n != 1 {
			return app.AssignmentResult{}, app.ErrConflict
		}
		return app.AssignmentResult{AssetID: v.AssetID, AssignmentIDs: []string{v.AssignmentID}}, nil
	}})
}

func assignmentPeriod(from, to pgtype.Timestamptz) (fleet.AssignmentPeriod, error) {
	start, err := allocationTime(from)
	if err != nil {
		return fleet.AssignmentPeriod{}, err
	}
	var end *time.Time
	if to.Valid {
		at, err := allocationTime(to)
		if err != nil {
			return fleet.AssignmentPeriod{}, err
		}
		end = &at
	}
	period, err := fleet.NewAssignmentPeriod(start, end)
	if err != nil {
		return fleet.AssignmentPeriod{}, app.ErrUnavailable
	}
	return period, nil
}

type assignmentScan struct {
	id, asset, employee, kind, reason, cause string
	from, to, recorded, ended                pgtype.Timestamptz
}

func assignmentView(row assignmentScan, at time.Time) (app.AssignmentView, error) {
	period, err := assignmentPeriod(row.from, row.to)
	if err != nil {
		return app.AssignmentView{}, err
	}
	recorded, err := exactMeterTime(row.recorded, 0)
	if err != nil {
		return app.AssignmentView{}, err
	}
	kind, err := fleet.ParseAssignmentKind(row.kind)
	if err != nil {
		return app.AssignmentView{}, app.ErrUnavailable
	}
	v := app.AssignmentView{ID: row.id, AssetID: row.asset, EmployeeID: row.employee, Kind: kind, Period: period, State: period.StateAt(at), Reason: row.reason, RecordedAt: recorded, EndCause: app.AssignmentEndCause(row.cause)}
	if row.ended.Valid {
		ended, err := exactMeterTime(row.ended, 0)
		if err != nil {
			return app.AssignmentView{}, err
		}
		v.EndedAt = &ended
	}
	return v, nil
}

func assignmentPage(rows []assignmentScan, now time.Time, limit int, keyed bool) (app.AssignmentPage, error) {
	page := app.AssignmentPage{Assignments: []app.AssignmentView{}, At: now}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		v, err := assignmentView(row, now)
		if err != nil {
			return app.AssignmentPage{}, err
		}
		page.Assignments = append(page.Assignments, v)
	}
	if more {
		last := page.Assignments[len(page.Assignments)-1]
		page.NextID = last.ID
		if keyed {
			from := last.Period.From
			page.NextFrom = &from
		}
	}
	return page, nil
}

// ListAssetAssignments reads one visible asset's assignments in start order
// with states derived at one database instant, between authority checks.
func (s *AssetStore) ListAssetAssignments(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.AssignmentFilter) (app.AssignmentPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssignmentPage{}, err
	}
	if !validID(asset) {
		return app.AssignmentPage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return app.AssignmentPage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.AssignmentPage{}, app.ErrUnavailable
	}
	var page app.AssignmentPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead); err != nil {
			return err
		}
		queries := q.New(tx)
		org := scope.OrgID().String()
		if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: asset}); err != nil {
			return err
		}
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		rows, err := queries.ListAssetAssignments(ctx, q.ListAssetAssignmentsParams{OrgID: org, AssetID: asset, AfterFrom: optionalTimestamp(filter.AfterFrom), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		scanned := make([]assignmentScan, 0, len(rows))
		for _, r := range rows {
			scanned = append(scanned, assignmentScan{r.ID, r.AssetID, r.EmployeeID, r.Kind, r.Reason, r.EndCause, r.ValidFrom, r.ValidTo, r.RecordedAt, r.EndedAt})
		}
		if page, err = assignmentPage(scanned, now, filter.Limit, true); err != nil {
			return err
		}
		return s.authorize(ctx, tx, scope, actor, member, asset, app.FleetRead)
	})
	if err != nil {
		return app.AssignmentPage{}, storeError(ctx, err)
	}
	return page, nil
}

// ListOwnAssignments reads the requesting membership's ACTIVE assignments at
// one database instant (B-BR-EMP-246); the target is the membership itself.
func (s *AssetStore) ListOwnAssignments(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, filter app.OwnAssignmentFilter) (app.AssignmentPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssignmentPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.AssignmentPage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.AssignmentPage{}, app.ErrUnavailable
	}
	var page app.AssignmentPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, member, app.FleetReadOwnAssignments); err != nil {
			return err
		}
		queries := q.New(tx)
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		rows, err := queries.ListOwnAssignments(ctx, q.ListOwnAssignmentsParams{OrgID: scope.OrgID().String(), MembershipID: member, At: timestamp(now), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		scanned := make([]assignmentScan, 0, len(rows))
		for _, r := range rows {
			scanned = append(scanned, assignmentScan{r.ID, r.AssetID, r.EmployeeID, r.Kind, r.Reason, r.EndCause, r.ValidFrom, r.ValidTo, r.RecordedAt, r.EndedAt})
		}
		if page, err = assignmentPage(scanned, now, filter.Limit, false); err != nil {
			return err
		}
		return s.authorize(ctx, tx, scope, actor, member, member, app.FleetReadOwnAssignments)
	})
	if err != nil {
		return app.AssignmentPage{}, storeError(ctx, err)
	}
	return page, nil
}

// portQueries validates the borrowed organization transaction of the port.
func portQueries(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, member string, at time.Time) (*q.Queries, pgtype.Timestamptz, error) {
	db = syncapp.Borrow(db)
	instant, err := fleet.AllocationInstant(at)
	if err != nil || !validID(member) {
		return nil, pgtype.Timestamptz{}, fleet.ErrInvalid
	}
	if db == nil || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, pgtype.Timestamptz{}, app.ErrUnavailable
	}
	return q.New(db), timestamp(instant), nil
}

// AssignedAtDBTX answers whether the asset is assigned to the membership at
// the instant (floored to the microsecond of every stored boundary).
func (s *AssetStore) AssignedAtDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, member, asset string, at time.Time) (bool, error) {
	if s == nil || !validID(asset) {
		return false, app.ErrUnavailable
	}
	queries, instant, err := portQueries(ctx, db, scope, member, at)
	if err != nil {
		return false, err
	}
	assigned, err := queries.AssetAssignedAt(ctx, q.AssetAssignedAtParams{OrgID: scope.OrgID().String(), AssetID: asset, MembershipID: member, At: instant})
	if err != nil {
		return false, storeError(ctx, err)
	}
	return assigned, nil
}

// AssetsAssignedAtDBTX lists, in identifier order, every asset assigned to
// the membership at the instant.
func (s *AssetStore) AssetsAssignedAtDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, member string, at time.Time) ([]string, error) {
	if s == nil {
		return nil, app.ErrUnavailable
	}
	queries, instant, err := portQueries(ctx, db, scope, member, at)
	if err != nil {
		return nil, err
	}
	assets, err := queries.AssetsAssignedAt(ctx, q.AssetsAssignedAtParams{OrgID: scope.OrgID().String(), MembershipID: member, At: instant})
	if err != nil {
		return nil, storeError(ctx, err)
	}
	for i, asset := range assets {
		if !validID(asset) || i > 0 && asset <= assets[i-1] {
			return nil, app.ErrUnavailable
		}
	}
	return append([]string{}, assets...), nil
}
