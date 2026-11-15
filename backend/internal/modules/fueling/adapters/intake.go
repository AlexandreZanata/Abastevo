package adapters

import (
	"context"
	"errors"
	"reflect"
	"slices"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

type intakeHandler struct {
	authority app.Authority
	readings  app.Readings
	store     app.Store
}

func absent(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// NewIntakeHandler composes the fueling.submit owner from real ports only.
func NewIntakeHandler(authority app.Authority, readings app.Readings, store app.Store) (syncapp.Handler, error) {
	if absent(authority) || absent(readings) || absent(store) {
		return nil, syncapp.ErrUnavailable
	}
	return &intakeHandler{authority: authority, readings: readings, store: store}, nil
}

// Descriptor declares no ACCEPTED_FOR_REVIEW: held fuelings stay retained
// (EDR-175/177) and never produce a review receipt here.
func (*intakeHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.OperationType, Schema: 1, WritePolicy: app.Policy(app.RecordAction), ReadPolicy: app.Policy(app.ReadAction), TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

// denied maps a current-source answer to a bounded transport refusal; an
// unknown cause stays an unavailable (503) source denial.
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

// request binds the operation to its exact signed source, then parses it.
func (h *intakeHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.Command, error) {
	if h == nil || absent(h.authority) || absent(h.readings) || absent(h.store) || ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return app.Command{}, syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return app.Command{}, err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != app.OperationType || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() || v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return app.Command{}, syncapp.SourceDenied(orgapi.NotFound)
	}
	return app.ParseOperation(op)
}

func action(mode syncapp.Mode) app.Action {
	if mode == syncapp.ReadReceipt {
		return app.ReadAction
	}
	return app.RecordAction
}

// root is the malformed-admission selector: current ORG authority only.
func (h *intakeHandler) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	if err := h.authority.CheckFuelingTx(ctx, syncapp.Borrow(db), source.Scope, source, app.Target{}, action(mode)); err != nil {
		return denied(err)
	}
	return nil
}

// classify decides the new fueling from retained history (B-BR-EMP-266).
func (h *intakeHandler) classify(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.CommandInput) (app.Disposition, error) {
	employee, err := h.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil {
		return "", denied(err)
	}
	disposition, err := h.authority.ClassifyFuelingTx(ctx, syncapp.Borrow(db), source.Scope, source, v.AssetID, employee == v.ResponsibleID, v.Declared.OccurredAt)
	if err != nil {
		return "", denied(err)
	}
	if disposition != app.Apply && disposition != app.HoldPermissionChanged && disposition != app.HoldDisposalEnded {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

// target proves authority, then the reading's meter, then authority again;
// a missing meter or segment is a pending dependency (fleet precedent).
func (h *intakeHandler) target(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.CommandInput) (app.Disposition, *fleetapp.MeterView, error) {
	disposition, err := h.classify(ctx, db, source, v)
	if err != nil || v.Reading == nil {
		return disposition, nil, err
	}
	view, lookupErr := h.readings.ReadMeterTargetTx(ctx, syncapp.Borrow(db), source.Scope, v.AssetID, v.Reading.MeterID, v.Reading.SegmentID)
	if disposition, err = h.classify(ctx, db, source, v); err != nil {
		return "", nil, err
	}
	if errors.Is(lookupErr, syncapp.ErrSourceDenied) {
		return "", nil, lookupErr
	}
	if lookupErr != nil && !errors.Is(lookupErr, fleetapp.ErrNotFound) {
		return "", nil, syncapp.ErrUnavailable
	}
	if lookupErr != nil || view.Reference.Asset().Validate(source.Scope) != nil || view.Reference.Asset().ID().Value() != v.AssetID || view.Reference.ID() != v.Reading.MeterID || view.Segment.ID() != v.Reading.SegmentID || view.Segment.Meter() != view.Reference {
		return "", nil, syncapp.ErrDependencyPending
	}
	cloned := view.Clone()
	return disposition, &cloned, nil
}

// readTarget resolves the record facts of a receipt read: the stored fueling
// of this operation, or the would-be own record of a refused one.
func (h *intakeHandler) readTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.CommandInput) (app.Target, *app.Record, error) {
	record, err := h.store.ReadFuelingTx(ctx, syncapp.Borrow(db), source.Scope, v.FuelingID)
	switch {
	case err == nil && record.OperationID == v.OperationID:
		return app.Target{FuelingID: v.FuelingID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID}, &record, nil
	case err == nil, errors.Is(err, app.ErrNotFound):
		return app.Target{FuelingID: v.FuelingID, AssetID: v.AssetID}, nil, nil
	}
	return app.Target{}, nil, syncapp.ErrUnavailable
}

func (h *intakeHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	command, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	v, err := command.Snapshot()
	if err != nil {
		return syncapp.ErrUnavailable
	}
	if mode == syncapp.ReadReceipt {
		target, _, err := h.readTarget(ctx, db, source, v)
		if err != nil {
			return err
		}
		if err := h.authority.CheckFuelingTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
			return denied(err)
		}
		return nil
	}
	_, _, err = h.target(ctx, db, source, v)
	return err
}

func malformed() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

// Apply stores one fueling with its reading evidence (B-BR-EMP-263..265).
// Order: authority and reading target, then the root and asset locks, then
// final MALFORMED checks, then holds (no fact, unavailable), then writes.
func (h *intakeHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, err := h.request(ctx, db, source, op, syncapp.Write)
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformed(), nil
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	case err != nil:
		return syncapp.Outcome{}, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	disposition, view, err := h.target(ctx, db, source, v)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	scope := source.Scope
	// The fleet lifecycle lookup takes the root then asset row locks, the
	// order of every asset and embedded-reading command.
	disposed, err := h.readings.MeterAssetDisposalTx(ctx, syncapp.Borrow(db), scope, v.AssetID)
	if err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	prepared, err := h.store.PrepareFuelingTx(ctx, syncapp.Borrow(db), scope, source, command)
	if errors.Is(err, app.ErrNotFound) {
		return syncapp.Outcome{}, denied(err)
	}
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if prepared.ExistingOperation == v.OperationID {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if prepared.AssetState == "DRAFT" || !prepared.ResponsibleActive || prepared.ExistingOperation != "" || !readingFits(v, view) {
		return malformed(), nil
	}
	// Held cases store nothing until a held-fueling owner exists (EDR-177).
	if disposition != app.Apply || disposed != nil && !v.Declared.OccurredAt.Before(*disposed) {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	arithmetic, err := fuel.Reconcile(v.Candidate)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if reading, ok, err := command.MeterCommand(); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	} else if ok {
		copy := source
		recorded, replay, err := h.readings.RecordEmbedded(ctx, syncapp.Borrow(db), scope, fleetapp.MeterSource{Device: &copy}, source.MembershipID, reading)
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		if err != nil || replay || !readingRecorded(source, v, *view, recorded.Reading) {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	if err := h.store.InsertFuelingTx(ctx, syncapp.Borrow(db), scope, source, command, prepared, arithmetic); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, v); err != nil || disposition != app.Apply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	revision := int64(1)
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

// readingFits applies the fleet reading intake's own checks before any
// write: the declared instant inside the meter's service and the value of
// the meter's kind within its modulus.
func readingFits(v app.CommandInput, view *fleetapp.MeterView) bool {
	if v.Reading == nil {
		return view == nil
	}
	if view == nil || v.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !v.Declared.OccurredAt.Before(*view.RemovedAt) {
		return false
	}
	_, err := meter.NewValue(view.Reference.Kind(), v.Reading.Value.Count(), view.Reference.Modulus())
	return err == nil && v.Reading.Value.Kind() == view.Reference.Kind()
}

func readingRecorded(source syncapp.Source, v app.CommandInput, view fleetapp.MeterView, reading meter.Reading) bool {
	actual := reading.Snapshot()
	if v.Reading == nil || actual.Reference.ID() != v.Reading.ReadingID || actual.Reference.Segment() != view.Segment || actual.Value.Kind() != v.Reading.Value.Kind() || actual.Value.Count() != v.Reading.Value.Count() || actual.Declared == nil || !actual.Declared.OccurredAt.Equal(v.Declared.OccurredAt) || actual.Declared.EntryZone != v.Declared.EntryZone || actual.Declared.DeviceOffsetSeconds != v.Declared.DeviceOffsetSeconds || actual.Author.OrganizationID() != source.Scope.OrgID() || actual.Author.ID() != source.MembershipID || actual.Operation.ID() != v.OperationID || actual.Origin != meter.Fueling || actual.Method != meter.Typed || actual.Photo != nil || actual.State != meter.Valid || actual.ReceivedAt.IsZero() {
		return false
	}
	capture := reading.CaptureSource()
	return capture.OrganizationID() == source.Scope.OrgID() && capture.Kind() == meter.CaptureEnrolledDevice && capture.ID() == source.EnrollmentID
}

// stored proves the stored fueling is exactly this operation's fact.
func stored(source syncapp.Source, v app.CommandInput, record app.Record) bool {
	c := v.Candidate.Snapshot()
	amounts := c.Amounts.Snapshot()
	same := func(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	sameText := func(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	reading := v.Reading == nil && record.ReadingID == "" && record.MeterID == "" && record.SegmentID == "" ||
		v.Reading != nil && record.ReadingID == v.Reading.ReadingID && record.MeterID == v.Reading.MeterID && record.SegmentID == v.Reading.SegmentID
	warnings := slices.IsSorted(record.Warnings) && len(record.Warnings) <= 2 && slices.Contains(record.Warnings, fuel.ArithmeticMismatch) == (record.Difference != nil && *record.Difference > 1)
	return record.ID == v.FuelingID && record.OperationID == v.OperationID && record.PayloadDigest == v.PayloadDigest && record.AssetID == v.AssetID && record.ResponsibleID == v.ResponsibleID &&
		record.AuthorMembershipID == source.MembershipID && record.EnrollmentID == source.EnrollmentID && record.Declared.OccurredAt.Equal(v.Declared.OccurredAt) &&
		record.Declared.EntryZone == v.Declared.EntryZone && record.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds &&
		record.Product == string(c.Quantity.Product().Code()) && record.Unit == string(c.Quantity.Product().Unit()) && record.Quantity == c.Quantity.Amount() &&
		record.Paid == amounts.Paid && same(record.Discount, amounts.Discount) && same(record.UnitPrice, amounts.UnitPrice) && sameText(record.UnitPriceText, amounts.SourcePriceText) &&
		record.Fill == string(c.Fill) && record.Payer == string(c.Payer) && record.Payment == string(c.Payment) && record.Origin == string(fuel.App) &&
		sameText(record.PlaceName, v.PlaceName) && sameText(record.MissingReason, v.MissingReason) && reading && warnings &&
		record.ReviewState == string(review.Submitted) && record.Revision == 1 && !record.ReceivedAt.IsZero()
}

func (h *intakeHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	command, err := h.request(ctx, db, source, op, mode)
	if err == nil && outcome.Result != protocol.Accepted && outcome.Result != protocol.Rejected && !(mode == syncapp.Write && outcome.Result == protocol.Retry && outcome.Code == "DEPENDENCY_PENDING") {
		return syncapp.ErrUnavailable
	}
	switch {
	case errors.Is(err, app.ErrMalformed):
		if err := h.root(ctx, db, source, mode); err != nil {
			return err
		}
		if outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" {
			return syncapp.ErrUnavailable
		}
		return nil
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	v, err := command.Snapshot()
	if err != nil {
		return syncapp.ErrUnavailable
	}
	if mode == syncapp.Write {
		disposition, _, err := h.target(ctx, db, source, v)
		if err != nil || outcome.Result != protocol.Accepted {
			return err
		}
		if disposition != app.Apply {
			return syncapp.ErrUnavailable
		}
	}
	target, record, err := h.readTarget(ctx, db, source, v)
	if err != nil {
		return err
	}
	if mode == syncapp.ReadReceipt {
		if err := h.authority.CheckFuelingTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
			return denied(err)
		}
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || outcome.Code != "" || len(outcome.Warnings) != 0 || record == nil || !stored(source, v, *record) {
		return syncapp.ErrUnavailable
	}
	return nil
}
