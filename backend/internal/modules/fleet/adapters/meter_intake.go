package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

const MeterReadingOperationType = "meter.reading.record"

type meterIntakeHandler struct {
	authority  app.MeterAuthority
	classifier app.MeterIntakeAuthority
	repository app.EmbeddedMeterRepository
	lookup     app.MeterIntakeLookup
	held       app.MeterHeldRepository
	proof      syncapp.ReviewProof
}

func NewMeterIntakeHandler(authority app.MeterAuthority, repository app.EmbeddedMeterRepository) (syncapp.Handler, error) {
	lookup, ok := repository.(app.MeterIntakeLookup)
	classifier, classifies := authority.(app.MeterIntakeAuthority)
	held, holds := repository.(app.MeterHeldRepository)
	proof, proves := repository.(syncapp.ReviewProof)
	if meterIntakeAbsent(authority) || meterIntakeAbsent(repository) || !ok || meterIntakeAbsent(lookup) || !classifies || meterIntakeAbsent(classifier) || !holds || meterIntakeAbsent(held) || !proves || meterIntakeAbsent(proof) {
		return nil, syncapp.ErrUnavailable
	}
	return &meterIntakeHandler{authority: authority, classifier: classifier, repository: repository, lookup: lookup, held: held, proof: proof}, nil
}

func (h *meterIntakeHandler) ReviewProof() syncapp.ReviewProof { return h.proof }

func meterIntakeAbsent(value any) bool {
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

func (*meterIntakeHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: MeterReadingOperationType, Schema: 1, WritePolicy: app.MeterPolicy(app.MeterRecordAction), ReadPolicy: app.MeterPolicy(app.MeterReadAction), TerminalResults: []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}}
}
func (h *meterIntakeHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.MeterReadingCommand, error) {
	if h == nil || meterIntakeAbsent(h.authority) || meterIntakeAbsent(h.repository) || meterIntakeAbsent(h.lookup) || ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return app.MeterReadingCommand{}, syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return app.MeterReadingCommand{}, err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != MeterReadingOperationType || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() || v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return app.MeterReadingCommand{}, syncapp.SourceDenied(orgapi.NotFound)
	}
	return meterIntakePayload(op)
}

// The envelope owns declaration and identity. Payload fields cannot supply
// author, capture, receipt, state or an embedded business origin.
func meterIntakePayload(op protocol.Operation) (app.MeterReadingCommand, error) {
	v := op.Snapshot()
	if !op.SupportsSchema() || v.Entity.Type != "meter_reading" || v.Entity.ExpectedRevision != nil {
		return app.MeterReadingCommand{}, meter.ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(op.PayloadBytes(), &fields) != nil || len(fields) != 6 {
		return app.MeterReadingCommand{}, meter.ErrInvalid
	}
	values := make(map[string]string, 6)
	for _, key := range []string{"asset_id", "meter_id", "segment_id", "kind", "value", "method"} {
		raw, ok := fields[key]
		var value string
		if !ok || json.Unmarshal(raw, &value) != nil {
			return app.MeterReadingCommand{}, meter.ErrInvalid
		}
		values[key] = value
	}
	value, err := meter.ParseBase(meter.Kind(values["kind"]), values["value"], nil)
	if err != nil {
		return app.MeterReadingCommand{}, meter.ErrInvalid
	}
	declared, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return app.MeterReadingCommand{}, meter.ErrInvalid
	}
	command, err := app.NewMeterReadingCommand(app.MeterReadingInput{
		OperationID: op.ID(), AssetID: values["asset_id"], MeterID: values["meter_id"], SegmentID: values["segment_id"], ReadingID: v.Entity.ID,
		Value: value, Declared: orgtime.Declaration{OccurredAt: declared, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds}, Origin: meter.Standalone, Method: meter.Method(values["method"]),
	})
	if err != nil {
		return app.MeterReadingCommand{}, meter.ErrInvalid
	}
	if values["method"] != string(meter.Typed) || len(v.Attachments) != 0 || len(v.DependsOn) != 0 || len(v.SoftReferences) != 0 {
		return app.MeterReadingCommand{}, syncapp.ErrUnavailable
	}
	return command, nil
}

func (h *meterIntakeHandler) authorityAt(ctx context.Context, db syncapp.DBTX, source syncapp.Source, asset string, mode syncapp.Mode) error {
	action := app.MeterRecordAction
	if mode == syncapp.ReadReceipt {
		action = app.MeterReadAction
	}
	copy := source
	if err := h.authority.CheckMeterTx(ctx, syncapp.Borrow(db), source.Scope, app.MeterSource{Device: &copy}, source.MembershipID, asset, action); err != nil {
		return meterIntakeSourceDenied(err)
	}
	return nil
}

