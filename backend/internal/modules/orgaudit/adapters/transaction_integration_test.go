//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestBorrowedAuditTransactionDoesNotCommitCallerWork(t *testing.T) {
	f, s := newAuditFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	sentinel := errors.New("synthetic outer rollback")
	err := database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
		if replay, err := s.RecordChangeTx(t.Context(), tx, a.Scope(t), auditEvent(), changeRevision); err != nil || replay {
			t.Fatal(replay, err)
		}
		if _, err := s.RecordChangeTx(t.Context(), tx, b.Scope(t), auditEvent(), changeRevision); err == nil {
			t.Fatal("borrowed scope mismatch accepted")
		}
		return sentinel
	})
	if err != sentinel {
		t.Fatal(err)
	}
	assertAuditState(t, f, a, 0, 1)
	err = database.WithOrgTx(t.Context(), f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
		_, err := s.RecordChangeTx(t.Context(), tx, a.Scope(t), auditEvent(), changeRevision)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	assertAuditState(t, f, a, 1, 2)
	if _, err := s.RecordChangeTx(context.Background(), nil, a.Scope(t), auditEvent(), changeRevision); err == nil {
		t.Fatal("missing transaction accepted")
	}
}
