package application_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type reviewOwner struct {
	*handler
	proof app.ReviewProof
	calls int
}

func (owner *reviewOwner) ReviewProof() app.ReviewProof { owner.calls++; return owner.proof }

type registeredProof struct {
	t         *testing.T
	held, own int
}

func (proof *registeredProof) CheckHeldTx(_ context.Context, db app.DBTX, _ app.Source, _ protocol.Operation, outcome app.Outcome) error {
	proof.held++
	if _, exposed := db.(interface{ Commit(context.Context) error }); exposed {
		proof.t.Error("review proof received transaction lifecycle")
	}
	if outcome.EntityRevision != nil {
		*outcome.EntityRevision = 999
	}
	if len(outcome.Warnings) > 0 {
		outcome.Warnings[0] = "MUTATED"
	}
	return app.ErrUnavailable // Descriptor proof tests never invent actual held facts.
}
func (proof *registeredProof) CheckOwnReceiptTx(_ context.Context, db app.DBTX, _ app.Source, _ protocol.Operation) (bool, error) {
	proof.own++
	if _, exposed := db.(interface{ Commit(context.Context) error }); exposed {
		proof.t.Error("own receipt proof received transaction lifecycle")
	}
	return false, app.ErrUnavailable
}

type proofLifecycle struct{}

func (proofLifecycle) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (proofLifecycle) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (proofLifecycle) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }
func (proofLifecycle) Commit(context.Context) error                            { return nil }

func TestRegistryRejectsEveryDeclaredReviewWithoutActualProof(t *testing.T) {
	for _, name := range []string{"legacy_default", "explicit", "nil", "typed_nil"} {
		t.Run(name, func(t *testing.T) {
			base := newHandler()
			base.descriptor.TerminalResults = []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}
			var owner app.Handler = base
			switch name {
			case "legacy_default":
				base.descriptor.TerminalResults = nil
			case "nil":
				owner = &reviewOwner{handler: base}
			case "typed_nil":
				var absent *registeredProof
				owner = &reviewOwner{handler: base, proof: absent}
			}
			if _, err := app.NewRegistry(owner); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("unimplemented review producer registered")
			}
		})
	}
	base := newHandler()
	base.descriptor.TerminalResults = []string{protocol.Accepted, protocol.Rejected}
	if _, err := app.NewRegistry(base); err != nil {
		t.Fatal("accepted/rejected-only producer gained a review obligation", err)
	}
}

func TestRegistryCapturesReviewProofOnceAndBorrowsItsInputs(t *testing.T) {
	first, late := &registeredProof{t: t}, &registeredProof{t: t}
	base := newHandler()
	base.descriptor.TerminalResults = []string{protocol.Accepted, protocol.AcceptedForReview, protocol.Rejected}
	owner := &reviewOwner{handler: base, proof: first}
	registry, err := app.NewRegistry(owner)
	if err != nil {
		t.Fatal(err)
	}
	owner.proof = late
	frozen, ok := registry.Lookup("synthetic.record")
	if !ok {
		t.Fatal("registered producer absent")
	}
	provider, ok := frozen.(app.ReviewProofProvider)
	if !ok || provider.ReviewProof() == nil {
		t.Fatal("actual source proof was not captured")
	}
	proof := provider.ReviewProof()
	revision := int64(1)
	outcome := app.Outcome{Result: protocol.AcceptedForReview, ReviewItemID: id(99), EntityRevision: &revision, Warnings: []string{"CLOCK_SKEW"}}
	if err := proof.CheckHeldTx(t.Context(), proofLifecycle{}, source(t), operation(t, 191, "synthetic.record"), outcome); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("fixture unexpectedly certified held source")
	}
	if _, err := proof.CheckOwnReceiptTx(t.Context(), proofLifecycle{}, source(t), operation(t, 191, "synthetic.record")); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("fixture unexpectedly certified own source")
	}
	if owner.calls != 1 || first.held != 1 || first.own != 1 || late.held+late.own != 0 {
		t.Fatal("registered proof changed with mutable owner")
	}
	if revision != 1 || outcome.Warnings[0] != "CLOCK_SKEW" {
		t.Fatal("proof mutated caller's immutable outcome")
	}
}