func meterIntakeSourceDenied(err error) error {
	if code, ok := orgapi.Registered(err); ok {
		return syncapp.SourceDenied(code)
	}
	for _, pair := range []struct{ source, target error }{
		{permission.ErrPermissionDenied, orgapi.Forbidden},
		{permission.ErrResourceNotFound, orgapi.NotFound},
		{app.ErrNotFound, orgapi.NotFound},
		{app.ErrUnauthenticated, orgapi.Unauthenticated},
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

func (h *meterIntakeHandler) target(ctx context.Context, db syncapp.DBTX, source syncapp.Source, command app.MeterReadingCommand, mode syncapp.Mode) (app.MeterView, error) {
	in, err := command.Snapshot()
	if err != nil {
		return app.MeterView{}, syncapp.ErrUnavailable
	}
	if _, err := h.commandAuthority(ctx, db, source, command, mode); err != nil {
		return app.MeterView{}, err
	}
	view, lookupErr := h.lookup.ReadMeterTargetTx(ctx, syncapp.Borrow(db), source.Scope, in.AssetID, in.MeterID, in.SegmentID)
	if _, err := h.commandAuthority(ctx, db, source, command, mode); err != nil {
		return app.MeterView{}, err
	}
	if errors.Is(lookupErr, syncapp.ErrSourceDenied) {
		return app.MeterView{}, lookupErr
	}
	if lookupErr != nil && !errors.Is(lookupErr, app.ErrNotFound) {
		return app.MeterView{}, syncapp.ErrUnavailable
	}
	if lookupErr != nil || view.Reference.Asset().Validate(source.Scope) != nil || view.Reference.Asset().ID().Value() != in.AssetID || view.Reference.ID() != in.MeterID || view.Segment.ID() != in.SegmentID || view.Segment.Meter() != view.Reference {
		if mode == syncapp.Write {
			return app.MeterView{}, syncapp.ErrDependencyPending
		}
		return app.MeterView{}, syncapp.ErrUnavailable
	}
	return view.Clone(), nil
}

func (h *meterIntakeHandler) commandAuthority(ctx context.Context, db syncapp.DBTX, source syncapp.Source, command app.MeterReadingCommand, mode syncapp.Mode) (app.MeterDisposition, error) {
	in, err := command.Snapshot()
	if err != nil {
		return "", syncapp.ErrUnavailable
	}
	if mode == syncapp.ReadReceipt {
		return app.MeterApply, h.authorityAt(ctx, db, source, in.AssetID, mode)
	}
	copy := source
	disposition, err := h.classifier.ClassifyMeterTx(ctx, syncapp.Borrow(db), source.Scope, app.MeterSource{Device: &copy}, source.MembershipID, command)
	if err != nil {
		return "", meterIntakeSourceDenied(err)
	}
	if disposition != app.MeterApply && disposition != app.MeterHoldPermissionChanged {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

func (h *meterIntakeHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	command, err := h.request(ctx, db, source, op, mode)
	if errors.Is(err, meter.ErrInvalid) {
		return h.authorityAt(ctx, db, source, source.Scope.OrgID().String(), mode)
	}
	if err != nil {
		return err
	}
	_, err = h.target(ctx, db, source, command, mode)
	return err
}

func malformedMeterOutcome() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

func (h *meterIntakeHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, err := h.request(ctx, db, source, op, syncapp.Write)
	if errors.Is(err, meter.ErrInvalid) {
		if err := h.authorityAt(ctx, db, source, source.Scope.OrgID().String(), syncapp.Write); err != nil {
			return syncapp.Outcome{}, err
		}
		return malformedMeterOutcome(), nil
	}
	if err != nil {
		return syncapp.Outcome{}, err
	}
	view, err := h.target(ctx, db, source, command, syncapp.Write)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	in, _ := command.Snapshot()
	if in.Declared.OccurredAt.Before(view.InstalledAt) || view.RemovedAt != nil && !in.Declared.OccurredAt.Before(*view.RemovedAt) {
		return malformedMeterOutcome(), nil
	}
	if _, err := meter.NewValue(view.Reference.Kind(), in.Value.Count(), view.Reference.Modulus()); err != nil || in.Value.Kind() != view.Reference.Kind() {
		return malformedMeterOutcome(), nil
	}
	disposition, err := h.commandAuthority(ctx, db, source, command, syncapp.Write)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if disposition == app.MeterHoldPermissionChanged {
		reference, err := h.held.HoldMeterReadingTx(ctx, syncapp.Borrow(db), source, op, command, "PERMISSION_CHANGED")
		if err != nil {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		id, err := kernel.ParseOrgID(reference.ReviewItemID)
		if err != nil || id.String() != reference.ReviewItemID || reference.Revision != 1 {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		return syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "PERMISSION_CHANGED", ReviewItemID: reference.ReviewItemID}, nil
	}
	copy := source
	recorded, _, recordErr := h.repository.RecordEmbedded(ctx, syncapp.Borrow(db), source.Scope, app.MeterSource{Device: &copy}, source.MembershipID, command)
	if disposition, err := h.commandAuthority(ctx, db, source, command, syncapp.Write); err != nil || disposition != app.MeterApply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	if errors.Is(recordErr, syncapp.ErrSourceDenied) {
		return syncapp.Outcome{}, recordErr
	}
	if recordErr != nil || !meterIntakeReading(source, op, command, view, recorded.Reading) {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	revision := int64(1)
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

func meterIntakeReading(source syncapp.Source, op protocol.Operation, command app.MeterReadingCommand, view app.MeterView, reading meter.Reading) bool {
	in, err := command.Snapshot()
	if err != nil {
		return false
	}
	actual := reading.Snapshot()
	if actual.Reference.ID() != in.ReadingID || actual.Reference.Segment() != view.Segment || actual.Value.Kind() != in.Value.Kind() || actual.Value.Count() != in.Value.Count() || actual.Declared == nil || !actual.Declared.OccurredAt.Equal(in.Declared.OccurredAt) || actual.Declared.EntryZone != in.Declared.EntryZone || actual.Declared.DeviceOffsetSeconds != in.Declared.DeviceOffsetSeconds || actual.Author.OrganizationID() != source.Scope.OrgID() || actual.Author.ID() != source.MembershipID || actual.Operation.OrganizationID() != source.Scope.OrgID() || actual.Operation.ID() != op.ID() || actual.Origin != meter.Standalone || actual.Method != meter.Typed || actual.Photo != nil || actual.State != meter.Valid || actual.ReceivedAt.IsZero() {
		return false
	}
	capture := reading.CaptureSource()
	return capture.OrganizationID() == source.Scope.OrgID() && capture.Kind() == meter.CaptureEnrolledDevice && capture.ID() == source.EnrollmentID
}

func (h *meterIntakeHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	command, err := h.request(ctx, db, source, op, mode)
	if err == nil && outcome.Result != protocol.Accepted && outcome.Result != protocol.AcceptedForReview && outcome.Result != protocol.Rejected && !(mode == syncapp.Write && outcome.Result == protocol.Retry && outcome.Code == "DEPENDENCY_PENDING") {
		return syncapp.ErrUnavailable
	}
	if errors.Is(err, meter.ErrInvalid) {
		if err := h.authorityAt(ctx, db, source, source.Scope.OrgID().String(), mode); err != nil {
			return err
		}
		if outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" && outcome.Code != "OPERATION_ID_REUSED" {
			return syncapp.ErrUnavailable
		}
		return nil
	}
	if err != nil {
		return err
	}
	view, err := h.target(ctx, db, source, command, mode)
	if err != nil {
		return err
	}
	if outcome.Result == protocol.AcceptedForReview {
		if outcome.Code != "PERMISSION_CHANGED" || outcome.EntityRevision != nil {
			return syncapp.ErrUnavailable
		}
		// Same classification as the Store's own proof checks: a failed REVIEW
		// proof withholds the whole signed response (B-BR-EMP-161).
		if err := h.proof.CheckHeldTx(ctx, syncapp.Borrow(db), source, op, outcome.Copy()); err != nil {
			if errors.Is(err, syncapp.ErrSourceDenied) {
				return err
			}
			return syncapp.SourceDenied(err)
		}
		return nil
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 {
		return syncapp.ErrUnavailable
	}
	if mode == syncapp.Write {
		if disposition, err := h.commandAuthority(ctx, db, source, command, mode); err != nil || disposition != app.MeterApply {
			if err != nil {
				return err
			}
			return syncapp.ErrUnavailable
		}
	}
	in, _ := command.Snapshot()
	reading, readErr := h.lookup.ReadMeterReadingTx(ctx, syncapp.Borrow(db), source.Scope, in.AssetID, in.MeterID, in.SegmentID, in.ReadingID)
	if err := h.authorityAt(ctx, db, source, in.AssetID, mode); err != nil {
		return err
	}
	if errors.Is(readErr, syncapp.ErrSourceDenied) {
		return readErr
	}
	if readErr != nil || !meterIntakeReading(source, op, command, view, reading) {
		return syncapp.ErrUnavailable
	}
	return nil
}
