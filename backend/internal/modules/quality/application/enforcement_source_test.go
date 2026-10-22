package application

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"strings"
	"testing"
)

func validEnforcementInput() EnforcementSourceInput {
	row := EnforcementSourceRecord{SubjectKind: "personal-shaped", UF: "DF", Municipality: "Synthetic", DocumentDate: "2026-01-02", DocumentNumber: "1", Result: strings.Repeat("source wording ", 10), EvidenceRef: "synthetic.xlsx#row:2"}
	row.Fingerprint = row.ContentHash()
	row.RecordKey = "enforcement-source:" + row.Fingerprint
	return EnforcementSourceInput{Context: EnforcementSourceContext{Profile: EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: row.DocumentDate, LastDocumentDay: row.DocumentDate}, Resources: []Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("b", 64), Records: []EnforcementSourceRecord{row}}
}
func TestEnforcementSourceAdmission(t *testing.T) {
	if e := validEnforcementInput().Validate(); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*EnforcementSourceInput){
		"count": func(a *EnforcementSourceInput) { a.Counts.Accepted = 0 }, "quarantine": func(a *EnforcementSourceInput) { a.Counts.Input++; a.Counts.Quarantined++ }, "profile": func(a *EnforcementSourceInput) { a.Context.Profile = "unknown" }, "path": func(a *EnforcementSourceInput) { a.Context.SourceFile = "../a" }, "clock": func(a *EnforcementSourceInput) { a.Context.DateSystem = "DAY" }, "bounds": func(a *EnforcementSourceInput) { a.Context.FirstDocumentDay = "2026-01-01" }, "date": func(a *EnforcementSourceInput) { a.Records[0].DocumentDate = "2026-02-30" }, "hash": func(a *EnforcementSourceInput) { a.Records[0].Result += "forged" }, "person ID": func(a *EnforcementSourceInput) { a.Records[0].SourceCNPJ = "12345678901" }, "locator": func(a *EnforcementSourceInput) { a.Records[0].EvidenceRef = "synthetic.xlsx#row:3" }, "member": func(a *EnforcementSourceInput) { a.Resources[0].Member = "archive" }, "budget": func(a *EnforcementSourceInput) { a.Resources[0].Bytes = 101 << 20 }, "repeated": func(a *EnforcementSourceInput) {
			a.Records = append(a.Records, a.Records[0])
			a.Counts = domain.Counts{Input: 2, Accepted: 2}
		},
	} {
		t.Run(name, func(t *testing.T) {
			a := validEnforcementInput()
			change(&a)
			if a.Validate() == nil {
				t.Fatal("invalid publication admitted")
			}
		})
	}
	empty := validEnforcementInput()
	empty.Records = nil
	empty.Counts = domain.Counts{}
	empty.Context.FirstDocumentDay = ""
	empty.Context.LastDocumentDay = ""
	if empty.Validate() != nil {
		t.Fatal("explicit empty snapshot refused")
	}
}
