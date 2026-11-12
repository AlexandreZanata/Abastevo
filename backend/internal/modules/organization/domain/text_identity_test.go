package domain_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

// Unique structure names/codes compare trimmed and case-insensitively, so text
// that only looks identical (invisible format characters, decomposed accents)
// must be refused rather than become a second "same" name or code.
func TestStructureNamesAndCodesRefuseInvisibleAndDecomposedText(t *testing.T) {
	refused := []string{"N01\u200b", "\u200dN01", "N\ufeff01", "S\u00adul", "Sa\u0303o Paulo", "Unidade\u2060Norte"}
	for _, raw := range refused {
		if _, err := d.CanonicalUnitProfile(d.UnitProfile{Name: raw, Code: "N01"}, "UTC"); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("unit name %q: %v", raw, err)
		}
		if _, err := d.CanonicalUnitProfile(d.UnitProfile{Name: "Norte", Code: raw}, "UTC"); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("unit code %q: %v", raw, err)
		}
		if _, err := d.CanonicalCostCentreProfile(d.CostCentreProfile{Name: raw, Code: "CC01"}); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("cost-centre name %q: %v", raw, err)
		}
		if _, err := d.CanonicalCostCentreProfile(d.CostCentreProfile{Name: "Frota", Code: raw}); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("cost-centre code %q: %v", raw, err)
		}
		if _, err := d.CanonicalCostCentreCode(raw); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("cost-centre lookup %q: %v", raw, err)
		}
		if _, err := d.CanonicalTeamProfile(d.TeamProfile{Name: raw}); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("team name %q: %v", raw, err)
		}
	}
	unit, err := d.CanonicalUnitProfile(d.UnitProfile{Name: " São Paulo Norte ", Code: " SP-01 "}, "UTC")
	if err != nil || unit.Name != "São Paulo Norte" || unit.Code != "SP-01" {
		t.Fatal("composed valid unit changed", unit, err)
	}
	if cc, err := d.CanonicalCostCentreProfile(d.CostCentreProfile{Name: "Manutenção", Code: "CC-Ç1"}); err != nil || cc.Code != "CC-Ç1" {
		t.Fatal("composed valid cost centre changed", cc, err)
	}
	if team, err := d.CanonicalTeamProfile(d.TeamProfile{Name: "Equipe Ação"}); err != nil || team.Name != "Equipe Ação" {
		t.Fatal("composed valid team changed", team, err)
	}
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(id)
	if _, err := d.NewDefaultUnit(scope, "7b0e2a54-0000-4000-8000-000000000002", "Sede\u200b", "UTC"); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("onboarding default unit accepted invisible text", err)
	}
	creation := d.CreationInput{OperationID: "7b0e2a54-0000-4000-8000-000000001001", Profile: d.Profile{Name: "Synthetic company", TimeZone: "UTC", Kinds: []d.Kind{d.Other}}, UnitName: "Sede\u200b", Terms: d.TermsAcceptance{Version: d.SyntheticTermsVersion, Digest: d.SyntheticTermsDigest, Accepted: true}}
	if _, err := d.NewCreation(creation); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("organization creation accepted an invisible first-unit name", err)
	}
	// The organization display name rule is unchanged: format characters inside
	// a non-unique display name remain allowed (only bidi overrides are refused).
	if _, err := d.NewPending(scope, d.Profile{Name: "Empresa\u200dTeste", TimeZone: "UTC"}); err != nil {
		t.Fatal("organization display-name rule changed", err)
	}
}
