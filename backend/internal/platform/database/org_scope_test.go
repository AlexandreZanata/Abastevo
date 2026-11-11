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
