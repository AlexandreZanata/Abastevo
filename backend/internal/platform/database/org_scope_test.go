package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

func TestWithOrgTxRejectsMissingScopeBeforeDatabaseAccess(t *testing.T) {
	err := database.WithOrgTx(context.Background(), nil, kernel.OrgScope{}, func(pgx.Tx) error {
		t.Fatal("missing scope reached the transaction callback")
		return nil
	})
	if !errors.Is(err, kernel.ErrInvalidOrgScope) {
		t.Fatalf("missing scope returned %v", err)
	}
}

func TestValidateOrgTxRejectsMissingProofBeforeDatabaseAccess(t *testing.T) {
	if err := database.ValidateOrgTx(t.Context(), nil, kernel.OrgScope{}); !errors.Is(err, kernel.ErrInvalidOrgScope) {
		t.Fatal(err)
	}
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000101")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.ValidateOrgTx(t.Context(), nil, scope); err != database.ErrInvalidOrgTx {
		t.Fatal(err)
	}
}
