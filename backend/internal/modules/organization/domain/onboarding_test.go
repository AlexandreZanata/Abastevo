package domain_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func creationInput() d.CreationInput {
	return d.CreationInput{OperationID: "7b0e2a54-0000-4000-8000-000000001001", Profile: d.Profile{Name: " Synthetic company ", TimeZone: "America/Cuiaba", Kinds: []d.Kind{d.Other, d.Carrier, d.Other}}, UnitName: " Main synthetic unit ", Terms: d.TermsAcceptance{Version: d.SyntheticTermsVersion, Digest: d.SyntheticTermsDigest, Accepted: true}}
}
func TestCreationCanonicalizesIntentAndPinsSyntheticTerms(t *testing.T) {
	raw, err := os.ReadFile("../../../../../contracts/testdata/enterprise/synthetic-organization-terms-v1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != d.SyntheticTermsDigest {
		t.Fatal("synthetic terms changed without version/digest revision")
	}
	input := creationInput()
	value, err := d.NewCreation(input)
	if err != nil {
		t.Fatal(err)
	}
	if value.Profile().Name != "Synthetic company" || value.UnitName() != "Main synthetic unit" || len(value.Profile().Kinds) != 2 {
		t.Fatal("noncanonical creation")
	}
	input.Profile.Name = "Synthetic company"
	input.UnitName = "Main synthetic unit"
	input.Profile.Kinds = []d.Kind{d.Carrier, d.Other}
	replay, err := d.NewCreation(input)
	if err != nil || replay.Digest() != value.Digest() {
		t.Fatal("equivalent intent changed digest", err)
	}
	input.UnitName = "Different unit"
	different, err := d.NewCreation(input)
	if err != nil || different.Digest() == value.Digest() {
		t.Fatal("changed intent kept digest", err)
	}
	profile := value.Profile()
	profile.Kinds[0] = d.Farm
	if value.Profile().Kinds[0] == d.Farm {
		t.Fatal("mutable profile escaped")
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
		if text := fmt.Sprintf(verb, value); strings.Contains(text, "Synthetic company") || strings.Contains(text, "synthetic unit") {
			t.Fatal("creation leaked descriptions")
		}
	}
}
func TestCreationRejectsMissingNamesZoneKindsOrTerms(t *testing.T) {
	for _, change := range []func(*d.CreationInput){func(v *d.CreationInput) { v.OperationID = "bad" }, func(v *d.CreationInput) { v.Profile.Name = " " }, func(v *d.CreationInput) { v.Profile.TimeZone = "" }, func(v *d.CreationInput) { v.Profile.TimeZone = "invalid/zone" }, func(v *d.CreationInput) { v.Profile.Kinds = nil }, func(v *d.CreationInput) { v.Profile.Kinds = []d.Kind{"unknown"} }, func(v *d.CreationInput) { v.UnitName = "\u200b" }, func(v *d.CreationInput) { v.UnitName = "unsafe\nunit" }, func(v *d.CreationInput) { v.Terms.Accepted = false }, func(v *d.CreationInput) { v.Terms.Version = "unreviewed-real-terms" }, func(v *d.CreationInput) { v.Terms.Digest = strings.Repeat("0", 64) }} {
		v := creationInput()
		change(&v)
		if _, err := d.NewCreation(v); err == nil {
			t.Fatal("invalid creation accepted")
		}
	}
}
func TestDefaultUnitHasScopedIdentityAndOrganizationZone(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := d.NewDefaultUnit(scope, "7b0e2a54-0000-4000-8000-000000000501", " Main ", "America/Cuiaba")
	if err != nil {
		t.Fatal(err)
	}
	v, err := unit.Snapshot(scope)
	if err != nil || v.Code != "MAIN" || v.Name != "Main" || v.TimeZone != "America/Cuiaba" || v.State != d.UnitActive || v.Revision != 1 {
		t.Fatal("invalid default unit", err)
	}
	if _, err := unit.Snapshot(kernel.OrgScope{}); err != d.ErrScope {
		t.Fatal(err)
	}
	if _, err := d.NewDefaultUnit(scope, "bad", "Main", "UTC"); err == nil {
		t.Fatal("invalid identity accepted")
	}
}
