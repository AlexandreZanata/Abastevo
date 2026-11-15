package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type outcomes struct {
	calls int
	err   error
}

func (o *outcomes) SettleReservation(_ context.Context, _ kernel.OrgScope, _ string, _ fleet.ReservationCommand) (fleet.ReservationState, error) {
	o.calls++
	return fleet.ReservationExpired, o.err
}

// EDR-193: the exact v1 envelope, one de-duplication key per outcome, static
// errors only, and nothing runs outside the envelope.
func TestReservationOutcomeJobEnvelope(t *testing.T) {
	org, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	id := "7b0e2a54-0000-4000-8000-000000009201"
	expire, miss := ReservationOutcomeDedupe(fleet.ExpireReservation, scope, id), ReservationOutcomeDedupe(fleet.MissReservation, scope, id)
	if expire == "" || miss == "" || expire == miss || ReservationOutcomeDedupe(fleet.FulfilReservation, scope, id) != "" || ReservationOutcomeDedupe(fleet.ExpireReservation, scope, "x") != "" {
		t.Fatal("de-duplication keys", expire, miss)
	}
	payload, _ := json.Marshal(ReservationOutcomePayload{Version: 1, OrgID: org.String(), ReservationID: id, Outcome: "EXPIRE"})
	gotScope, gotID, command, err := ParseReservationOutcome(payload)
	if err != nil || gotScope != scope || gotID != id || command != fleet.ExpireReservation {
		t.Fatal("envelope", err)
	}
	for _, raw := range []string{`{"version":2,"org_id":"7b0e2a54-0000-4000-8000-000000000001","reservation_id":"7b0e2a54-0000-4000-8000-000000009201","outcome":"EXPIRE"}`, `{"version":1,"org_id":"7b0e2a54-0000-4000-8000-000000000001","reservation_id":"7b0e2a54-0000-4000-8000-000000009201","outcome":"FULFIL"}`, `{"version":1,"org_id":"7b0e2a54-0000-4000-8000-000000000001","reservation_id":"7b0e2a54-0000-4000-8000-000000009201","outcome":"EXPIRE","extra":1}`} {
		if _, _, _, err := ParseReservationOutcome([]byte(raw)); err == nil {
			t.Fatal("invalid envelope accepted", raw)
		}
	}
	port := &outcomes{}
	handler := ReservationOutcomeHandler{Outcomes: port}
	job := jobs.Job{Kind: ReservationOutcomeKind, Payload: payload, DedupeKey: expire}
	if err := handler.Handle(t.Context(), job); err != nil || port.calls != 1 {
		t.Fatal("handle", err)
	}
	for _, c := range []struct {
		err, want error
	}{{app.ErrNotFound, nil}, {app.ErrNotDue, app.ErrNotDue}, {errors.New("provider text"), app.ErrUnavailable}} {
		port.err = c.err
		if got := handler.Handle(t.Context(), job); !errors.Is(got, c.want) && !(c.want == nil && got == nil) {
			t.Fatal("handler error mapping", c.err, got)
		}
	}
	calls := port.calls
	for _, bad := range []jobs.Job{{Kind: "other", Payload: payload, DedupeKey: expire}, {Kind: ReservationOutcomeKind, Payload: payload, DedupeKey: miss}} {
		if handler.Handle(t.Context(), bad) == nil {
			t.Fatal("job outside its envelope ran")
		}
	}
	if port.calls != calls {
		t.Fatal("a refused job reached the store")
	}
}
