package adapters_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"testing"
)

func TestRepositoryValidatesBeforeDatabaseAccess(t *testing.T) {
	ctx := t.Context()
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	e, err := d.NewEmployee(scope, "7b0e2a54-0000-4000-8000-000000000501")
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.NewInvited(scope, "7b0e2a54-0000-4000-8000-000000000601", e, d.ExistingAccount)
	if err != nil {
		t.Fatal(err)
	}
	s := adapters.NewStore(nil)
	if _, err := s.Get(ctx, kernel.OrgScope{}, "bad"); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := s.GetEmployee(ctx, scope, "bad"); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.ActiveByAccount(ctx, scope, ""); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if changed, err := s.Create(ctx, scope, d.Membership{}); changed || err != d.ErrScope {
		t.Fatal(changed, err)
	}
	if changed, err := s.CreateEmployee(ctx, scope, d.Employee{}); changed || err != d.ErrScope {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, scope, 0, m); changed || err != d.ErrInvalid {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, scope, math.MaxInt64, m); changed || err != d.ErrRevision {
		t.Fatal(changed, err)
	}
	if changed, err := s.Save(ctx, kernel.OrgScope{}, 1, m); changed || err != d.ErrScope {
		t.Fatal(changed, err)
	}
	inactive, err := e.Transition(scope, 1, d.EmployeeInactive, d.EmployeeConditions{OwnershipProtected: true, ReasonRecorded: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEmployee(ctx, scope, inactive); err != d.ErrInvalid {
		t.Fatal(err)
	}
	end, err := m.Transition(scope, 1, d.MembershipEnded, d.MembershipConditions{EndReason: d.AdmissionEnded})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, scope, end); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.GetTx(ctx, nil, scope, "bad"); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.GetEmployeeTx(ctx, nil, scope, ""); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.ActiveByAccountTx(ctx, nil, scope, ""); err != d.ErrInvalid {
		t.Fatal(err)
	}
	if _, err := s.CreateTx(ctx, nil, scope, d.Membership{}); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := s.CreateEmployeeTx(ctx, nil, scope, d.Employee{}); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := s.SaveTx(ctx, nil, scope, 0, m); err != d.ErrInvalid {
		t.Fatal(err)
	}
	for _, check := range []func(*adapters.Store) error{
		func(s *adapters.Store) error {
			_, err := s.Get(ctx, scope, "7b0e2a54-0000-4000-8000-000000000601")
			return err
		},
		func(s *adapters.Store) error {
			_, err := s.GetEmployee(ctx, scope, "7b0e2a54-0000-4000-8000-000000000501")
			return err
		},
		func(s *adapters.Store) error {
			_, err := s.ActiveByAccount(ctx, scope, "7b0e2a54-0000-4000-8000-000000000401")
			return err
		},
		func(s *adapters.Store) error { _, err := s.Create(ctx, scope, m); return err },
		func(s *adapters.Store) error { _, err := s.CreateEmployee(ctx, scope, e); return err },
		func(s *adapters.Store) error { _, err := s.Save(ctx, scope, 1, end); return err },
	} {
		if err := check(s); err != app.ErrUnavailable {
			t.Fatal(err)
		}
		if err := check(nil); err != app.ErrUnavailable {
			t.Fatal(err)
		}
	}
	pool, err := pgxpool.New(ctx, "postgres://fixture@127.0.0.1/fixture?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	if _, err := adapters.NewStore(pool).Create(ctx, scope, m); err != app.ErrUnavailable {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { _, err := s.GetTx(ctx, nil, scope, "7b0e2a54-0000-4000-8000-000000000601"); return err },
		func() error {
			_, err := s.GetEmployeeTx(ctx, nil, scope, "7b0e2a54-0000-4000-8000-000000000501")
			return err
		},
		func() error {
			_, err := s.ActiveByAccountTx(ctx, nil, scope, "7b0e2a54-0000-4000-8000-000000000401")
			return err
		},
		func() error { _, err := s.CreateTx(ctx, nil, scope, m); return err },
		func() error { _, err := s.CreateEmployeeTx(ctx, nil, scope, e); return err },
		func() error { _, err := s.SaveTx(ctx, nil, scope, 1, end); return err },
	} {
		if err := check(); err != app.ErrUnavailable {
			t.Fatal(err)
		}
	}
}
