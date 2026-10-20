package domain

import "testing"

func TestCurrentIdentityKeepsOperationalClocksSeparate(t *testing.T) {
	record := SourceRecord{RecordKey: "source:sample", CNPJ: "11222333000181", ObservedAt: "2016-01-04"}
	current := IdentityEvidence{CNPJ: record.CNPJ, StationID: "station-a", Current: true, StationActive: true}
	for _, tc := range []struct {
		name     string
		evidence []IdentityEvidence
		branch   bool
		want     ResolutionKind
	}{
		{"unique current", []IdentityEvidence{current}, false, ResolutionBound},
		{"same canonical retained rows", []IdentityEvidence{current, {CNPJ: record.CNPJ, StationID: "station-a"}}, false, ResolutionBound},
		{"multiple canonicals", []IdentityEvidence{current, {CNPJ: record.CNPJ, StationID: "station-b"}}, false, ResolutionAmbiguous},
		{"retired only", []IdentityEvidence{{CNPJ: record.CNPJ, StationID: "station-a", StationActive: true}}, false, ResolutionSuccession},
		{"inactive station", []IdentityEvidence{{CNPJ: record.CNPJ, StationID: "station-a", Current: true}}, false, ResolutionSuccession},
		{"foreign branch", nil, true, ResolutionWrongBranch},
		{"missing", nil, false, ResolutionUnmatched},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := ResolveCurrentIdentity(record, "2026-10-16", tc.evidence, tc.branch)
			if d.Kind != tc.want || d.Link.Active != (tc.want == ResolutionBound) || d.ObservedAt != record.ObservedAt {
				t.Fatal(d)
			}
		})
	}
	record.ObservedAt = "2016-02-30"
	if d := ResolveCurrentIdentity(record, "2026-10-16", []IdentityEvidence{current}, false); d.Kind != ResolutionUnmatched {
		t.Fatal("invalid observation bound", d)
	}
}
