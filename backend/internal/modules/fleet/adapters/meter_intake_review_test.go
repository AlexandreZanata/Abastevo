package adapters_test

import (
	"context"
	"errors"
	"testing"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// The ordinary unit fixture only owns unchanged permission and no held source.
// Tests requiring a review use the explicit producer below; runtime never uses
// these fixture classification methods as a fallback.
func (a *intakeAuthority) ClassifyMeterTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member string, command app.MeterReadingCommand) (app.MeterDisposition, error) {
	in, err := command.Snapshot()
	if err != nil {
		return "", err
	}
	if err := a.CheckMeterTx(ctx, db, scope, source, member, in.AssetID, app.MeterRecordAction); err != nil {
		return "", err
	}
	return app.MeterApply, nil
}
func (r *intakeRepository) HoldMeterReadingTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, app.MeterReadingCommand, string) (app.ReviewReference, error) {
	return app.ReviewReference{}, syncapp.ErrUnavailable
}
func (r *intakeRepository) CheckHeldTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation, syncapp.Outcome) error {
	return syncapp.ErrUnavailable
}
func (r *intakeRepository) CheckOwnReceiptTx(context.Context, syncapp.DBTX, syncapp.Source, protocol.Operation) (bool, error) {
	return false, nil
}

type authorityWithoutClassifier struct{ app.MeterAuthority }
type repositoryWithoutHeldProof struct {
	app.EmbeddedMeterRepository
	app.MeterIntakeLookup
}
type repositoryWithoutProof struct {
	app.EmbeddedMeterRepository
	app.MeterIntakeLookup
	app.MeterHeldRepository
}

func TestMeterIntakeRequiresCapturedHistoricalAndHeldSources(t *testing.T) {
	_, authority, repository, _ := intakeFixture(t)
	for _, test := range []struct {
		name       string
		authority  app.MeterAuthority
		repository app.EmbeddedMeterRepository
	}{
		{"history", authorityWithoutClassifier{authority}, repository},
		{"held", authority, repositoryWithoutHeldProof{repository, repository}},
		{"proof", authority, repositoryWithoutProof{repository, repository, repository}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if handler, err := fleetpg.NewMeterIntakeHandler(test.authority, test.repository); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
				t.Fatal("missing real review source entered registry")
			}
		})
	}
}

type reviewIntakeAuthority struct {
	*intakeAuthority
	disposition app.MeterDisposition
	classifyErr error
	classified  int
}

func (a *reviewIntakeAuthority) ClassifyMeterTx(_ context.Context, db syncapp.DBTX, scope kernel.OrgScope, source app.MeterSource, member string, command app.MeterReadingCommand) (app.MeterDisposition, error) {
	a.classified++
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		a.t.Fatal("classifier received transaction lifecycle")
	}
	in, err := command.Snapshot()
	if err != nil || source.Device == nil || source.Device.Scope != scope || source.Device.MembershipID != member || in.AssetID != intakeID(10) {
		a.t.Fatal("classifier received fabricated source")
	}
	return a.disposition, a.classifyErr
}

type reviewIntakeRepository struct {
	*intakeRepository
	held, proofs int
	holdErr      error
	reference    app.ReviewReference
}

func (r *reviewIntakeRepository) HoldMeterReadingTx(_ context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, command app.MeterReadingCommand, code string) (app.ReviewReference, error) {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("held producer received transaction lifecycle")
	}
	in, err := command.Snapshot()
	if err != nil || in.OperationID != op.ID() || source.MembershipID != op.Snapshot().MembershipID || code != "PERMISSION_CHANGED" {
		r.t.Fatal("held source lost immutable intent")
	}
	r.held++
	return r.reference, r.holdErr
}
func (r *reviewIntakeRepository) CheckHeldTx(_ context.Context, db syncapp.DBTX, _ syncapp.Source, _ protocol.Operation, outcome syncapp.Outcome) error {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("held proof received transaction lifecycle")
	}
	r.proofs++
	if r.held != 1 || outcome.Result != protocol.AcceptedForReview || outcome.Code != "PERMISSION_CHANGED" || outcome.ReviewItemID != r.reference.ReviewItemID {
		return syncapp.ErrUnavailable
	}
	return nil
}
func TestMeterIntakeChangedPermissionUsesActualHeldProducer(t *testing.T) {
	source, baseAuthority, baseRepository, _ := intakeFixture(t)
	authority := &reviewIntakeAuthority{intakeAuthority: baseAuthority, disposition: app.MeterHoldPermissionChanged}
	repository := &reviewIntakeRepository{intakeRepository: baseRepository, reference: app.ReviewReference{ReviewItemID: intakeID(44), Revision: 1}}
	handler, err := fleetpg.NewMeterIntakeHandler(authority, repository)
	if err != nil {
		t.Fatal(err)
	}
	op := intakeOperation(t, source, nil)
	db := &intakeTx{}
	if err := handler.Admit(t.Context(), db, source, op, syncapp.Write); err != nil {
		t.Fatal(err)
	}
	outcome, err := handler.Apply(t.Context(), db, source, op)
	if err != nil || outcome.Result != protocol.AcceptedForReview || outcome.Code != "PERMISSION_CHANGED" || outcome.ReviewItemID != repository.reference.ReviewItemID || outcome.EntityRevision != nil || repository.held != 1 || repository.recorded != 0 || authority.classified == 0 {
		t.Fatal("permission change applied reading or fabricated review", err)
	}
	if err := handler.Confirm(t.Context(), db, source, op, syncapp.Write, outcome); err != nil {
		t.Fatal(err)
	}
	registry, err := syncapp.NewRegistry(handler)
	if err != nil {
		t.Fatal(err)
	}
	registered, exists := registry.Lookup(fleetpg.MeterReadingOperationType)
	if !exists {
		t.Fatal("actual owner missing from registry")
	}
	proof, ok := registered.(syncapp.ReviewProofProvider)
	if !ok || proof.ReviewProof() == nil {
		t.Fatal("real held proof not captured")
	}
	if err := proof.ReviewProof().CheckHeldTx(t.Context(), db, source, op, outcome); err != nil {
		t.Fatal(err)
	}
	if repository.proofs == 0 {
		t.Fatal("review source not verified")
	}
}
