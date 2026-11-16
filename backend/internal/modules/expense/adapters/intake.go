package adapters

import (
	"context"
	"errors"
	"reflect"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

type intakeHandler struct {
	authority app.Authority
	lifecycle app.Lifecycle
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

// NewIntakeHandler composes the expense.submit owner from real ports only.
func NewIntakeHandler(authority app.Authority, lifecycle app.Lifecycle, store app.Store) (syncapp.Handler, error) {
	if absent(authority) || absent(lifecycle) || absent(store) {
		return nil, syncapp.ErrUnavailable
	}
	return &intakeHandler{authority: authority, lifecycle: lifecycle, store: store}, nil
}

// Descriptor declares no ACCEPTED_FOR_REVIEW: held expenses stay retained
// (EDR-289) and never produce a review receipt here.
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
	if h == nil || absent(h.authority) || absent(h.lifecycle) || absent(h.store) || ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
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
	if err := h.authority.CheckExpenseTx(ctx, syncapp.Borrow(db), source.Scope, source, app.Target{}, action(mode)); err != nil {
		return denied(err)
	}
	return nil
}

// classify decides the new expense from retained history (B-BR-EMP-448).
func (h *intakeHandler) classify(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.CommandInput) (app.Disposition, error) {
	disposition, err := h.authority.ClassifyExpenseTx(ctx, syncapp.Borrow(db), source.Scope, source, v.AssetID, v.Declared.OccurredAt)
	if err != nil {
		return "", denied(err)
	}
	if disposition != app.Apply && disposition != app.HoldPermissionChanged && disposition != app.HoldDisposalEnded {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

// readTarget resolves the record facts of a receipt read: the stored expense
// of this operation, or the would-be own record of a refused one.
func (h *intakeHandler) readTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, v app.CommandInput) (app.Target, *app.Record, error) {
	record, err := h.store.ReadExpenseTx(ctx, syncapp.Borrow(db), source.Scope, v.ExpenseID)
	switch {
	case err == nil && record.OperationID == v.OperationID:
		return app.Target{ExpenseID: v.ExpenseID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID}, &record, nil
	case err == nil, errors.Is(err, app.ErrNotFound):
		return app.Target{ExpenseID: v.ExpenseID, AssetID: v.AssetID}, nil, nil
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
		if err := h.authority.CheckExpenseTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
			return denied(err)
		}
		return nil
	}
	_, err = h.classify(ctx, db, source, v)
	return err
}

func malformed() syncapp.Outcome {
	return syncapp.Outcome{Result: protocol.Rejected, Code: "MALFORMED"}
}

// Apply stores one direct expense (B-BR-EMP-447). Order: authority, then the
// root and asset locks, then final MALFORMED checks, then holds (no fact,
// unavailable), then the write and authority again.
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
	disposition, err := h.classify(ctx, db, source, v)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	scope := source.Scope
	// The fleet lifecycle lookup takes the root then asset row locks, the
	// order of every asset and embedded-reading command.
	disposed, err := h.lifecycle.MeterAssetDisposalTx(ctx, syncapp.Borrow(db), scope, v.AssetID)
	if err != nil {
		if errors.Is(err, syncapp.ErrSourceDenied) {
			return syncapp.Outcome{}, err
		}
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	prepared, err := h.store.PrepareExpenseTx(ctx, syncapp.Borrow(db), scope, source, command)
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
	if prepared.AssetState == "DRAFT" || prepared.ExistingOperation != "" {
		return malformed(), nil
	}
	// Held cases store nothing until a held-expense owner exists (EDR-289).
	if disposition != app.Apply || disposed != nil && !v.Declared.OccurredAt.Before(*disposed) {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	if err := h.store.InsertDirectTx(ctx, syncapp.Borrow(db), scope, source, command, prepared); err != nil {
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

// stored proves the stored expense is exactly this operation's fact.
func stored(source syncapp.Source, v app.CommandInput, record app.Record) bool {
	return record.ID == v.ExpenseID && record.OriginKind == string(domain.KindDirect) && record.SourceID == v.ExpenseID && record.FuelingID == "" &&
		record.SourceRevision == app.SourceRevision && record.OperationID == v.OperationID && record.PayloadDigest == v.PayloadDigest &&
		record.AssetID == v.AssetID && record.AuthorMembershipID == source.MembershipID && record.EnrollmentID == source.EnrollmentID &&
		record.Category == v.Category && record.Paid == v.Paid && record.Company == v.Company && record.Payer == v.Payer &&
		record.Declared.OccurredAt.Equal(v.Declared.OccurredAt) && record.Declared.EntryZone == v.Declared.EntryZone &&
		record.Declared.DeviceOffsetSeconds == v.Declared.DeviceOffsetSeconds && record.ReviewState == app.Submitted && record.Revision == 1 &&
		!record.ReceivedAt.IsZero()
}

func (h *intakeHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	command, err := h.request(ctx, db, source, op, mode)
	if err == nil && outcome.Result != protocol.Accepted && outcome.Result != protocol.Rejected {
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
		disposition, err := h.classify(ctx, db, source, v)
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
		if err := h.authority.CheckExpenseTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.ReadAction); err != nil {
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
