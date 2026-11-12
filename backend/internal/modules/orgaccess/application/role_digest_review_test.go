package application_test

import (
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

func TestRoleIntentDigestUsesTargetAndLosslessFiniteTimes(t *testing.T) {
	s := testScope(t)
	in := app.RoleAssignInput{OperationID: "7b0e2a54-0000-4000-8000-000000000010", MembershipID: "7b0e2a54-0000-4000-8000-000000000020", Role: d.Admin, Scope: d.OrgScope, TargetID: s.OrgID().String(), Reason: "Synthetic grant", ValidFrom: time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC)}
	base := in.Digest(s)
	other := in
	other.TargetID = "7b0e2a54-0000-4000-8000-000000000099"
	if other.Digest(s) == base {
		t.Fatal("target missing from command digest")
	}
	// UnixNano cannot represent the complete contract range. Dates separated by
	// its overflow period must still be different accepted command intents.
	other = in
	other.ValidFrom = in.ValidFrom.Add(time.Duration(1<<63 - 1)).Add(time.Duration(1<<63 - 1)).Add(2 * time.Nanosecond)
	if other.ValidFrom.UnixNano() != in.ValidFrom.UnixNano() || other.Digest(s) == base {
		t.Fatal("time overflow aliased command intent")
	}
	other = in
	other.ValidFrom = in.ValidFrom.In(time.FixedZone("synthetic", 3600))
	if other.Digest(s) != base {
		t.Fatal("equivalent instant has different intent")
	}
	other = in
	other.ValidFrom = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if other.Validate(s) == nil {
		t.Fatal("out-of-contract timestamp accepted")
	}
}
