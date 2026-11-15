package adapters

import (
	"context"
	"errors"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// meterLateHeld is the only code a late source can hold (EMP-OFFL-009).
const meterLateHeld = syncapp.LateHeldCode

// HoldLate retains one pre-end reading of an exact ended enrollment period for
// review (EMP-OFFL-009, B-BR-EMP-304). It needs no current authority, reads no
// record for the source and never applies a reading: it validates the payload
// and the target in the organization, then stores the OFFL-008 held fact. A
// target unknown in the organization cannot be a pending dependency of a
// reading declared before the end, so it is MALFORMED (EDR-198).
func (h *meterIntakeHandler) HoldLate(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, code string) (syncapp.Outcome, error) {
	if code != meterLateHeld || h == nil || meterIntakeAbsent(h.lookup) || meterIntakeAbsent(h.held) {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	command, err := h.request(ctx, db, source, op, syncapp.Write)
	if errors.Is(err, meter.ErrInvalid) {
		return malformedMeterOutcome(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	view, err := h.lookup.ReadMeterTargetTx(ctx, syncapp.Borrow(db), source.Scope, in.AssetID, in.MeterID, in.SegmentID)
	if errors.Is(err, syncapp.ErrSourceDenied) {
		return syncapp.Outcome{}, err
	}
	if errors.Is(err, app.ErrNotFound) || err == nil && (view.Reference.Asset().Validate(source.Scope) != nil || view.Reference.Asset().ID().Value() != in.AssetID || view.Reference.ID() != in.MeterID || view.Segment.ID() != in.SegmentID || view.Segment.Meter() != view.Reference) {
		return malformedMeterOutcome(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if in.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !in.Declared.OccurredAt.Before(*view.RemovedAt) {
		return malformedMeterOutcome(), nil
	}
	if _, err := meter.NewValue(view.Reference.Kind(), in.Value.Count(), view.Reference.Modulus()); err != nil || in.Value.Kind() != view.Reference.Kind() {
		return malformedMeterOutcome(), nil
	}
	reference, err := h.held.HoldMeterReadingTx(ctx, syncapp.Borrow(db), source, op, command, code)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	id, err := kernel.ParseOrgID(reference.ReviewItemID)
	if err != nil || id.String() != reference.ReviewItemID || reference.Revision != 1 {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: code, ReviewItemID: reference.ReviewItemID}, nil
}

var _ syncapp.LateHolder = (*meterIntakeHandler)(nil)
