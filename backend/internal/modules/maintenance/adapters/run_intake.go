package adapters

import (
	"context"
	"errors"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// ---- signed check.run.complete (EMP-CHECK-002, B-BR-EMP-439..441) ----

type runHandler struct {
	authority   app.RunAuthority
	locks       app.AssetLocks
	allocations app.Allocations
	store       app.RunStore
}

// NewRunHandler composes the check.run.complete owner from real ports only:
// the intake authority, the fleet lifecycle lookup that takes the root and
// asset locks, the FLEET-005 allocation snapshot and the run store (EDR-284).
func NewRunHandler(authority app.RunAuthority, locks app.AssetLocks, allocations app.Allocations, store app.RunStore) (syncapp.Handler, error) {
	h := &runHandler{authority: authority, locks: locks, allocations: allocations, store: store}
	if !h.ready() {
		return nil, syncapp.ErrUnavailable
	}
	return h, nil
}

func (h *runHandler) ready() bool {
	return h != nil && !absentPort(h.authority) && !absentPort(h.locks) && !absentPort(h.allocations) && !absentPort(h.store)
}

// Descriptor: no held path, so only ACCEPTED and REJECTED are final; a
// disagreement is accepted and held for review in the run itself (EDR-284).
func (*runHandler) Descriptor() syncapp.Descriptor {
	return syncapp.Descriptor{Type: app.RunOperationType, Schema: 1, WritePolicy: app.RunPolicy(app.RunWrite), ReadPolicy: app.RunPolicy(app.RunReceipt),
		TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
}

func runAction(mode syncapp.Mode) app.RunAction {
	if mode == syncapp.ReadReceipt {
		return app.RunReceipt
	}
	return app.RunWrite
}

func (h *runHandler) request(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) (app.RunCommand, app.RunInput, error) {
	if !h.ready() || ctx == nil || syncapp.Borrow(db) == nil || (mode != syncapp.Write && mode != syncapp.ReadReceipt) {
		return app.RunCommand{}, app.RunInput{}, syncapp.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return app.RunCommand{}, app.RunInput{}, err
	}
	v := op.Snapshot()
	if source.Validate() != nil || v.Type != app.RunOperationType || v.OwnerAccountID != source.Actor.AccountID || v.OrgID != source.Scope.OrgID().String() ||
		v.APIOrigin != source.Origin || v.MembershipID != source.MembershipID || v.EnrollmentID != source.EnrollmentID {
		return app.RunCommand{}, app.RunInput{}, syncapp.SourceDenied(orgapi.NotFound)
	}
	command, err := app.ParseRun(op)
	if err != nil {
		return app.RunCommand{}, app.RunInput{}, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.RunCommand{}, app.RunInput{}, syncapp.ErrUnavailable
	}
	return command, in, nil
}

// root is the malformed-admission selector: current ORG authority only.
func (h *runHandler) root(ctx context.Context, db syncapp.DBTX, source syncapp.Source, mode syncapp.Mode) error {
	if err := h.authority.CheckRunTx(ctx, syncapp.Borrow(db), source.Scope, source, app.RunTarget{}, runAction(mode)); err != nil {
		return denied(err)
	}
	return nil
}

// classify decides the run on its asset from retained history (EDR-118);
// an unknown or another organization's asset is NOT_FOUND.
func (h *runHandler) classify(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.RunInput) (app.ReportDisposition, error) {
	disposition, err := h.authority.ClassifyRunTx(ctx, syncapp.Borrow(db), source.Scope, source, in.AssetID, in.Declared.OccurredAt)
	if err != nil {
		return "", denied(err)
	}
	if disposition != app.ReportApply && disposition != app.ReportPermissionMoved && disposition != app.ReportDisposalEnded {
		return "", syncapp.SourceDenied(syncapp.ErrUnavailable)
	}
	return disposition, nil
}

// readTarget resolves the record facts of a receipt read: the stored run of
// this operation, or the would-be own run of a refused one.
func (h *runHandler) readTarget(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.RunInput) (app.RunTarget, *app.RunRecord, error) {
	record, err := h.store.ReadRunTx(ctx, syncapp.Borrow(db), source.Scope, in.RunID)
	switch {
	case err == nil && record.OperationID == in.OperationID:
		return app.RunTarget{RunID: in.RunID, AssetID: record.AssetID, UnitID: record.UnitID, CostCentreID: record.CostCentreID, AuthorMembershipID: record.AuthorMembershipID}, &record, nil
	case err == nil, errors.Is(err, app.ErrNotFound):
		return app.RunTarget{RunID: in.RunID, AssetID: in.AssetID}, nil, nil
	}
	return app.RunTarget{}, nil, syncapp.ErrUnavailable
}

func (h *runHandler) Admit(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode) error {
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
		if err := h.authority.CheckRunTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.RunReceipt); err != nil {
			return denied(err)
		}
		return nil
	}
	_, err = h.classify(ctx, db, source, in)
	return err
}

