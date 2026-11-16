package adapters

import (
	"context"
	"errors"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// ports are the real sources shared by the two trip owners.
type ports struct {
	authority app.Authority
	readings  app.Readings
	store     app.Store
}

func (p ports) ready() bool { return !absent(p.authority) && !absent(p.readings) && !absent(p.store) }

// denied maps a current-source answer to a bounded transport refusal; an
// unknown cause stays an unavailable (503) source denial (FUEL-002).
func denied(err error) error {
	if errors.Is(err, syncapp.ErrSourceDenied) {
		return err
	}
	if code, ok := orgapi.Registered(err); ok {
		return syncapp.SourceDenied(code)
	}
	for _, pair := range []struct{ source, target error }{
		{permission.ErrPermissionDenied, orgapi.Forbidden},
		{permission.ErrResourceNotFound, orgapi.NotFound},
		{fleetapp.ErrNotFound, orgapi.NotFound},
		{app.ErrNotFound, orgapi.NotFound},
		{fleetapp.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
	} {
		if errors.Is(err, pair.source) {
			return syncapp.SourceDenied(pair.target)
		}
	}
	return syncapp.SourceDenied(err)
}

func bound(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, kind string) error {
	if ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != kind || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() || v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return syncapp.SourceDenied(orgapi.NotFound)
	}
	return nil
}

func action(mode syncapp.Mode) app.Action {
	if mode == syncapp.ReadReceipt {
		return app.ReadAction
	}
	return app.RecordAction
}

func malformed() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

// root is the malformed-admission selector: current ORG authority only.
func (p ports) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	if err := p.authority.CheckTripTx(ctx, syncapp.Borrow(db), source.Scope, source, app.Target{}, action(mode)); err != nil {
		return denied(err)
	}
	return nil
}

// tripTarget is a stored trip's authority target (EMP-TRIP-012): its stored
// structure and the member responsible for it now, or at the instant when one
// is given; a member that was ever responsible re-reads the receipts of the
// operations it sent as its own record (B-BR-EMP-395).
func tripTarget(record app.Record, source syncapp.Source, mode syncapp.Mode, at *time.Time) app.Target {
	member, _ := record.Responsible()
	if at != nil {
		member, _ = record.ResponsibleAt(*at)
	}
	if mode == syncapp.ReadReceipt && record.Party(source.MembershipID) {
		member = source.MembershipID
	}
	return app.Target{TripID: record.ID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: member}
}

