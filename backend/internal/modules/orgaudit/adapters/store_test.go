package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

func auditCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	catalog, err := domain.NewCatalog([]domain.Definition{{
		Action: "org.settings.changed", TargetType: "org.settings", Reasons: []string{"correction"}, RequireReason: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func auditEvent() domain.Event {
	return domain.Event{
		OperationID: "7b0e2a54-0000-4000-8000-000000000010",
		ActorID:     "7b0e2a54-0000-4000-8000-000000000011",
		TargetID:    "7b0e2a54-0000-4000-8200-000000000001",
		Action:      "org.settings.changed", TargetType: "org.settings",
		ReasonCode: "correction", RequestHash: strings.Repeat("a", 64),
	}
}

func auditScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestInvalidAuditCannotReachDatabase(t *testing.T) {
	store := adapters.NewStore(nil, auditCatalog(t))
	work := func(context.Context, pgx.Tx) error { t.Fatal("invalid event executed work"); return nil }
	if _, err := store.RecordChange(t.Context(), kernel.OrgScope{}, auditEvent(), work); !errors.Is(err, kernel.ErrInvalidOrgScope) {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context(), kernel.OrgScope{}, domain.Filter{}); !errors.Is(err, kernel.ErrInvalidOrgScope) {
		t.Fatal(err)
	}
	event := auditEvent()
	event.ReasonCode = "synthetic receipt content"
	if _, err := store.RecordChange(t.Context(), auditScope(t), event, work); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.RecordChange(t.Context(), auditScope(t), auditEvent(), nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context(), auditScope(t), domain.Filter{Limit: 101}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestClosedPoolHasBoundedAuditErrors(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://fixture@127.0.0.1/fixture?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	store := adapters.NewStore(pool, auditCatalog(t))
	if _, err := store.RecordChange(t.Context(), auditScope(t), auditEvent(), func(context.Context, pgx.Tx) error {
		t.Fatal("closed pool ran work")
		return nil
	}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context(), auditScope(t), domain.Filter{}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