// recheck reads the named template's facts and applies the pure recheck
// (B-BR-EMP-440); a version not stored here or a refused run is MALFORMED.
func (h *runHandler) recheck(ctx context.Context, db syncapp.DBTX, source syncapp.Source, in app.RunInput) (checklist.Recheck, bool, error) {
	template, err := h.store.RunTemplateTx(ctx, syncapp.Borrow(db), source.Scope, in.TemplateID, in.Version, in.AssetID)
	if errors.Is(err, app.ErrNotFound) {
		return checklist.Recheck{}, false, nil
	}
	if err != nil {
		return checklist.Recheck{}, false, syncapp.ErrUnavailable
	}
	result, err := checklist.RecheckRun(template.Facts, app.RecheckClaim(in, template.CategoryID))
	switch {
	case err == nil:
		return result, true, nil
	case errors.Is(err, checklist.ErrInvalid), errors.Is(err, checklist.ErrAnswerNotAllowed), errors.Is(err, checklist.ErrPhotoRequired), errors.Is(err, checklist.ErrIncomplete):
		return checklist.Recheck{}, false, nil
	}
	return checklist.Recheck{}, false, syncapp.ErrUnavailable
}

func (h *runHandler) Apply(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (syncapp.Outcome, error) {
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
	existing, err := h.store.RunOperationTx(ctx, syncapp.Borrow(db), source.Scope, in.RunID)
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
	result, wellFormed, err := h.recheck(ctx, db, source, in)
	if err != nil {
		return syncapp.Outcome{}, err
	}
	if !wellFormed {
		return malformed(), nil
	}
	// Changed rights store nothing until a held owner exists (EDR-284).
	if disposition != app.ReportApply {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	employee, err := h.store.AuthorEmployeeTx(ctx, syncapp.Borrow(db), source.Scope, source.MembershipID)
	if err != nil {
		return syncapp.Outcome{}, syncapp.ErrUnavailable
	}
	facts := app.RunFacts{EmployeeID: employee}
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
	if err := h.store.InsertRunTx(ctx, syncapp.Borrow(db), source.Scope, source, command, result, facts); err != nil {
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

func (h *runHandler) Confirm(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, mode syncapp.Mode, outcome syncapp.Outcome) error {
	_, in, err := h.request(ctx, db, source, op, mode)
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
	if mode == syncapp.Write {
		disposition, err := h.classify(ctx, db, source, in)
		if err != nil {
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
		if err := h.authority.CheckRunTx(ctx, syncapp.Borrow(db), source.Scope, source, target, app.RunReceipt); err != nil {
			return denied(err)
		}
	}
	if outcome.Result != protocol.Accepted {
		return nil
	}
	if outcome.EntityRevision == nil || *outcome.EntityRevision != 1 || outcome.Code != "" || len(outcome.Warnings) != 0 || record == nil || !sameRun(source, in, *record) {
		return syncapp.ErrUnavailable
	}
	return nil
}

// sameRun proves the stored run is this operation's fact as declared.
func sameRun(source syncapp.Source, in app.RunInput, r app.RunRecord) bool {
	if r.ID != in.RunID || r.OperationID != in.OperationID || r.PayloadDigest != in.PayloadDigest || r.AssetID != in.AssetID || r.TemplateID != in.TemplateID ||
		r.Context != in.Context || r.Version != in.Version || r.PolicyRevision != in.PolicyRevision || !r.StartedAt.Equal(in.StartedAt) ||
		r.DeviceOutcome != in.DeviceOutcome || r.Acknowledged != in.Acknowledged || r.AuthorAccountID != source.Actor.AccountID ||
		r.AuthorMembershipID != source.MembershipID || r.EnrollmentID != source.EnrollmentID || !r.Declared.OccurredAt.Equal(in.Declared.OccurredAt) ||
		r.Declared.EntryZone != in.Declared.EntryZone || r.Declared.DeviceOffsetSeconds != in.Declared.DeviceOffsetSeconds || len(r.Responses) != len(in.Responses) {
		return false
	}
	for i := range in.Responses {
		if r.Responses[i].Answer != in.Responses[i].Answer || r.Responses[i].Note != in.Responses[i].Note || r.Responses[i].Photo != in.Responses[i].Photo {
			return false
		}
	}
	return true
}
