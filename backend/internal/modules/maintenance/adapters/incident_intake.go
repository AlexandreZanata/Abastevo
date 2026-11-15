package adapters

import (
	"context"
	"errors"
	"reflect"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// ---- signed check.incident.report (EMP-CHECK-003, B-BR-EMP-369..371) ----

type reportHandler struct {
	authority   app.ReportAuthority
	locks       app.AssetLocks
	allocations app.Allocations
	trips       app.Trips
	store       app.ReportStore
}

func absentPort(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// NewReportHandler composes the check.incident.report owner from real ports
// only: the intake authority, the fleet lifecycle lookup that takes the root
// and asset locks, the FLEET-005 allocation snapshot, the trip store and the
// report store (EDR-238).
func NewReportHandler(authority app.ReportAuthority, locks app.AssetLocks, allocations app.Allocations, trips app.Trips, store app.ReportStore) (syncapp.Handler, error) {
	h := &reportHandler{authority: authority, locks: locks, allocations: allocations, trips: trips, store: store}
	if !h.ready() {
		return nil, syncapp.ErrUnavailable
	}
	return h, nil
}

func (h *reportHandler) ready() bool {
	return h != nil && !absentPort(h.authority) && !absentPort(h.locks) && !absentPort(h.allocations) && !absentPort(h.trips) && !absentPort(h.store)
}

// Descriptor: no held path, so only ACCEPTED and REJECTED are final.
func (*reportHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.ReportOperationType, Schema: 1, WritePolicy: app.ReportPolicy(app.ReportWrite), ReadPolicy: app.ReportPolicy(app.ReportReceipt),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

// denied maps a current-source answer to a bounded transport refusal; an
// unknown cause stays an unavailable source denial (FUEL-002, TRIP-002).
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
		{fleet.ErrNotFound, orgapi.NotFound},
		{app.ErrNotFound, orgapi.NotFound},
		{fleet.ErrUnauthenticated, orgapi.Unauthenticated},
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

func malformed() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

func action(mode syncapp.Mode) app.ReportAction {
	if mode == syncapp.ReadReceipt {
		return app.ReportReceipt
	}
	return app.ReportWrite
}

func (h *reportHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.ReportCommand, app.ReportInput, error) {
	if !h.ready() || ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return app.ReportCommand{}, app.ReportInput{}, syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return app.ReportCommand{}, app.ReportInput{}, err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != app.ReportOperationType || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() ||
		v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return app.ReportCommand{}, app.ReportInput{}, syncapp.SourceDenied(orgapi.NotFound)
	}
	command, err := app.ParseReport(op)
	if err != nil {
		return app.ReportCommand{}, app.ReportInput{}, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.ReportCommand{}, app.ReportInput{}, syncapp.ErrUnavailable
	}
	return command, in, nil
}

// root is the malformed-admission selector: current ORG authority only.
func (h *reportHandler) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	if err := h.authority.CheckReportTx(ctx, syncapp.Borrow(db), source.Scope, source, app.ReportTarget{}, action(mode)); err != nil {
		return denied(err)
	}
	return nil
}

// classify decides the report on its asset from retained history (EDR-118);
// an unknown or another organization's asset is NOT_FOUND.
func (h *reportHandler) classify(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.ReportInput) (app.ReportDisposition, error) {
	disposition, err := h.authority.ClassifyReportTx(ctx, syncapp.Borrow(db), source.Scope, source, in.AssetID, in.Declared.OccurredAt)
	if err != nil {
		return "", denied(err)
	}
	if disposition != app.ReportApply && disposition != app.ReportPermissionMoved && disposition != app.ReportDisposalEnded {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

// trip reads a named trip; a trip not stored here (undelivered, held or
// another organization's) is a pending dependency that reveals nothing.
func (h *reportHandler) trip(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.ReportInput) (*tripapp.Record, error) {
	if in.TripID == "" {
		return nil, nil
	}
	record, err := h.trips.ReadTripTx(ctx, syncapp.Borrow(db), source.Scope, in.TripID)
	if errors.Is(err, tripapp.ErrNotFound) {
		return nil, syncapp.ErrDependencyPending
	}
	if err != nil {
		return nil, syncapp.ErrUnavailable
	}
	return &record, nil
}

// readTarget resolves the record facts of a receipt read: the stored report
// of this operation, or the would-be own report of a refused one.
func (h *reportHandler) readTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.ReportInput) (app.ReportTarget, *app.ReportRecord, error) {
	record, err := h.store.ReadReportTx(ctx, syncapp.Borrow(db), source.Scope, in.ReportID)
	switch {
	case err == nil && record.OperationID == in.OperationID:
		return app.ReportTarget{ReportID: in.ReportID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: record.ReporterMembershipID}, &record, nil
	case err == nil, errors.Is(err, app.ErrNotFound):
		return app.ReportTarget{ReportID: in.ReportID, AssetID: in.AssetID}, nil, nil
	}
	return app.ReportTarget{}, nil, syncapp.ErrUnavailable
}

func (h *reportHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
	_, in, err := h.request(ctx, db, source, op, mode)
	switch {
	case errors.Is(err, app.ErrMalformed):
		return h.root(ctx, db, source, mode)
	case errors.Is(err, app.ErrUnsupported):
		return syncapp.ErrUnavailable
	case err != nil:
		return err
	}
	if mode == syncapp.ReadReceipt {
		target, _, err := h.readTarget(ctx, db, source, in)
		if err != nil {
			return err
		}
		if err := h.authority.CheckReportTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReportReceipt); err != nil {
			return denied(err)
		}
		return nil
	}
	if _, err := h.classify(ctx, db, source, in); err != nil {
		return err
	}
	_, err = h.trip(ctx, db, source, in)
	return err
}

func (h *reportHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
	command, in, err := h.request(ctx, db, source, op, syncapp.Write)
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
	disposition, err := h.classify(ctx, db, source, in)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if _, err := h.locks.MeterAssetDisposalTx(ctx, syncapp.Borrow(db), source.Scope, in.AssetID); err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	existing, err := h.store.ReportOperationTx(ctx, syncapp.Borrow(db), source.Scope, in.ReportID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if existing == in.OperationID {
		// A stored fact without its stored receipt is never re-acknowledged.
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if existing != "" {
		return malformed(), nil
	}
	state, err := h.store.AssetStateTx(ctx, syncapp.Borrow(db), source.Scope, in.AssetID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if state == "DRAFT" {
		return malformed(), nil
	}
	// Re-read the trip under the locks the trip finish also takes.
	record, err := h.trip(ctx, db, source, in)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if record != nil {
		switch err := app.TripWindow(*record, in); {
		case errors.Is(err, app.ErrForbiddenTrip):
			return syncapp.Outcome{}, syncapp.SourceDenied(orgapi.Forbidden)
		case errors.Is(err, app.ErrMalformed):
			return malformed(), nil
		case err != nil:
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
	}
	// Changed rights store nothing until a held owner exists (EDR-238).
	if disposition != app.ReportApply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	employee, err := h.store.ReporterEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	facts := app.ReportFacts{EmployeeID: employee}
	snapshot, err := h.allocations.UnitAndCostCentreAtDBTX(ctx, syncapp.Borrow(db), source.Scope, in.AssetID, in.Declared.OccurredAt)
	switch {
	case err == nil:
		if snapshot.AssetID != in.AssetID {
			return syncapp.Outcome{}, syncapp.ErrUnavailable
		}
		facts.UnitID, facts.CostCentreID = snapshot.UnitID, snapshot.CostCentreID
	case errors.Is(err, fleet.ErrUnallocated):
	default:
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.store.InsertReportTx(ctx, syncapp.Borrow(db), source.Scope, source, command, facts); err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if disposition, err := h.classify(ctx, db, source, in); err != nil || disposition != app.ReportApply {
		if err == nil {
			err = syncapp.ErrUnavailable
		}
		return syncapp.Outcome{}, err
	}
	revision := int64(1)
	return syncapp.Outcome{Result: protocol.Accepted, EntityRevision: &revision}, nil
}

func (h *reportHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	_, in, err := h.request(ctx, db, source, op, mode)
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
	if mode == syncapp.Write {
		disposition, err := h.classify(ctx, db, source, in)
		if err != nil {
			return err
		}
		if _, err := h.trip(ctx, db, source, in); err != nil {
			if errors.Is(err, syncapp.ErrDependencyPending) && outcome.Result == protocol.Retry {
				return nil
			}
			return err
		}
		if outcome.Result == protocol.Accepted && disposition != app.ReportApply {
			return syncapp.ErrUnavailable
		}
	}
	target, record, err := h.readTarget(ctx, db, source, in)
	if err != nil {
		return err
	}
	if mode == syncapp.ReadReceipt {
		if err := h.authority.CheckReportTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReportReceipt); err != nil {
			return denied(err)
		}
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || outcome.Code != "" || len(outcome.Warnings) != 0 || record == nil || !sameReport(source, in, *record) {
		return syncapp.ErrUnavailable
	}
	return nil
}
