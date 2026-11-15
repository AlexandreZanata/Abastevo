package adapters

import (
	"context"
	"errors"
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
)

var _ app.AttachmentRepository = (*AssetStore)(nil)
var _ app.AttachedImplementsPort = (*AssetStore)(nil)

// The fleet.md §9 implement facts are the exact audit actions.
const (
	implementAttachedAction = "fleet.implement.attached"
	implementDetachedAction = "fleet.implement.detached"
)

func attachmentAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: implementAttachedAction, TargetType: "asset"}, {Action: implementDetachedAction, TargetType: "asset"}}
}

// attachmentError maps the AC1 exclusion refusal before the shared mapping.
func attachmentError(ctx context.Context, err error) error {
	var pe *pgconn.PgError
	if ctx.Err() == nil && errors.As(err, &pe) && pe.Code == "23P01" && pe.ConstraintName == "org_asset_implement_attachments_no_overlap" {
		return app.ErrAttachmentOverlap
	}
	if errors.Is(err, fleet.ErrAttachmentEnded) || errors.Is(err, app.ErrAttachmentOverlap) {
		return err
	}
	return storeError(ctx, err)
}

// attachmentCommand is one write of EMP-FLEET-011 bound to its receipt. The
// effect returns the attachment and the other asset, which must also be
// covered by fresh fleet.asset.manage before anything is written.
type attachmentCommand struct {
	asset, operation, kind, action, digest string
	effect                                 func(*q.Queries, time.Time, func(string) error) (string, error)
}

func (s *AssetStore) attachmentTx(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, c attachmentCommand) (string, bool, error) {
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil || c.digest == "" {
		return "", false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var target string
	replayed := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetManage); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		prior, err := queries.AttachmentCommandReceipt(ctx, q.AttachmentCommandReceiptParams{OrgID: org, OperationID: c.operation})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != c.kind || prior.RequestHash != c.digest || prior.AssetID != c.asset {
				return app.ErrOperationReused
			}
			target, replayed = prior.AttachmentID, true
			return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		other := func(asset string) error { return s.authorize(ctx, tx, scope, actor, member, asset, app.FleetManage) }
		event := audit.Event{OperationID: c.operation, ActorID: actor.AccountID, Action: c.action, TargetType: "asset", TargetID: c.asset, RequestHash: c.digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			if _, err := queries.AssetLifecycleTarget(ctx, q.AssetLifecycleTargetParams{OrgID: org, ID: c.asset}); err != nil {
				return err
			}
			now, err := txNow(ctx, queries)
			if err != nil {
				return err
			}
			if target, err = c.effect(queries, now, other); err != nil {
				return err
			}
			return queries.InsertAttachmentCommand(ctx, q.InsertAttachmentCommandParams{OrgID: org, OperationID: c.operation, ActorID: actor.AccountID, AssetID: c.asset, Kind: c.kind, RequestHash: c.digest, AttachmentID: target})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.authorize(ctx, tx, scope, actor, member, c.asset, app.FleetManage)
	})
	if err != nil {
		return "", false, attachmentError(ctx, err)
	}
	return target, replayed, nil
}

// AttachImplement attaches the path implement to a VEHICLE or MACHINE from
// the given start (or the transaction instant); the exclusion constraint
// refuses an overlapping attachment of the implement, also concurrently.
func (s *AssetStore) AttachImplement(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, id string, command app.AttachmentCreate) (app.AttachmentResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AttachmentResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AttachmentResult{}, false, err
	}
	if !validID(id) {
		return app.AttachmentResult{}, false, fleet.ErrInvalid
	}
	org := scope.OrgID().String()
	target, replayed, err := s.attachmentTx(ctx, scope, actor, member, attachmentCommand{asset: v.ImplementID, operation: v.OperationID, kind: "ATTACH", action: implementAttachedAction, digest: command.Digest(scope), effect: func(queries *q.Queries, now time.Time, other func(string) error) (string, error) {
		if err := other(v.PoweredAssetID); err != nil {
			return "", err
		}
		implement, err := queries.AttachmentAssetKind(ctx, q.AttachmentAssetKindParams{OrgID: org, ID: v.ImplementID})
		if err != nil {
			return "", err
		}
		powered, err := queries.AttachmentAssetKind(ctx, q.AttachmentAssetKindParams{OrgID: org, ID: v.PoweredAssetID})
		if err != nil {
			return "", err
		}
		if err := fleet.ValidAttachmentKinds(fleet.Kind(implement.Kind), fleet.Kind(powered.Kind)); err != nil {
			return "", err
		}
		start := now
		if v.Start != nil {
			if v.Start.Before(now) {
				return "", fleet.ErrInvalid
			}
			start = *v.Start
		}
		period, err := fleet.NewAssignmentPeriod(start, v.End)
		if err != nil {
			return "", err
		}
		if err := fleet.ValidNewAttachment(period, now); err != nil {
			return "", err
		}
		return id, queries.InsertAttachment(ctx, q.InsertAttachmentParams{OrgID: org, ID: id, ImplementID: v.ImplementID, PoweredAssetID: v.PoweredAssetID, ValidFrom: timestamp(period.From), ValidTo: optionalTimestamp(period.To), OperationID: v.OperationID, AttachedBy: actor.AccountID})
	}})
	if err != nil {
		return app.AttachmentResult{}, false, err
	}
	return app.AttachmentResult{AssetID: v.ImplementID, AttachmentID: target}, replayed, nil
}

