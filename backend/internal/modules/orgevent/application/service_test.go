package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/jackc/pgx/v5"
)

const id = "7b0e2a54-0000-4000-8000-000000000001"

type runner struct {
	failure error
	replay  bool
	calls   int
	bound   audit.Event
}

func (r *runner) RecordChange(ctx context.Context, s kernel.OrgScope, a audit.Event, work func(context.Context, pgx.Tx) error) (bool, error) {
	r.calls++
	r.bound = a
	if r.failure != nil {
		return false, r.failure
	}
	if r.replay {
		return true, nil
	}
	return false, work(ctx, nil)
}

type writer struct {
	failure   error
	envelopes []domain.Envelope
}

func (w *writer) Enqueue(_ context.Context, _ kernel.OrgScope, _ pgx.Tx, e domain.Envelope, _ string) error {
	w.envelopes = append(w.envelopes, e)
	return w.failure
}
func fixture(t *testing.T) (application.Service, *runner, *writer, kernel.OrgScope, audit.Event, domain.Event) {
	t.Helper()
	org, err := kernel.ParseOrgID(id)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewCatalog([]domain.Definition{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh", "review.prepare"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, w := &runner{}, &writer{}
	return application.Service{Enabled: true, Catalog: c, Audit: r, Writer: w}, r, w, scope,
		audit.Event{OperationID: id, ActorID: id, Action: "example.record.changed", TargetType: "example.record", TargetID: id, RequestHash: strings.Repeat("a", 64)},
		domain.Event{ID: id, Name: "example.record.changed", TargetType: "example.record", TargetID: id, Revision: 1}
}
func TestProducerSharesAuditAndCallbackAndSkipsReplayedWork(t *testing.T) {
	s, r, w, scope, a, e := fixture(t)
	workCalls := 0
	work := func(context.Context, kernel.OrgScope, pgx.Tx) error { workCalls++; return nil }
	replayed, err := s.Record(t.Context(), scope, a, e, []string{"review.prepare", "projection.refresh"}, work)
	if err != nil || replayed || workCalls != 1 || len(w.envelopes) != 2 || r.bound.RequestHash == a.RequestHash {
		t.Fatal("incomplete atomic intent", err)
	}
	if w.envelopes[0].Consumer != "projection.refresh" || w.envelopes[0].OrgID != id {
		t.Fatal("not canonical/scoped")
	}
	r.replay = true
	replayed, err = s.Record(t.Context(), scope, a, e, []string{"projection.refresh", "review.prepare"}, work)
	if err != nil || !replayed || workCalls != 1 || len(w.envelopes) != 2 {
		t.Fatal("replay repeated work", err)
	}
}
func TestProducerFailsClosedWithoutPortsOrAuthority(t *testing.T) {
	for _, kind := range []string{"disabled", "catalog", "runner", "writer", "callback", "write failure", "enqueue failure", "audit failure", "reuse", "denied"} {
		t.Run(kind, func(t *testing.T) {
			s, r, w, scope, a, e := fixture(t)
			work := application.Work(func(context.Context, kernel.OrgScope, pgx.Tx) error { return nil })
			want := domain.ErrUnavailable
			switch kind {
			case "disabled":
				s.Enabled = false
				want = domain.ErrDisabled
			case "catalog":
				e.Revision = 0
				want = domain.ErrInvalid
			case "runner":
				s.Audit = nil
			case "writer":
				s.Writer = nil
			case "callback":
				work = nil
				want = domain.ErrInvalid
			case "write failure":
				work = func(context.Context, kernel.OrgScope, pgx.Tx) error { return errors.New("PRIVATE-SQL-CONTENT") }
			case "enqueue failure":
				w.failure = errors.New("PRIVATE-QUEUE-CONTENT")
			case "audit failure":
				r.failure = errors.New("PRIVATE-AUDIT-CONTENT")
			case "reuse":
				r.failure = audit.ErrReused
				want = domain.ErrReused
			case "denied":
				work = func(context.Context, kernel.OrgScope, pgx.Tx) error { return domain.ErrDenied }
				want = domain.ErrDenied
			}
			if _, err := s.Record(t.Context(), scope, a, e, []string{"projection.refresh"}, work); !errors.Is(err, want) {
				t.Fatal(err, want)
			}
		})
	}
}
