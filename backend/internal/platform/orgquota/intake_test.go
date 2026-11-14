package orgquota

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func intakeDefinition() TransportDefinition {
	q := identity.OperationQuota{Limit: 3, Window: time.Hour}
	return TransportDefinition{Class: "synthetic.intake", Organization: q, Account: q, Device: q, Rationale: "Synthetic intake boundary; no production cap."}
}
func TestIntakeBudgetRequiresExplicitOwnedPolicyAndBoundedUnits(t *testing.T) {
	key := bytes.Repeat([]byte{11}, 32)
	d := intakeDefinition()
	for _, mutate := range []func(*TransportDefinition){func(d *TransportDefinition) { d.Class = "" }, func(d *TransportDefinition) { d.Rationale = "" }, func(d *TransportDefinition) { d.Organization.Limit = 0 }, func(d *TransportDefinition) { d.Account.Window = 25 * time.Hour }, func(d *TransportDefinition) { d.Device.Window = time.Nanosecond }} {
		bad := d
		mutate(&bad)
		if _, e := NewIntakeBudget(&pgxpool.Pool{}, key, bad); e == nil {
			t.Fatal("unowned intake policy accepted")
		}
	}
	b, e := NewIntakeBudget(&pgxpool.Pool{}, key, d)
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []int{-1, 51} {
		if e := b.CheckBatch(context.Background(), principal(t, orgA), n); e != orgapi.Unavailable {
			t.Fatal("unbounded units reached storage", e)
		}
	}
	p := principal(t, orgA)
	p.Channel = "web"
	p.DeviceKeyID = ""
	p.ContributorID = ""
	if e := b.CheckBatch(context.Background(), p, 1); e != orgapi.Unavailable {
		t.Fatal("cookie plane spent signed budget", e)
	}
	var missing *IntakeBudget
	if e := missing.CheckBatch(context.Background(), principal(t, orgA), 1); e != orgapi.Unavailable {
		t.Fatal("missing budget fallback", e)
	}
	for _, v := range []any{d, b, *b} {
		if !strings.Contains(fmt.Sprintf("%v %+v %#v", v, v, v), "redacted") {
			t.Fatal("transport policy diagnostic")
		}
	}
}
