package orgquota

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

const orgA = "7b0e2a54-0000-4000-8000-000000000001"
const orgB = "7b0e2a54-0000-4000-8000-000000000002"
const account = "7b0e2a54-0000-4000-8000-000000000011"
const device = "7b0e2a54-0000-4000-8000-000000000012"

var writeRule = orgapi.Rule{Permission: "org.settings.manage", Write: true}

func definitions() []Definition {
	q := identity.OperationQuota{Limit: 3, Window: time.Hour}
	return []Definition{{Class: "synthetic.command", Permissions: []string{"org.settings.manage"}, Organization: q, Account: q, Device: q, Rationale: "Synthetic boundary fixture; not a production policy."}}
}

func TestSourcePolicyAndKeyAreCopiedAndRedactFormatting(t *testing.T) {
	key := bytes.Repeat([]byte{5}, 32)
	defs := definitions()
	g, err := New(&pgxpool.Pool{}, key, defs)
	if err != nil {
		t.Fatal(err)
	}
	before := g.subjects("synthetic.command", principal(t, orgA))
	key[0] = 9
	defs[0].Permissions[0] = "changed.permission"
	defs[0].Organization.Limit = 999
	after := g.subjects("synthetic.command", principal(t, orgA))
	if before != after || g.Validate(writeRule) != nil {
		t.Fatal("caller changed active policy or subjects")
	}
	if g.Validate(orgapi.Rule{Permission: "changed.permission", Write: true}) == nil {
		t.Fatal("source mapping aliased caller")
	}
	for _, v := range []any{g, *g, Definition{Rationale: "PRIVATE-RATIONALE"}} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("quota", "value", v)
		out := fmt.Sprintf("%v %+v %#v", v, v, v) + log.String()
		if !strings.Contains(out, "redacted") || strings.Contains(out, "PRIVATE-") {
			t.Fatal("policy leaked")
		}
	}
}

func TestInvalidPrincipalCannotReachStorage(t *testing.T) {
	g := syntheticGuard(t)
	for _, kind := range []string{"scope", "membership", "account", "channel", "android key", "android owner", "web key", "web owner"} {
		p := principal(t, orgA)
		switch kind {
		case "scope":
			p.Scope = kernel.OrgScope{}
		case "membership":
			p.MembershipID = ""
		case "account":
			p.AccountID = ""
		case "channel":
			p.Channel = "untrusted"
		case "android key":
			p.DeviceKeyID = "untrusted"
		case "android owner":
			p.ContributorID = ""
		case "web key":
			p.Channel = "web"
			p.ContributorID = ""
		case "web owner":
			p.Channel = "web"
			p.DeviceKeyID = ""
		}
		if err := g.Check(context.Background(), p, writeRule); !errors.Is(err, orgapi.Unavailable) {
			t.Fatal(kind, err)
		}
	}
	if err := g.Check(context.Background(), orgapi.Principal{}, orgapi.Rule{}); err != nil {
		t.Fatal("read reached storage", err)
	}
	if err := g.Check(context.Background(), orgapi.Principal{}, orgapi.Rule{Write: true, Permission: "missing.permission"}); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal(err)
	}
	var missing *Guard
	if err := missing.Check(context.Background(), principal(t, orgA), writeRule); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal(err)
	}
}
func principal(t *testing.T, org string) orgapi.Principal {
	t.Helper()
	id, err := kernel.ParseOrgID(org)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return orgapi.Principal{Actor: orgapi.Actor{AccountID: account, ContributorID: account, DeviceKeyID: device, Channel: "android"}, Scope: s, MembershipID: account}
}
func syntheticGuard(t *testing.T) *Guard {
	t.Helper()
	// Policy/subject tests never access this unconnected pool.
	g, err := New(&pgxpool.Pool{}, []byte(strings.Repeat("s", 32)), definitions())
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func TestSubjectsArePrivateScopedAndDeviceRotationCannotChangeAggregateBuckets(t *testing.T) {
	g := syntheticGuard(t)
	p := principal(t, orgA)
	a := g.subjects("synthetic.command", p)
	p.DeviceKeyID = orgB
	b := g.subjects("synthetic.command", p)
	if a[0] != b[0] || a[1] != b[1] || a[2] == b[2] {
		t.Fatal("device rotation changed aggregate subjects")
	}
	p = principal(t, orgB)
	foreign := g.subjects("synthetic.command", p)
	otherClass := g.subjects("synthetic.other", principal(t, orgA))
	for i, subject := range a {
		if subject == foreign[i] || subject == otherClass[i] || strings.Contains(subject, orgA) || strings.Contains(subject, account) || strings.Contains(subject, device) {
			t.Fatal("quota subject leaked or crossed scopes")
		}
	}
	p = principal(t, orgA)
	p.Channel = "web"
	p.DeviceKeyID = ""
	p.ContributorID = ""
	web := g.subjects("synthetic.command", p)
	if web[0] != a[0] || web[1] != a[1] || web[2] == a[2] {
		t.Fatal("web scope did not share aggregate budgets")
	}
}
func TestRegistrationRejectsMissingUnknownOrInvalidBudgets(t *testing.T) {
	for _, kind := range []string{"pool", "key", "empty", "class", "permission", "rationale", "org", "account", "device", "precision", "retention", "duplicate class", "duplicate permission"} {
		t.Run(kind, func(t *testing.T) {
			pool := &pgxpool.Pool{}
			key := []byte(strings.Repeat("s", 32))
			defs := definitions()
			switch kind {
			case "pool":
				pool = nil
			case "key":
				key = nil
			case "empty":
				defs = nil
			case "class":
				defs[0].Class = "private free text"
			case "permission":
				defs[0].Permissions = []string{"private text"}
			case "rationale":
				defs[0].Rationale = ""
			case "org":
				defs[0].Organization.Limit = 0
			case "account":
				defs[0].Account.Window = 0
			case "device":
				defs[0].Device.Limit = -1
			case "precision":
				defs[0].Device.Window = time.Nanosecond
			case "retention":
				defs[0].Device.Window = 25 * time.Hour
			case "duplicate class":
				defs = append(defs, defs[0])
			case "duplicate permission":
				defs = append(defs, defs[0])
				defs[1].Class = "synthetic.other"
			}
			if _, err := New(pool, key, defs); err == nil {
				t.Fatal("invalid quota policy accepted")
			}
		})
	}
	g := syntheticGuard(t)
	if err := g.Validate(orgapi.Rule{Write: true, Permission: "missing.permission"}); err == nil {
		t.Fatal("unknown policy accepted")
	}
	if err := g.Validate(orgapi.Rule{Permission: "missing.permission"}); err != nil {
		t.Fatal("read incorrectly requires write quota")
	}
	p := principal(t, orgA)
	p.Actor.AccountID = "private-invalid"
	if err := g.Check(context.Background(), p, orgapi.Rule{Write: true, Permission: "org.settings.manage"}); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal(err)
	}
}
