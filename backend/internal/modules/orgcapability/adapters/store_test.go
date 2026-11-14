package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

func TestCapabilityAdapterRejectsInvalidOrIncompleteBoundary(t *testing.T) {
	catalog, _ := domain.NewCatalog([]string{"fuelings"})
	store := adapters.NewStore(nil, catalog, nil)
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(id)
	actor := domain.Actor{AccountID: id.String()}
	change := domain.Change{OperationID: id.String(), Capability: "fuelings", Enabled: true}
	if _, err := store.State(t.Context(), scope, "fuelings"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := store.State(t.Context(), scope, "unknown"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.State(t.Context(), kernel.OrgScope{}, "fuelings"); err == nil {
		t.Fatal("zero scope accepted")
	}
	if _, err := store.Set(t.Context(), scope, actor, change); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing audit accepted", err)
	}
	if _, err := store.Set(t.Context(), scope, domain.Actor{}, change); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	change.ExpectedRevision = -1
	if _, err := store.Set(t.Context(), scope, actor, change); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.Set(t.Context(), kernel.OrgScope{}, actor, change); err == nil {
		t.Fatal("zero scope accepted")
	}
	if err := store.GuardWrite(t.Context(), nil, scope, "fuelings"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if err := store.GuardWrite(t.Context(), nil, scope, "unknown"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	work := func(context.Context, pgx.Tx) error { return nil }
	if err := store.Run(t.Context(), scope, "fuelings", domain.Read, work); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := store.Run(t.Context(), scope, "unknown", domain.Write, work); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if err := store.Run(t.Context(), scope, "fuelings", domain.Write, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if err := store.Run(t.Context(), scope, "fuelings", domain.Manage, work); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestCapabilityTransactionStateRequiresExplicitValidBoundary(t *testing.T) {
	catalog, _ := domain.NewCatalog([]string{"organizations"})
	store := adapters.NewStore(nil, catalog, nil)
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	scope, _ := kernel.NewOrgScope(id)
	if _, e := store.StateTx(t.Context(), nil, scope, "organizations"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("missing transaction accepted", e)
	}
	if _, e := store.StateTx(t.Context(), nil, scope, "unknown"); !errors.Is(e, domain.ErrInvalid) {
		t.Fatal("unknown capability accepted", e)
	}
	if _, e := store.StateTx(t.Context(), nil, kernel.OrgScope{}, "organizations"); e == nil {
		t.Fatal("unscoped advisory read accepted")
	}
	var absent *adapters.Store
	if _, e := absent.StateTx(t.Context(), nil, scope, "organizations"); !errors.Is(e, domain.ErrUnavailable) {
		t.Fatal("missing source accepted", e)
	}
}
