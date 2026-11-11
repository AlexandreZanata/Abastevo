package adapters_test

import (
	"errors"
	"math"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scopeFor(t *testing.T, id string) kernel.OrgScope {
	t.Helper()
	value, err := kernel.ParseOrgID(id)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(value)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
func pending(t *testing.T, scope kernel.OrgScope, name string) domain.Organization {
	t.Helper()
	value, err := domain.NewPending(scope, domain.Profile{Name: name, TimeZone: "America/Cuiaba", Kinds: []domain.Kind{domain.Other, domain.Carrier, domain.Other}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func configured(t *testing.T, scope kernel.OrgScope, value domain.Organization, name string) domain.Organization {
	t.Helper()
	before, err := value.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	before.Profile.Name = name
	after, err := value.ConfigurePending(scope, before.Revision, before.Profile)
	if err != nil {
		t.Fatal(err)
	}
	return after
}
func TestRepositoryGuardsBeforeDatabaseAccess(t *testing.T) {
	ctx := t.Context()
	scope := scopeFor(t, "7b0e2a54-0000-4000-8000-000000000101")
	value := pending(t, scope, "Synthetic company")
	s := adapters.NewStore(nil)
	if _, err := s.Get(ctx, kernel.OrgScope{}); err != domain.ErrScope {
		t.Fatal(err)
	}
	if changed, err := s.Create(ctx, kernel.OrgScope{}, value); changed || err != domain.ErrScope {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, scope, 0, value); changed || err != domain.ErrInvalid {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, scope, math.MaxInt64, value); changed || err != domain.ErrRevision {
		t.Fatal(changed, err)
	}
	if changed, err := s.Create(ctx, scope, configured(t, scope, value, "Changed")); changed || err != domain.ErrInvalid {
		t.Fatal(changed, err)
	}
	other := scopeFor(t, "7b0e2a54-0000-4000-8000-000000000102")
	if _, err := s.Create(ctx, other, value); err != domain.ErrScope {
		t.Fatal(err)
	}
	if _, err := s.GetTx(ctx, nil, scope); err != app.ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := s.CreateTx(ctx, nil, scope, value); err != app.ErrUnavailable {
		t.Fatal(err)
	}
	if _, err := s.SaveTx(ctx, nil, scope, 1, configured(t, scope, value, "Changed")); err != app.ErrUnavailable {
		t.Fatal(err)
	}
	var absent *adapters.Store
	if _, err := absent.Get(ctx, scope); err != app.ErrUnavailable {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, "postgres://fixture@127.0.0.1/fixture?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	closed := adapters.NewStore(pool)
	if _, err := closed.Get(ctx, scope); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	if changed, err := closed.Create(ctx, scope, value); changed || err != app.ErrUnavailable {
		t.Fatal(changed, err)
	}
	if changed, err := closed.Save(ctx, scope, 1, configured(t, scope, value, "Changed")); changed || err != app.ErrUnavailable {
		t.Fatal(changed, err)
	}
}
