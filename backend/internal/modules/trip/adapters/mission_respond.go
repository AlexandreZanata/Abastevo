package adapters

import (
	"context"
	"errors"

	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// ---- trip.mission.respond (EMP-TRIP-008, B-BR-EMP-356, EDR-229) ----

type missionResponseHandler struct {
	authority app.MissionResponseAuthority
	store     app.MissionResponseStore
}

// NewMissionResponseHandler composes the trip.mission.respond owner from
// real ports only: the device trip authority and the mission store. It has
// no held fact, no review proof and no late holder: the response is decided
// only on current facts at receipt.
func NewMissionResponseHandler(authority app.MissionResponseAuthority, store app.MissionResponseStore) (syncapp.Handler, error) {
	if absent(authority) || absent(store) {
		return nil, syncapp.ErrUnavailable
	}
	return &missionResponseHandler{authority: authority, store: store}, nil
}

func (*missionResponseHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.MissionRespondOperationType, Schema: 1, WritePolicy: app.MissionPolicy(app.MissionRespond), ReadPolicy: app.MissionPolicy(app.MissionReceipt),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

func (h *missionResponseHandler) ready() bool {
	return h != nil && !absent(h.authority) && !absent(h.store)
}

func missionAction(mode syncapp.Mode) app.MissionAction {
	if mode == syncapp.ReadReceipt {
		return app.MissionReceipt
	}
	return app.MissionRespond
}

func (h *missionResponseHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.RespondCommand, app.RespondInput, error) {
	if !h.ready() {
		return app.RespondCommand{}, app.RespondInput{}, syncapp.ErrUnavailable
	}
	if err := bound(ctx, db, source, op, mode, app.MissionRespondOperationType); err != nil {
		return app.RespondCommand{}, app.RespondInput{}, err
	}
	command, err := app.ParseRespond(op)
	if err != nil {
		return app.RespondCommand{}, app.RespondInput{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.RespondCommand{}, app.RespondInput{}, syncapp.ErrUnavailable
	}
	return command, v, nil
}

func (h *missionResponseHandler) check(ctx context.Context, db syncapp.DBTX, source syncapp.Source, target app.MissionTarget, action app.MissionAction) error {
	if err := h.authority.CheckMissionResponseTx(ctx, syncapp.Borrow(db), source.Scope, source, target, action); err != nil {
		return denied(err)
	}
	return nil
}

// root is the malformed-admission selector: current ORG authority only.
func (h *missionResponseHandler) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	return h.check(ctx, db, source, app.MissionTarget{}, missionAction(mode))
}

func targetOf(record app.MissionRecord) app.MissionTarget {
	return app.MissionTarget{MissionID: record.View.MissionID, AssigneeMembershipID: record.AssigneeMembershipID, UnitID: record.View.UnitID, AssetID: record.View.AssetID}
}

// mission reads the stored mission and proves the current response
// authority over it. A mission not stored here (another organization's
// included) admits only the current device and is NOT_FOUND (EDR-229).
func (h *missionResponseHandler) mission(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.RespondInput, lock bool) (app.MissionRecord, error) {
	record, err := h.store.MissionTx(ctx, syncapp.Borrow(db), source.Scope, v.MissionID, lock)
	if errors.Is(err, app.ErrNotFound) {
		if err := h.check(ctx, db, source, app.MissionTarget{MissionID: v.MissionID}, app.MissionRespond); err != nil {
			return app.MissionRecord{}, err
		}
		return app.MissionRecord{}, syncapp.SourceDenied(orgapi.NotFound)
	}
	if err != nil {
		return app.MissionRecord{}, syncapp.ErrUnavailable
	}
	if err := h.check(ctx, db, source, targetOf(record), app.MissionRespond); err != nil {
		return app.MissionRecord{}, err
	}
	return record, nil
}

// receipt proves a receipt read: the member's own stored response.
func (h *missionResponseHandler) receipt(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.RespondInput) (app.MissionResponseRecord, error) {
	stored, err := h.store.ResponseTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID)
	if err != nil {
		return app.MissionResponseRecord{}, syncapp.ErrUnavailable
	}
	record, err := h.store.MissionTx(ctx, syncapp.Borrow(db), source.Scope, v.MissionID, false)
	if err != nil || stored.MissionID != v.MissionID {
		return app.MissionResponseRecord{}, syncapp.ErrUnavailable
	}
	target := app.MissionTarget{MissionID: record.View.MissionID, ResponderMembershipID: stored.ActorMembershipID, UnitID: record.View.UnitID, AssetID: record.View.AssetID}
	if err := h.check(ctx, db, source, target, app.MissionReceipt); err != nil {
		return app.MissionResponseRecord{}, err
	}
	return stored, nil
}

func (h *missionResponseHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case err != nil:
		return err
	}
	if mode == syncapp.ReadReceipt {
		_, err := h.receipt(ctx, db, source, v)
		return err
	}
	_, err = h.mission(ctx, db, source, v, false)
	return err
}

