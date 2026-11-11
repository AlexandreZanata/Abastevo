package orgapi_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestOrganizationCursorsBindEveryPrivateDimension(t *testing.T) {
	gateway, _ := gatewayFixture(t)
	id, err := kernel.ParseOrgID(orgA)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	principal := orgapi.Principal{Actor: orgapi.Actor{AccountID: accountID}, Scope: scope, MembershipID: membershipID}
	token, err := gateway.SealCursor(principal, route, "filter", "after")
	if err != nil {
		t.Fatal(err)
	}
	if key, err := gateway.OpenCursor(principal, route, "filter", token); err != nil || key != "after" {
		t.Fatal("valid cursor failed")
	}
	if key, err := gateway.OpenCursor(principal, route, "filter", ""); err != nil || key != "" {
		t.Fatal("first page failed")
	}
	otherID, err := kernel.ParseOrgID(orgB)
	if err != nil {
		t.Fatal(err)
	}
	otherScope, err := kernel.NewOrgScope(otherID)
	if err != nil {
		t.Fatal(err)
	}
	otherOrg, otherAccount, otherMember := principal, principal, principal
	otherOrg.Scope = otherScope
	otherAccount.AccountID = familyID
	otherMember.MembershipID = familyID
	for _, p := range []orgapi.Principal{otherOrg, otherAccount, otherMember, {}} {
		if _, err := gateway.OpenCursor(p, route, "filter", token); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("foreign cursor accepted")
		}
	}
	for _, query := range [][2]string{{route + "/other", "filter"}, {route, "other-filter"}, {route, "filter\x00other"}} {
		if _, err := gateway.OpenCursor(principal, query[0], query[1], token); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("changed route or filters accepted")
		}
	}
	changedOrigin := gateway
	changedOrigin.Origin = "https://other.example.invalid"
	if _, err := changedOrigin.OpenCursor(principal, route, "filter", token); !errors.Is(err, orgapi.Malformed) {
		t.Fatal("foreign origin accepted")
	}
	for _, value := range []string{"malformed", token[:len(token)-1] + "!", strings.Repeat("x", 2049)} {
		if _, err := gateway.OpenCursor(principal, route, "filter", value); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("invalid cursor accepted")
		}
	}
	at := gateway.Now()
	gateway.Now = func() time.Time { return at.Add(httpapi.CursorTTL) }
	if _, err := gateway.OpenCursor(principal, route, "filter", token); !errors.Is(err, orgapi.Malformed) {
		t.Fatal("cursor accepted at its expiry deadline")
	}
}

func TestPageBounds(t *testing.T) {
	if p, err := (orgapi.Page{}).Normalize(); err != nil || p.Limit != 20 {
		t.Fatal("page default changed")
	}
	for _, p := range []orgapi.Page{{Limit: -1}, {Limit: 101}, {Cursor: strings.Repeat("x", 2049)}} {
		if _, err := p.Normalize(); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("unbounded page accepted")
		}
	}
}
