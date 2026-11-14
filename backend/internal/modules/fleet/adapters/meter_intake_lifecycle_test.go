package adapters_test

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// disposalIntakeRepository exposes the asset's actual current disposal port and
// records which held code the handler chose; it never decides the outcome.
type disposalIntakeRepository struct {
	*intakeRepository
	disposal     *time.Time
	disposalErr  error
	disposals    int
	held, proofs int
	codes        []string
	reference    app.ReviewReference
}

func (r *disposalIntakeRepository) MeterAssetDisposalTx(_ context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (*time.Time, error) {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		r.t.Fatal("disposal port received transaction lifecycle")
	}
	if asset != intakeID(10) || scope.Validate() != nil {
		r.t.Fatal("disposal port received a fabricated target")
	}
	r.disposals++
	if r.disposal == nil {
		return nil, r.disposalErr
	}
	at := *r.disposal
	return &at, r.disposalErr
}
func (r *disposalIntakeRepository) HoldMeterReadingTx(_ context.Context, _ syncapp.DBTX, _ syncapp.Source, _ protocol.Operation, _ app.MeterReadingCommand, code string) (app.ReviewReference, error) {
	r.held++
	r.codes = append(r.codes, code)
	return r.reference, nil
}
func (r *disposalIntakeRepository) CheckHeldTx(_ context.Context, _ syncapp.DBTX, _ syncapp.Source, _ protocol.Operation, outcome syncapp.Outcome) error {
	r.proofs++
	if outcome.Result != protocol.AcceptedForReview || outcome.ReviewItemID != r.reference.ReviewItemID || len(r.codes) == 0 || outcome.Code != r.codes[len(r.codes)-1] {
		return syncapp.ErrUnavailable
	}
	return nil
}

// The operation fixture declares 2026-11-13T12:00:00.000000123Z.
var intakeDeclared = time.Date(2026, 11, 13, 12, 0, 0, 123, time.UTC)

func TestMeterIntakeHoldsReadingsAtOrAfterTheExactDisposalInstant(t *testing.T) {
	for _, test := range []struct {
		name     string
		disposal time.Duration
		held     bool
	}{
		{"declared one nanosecond before disposal", time.Nanosecond, false},
		{"declared exactly at disposal", 0, true},
		{"declared one nanosecond after disposal", -time.Nanosecond, true},
		{"declared long after disposal", -48 * time.Hour, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, authority, base, _ := intakeFixture(t)
			disposal := intakeDeclared.Add(test.disposal)
			repository := &disposalIntakeRepository{intakeRepository: base, disposal: &disposal, reference: app.ReviewReference{ReviewItemID: intakeID(45), Revision: 1}}
			handler, err := fleetpg.NewMeterIntakeHandler(authority, repository)
			if err != nil {
				t.Fatal(err)
			}
			op := intakeOperation(t, source, nil)
			outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
			if err != nil {
				t.Fatal("disposal boundary refused instead of deciding", err)
			}
			if test.held {
				if outcome.Result != protocol.AcceptedForReview || outcome.Code != "ASSET_UNAVAILABLE" || outcome.ReviewItemID != intakeID(45) || outcome.EntityRevision != nil || repository.recorded != 0 || repository.held != 1 || repository.codes[0] != "ASSET_UNAVAILABLE" {
					t.Fatal("post-disposal reading was not held for review", outcome)
				}
				if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.Write, outcome); err != nil || repository.proofs != 1 {
					t.Fatal("held disposal outcome lacks its actual proof", err)
				}
				return
			}
			if outcome.Result != protocol.Accepted || repository.recorded != 1 || repository.held != 0 || repository.disposals == 0 {
				t.Fatal("pre-disposal reading lost normal handling", outcome)
			}
		})
	}
}

func TestMeterIntakeKeepsNormalHandlingWithoutDisposalAndPermissionPrecedence(t *testing.T) {
	source, authority, base, _ := intakeFixture(t)
	repository := &disposalIntakeRepository{intakeRepository: base, reference: app.ReviewReference{ReviewItemID: intakeID(46), Revision: 1}}
	handler, err := fleetpg.NewMeterIntakeHandler(authority, repository)
	if err != nil {
		t.Fatal(err)
	}
	op := intakeOperation(t, source, nil)
	if outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op); err != nil || outcome.Result != protocol.Accepted || repository.disposals != 1 || repository.held != 0 {
		t.Fatal("asset without current disposal changed handling", err)
	}
	disposal := intakeDeclared.Add(-time.Hour)
	changed := &reviewIntakeAuthority{intakeAuthority: authority, disposition: app.MeterHoldPermissionChanged}
	precedence := &disposalIntakeRepository{intakeRepository: base, disposal: &disposal, reference: app.ReviewReference{ReviewItemID: intakeID(47), Revision: 1}}
	handler, err = fleetpg.NewMeterIntakeHandler(changed, precedence)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op)
	if err != nil || outcome.Code != "PERMISSION_CHANGED" || len(precedence.codes) != 1 || precedence.codes[0] != "PERMISSION_CHANGED" {
		t.Fatal("permission change lost precedence over disposal", err, outcome)
	}
	failing := &disposalIntakeRepository{intakeRepository: base, disposalErr: errors.New("Synthetic private SQL detail"), reference: app.ReviewReference{ReviewItemID: intakeID(48), Revision: 1}}
	handler, err = fleetpg.NewMeterIntakeHandler(authority, failing)
	if err != nil {
		t.Fatal(err)
	}
	recorded := base.recorded
	if outcome, err := handler.Apply(t.Context(), &intakeTx{}, source, op); !errors.Is(err, syncapp.ErrUnavailable) || outcome.Result != "" || base.recorded != recorded || failing.held != 0 {
		t.Fatal("unknown disposal source became an outcome", err)
	}
	undeclared := syncapp.Outcome{Result: protocol.AcceptedForReview, Code: "ASSET_BUSY", ReviewItemID: intakeID(48)}
	if err := handler.Confirm(t.Context(), &intakeTx{}, source, op, syncapp.Write, undeclared); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("undeclared held code confirmed", err)
	}
}

type repositoryWithoutLifecycle struct {
	app.EmbeddedMeterRepository
	app.MeterIntakeLookup
	app.MeterHeldRepository
	syncapp.ReviewProof
}

func TestMeterIntakeRequiresTheActualAssetLifecycleSource(t *testing.T) {
	_, authority, repository, _ := intakeFixture(t)
	if handler, err := fleetpg.NewMeterIntakeHandler(authority, repositoryWithoutLifecycle{repository, repository, repository, repository}); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("intake without asset lifecycle source entered registry")
	}
}
