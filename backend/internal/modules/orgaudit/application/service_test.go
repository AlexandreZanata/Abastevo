package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

type readPolicy struct {
	allow  bool
	calls  int
	access application.ReadAccess
}

func (policy *readPolicy) AuthorizeAuditRead(_ context.Context, _ kernel.OrgScope, access application.ReadAccess) error {
	policy.calls++
	policy.access = access
	if !policy.allow {
		return domain.ErrDenied
	}
	return nil
}

type reader struct {
	calls  int
	scope  kernel.OrgScope
	filter domain.Filter
}

func (store *reader) List(_ context.Context, scope kernel.OrgScope, filter domain.Filter) ([]domain.Record, error) {
	store.calls++
	store.scope, store.filter = scope, filter
	return []domain.Record{}, nil
}

func TestReadRequiresCurrentAuthorityBeforeStorage(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	access := application.ReadAccess{ActorID: "7b0e2a54-0000-4000-8000-000000000011"}
	policy, store := &readPolicy{}, &reader{}
	service := application.Service{Reader: store, Authorizer: policy}
	if _, err := service.List(t.Context(), scope, access, domain.Filter{}); !errors.Is(err, domain.ErrDenied) || store.calls != 0 {
		t.Fatal("denied read reached storage")
	}
	policy.allow = true
	access.SupportSessionID = "7b0e2a54-0000-4000-8000-000000000012"
	if _, err := service.List(t.Context(), scope, access, domain.Filter{}); err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.scope != scope || store.filter.Limit != 20 || policy.access != access {
		t.Fatal("read lost scope, bounds or support attribution")
	}
	policy.allow = false
	if _, err := service.List(t.Context(), scope, access, domain.Filter{}); !errors.Is(err, domain.ErrDenied) ||
		store.calls != 1 || policy.calls != 3 {
		t.Fatal("revoked authority reused an earlier read decision")
	}

	for _, scenario := range []struct {
		name   string
		scope  kernel.OrgScope
		access application.ReadAccess
		filter domain.Filter
	}{
		{"missing scope", kernel.OrgScope{}, access, domain.Filter{}},
		{"missing actor", scope, application.ReadAccess{}, domain.Filter{}},
		{"invalid support reference", scope, application.ReadAccess{ActorID: access.ActorID, SupportSessionID: "bad"}, domain.Filter{}},
		{"unbounded page", scope, access, domain.Filter{Limit: 101}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := service.List(t.Context(), scenario.scope, scenario.access, scenario.filter); err == nil {
				t.Fatal("invalid read succeeded")
			}
			if store.calls != 1 || policy.calls != 3 {
				t.Fatal("invalid read reached a port")
			}
		})
	}
	service.Authorizer = nil
	if _, err := service.List(t.Context(), scope, access, domain.Filter{}); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("missing authority was allowed")
	}
	service.Authorizer, service.Reader, policy.allow = policy, nil, true
	if _, err := service.List(t.Context(), scope, access, domain.Filter{}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing reader succeeded")
	}
}
