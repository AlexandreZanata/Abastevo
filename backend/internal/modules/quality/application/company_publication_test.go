package application

import (
	"reflect"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func companyPublicationPlan() *CompanyAssociationInput {
	return &CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20", Decisions: []domain.CompanyObservationDecision{
		{Kind: domain.ResolutionBound, RecordKey: "one", StationID: "bbbbbbbb-2222-4222-8222-000000000001", Scope: domain.CurrentCompanyOnly, Reason: "exact current cnpj; historical business window unknown", DecidedAt: "2026-10-20", PolicyVersion: domain.CompanyObservationPolicyVersion},
		{Kind: domain.ResolutionBound, RecordKey: "two", StationID: "bbbbbbbb-2222-4222-8222-000000000001", Scope: domain.CurrentCompanyOnly, Reason: "exact current cnpj; historical business window unknown", DecidedAt: "2026-10-20", PolicyVersion: domain.CompanyObservationPolicyVersion},
	}}
}
func TestCompanyPublicationCompleteCensusAndFrozenOperationalDay(t *testing.T) {
	plan := companyPublicationPlan()
	sources := map[string]string{"one": "11222333000181", "two": "11222333000181"}
	bindings, e := plan.Bindings(sources)
	if e != nil || len(bindings) != 1 || bindings[0].CNPJ != "11222333000181" {
		t.Fatal("company bindings do not derive from actual source", e)
	}
	first := plan.PublicationIdentity()
	plan.DecisionDay = "2026-10-21"
	for i := range plan.Decisions {
		plan.Decisions[i].DecidedAt = plan.DecisionDay
	}
	plan.Decisions[0], plan.Decisions[1] = plan.Decisions[1], plan.Decisions[0]
	if !reflect.DeepEqual(first, plan.PublicationIdentity()) {
		t.Fatal("operational day/order changed frozen identity")
	}
	for _, d := range plan.Decisions {
		if d.DecidedAt != "2026-10-21" {
			t.Fatal("identity projection mutated input")
		}
	}
	empty := &CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20"}
	if b, e := empty.Bindings(nil); e != nil || len(b) != 0 || empty.PublicationIdentity() == nil {
		t.Fatal("empty declared policy lost", e)
	}
	var absent *CompanyAssociationInput
	if b, e := absent.Bindings(sources); e != nil || len(b) != 0 || absent.PublicationIdentity() != nil {
		t.Fatal("absent policy changed legacy behavior")
	}
}
func TestCompanyPublicationRejectsMissingForeignContradictoryOrRelabelledDecisions(t *testing.T) {
	sources := map[string]string{"one": "11222333000181", "two": "11222333000181"}
	for name, change := range map[string]func(*CompanyAssociationInput){
		"subset":                        func(p *CompanyAssociationInput) { p.Decisions = p.Decisions[:1] },
		"foreign key":                   func(p *CompanyAssociationInput) { p.Decisions[0].RecordKey = "foreign" },
		"duplicate":                     func(p *CompanyAssociationInput) { p.Decisions[0].RecordKey = "two" },
		"policy":                        func(p *CompanyAssociationInput) { p.PolicyVersion = "unknown" },
		"row policy":                    func(p *CompanyAssociationInput) { p.Decisions[0].PolicyVersion = "unknown" },
		"day":                           func(p *CompanyAssociationInput) { p.DecisionDay = "2026-02-30" },
		"row day":                       func(p *CompanyAssociationInput) { p.Decisions[0].DecidedAt = "2026-10-21" },
		"scope":                         func(p *CompanyAssociationInput) { p.Decisions[0].Scope = "HISTORICAL_SITE" },
		"history":                       func(p *CompanyAssociationInput) { p.Decisions[0].HistoricalBusinessWindowKnown = true },
		"reason":                        func(p *CompanyAssociationInput) { p.Decisions[0].Reason = "invented guilt" },
		"kind":                          func(p *CompanyAssociationInput) { p.Decisions[0].Kind = "final_guilt" },
		"station shape":                 func(p *CompanyAssociationInput) { p.Decisions[0].StationID = "invalid" },
		"contradictory company station": func(p *CompanyAssociationInput) { p.Decisions[0].StationID = "bbbbbbbb-2222-4222-8222-000000000002" },
		"contradictory company kind": func(p *CompanyAssociationInput) {
			p.Decisions[0].Kind = domain.ResolutionUnmatched
			p.Decisions[0].StationID = ""
			p.Decisions[0].Scope = ""
			p.Decisions[0].Reason = domain.ReasonUnmatchedID
		},
	} {
		t.Run(name, func(t *testing.T) {
			plan := companyPublicationPlan()
			change(plan)
			if b, e := plan.Bindings(sources); e == nil || b != nil {
				t.Fatal("invalid company publication accepted")
			}
		})
	}
}
