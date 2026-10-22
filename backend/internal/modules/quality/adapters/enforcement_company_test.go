package adapters

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

const (
	testCNPJ    = "36754752000195"
	testStation = "11111111-1111-1111-1111-111111111111"
	testDay     = "2026-10-22"
)

func enforcementRecord(key, cnpj string) application.EnforcementSourceRecord {
	return application.EnforcementSourceRecord{RecordKey: key, SourceCNPJ: cnpj, SubjectKind: "company-shaped", UF: "DF", Municipality: "BRASILIA", Segment: "Revenda de Combustíveis", DocumentDate: "2026-10-04", DocumentNumber: "712508", Procedure: "Procedimento sintético", Result: "Resultado sintético", EvidenceRef: "synthetic.xlsx#row:2", Fingerprint: key}
}

func companyLookup(evidence map[string][]domain.IdentityEvidence) application.CompanyIdentityLookup {
	return func(ctx context.Context, cnpjs []string) (application.CompanyIdentitySnapshot, error) {
		snapshot := application.CompanyIdentitySnapshot{}
		for _, cnpj := range cnpjs {
			snapshot.Evidence = append(snapshot.Evidence, evidence[cnpj]...)
		}
		return snapshot, nil
	}
}

func boundEvidence() map[string][]domain.IdentityEvidence {
	return map[string][]domain.IdentityEvidence{
		testCNPJ: {{CNPJ: testCNPJ, StationID: testStation, Current: true, StationActive: true}},
	}
}

func TestPlanEnforcementCompanyAssociationsBindsExactCurrentCNPJ(t *testing.T) {
	decisions, err := PlanEnforcementCompanyAssociations(context.Background(),
		[]application.EnforcementSourceRecord{enforcementRecord("enforcement-source:aaa", testCNPJ)},
		testDay, companyLookup(boundEvidence()))
	if err != nil || len(decisions) != 1 {
		t.Fatal("plan refused", err)
	}
	d := decisions[0]
	if d.Kind != domain.ResolutionBound || d.StationID != testStation || d.Scope != domain.CurrentCompanyOnly || d.HistoricalBusinessWindowKnown {
		t.Fatal("not an exact current binding", d)
	}
}

func TestPlanEnforcementCompanyAssociationsRefusesSourceGlobalRecords(t *testing.T) {
	_, err := PlanEnforcementCompanyAssociations(context.Background(),
		[]application.EnforcementSourceRecord{enforcementRecord("enforcement-source:bbb", "")},
		testDay, companyLookup(boundEvidence()))
	if err == nil {
		t.Fatal("record without company identifier was planned")
	}
}

func TestPlanEnforcementCompanyAssociationsRefusesFuzzyBinding(t *testing.T) {
	// Same base CNPJ with a different branch digit, plus a name-only hint,
	// must not bind. Only the exact full identifier binds.
	other := "36754752000276"
	evidence := map[string][]domain.IdentityEvidence{
		testCNPJ: {{CNPJ: testCNPJ, StationID: testStation, Current: true, StationActive: true}},
	}
	decisions, err := PlanEnforcementCompanyAssociations(context.Background(),
		[]application.EnforcementSourceRecord{enforcementRecord("enforcement-source:ccc", other)},
		testDay, companyLookup(evidence))
	if err != nil || len(decisions) != 1 {
		t.Fatal("plan refused", err)
	}
	if decisions[0].Kind != domain.ResolutionUnmatched {
		t.Fatal("fuzzy branch bound", decisions[0])
	}
}

func TestRevalidateFlagsRetirementSuccessionAndRebound(t *testing.T) {
	const (
		retiredCNPJ   = "36754752000195"
		succeededCNPJ = "11222333000181"
		reboundCNPJ   = "04218406000104"
		stableCNPJ    = "00428184000195"
	)
	stored := []domain.CompanyObservationDecision{
		{Kind: domain.ResolutionBound, RecordKey: "enforcement-source:retired", StationID: testStation},
		{Kind: domain.ResolutionBound, RecordKey: "enforcement-source:succeeded", StationID: testStation},
		{Kind: domain.ResolutionUnmatched, RecordKey: "enforcement-source:rebound"},
		{Kind: domain.ResolutionBound, RecordKey: "enforcement-source:stable", StationID: testStation},
	}
	cnpjs := map[string]string{
		"enforcement-source:retired":   retiredCNPJ,
		"enforcement-source:succeeded": succeededCNPJ,
		"enforcement-source:rebound":   reboundCNPJ,
		"enforcement-source:stable":    stableCNPJ,
	}
	// Retirement: no evidence. Succession: station present but not current.
	// Rebound: fresh current evidence for a previously unbound record.
	// Stable: unchanged current evidence.
	fresh := map[string][]domain.IdentityEvidence{
		succeededCNPJ: {{CNPJ: succeededCNPJ, StationID: testStation}},
		reboundCNPJ:   {{CNPJ: reboundCNPJ, StationID: testStation, Current: true, StationActive: true}},
		stableCNPJ:    {{CNPJ: stableCNPJ, StationID: testStation, Current: true, StationActive: true}},
	}
	got := RevalidateEnforcementCompanyAssociations(stored, fresh, cnpjs, testDay)
	actions := map[string]string{}
	for _, d := range got {
		actions[d.RecordKey] = d.Action
	}
	if actions["enforcement-source:retired"] != "unbind" {
		t.Fatal("retirement not flagged for unbinding", got)
	}
	if actions["enforcement-source:succeeded"] != "unbind" {
		t.Fatal("succession not flagged for unbinding", got)
	}
	if actions["enforcement-source:rebound"] != "review" {
		t.Fatal("rebound not flagged for review", got)
	}
	if _, ok := actions["enforcement-source:stable"]; ok {
		t.Fatal("stable binding flagged", got)
	}
	// Deterministic: a restart re-runs the same flags, never new ones.
	again := RevalidateEnforcementCompanyAssociations(stored, fresh, cnpjs, testDay)
	if len(again) != len(got) {
		t.Fatal("revalidation not restart-safe")
	}
}

func TestRevalidateNeverBindsOrWrites(t *testing.T) {
	stored := []domain.CompanyObservationDecision{
		{Kind: domain.ResolutionUnmatched, RecordKey: "enforcement-source:review"},
	}
	cnpjs := map[string]string{"enforcement-source:review": testCNPJ}
	fresh := map[string][]domain.IdentityEvidence{
		testCNPJ: {{CNPJ: testCNPJ, StationID: testStation, Current: true, StationActive: true}},
	}
	got := RevalidateEnforcementCompanyAssociations(stored, fresh, cnpjs, testDay)
	if len(got) != 1 || got[0].Action != "review" {
		t.Fatal("rebound must be review-only", got)
	}
}
