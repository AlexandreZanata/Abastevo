//go:build integration

package adapters_test

import (
	"testing"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// EMP-TRIP-005 (found by its native acceptance; B-BR-EMP-280..285, EDR-244):
// with the runtime's METER-003 plausibility on, a trip's own departure or
// final reading that the validator keeps aside as SUSPECT must still let the
// start and finish be accepted, like the meter and fueling intakes do. Before
// the fix both stayed RETRY/UNAVAILABLE forever and nothing was stored.
func TestTripIntakeAcceptsSuspectTripReadingsWithThePlausibilityPolicyOn(t *testing.T) {
	x := tripFixture(t)
	policy := meter.DefaultPlausibility()
	x.meters.Plausibility = &policy
	d := x.syncFixtureState
	// The opening reading is 0 m six hours before; 184 320 km now is an implausible rate.
	at := x.declaredNow(t)
	start := x.startOp(t, d, at, nil)
	ack, err := x.process(t, d, start)
	x.accepted(t, start, ack, err, 1, 1)
	x.replays(t, d, start, ack)
	finish := x.finishOp(t, d, tripOf(start), at.Add(time.Second), func(_, payload map[string]any) {
		// Below the departure: READING_BELOW_PREVIOUS on the same segment.
		payload["reading"].(map[string]any)["value"] = "184000000"
	})
	done, err := x.process(t, d, finish)
	x.accepted(t, finish, done, err, 2, 1)
	org := x.scope.OrgID().String()
	var states []string
	rows, err := x.f.Admin.Query(t.Context(), `SELECT r.state FROM org_meter_readings r WHERE r.org_id=$1 AND r.operation_id IN($2,$3) ORDER BY r.occurred_at`, org, start.ID(), finish.ID())
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			t.Fatal(err)
		}
		states = append(states, state)
	}
	rows.Close()
	if len(states) != 2 || states[0] != string(meter.Suspect) || states[1] != string(meter.Suspect) {
		t.Fatal("the trip readings were not kept aside as SUSPECT", states)
	}
	var completed bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT state='COMPLETED' FROM org_trips WHERE org_id=$1 AND id=$2`, org, tripOf(start)).Scan(&completed); err != nil || !completed {
		t.Fatal("the trip with suspect readings was not completed", err)
	}
	if done.Result() != protocol.Accepted {
		t.Fatal("finish result", done.Result())
	}
}
