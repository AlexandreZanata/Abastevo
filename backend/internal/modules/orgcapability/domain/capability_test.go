package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

const id = "7b0e2a54-0000-4000-8000-000000000001"

func TestRegisteredCapabilitiesAndClosedDefaults(t *testing.T) {
	codes := []string{"fuelings", "expenses"}
	catalog, err := domain.NewCatalog(codes)
	if err != nil {
		t.Fatal(err)
	}
	codes[0] = "unregistered"
	if err := catalog.Validate("fuelings"); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"", "unregistered", "Fuelings", "fuelings ", "fuelings\n", "document/secret", string(make([]byte, 65))} {
		if !errors.Is(catalog.Validate(code), domain.ErrInvalid) {
			t.Fatalf("accepted %q", code)
		}
	}
	for _, codes := range [][]string{nil, {""}, {"fuelings", "fuelings"}, {"api_key=secret"}} {
		if _, err := domain.NewCatalog(codes); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("accepted %q", codes)
		}
	}
	if err := (domain.Catalog{}).Validate("fuelings"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if !errors.Is((domain.State{}).CheckWrite(), domain.ErrEntitlementRequired) {
		t.Fatal("absent flag allowed a write")
	}
	if err := (domain.State{Enabled: true, Revision: 1}).CheckWrite(); err != nil {
		t.Fatal(err)
	}
}

func TestActorChangeAndCanonicalDigest(t *testing.T) {
	catalog, _ := domain.NewCatalog([]string{"fuelings", "expenses"})
	actor := domain.Actor{AccountID: id}
	if err := actor.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []domain.Actor{{}, {AccountID: "00000000-0000-0000-0000-000000000000"}, {AccountID: id, DelegationID: "bad"}} {
		if !errors.Is(actor.Validate(), domain.ErrInvalid) {
			t.Fatal("invalid actor accepted")
		}
	}
	change := domain.Change{OperationID: id, Capability: "fuelings", Enabled: true}
	if err := change.Validate(catalog); err != nil {
		t.Fatal(err)
	}
	for _, altered := range []domain.Change{
		{Capability: "fuelings"}, {OperationID: id, Capability: "unknown"},
		{OperationID: id, Capability: "fuelings", ExpectedRevision: -1},
		{OperationID: id, Capability: "fuelings", ExpectedRevision: math.MaxInt64},
	} {
		if !errors.Is(altered.Validate(catalog), domain.ErrInvalid) {
			t.Fatal("invalid change accepted")
		}
	}
	org, _ := kernel.ParseOrgID(id)
	scope, _ := kernel.NewOrgScope(org)
	digest, err := change.Digest(scope)
	if err != nil || len(digest) != 64 {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
	if second, _ := change.Digest(scope); second != digest {
		t.Fatal("unstable digest")
	}
	for _, altered := range []domain.Change{
		{OperationID: id, Capability: "expenses", Enabled: true},
		{OperationID: id, Capability: "fuelings", Enabled: false},
		{OperationID: id, Capability: "fuelings", Enabled: true, ExpectedRevision: 1},
		{OperationID: "7b0e2a54-0000-4000-8000-000000000002", Capability: "fuelings", Enabled: true},
	} {
		if other, _ := altered.Digest(scope); other == digest {
			t.Fatal("digest omitted change input")
		}
	}
	otherOrg, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000002")
	otherScope, _ := kernel.NewOrgScope(otherOrg)
	if other, _ := change.Digest(otherScope); other == digest {
		t.Fatal("digest omitted organization")
	}
	if _, err := change.Digest(kernel.OrgScope{}); err == nil {
		t.Fatal("zero scope accepted")
	}
}