// responseRefusal maps a pure refusal at receipt to a transport denial that
// stores nothing (EDR-229).
func responseRefusal(err error) error {
	switch {
	case errors.Is(err, trip.ErrForbidden):
		return syncapp.SourceDenied(orgapi.Forbidden)
	case errors.Is(err, trip.ErrConflict), errors.Is(err, trip.ErrInvalidTransition), errors.Is(err, trip.ErrUnsupportedCommand):
		return syncapp.SourceDenied(orgapi.Conflict)
	}
	return syncapp.ErrUnavailable
}

func (h *missionResponseHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case err != nil:
		return syncapp.Outcome{}, err
	}
	// The head row lock serializes responses and company commands.
	record, err := h.mission(ctx, db, source, v, true)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if _, err := h.store.ResponseTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID); !errors.Is(err, app.ErrNotFound) {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	current, err := record.Domain(source.Scope)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	member, err := trip.NewMembershipRef(source.Scope, source.MembershipID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	next, err := current.Respond(source.Scope, v.ExpectedRevision, member, string(v.Response), v.Reason)
	if err != nil {
		return syncapp.Outcome{}, responseRefusal(err)
	}
	if err := h.store.RecordResponseTx(ctx, syncapp.Borrow(db), source.Scope, source, command, record, next); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.check(ctx, db, source, targetOf(record), app.MissionRespond); err != nil {
		return syncapp.Outcome{}, err
	}
	revision := next.Snapshot().Revision
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

func sameReason(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func (h *missionResponseHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	_, v, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, mode); err != nil {
			return err
		}
		if outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" {
			return syncapp.ErrUnavailable
		}
		return nil
	case err != nil:
		return err
	}
	switch outcome.Result {
	case protocol.Accepted:
		if outcome.EntityRevision == nil || outcome.Code != "" || len(outcome.Warnings) != 0 {
			return syncapp.ErrUnavailable
		}
		var stored app.MissionResponseRecord
		if mode == syncapp.ReadReceipt {
			if stored, err = h.receipt(ctx, db, source, v); err != nil {
				return err
			}
		} else {
			if _, err := h.mission(ctx, db, source, v, false); err != nil {
				return err
			}
			if stored, err = h.store.ResponseTx(ctx, syncapp.Borrow(db), source.Scope, v.OperationID); err != nil || stored.MissionID != v.MissionID {
				return syncapp.ErrUnavailable
			}
		}
		if stored.OperationID != v.OperationID || stored.ActorMembershipID != source.MembershipID || stored.ActorAccountID != source.Actor.AccountID ||
			stored.EnrollmentID != source.EnrollmentID || stored.Revision != *outcome.EntityRevision || stored.Revision != v.ExpectedRevision+1 ||
			stored.Response != v.Response || !sameReason(stored.Reason, v.Reason) {
			return syncapp.ErrUnavailable
		}
		return nil
	case protocol.Rejected:
		// Only a reused identifier reaches here with a well-formed body.
		if mode != syncapp.Write || outcome.Code != "OPERATION_ID_REUSED" {
			return syncapp.ErrUnavailable
		}
		_, err := h.mission(ctx, db, source, v, false)
		return err
	}
	return syncapp.ErrUnavailable
}
