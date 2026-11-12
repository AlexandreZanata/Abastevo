package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

type authority struct {
	calls  int
	denied bool
	last   domain.Intent
}

func (a *authority) Authorize(_ context.Context, _ kernel.OrgScope, _ domain.Actor, _ string, intent domain.Intent) error {
	a.calls++
	a.last = intent
	if a.denied {
		return domain.ErrDenied
	}
	return nil
}

type repository struct {
	calls   int
	flag    domain.State
	failure error
	replay  bool
}

func (r *repository) GuardWrite(context.Context, pgx.Tx, kernel.OrgScope, string) error {
	r.calls++
	if r.failure != nil {
		return r.failure
	}
	return r.flag.CheckWrite()
}

func (r *repository) State(context.Context, kernel.OrgScope, string) (domain.State, error) {
	r.calls++
	return r.flag, r.failure
}
func (r *repository) Set(context.Context, kernel.OrgScope, domain.Actor, domain.Change) (bool, error) {
	r.calls++
	return r.replay, r.failure
}
func (r *repository) Run(ctx context.Context, _ kernel.OrgScope, _ string, intent domain.Intent, work func(context.Context, pgx.Tx) error) error {
	r.calls++
	if r.failure != nil {
		return r.failure
	}
	if intent == domain.Write {
		if err := r.flag.CheckWrite(); err != nil {
			return err
		}
	}
	return work(ctx, nil)
}

func fixture(t *testing.T) (application.Service, *authority, *repository, kernel.OrgScope, domain.Actor) {
	t.Helper()
	catalog, _ := domain.NewCatalog([]string{"fuelings"})
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(id)
	a, r := &authority{}, &repository{}
	return application.Service{Enabled: true, Catalog: catalog, Authorizer: a, Repository: r}, a, r, scope, domain.Actor{AccountID: id.String()}
}

func TestPerOrganizationDisableRetainsAuthorizedReadsAndExport(t *testing.T) {
	s, a, r, scope, actor := fixture(t)
	calls := 0
	work := func(context.Context, pgx.Tx) error { calls++; return nil }
	for _, intent := range []domain.Intent{domain.Read, domain.Export} {
		if err := s.Run(t.Context(), scope, actor, "fuelings", intent, work); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal("disabled capability lost reads or exports")
	}
	if err := s.Run(t.Context(), scope, actor, "fuelings", domain.Write, work); !errors.Is(err, domain.ErrEntitlementRequired) {
		t.Fatal(err)
	}
	r.flag = domain.State{Enabled: true, Revision: 1}
	if err := s.Run(t.Context(), scope, actor, "fuelings", domain.Write, work); err != nil {
		t.Fatal(err)
	}
	a.denied = true
	for _, intent := range []domain.Intent{domain.Read, domain.Export, domain.Write} {
		if err := s.Run(t.Context(), scope, actor, "fuelings", intent, work); !errors.Is(err, domain.ErrDenied) {
			t.Fatal("enabled capability granted a role", err)
		}
	}
	if calls != 3 || r.calls != 4 || a.calls != 7 {
		t.Fatalf("work=%d repository=%d auth=%d", calls, r.calls, a.calls)
	}
}

func TestGlobalOffAndAuthorizationAreIndependentOfProvisioning(t *testing.T) {
	s, a, r, scope, actor := fixture(t)
	s.Enabled = false
	for _, intent := range []domain.Intent{domain.Read, domain.Export, domain.Write} {
		if err := s.Run(t.Context(), scope, actor, "fuelings", intent, func(context.Context, pgx.Tx) error { t.Fatal("global off ran work"); return nil }); !errors.Is(err, domain.ErrSurfaceDisabled) {
			t.Fatal(err)
		}
	}
	if _, err := s.State(t.Context(), scope, actor, "fuelings"); !errors.Is(err, domain.ErrSurfaceDisabled) {
		t.Fatal(err)
	}
	if a.calls != 0 || r.calls != 0 {
		t.Fatal("hidden surface reached ports")
	}
	change := domain.Change{OperationID: actor.AccountID, Capability: "fuelings", Enabled: true}
	if _, err := s.Set(t.Context(), scope, actor, change); err != nil || a.last != domain.Manage {
		t.Fatal("operator provisioning unavailable", err)
	}
	a.denied = true
	if _, err := s.Set(t.Context(), scope, actor, change); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("unauthorized operator", err)
	}
	if r.calls != 1 {
		t.Fatal("denied operator reached storage")
	}
}

func TestCapabilityServiceFailsClosed(t *testing.T) {
	s, a, r, scope, actor := fixture(t)
	work := func(context.Context, pgx.Tx) error { return nil }
	for _, code := range []string{"", "unknown"} {
		if err := s.Run(t.Context(), scope, actor, code, domain.Write, work); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if err := s.Run(t.Context(), scope, actor, "fuelings", domain.Manage, work); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Run(t.Context(), scope, actor, "fuelings", domain.Write, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.State(t.Context(), kernel.OrgScope{}, actor, "fuelings"); err == nil {
		t.Fatal("zero scope accepted")
	}
	if _, err := s.State(t.Context(), scope, domain.Actor{}, "fuelings"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := s.Set(t.Context(), scope, actor, domain.Change{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if a.calls != 0 || r.calls != 0 {
		t.Fatal("invalid input reached ports")
	}
	s.Authorizer = nil
	if _, err := s.State(t.Context(), scope, actor, "fuelings"); !errors.Is(err, domain.ErrDenied) {
		t.Fatal(err)
	}
	s.Authorizer = a
	s.Repository = nil
	if _, err := s.State(t.Context(), scope, actor, "fuelings"); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	s.Repository = r
	r.failure = errors.New("synthetic storage failure")
	if _, err := s.State(t.Context(), scope, actor, "fuelings"); !errors.Is(err, r.failure) {
		t.Fatal(err)
	}
	r.failure = nil
	r.replay = true
	if replay, err := s.Set(t.Context(), scope, actor, domain.Change{OperationID: actor.AccountID, Capability: "fuelings"}); err != nil || !replay {
		t.Fatal(replay, err)
	}
}

// The repository owns SQL; this token ensures authority is rechecked before it.
type transaction struct{ pgx.Tx }

func TestExistingTransactionGuardRechecksAuthorityAndCurrentFlag(t *testing.T) {
	s, a, r, scope, actor := fixture(t)
	tx := transaction{}
	if err := s.GuardWrite(t.Context(), nil, scope, actor, "fuelings"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	s.Enabled = false
	if err := s.GuardWrite(t.Context(), tx, scope, actor, "fuelings"); !errors.Is(err, domain.ErrSurfaceDisabled) {
		t.Fatal(err)
	}
	s.Enabled = true
	if err := s.GuardWrite(t.Context(), tx, scope, actor, "unknown"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.GuardWrite(t.Context(), tx, scope, actor, "fuelings"); !errors.Is(err, domain.ErrEntitlementRequired) {
		t.Fatal(err)
	}
	r.flag = domain.State{Enabled: true, Revision: 1}
	if err := s.GuardWrite(t.Context(), tx, scope, actor, "fuelings"); err != nil {
		t.Fatal(err)
	}
	a.denied = true
	if err := s.GuardWrite(t.Context(), tx, scope, actor, "fuelings"); !errors.Is(err, domain.ErrDenied) {
		t.Fatal(err)
	}
	if r.calls != 2 || a.calls != 3 {
		t.Fatal("denied guard reached transaction", r.calls, a.calls)
	}
}
