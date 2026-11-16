package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/trip"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// EMP-TRIP-018 open trips when a membership ends (B-BR-EMP-404..406,
// EDR-260/261).

// OpenWork lists the trips in progress a membership started as offboarding
// handover keys TRIP:<trip id> (the IDN-010 open-work port). It reads only
// and never closes, reassigns or deletes a trip.
type MembershipOpenWork struct{}

func (MembershipOpenWork) OpenWorkKeysTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, membership string) ([]string, error) {
	if tx == nil || scope.Validate() != nil || !canonicalID(membership) || database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return nil, app.ErrUnavailable
	}
	ids, err := q.New(tx).OpenTripsOfAuthor(ctx, q.OpenTripsOfAuthorParams{OrgID: scope.OrgID().String(), MembershipID: membership})
	if err != nil {
		return nil, storeError(err)
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if !canonicalID(id) {
			return nil, app.ErrUnavailable
		}
		keys = append(keys, "TRIP:"+id)
	}
	return keys, nil
}

// HoldLate keeps one trip.finish of the trip's author from its exact ended
// period for review (EMP-OFFL-009 late intake, B-BR-EMP-406): it needs no
// current authority and never completes the trip. It validates the payload,
// the stored trip of the organization (unknown: MALFORMED, EDR-198), the
// author, the pure finish rules and the reading's meter, then stores the
// OFFL-008 held finish with the late code.
func (h *finishHandler) HoldLate(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, code string) (syncapp.Outcome, error) {
	if code != syncapp.LateHeldCode || h == nil || !h.ready() {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	command, v, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return malformed(), nil
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	case err != nil:
		return syncapp.Outcome{}, err
	}
	record, err := h.store.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, v.TripID)
	if errors.Is(err, app.ErrNotFound) {
		return malformed(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	// EMP-TRIP-012: a finish comes from the member currently responsible.
	if responsible, _ := record.Responsible(); responsible != source.MembershipID {
		return malformed(), nil
	}
	if _, err := app.FinishDomain(source.Scope, record, v); err != nil {
		if errors.Is(err, app.ErrUnavailable) {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		return malformed(), nil
	}
	var view *fleetapp.MeterView
	if r := v.Reading; r != nil {
		target, err := h.readings.ReadMeterTargetTx(ctx, syncapp.Borrow(db), source.Scope, record.AssetID, r.MeterID, r.SegmentID)
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		if errors.Is(err, fleetapp.ErrNotFound) {
			return malformed(), nil
		}
		if err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		cloned := target.Clone()
		view = &cloned
	}
	if !readingFits(v.Event, view) {
		return malformed(), nil
	}
	review, err := h.store.HoldFinishTx(ctx, syncapp.Borrow(db), source, op, command, code)
	return held(review, code, err)
}

var _ syncapp.LateHolder = (*finishHandler)(nil)