// DetachImplement ends one attachment of the path asset (as implement or
// powered asset) that is not ENDED, at the given end or the transaction
// instant; an end before the attach instant is refused.
func (s *AssetStore) DetachImplement(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, command app.AttachmentDetach) (app.AttachmentResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AttachmentResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AttachmentResult{}, false, err
	}
	org := scope.OrgID().String()
	target, replayed, err := s.attachmentTx(ctx, scope, actor, member, attachmentCommand{asset: v.AssetID, operation: v.OperationID, kind: "DETACH", action: implementDetachedAction, digest: command.Digest(scope), effect: func(queries *q.Queries, now time.Time, other func(string) error) (string, error) {
		row, err := queries.AttachmentForDetach(ctx, q.AttachmentForDetachParams{OrgID: org, ID: v.AttachmentID, AssetID: v.AssetID})
		if err != nil {
			return "", err
		}
		counterpart := row.PoweredAssetID
		if counterpart == v.AssetID {
			counterpart = row.ImplementID
		}
		if err := other(counterpart); err != nil {
			return "", err
		}
		if row.Detached {
			return "", fleet.ErrAttachmentEnded
		}
		period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
		if err != nil {
			return "", err
		}
		ended, err := fleet.DetachAt(period, v.End, now)
		if err != nil {
			return "", err
		}
		n, err := queries.DetachAttachmentRow(ctx, q.DetachAttachmentRowParams{ValidTo: timestamp(*ended.To), DetachOperationID: v.OperationID, DetachedBy: actor.AccountID, DetachReason: v.Reason, OrgID: org, ID: v.AttachmentID})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", app.ErrConflict
		}
		return v.AttachmentID, nil
	}})
	if err != nil {
		return app.AttachmentResult{}, false, err
	}
	return app.AttachmentResult{AssetID: v.AssetID, AttachmentID: target}, replayed, nil
}

// ListAttachments reads the attachments where the asset is the implement or
// the powered asset, in start order, with states at one database instant.
func (s *AssetStore) ListAttachments(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, asset string, filter app.PeriodFilter) (app.AttachmentPage, error) {
	var page app.AttachmentPage
	err := s.assetRead(ctx, scope, actor, member, asset, filter, func(_ pgx.Tx, queries *q.Queries, org string, now time.Time) error {
		rows, err := queries.ListAssetAttachments(ctx, q.ListAssetAttachmentsParams{OrgID: org, AssetID: asset, AfterFrom: optionalTimestamp(filter.AfterFrom), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.AttachmentPage{Attachments: []app.AttachmentView{}, At: now}
		for i, row := range rows {
			if i == filter.Limit {
				last := page.Attachments[i-1]
				page.NextFrom, page.NextID = &last.Period.From, last.ID
				break
			}
			period, err := assignmentPeriod(row.ValidFrom, row.ValidTo)
			if err != nil {
				return err
			}
			recorded, err := exactMeterTime(row.RecordedAt, 0)
			if err != nil {
				return err
			}
			detached, err := optionalExact(row.DetachedAt)
			if err != nil {
				return err
			}
			page.Attachments = append(page.Attachments, app.AttachmentView{ID: row.ID, ImplementID: row.ImplementID, PoweredAssetID: row.PoweredAssetID, Period: period, State: period.StateAt(now), RecordedAt: recorded, DetachedAt: detached, DetachReason: row.DetachReason})
		}
		return nil
	})
	return page, err
}

// AttachedImplementsDBTX lists, in identifier order, the implements attached
// to the powered asset at the instant (floored to the microsecond); the trip
// start records them (BR-FLEET-033, EMP-TRIP-002). It takes no locks.
func (s *AssetStore) AttachedImplementsDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, powered string, at time.Time) ([]string, error) {
	if s == nil || !validID(powered) {
		return nil, app.ErrUnavailable
	}
	queries, instant, err := portQueries(ctx, db, scope, powered, at)
	if err != nil {
		return nil, err
	}
	ids, err := queries.ImplementsAttachedAt(ctx, q.ImplementsAttachedAtParams{OrgID: scope.OrgID().String(), PoweredAssetID: powered, At: instant})
	if err != nil {
		return nil, storeError(ctx, err)
	}
	for i, id := range ids {
		if !validID(id) || i > 0 && id <= ids[i-1] {
			return nil, app.ErrUnavailable
		}
	}
	return append([]string{}, ids...), nil
}

// PoweredAssetDBTX answers the powered asset the implement is attached to at
// the instant, if any.
func (s *AssetStore) PoweredAssetDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, implement string, at time.Time) (string, bool, error) {
	if s == nil || !validID(implement) {
		return "", false, app.ErrUnavailable
	}
	queries, instant, err := portQueries(ctx, db, scope, implement, at)
	if err != nil {
		return "", false, err
	}
	rows, err := queries.PoweredAssetAt(ctx, q.PoweredAssetAtParams{OrgID: scope.OrgID().String(), ImplementID: implement, At: instant})
	if err != nil {
		return "", false, storeError(ctx, err)
	}
	switch {
	case len(rows) == 0:
		return "", false, nil
	case len(rows) > 1 || !validID(rows[0]):
		return "", false, app.ErrUnavailable
	}
	return rows[0], true, nil
}
