package domain

import (
	"reflect"
	"testing"
)

func TestCompanyObservationAssociationNeverClaimsHistoricalSite(t *testing.T) {
	record := CompanyObservationRecord{RecordKey: "observation:synthetic", CNPJ: "11222333000181"}
	current := IdentityEvidence{CNPJ: record.CNPJ, StationID: "station-a", Current: true, StationActive: true}
	for _, tc := range []struct {
		name   string
		rows   []IdentityEvidence
		branch bool
		want   ResolutionKind
	}{
		{"one current", []IdentityEvidence{current}, false, ResolutionBound},
		{"retained same identity", []IdentityEvidence{current, {CNPJ: record.CNPJ, StationID: "station-a"}}, false, ResolutionBound},
		{"retired predecessor", []IdentityEvidence{current, {CNPJ: record.CNPJ, StationID: "station-b"}}, false, ResolutionAmbiguous},
		{"two current", []IdentityEvidence{current, {CNPJ: record.CNPJ, StationID: "station-b", Current: true, StationActive: true}}, false, ResolutionAmbiguous},
		{"retired only", []IdentityEvidence{{CNPJ: record.CNPJ, StationID: "station-a", StationActive: true}}, false, ResolutionSuccession},
		{"inactive", []IdentityEvidence{{CNPJ: record.CNPJ, StationID: "station-a", Current: true}}, false, ResolutionSuccession},
		{"split flags cannot merge", []IdentityEvidence{{CNPJ: record.CNPJ, StationID: "station-a", Current: true}, {CNPJ: record.CNPJ, StationID: "station-a", StationActive: true}}, false, ResolutionSuccession},
		{"wrong branch", nil, true, ResolutionWrongBranch},
		{"missing", nil, false, ResolutionUnmatched},
		{"foreign exact evidence", []IdentityEvidence{{CNPJ: "11222333000262", StationID: "station-b", Current: true, StationActive: true}}, true, ResolutionWrongBranch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]IdentityEvidence(nil), tc.rows...)
			d := ResolveCompanyObservation(record, "2026-10-20", tc.rows, tc.branch)
			if d.Kind != tc.want || d.RecordKey != record.RecordKey || d.DecidedAt != "2026-10-20" || d.HistoricalBusinessWindowKnown || d.PolicyVersion != "current-company-v1" {
				t.Fatal("company identity semantics lost", d)
			}
			if d.Kind == ResolutionBound {
				if d.Scope != CurrentCompanyOnly || d.StationID != "station-a" {
					t.Fatal("company-only scope lost", d)
				}
			} else if d.StationID != "" || d.Scope != "" {
				t.Fatal("review assigned station", d)
			}
			if !reflect.DeepEqual(tc.rows, before) {
				t.Fatal("evidence mutated")
			}
			reverse := append([]IdentityEvidence(nil), tc.rows...)
			for i, j := 0, len(reverse)-1; i < j; i, j = i+1, j-1 {
				reverse[i], reverse[j] = reverse[j], reverse[i]
			}
			if got := ResolveCompanyObservation(record, "2026-10-20", reverse, tc.branch); !reflect.DeepEqual(got, d) {
				t.Fatal("evidence order changes company decision")
			}
		})
	}
}

func TestCompanyObservationInvalidInputNeverAssociates(t *testing.T) {
	current := []IdentityEvidence{{CNPJ: "11222333000181", StationID: "station-a", Current: true, StationActive: true}}
	for _, tc := range []struct{ key, cnpj, day string }{
		{"", "11222333000181", "2026-10-20"}, {"k\nprivate", "11222333000181", "2026-10-20"},
		{"k", "00000000000", "2026-10-20"},
		{"k", "11222333000181", "2026-02-30"}, {"k", "11222333000181", ""},
	} {
		d := ResolveCompanyObservation(CompanyObservationRecord{RecordKey: tc.key, CNPJ: tc.cnpj}, tc.day, current, false)
		if d.Kind != ResolutionUnmatched || d.StationID != "" || d.Scope != "" {
			t.Fatal("invalid company record bound", d)
		}
	}
}
