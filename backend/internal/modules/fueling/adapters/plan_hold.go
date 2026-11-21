package adapters

import (
	"bytes"
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fueling"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

type planHoldAuthority interface {
	app.Authority
	ClassifyFuelingForPlanHoldTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, bool, time.Time) (app.Disposition, error)
}

type planAwareIntake struct {
	*intakeHandler
	authority planHoldAuthority
	proof     planHoldProof
	newID     func() (string, error)
}

func NewPlanAwareIntakeHandler(authority planHoldAuthority, readings app.Readings, store *Store, costs app.CostProjection,
	newID func() (string, error),
) (syncapp.Handler, error) {
	intake, err := newIntakeHandler(authority, readings, store, costs)
	if err != nil || newID == nil || store == nil || absent(store.Allocations) {
		return nil, syncapp.ErrUnavailable
	}
	return &planAwareIntake{intakeHandler: intake, authority: authority, proof: planHoldProof{store: store}, newID: newID}, nil
}

func (h *planAwareIntake) Descriptor() syncapp.Descriptor {
	d := h.intakeHandler.Descriptor()
	d.TerminalResults = append(d.TerminalResults, protocol.AcceptedForReview)
	return d
}

func (h *planAwareIntake) ReviewProof() syncapp.ReviewProof { return h.proof }

func (h *planAwareIntake) HoldLate(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, code string) (syncapp.Outcome, error) {
	if code != syncapp.StatePlanCode {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	command, err := h.request(ctx, db, source, op, syncapp.Write)
	if errors.Is(err, app.ErrMalformed) {
		return malformed(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	store := h.proof.store
	scope := source.Scope
	borrowed := syncapp.Borrow(db)
	employee, err := store.AuthorEmployeeTx(ctx, borrowed, scope, source.MembershipID)
	if err != nil {
		return syncapp.Outcome{}, denied(err)
	}
	if _, err := h.authority.ClassifyFuelingForPlanHoldTx(ctx, borrowed, scope, source, v.AssetID,
		employee == v.ResponsibleID, v.Declared.OccurredAt); err != nil {
		return syncapp.Outcome{}, denied(err)
	}
	disposed, err := h.readings.MeterAssetDisposalTx(ctx, borrowed, scope, v.AssetID)
	if err != nil {
		return syncapp.Outcome{}, denied(err)
	}
	facts, err := store.PrepareFuelingTx(ctx, borrowed, scope, source, command)
	if err != nil {
		return syncapp.Outcome{}, denied(err)
	}
	if facts.AssetState == "DRAFT" || facts.ExistingOperation != "" || !facts.ResponsibleActive ||
		disposed != nil && !v.Declared.OccurredAt.Before(*disposed) {
		return malformed(), nil
	}
	if v.Reading != nil {
		view, err := h.readings.ReadMeterTargetTx(ctx, borrowed, scope, v.AssetID, v.Reading.MeterID, v.Reading.SegmentID)
		if errors.Is(err, fleet.ErrNotFound) || err == nil &&
			(view.Reference.Asset().Validate(scope) != nil || view.Reference.Asset().ID().Value() != v.AssetID ||
				view.Reference.ID() != v.Reading.MeterID || view.Segment.ID() != v.Reading.SegmentID || view.Segment.Meter() != view.Reference ||
				!readingFits(v, &view)) {
			return malformed(), nil
		}
		if err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	queries, _, err := store.port(ctx, db, scope)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	org := scope.OrgID().String()
	exists, err := queries.HeldFuelingExists(ctx, q.HeldFuelingExistsParams{OrgID: org, FuelingID: v.FuelingID})
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if exists {
		return malformed(), nil
	}
	id, err := h.newID()
	if err != nil || !audit.ValidID(id) {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if written, err := queries.CreateFuelingPlanReviewItem(ctx, q.CreateFuelingPlanReviewItemParams{
		OrgID: org, OperationID: op.ID(), ReviewID: id,
	}); err != nil || written != 1 {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := queries.InsertHeldFueling(ctx, q.InsertHeldFuelingParams{
		OrgID: org, ReviewID: id, OperationID: op.ID(), FuelingID: v.FuelingID, AssetID: v.AssetID,
		ResponsibleID: v.ResponsibleID, CanonicalOperation: op.CanonicalBytes(),
	}); err != nil {
		return syncapp.Outcome{}, storeError(err)
	}
	return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: code, ReviewItemID: id}, nil
}

type planHoldProof struct{ store *Store }

func (p planHoldProof) row(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (q.GetHeldFuelingRow, error) {
	if source.Validate() != nil || op.Snapshot().Type != app.OperationType {
		return q.GetHeldFuelingRow{}, syncapp.ErrUnavailable
	}
	queries, _, err := p.store.port(ctx, db, source.Scope)
	if err != nil {
		return q.GetHeldFuelingRow{}, syncapp.ErrUnavailable
	}
	row, err := queries.GetHeldFueling(ctx, q.GetHeldFuelingParams{
		OrgID: source.Scope.OrgID().String(), OperationID: op.ID(), AccountID: source.Actor.AccountID,
		Origin: source.Origin, MembershipID: source.MembershipID, EnrollmentID: source.EnrollmentID,
	})
	if err != nil {
		return q.GetHeldFuelingRow{}, err
	}
	if !audit.ValidID(row.ReviewID) || !bytes.Equal(row.CanonicalOperation, op.CanonicalBytes()) {
		return q.GetHeldFuelingRow{}, syncapp.ErrUnavailable
	}
	return row, nil
}

func verifyPlanHeldReceipt(row q.GetHeldFuelingRow, op protocol.Operation) error {
	ack, err := protocol.ParseAcknowledgement(row.CanonicalAck, op)
	if err != nil || ack.Result() != protocol.AcceptedForReview || ack.Code() != syncapp.StatePlanCode ||
		ack.Snapshot().ReviewItemID != row.ReviewID || ack.Snapshot().EntityRevision != nil {
		return syncapp.ErrUnavailable
	}
	return nil
}

func (p planHoldProof) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	row, err := p.row(ctx, db, source, op)
	if err != nil || outcome.Result != protocol.AcceptedForReview || outcome.Code != syncapp.StatePlanCode ||
		outcome.ReviewItemID != row.ReviewID || outcome.EntityRevision != nil {
		return syncapp.ErrUnavailable
	}
	if len(row.CanonicalAck) != 0 {
		return verifyPlanHeldReceipt(row, op)
	}
	return nil
}

func (p planHoldProof) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	row, err := p.row(ctx, db, source, op)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := verifyPlanHeldReceipt(row, op); err != nil {
		return false, err
	}
	return true, nil
}

var _ syncapp.LateHolder = (*planAwareIntake)(nil)
var _ syncapp.ReviewProofProvider = (*planAwareIntake)(nil)
