package adapters_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

type accessRunner struct {
	calls    int
	event    domain.Event
	replayed bool
	failure  error
}

func (r *accessRunner) RecordChange(_ context.Context, _ kernel.OrgScope, event domain.Event, _ func(context.Context, pgx.Tx) error) (bool, error) {
	r.calls++
	r.event = event
	return r.replayed, r.failure
}

func documentRead() domain.SensitiveAccess {
	return domain.SensitiveAccess{
		Category: domain.DocumentContent, Stage: domain.AccessServed,
		AccountID: "7b0e2a54-0000-4000-8000-000000000031", MembershipID: "7b0e2a54-0000-4000-8000-000000000032",
		TargetID: "7b0e2a54-0000-4000-8000-000000000033", AccessID: "7b0e2a54-0000-4000-8000-000000000034",
		RequestID: "7b0e2a54-0000-4000-8000-000000000035", Binding: strings.Repeat("d", 64),
	}
}

// B-BR-EMP-208/210: the recorder writes exactly the derived event and every
// failure, conflict or replay is a refusal, so the caller serves nothing.
func TestAccessLogFailsClosedOnEveryRecordOutcome(t *testing.T) {
	scope := auditScope(t)
	runner := &accessRunner{}
	log := adapters.AccessLog{Runner: runner}
	item := documentRead()
	want, err := item.Event(scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.RecordAccess(t.Context(), scope, item); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || runner.event != want {
		t.Fatalf("recorded %+v, want %+v", runner.event, want)
	}
	for name, setup := range map[string]struct {
		replayed bool
		failure  error
		want     error
	}{
		"replay":      {replayed: true, want: domain.ErrReplayed},
		"reuse":       {failure: domain.ErrReused, want: domain.ErrReused},
		"invalid":     {failure: domain.ErrInvalid, want: domain.ErrInvalid},
		"unavailable": {failure: errors.New("SYNTHETIC-PRIVATE-DATABASE-DETAIL"), want: domain.ErrUnavailable},
		"canceled":    {failure: context.Canceled, want: domain.ErrUnavailable},
	} {
		runner.replayed, runner.failure = setup.replayed, setup.failure
		err := log.RecordAccess(t.Context(), scope, item)
		if !errors.Is(err, setup.want) || strings.Contains(err.Error(), "SYNTHETIC") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	runner.calls = 0
	invalid := item
	invalid.TargetID = "-23.550520,-46.633308"
	if err := log.RecordAccess(t.Context(), scope, invalid); !errors.Is(err, domain.ErrInvalid) || runner.calls != 0 {
		t.Fatal("invalid access reached the runner", err)
	}
	if err := log.RecordAccess(t.Context(), kernel.OrgScope{}, item); !errors.Is(err, domain.ErrInvalid) || runner.calls != 0 {
		t.Fatal("unscoped access reached the runner", err)
	}
	if err := (adapters.AccessLog{}).RecordAccess(t.Context(), scope, item); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("missing runner did not fail closed", err)
	}
}