// classify decides an event on the asset from retained history (EDR-118).
func (p ports) classify(ctx context.Context, db syncapp.DBTX, source syncapp.Source, asset, driver string, at time.Time) (app.Disposition, error) {
	employee, err := p.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil {
		return "", denied(err)
	}
	disposition, err := p.authority.ClassifyTripTx(ctx, syncapp.Borrow(db), source.Scope, source, asset, employee == driver, at)
	if err != nil {
		return "", denied(err)
	}
	if disposition != app.Apply && disposition != app.HoldPermissionChanged && disposition != app.HoldDisposalEnded {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

// meterTarget proves authority, then the event reading's meter, then
// authority again; a missing meter or segment is a pending dependency.
func (p ports) meterTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, asset, driver string, e app.Event) (app.Disposition, *fleetapp.MeterView, error) {
	disposition, err := p.classify(ctx, db, source, asset, driver, e.Declared.OccurredAt)
	if err != nil || e.Reading == nil {
		return disposition, nil, err
	}
	view, lookupErr := p.readings.ReadMeterTargetTx(ctx, syncapp.Borrow(db), source.Scope, asset, e.Reading.MeterID, e.Reading.SegmentID)
	if disposition, err = p.classify(ctx, db, source, asset, driver, e.Declared.OccurredAt); err != nil {
		return "", nil, err
	}
	if errors.Is(lookupErr, syncapp.ErrSourceDenied) {
		return "", nil, lookupErr
	}
	if lookupErr != nil && !errors.Is(lookupErr, fleetapp.ErrNotFound) {
		return "", nil, syncapp.ErrUnavailable
	}
	if lookupErr != nil || view.Reference.Asset().Validate(source.Scope) != nil || view.Reference.Asset().ID().Value() != asset || view.Reference.ID() != e.Reading.MeterID || view.Segment.ID() != e.Reading.SegmentID || view.Segment.Meter() != view.Reference {
		return "", nil, syncapp.ErrDependencyPending
	}
	cloned := view.Clone()
	return disposition, &cloned, nil
}

// readingFits applies the fleet reading intake's own checks before any write.
func readingFits(e app.Event, view *fleetapp.MeterView) bool {
	if e.Reading == nil {
		return view == nil
	}
	if view == nil || e.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !e.Declared.OccurredAt.Before(*view.RemovedAt) {
		return false
	}
	_, err := meter.NewValue(view.Reference.Kind(), e.Reading.Value.Count(), view.Reference.Modulus())
	return err == nil && e.Reading.Value.Kind() == view.Reference.Kind()
}

// record stores the embedded reading of an event with its trip origin.
func (p ports) record(ctx context.Context, db syncapp.DBTX, source syncapp.Source, asset string, e app.Event, view *fleetapp.MeterView, origin meter.Origin) error {
	reading, ok, err := app.MeterCommand(e, asset, origin)
	if err != nil {
		return syncapp.ErrUnavailable
	}
	if !ok {
		return nil
	}
	copy := source
	recorded, replay, err := p.readings.RecordEmbedded(ctx, syncapp.Borrow(db), source.Scope, fleetapp.MeterSource{Device: &copy}, source.MembershipID, reading)
	if errors.Is(err, syncapp.ErrSourceDenied) {
		return err
	}
	if err != nil || replay || !readingRecorded(source, e, *view, recorded.Reading, origin) {
		return syncapp.ErrUnavailable
	}
	return nil
}

// readingRecorded proves the trip's own embedded reading; a reading the
// METER-003 validator keeps aside as SUSPECT is still the trip's fact, as for
// the meter and fueling intakes (B-BR-EMP-280..285, EMP-TRIP-005).
func readingRecorded(source syncapp.Source, e app.Event, view fleetapp.MeterView, reading meter.Reading, origin meter.Origin) bool {
	actual := reading.Snapshot()
	if e.Reading == nil || actual.Reference.ID() != e.Reading.ReadingID || actual.Reference.Segment() != view.Segment || actual.Value.Kind() != e.Reading.Value.Kind() || actual.Value.Count() != e.Reading.Value.Count() || actual.Declared == nil || !actual.Declared.OccurredAt.Equal(e.Declared.OccurredAt) || actual.Declared.EntryZone != e.Declared.EntryZone || actual.Declared.DeviceOffsetSeconds != e.Declared.DeviceOffsetSeconds || actual.Author.OrganizationID() != source.Scope.OrgID() || actual.Author.ID() != source.MembershipID || actual.Operation.ID() != e.OperationID || actual.Origin != origin || actual.Method != meter.Typed || actual.Photo != nil || actual.State != meter.Valid && actual.State != meter.Suspect || actual.ReceivedAt.IsZero() {
		return false
	}
	capture := reading.CaptureSource()
	return capture.OrganizationID() == source.Scope.OrgID() && capture.Kind() == meter.CaptureEnrolledDevice && capture.ID() == source.EnrollmentID
}

// lock takes the organization root and asset row locks through the fleet
// lifecycle lookup and returns the current disposal instant.
func (p ports) lock(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (*time.Time, error) {
	disposed, err := p.readings.MeterAssetDisposalTx(ctx, syncapp.Borrow(db), scope, asset)
	if err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return nil, err
		}
		return nil, syncapp.ErrUnavailable
	}
	return disposed, nil
}

// held turns a stored held fact into its review outcome.
func held(review, code string, err error) (syncapp.Outcome, error) {
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	id, err := kernel.ParseOrgID(review)
	if err != nil || id.String() != review {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: code, ReviewItemID: review}, nil
}

// checkHeld proves a held outcome through the store's review proof.
func (p ports) checkHeld(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome, valid bool) error {
	if !valid || outcome.EntityRevision != nil {
		return syncapp.ErrUnavailable
	}
	if err := p.store.CheckHeldTx(ctx, syncapp.Borrow(db), source, op, outcome.Copy()); err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return err
		}
		return syncapp.SourceDenied(err)
	}
	return nil
}

func confirmable(err error, mode syncapp.Mode, outcome syncapp.Outcome) bool {
	return err != nil || outcome.Result == protocol.Accepted || outcome.Result == protocol.AcceptedForReview || outcome.Result == protocol.Rejected ||
		mode == syncapp.Write && outcome.Result == protocol.Retry && outcome.Code == "DEPENDENCY_PENDING"
}

// confirmMalformed re-proves a final refusal of a malformed operation.
func (p ports) confirmMalformed(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode, outcome syncapp.Outcome) error {
	if err := p.root(ctx, db, source, mode); err != nil {
		return err
	}
	if outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" {
		return syncapp.ErrUnavailable
	}
	return nil
}

func sameClock(a, b *trip.RunClock) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func sameText(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func sameReading(r *app.Reading, meterID, segmentID, readingID string) bool {
	return r == nil && meterID == "" && segmentID == "" && readingID == "" || r != nil && r.MeterID == meterID && r.SegmentID == segmentID && r.ReadingID == readingID
}
