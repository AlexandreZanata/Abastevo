package domain

import (
	"reflect"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

type fakeDirectory struct {
	byCNPJ map[string][]Binding
	byBase map[string][]Binding
}

func (f fakeDirectory) ByCNPJ(cnpj string) []Binding { return f.byCNPJ[cnpj] }
func (f fakeDirectory) ByBase(base string) []Binding { return f.byBase[base] }

func mustNormalized(t *testing.T, text string) string {
	t.Helper()
	cnpj, err := kernel.ParseCNPJ(text)
	if err != nil {
		t.Fatalf("valid CNPJ %q rejected: %v", text, err)
	}
	return cnpj.Normalized()
}

func TestResolveExactNumericAlphanumericLeadingZero(t *testing.T) {
	for text := range map[string]bool{
		"11.222.333/0001-81": true,
		"12ABC345/01DE-35":   true,
		"04.218.406/0001-04": true,
		"12abc345/01de-35":   true,
		"11222333000181":     true,
	} {
		normalized := mustNormalized(t, text)
		directory := fakeDirectory{
			byCNPJ: map[string][]Binding{
				normalized: {{StationID: "st-1", CNPJ: normalized, Name: "Posto X"}},
			},
		}
		got := Resolve(SourceRecord{
			RecordKey: "pmqc:S-1", CNPJ: text, ObservedAt: "2026-09-04",
		}, "2026-10-14", directory)
		if got.Kind != ResolutionBound || !got.Link.Active || got.Link.StationID != "st-1" {
			t.Errorf("CNPJ %q -> %+v, want bound link to st-1", text, got)
		}
		if got.ObservedAt != "2026-09-04" || got.DecidedAt != "2026-10-14" {
			t.Errorf("CNPJ %q clocks not independent/explicit: %+v", text, got)
		}
	}
}

func TestResolveTwoActiveMatchesAmbiguous(t *testing.T) {
	cnpj := mustNormalized(t, "11.222.333/0001-81")
	directory := fakeDirectory{
		byCNPJ: map[string][]Binding{
			cnpj: {
				{StationID: "st-a", CNPJ: cnpj},
				{StationID: "st-b", CNPJ: cnpj},
			},
		},
	}
	got := Resolve(SourceRecord{
		RecordKey: "pmqc:S-1", CNPJ: cnpj, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if got.Kind != ResolutionAmbiguous || got.Reason != ReasonAmbiguous {
		t.Fatalf("two matches -> %+v, want ambiguous", got)
	}
	if got.Link.Active {
		t.Error("collided link stays active")
	}
}

func TestResolveWrongBranchNeverBinds(t *testing.T) {
	known := mustNormalized(t, "11.222.333/0001-81")
	// Same base, different branch. Checksum ownership stays with
	// Directory/kernel; resolution only routes shape-valid full CNPJs.
	foreign := "11222333000299"
	directory := fakeDirectory{
		byBase: map[string][]Binding{
			"11222333": {{StationID: "st-1", CNPJ: known}},
		},
	}
	got := Resolve(SourceRecord{
		RecordKey: "pmqc:S-9", CNPJ: foreign, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if got.Kind != ResolutionWrongBranch {
		t.Fatalf("wrong branch -> %+v, want wrong_branch review", got)
	}
	if got.Link.Active || got.Link.StationID != "" {
		t.Errorf("wrong branch assigned a station: %+v", got.Link)
	}
}

func TestResolveHistoricalReplacementSuccession(t *testing.T) {
	cnpj := mustNormalized(t, "11.222.333/0001-81")
	directory := fakeDirectory{
		byCNPJ: map[string][]Binding{
			cnpj: {{
				StationID: "st-old", CNPJ: cnpj,
				ValidFrom: "2020-01-01", ValidTo: "2024-01-01",
			}},
		},
	}
	got := Resolve(SourceRecord{
		RecordKey: "pmqc:S-1", CNPJ: cnpj, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if got.Kind != ResolutionSuccession || got.Link.Active {
		t.Fatalf("post-window observation -> %+v, want inactive succession review", got)
	}
	// Inside the window the same binding still binds.
	inWindow := Resolve(SourceRecord{
		RecordKey: "pmqc:S-0", CNPJ: cnpj, ObservedAt: "2023-06-01",
	}, "2026-10-14", directory)
	if inWindow.Kind != ResolutionBound {
		t.Errorf("in-window observation -> %+v, want bound", inWindow)
	}
}

func TestResolveRenamedStationStaysBound(t *testing.T) {
	cnpj := mustNormalized(t, "11.222.333/0001-81")
	directory := fakeDirectory{
		byCNPJ: map[string][]Binding{
			cnpj: {{StationID: "st-1", CNPJ: cnpj, Name: "Posto Nome Novo LTDA"}},
		},
	}
	// SourceRecord carries no name: names never participate in binding.
	got := Resolve(SourceRecord{
		RecordKey: "pmqc:S-1", CNPJ: cnpj, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if got.Kind != ResolutionBound || !got.Link.Active {
		t.Errorf("renamed station -> %+v, want bound (CNPJ decides, not name)", got)
	}
}

func TestResolveSameAddressDifferentCNPJsNeverMerge(t *testing.T) {
	first := mustNormalized(t, "11.222.333/0001-81")
	second := mustNormalized(t, "04.218.406/0001-04")
	directory := fakeDirectory{
		byCNPJ: map[string][]Binding{
			first:  {{StationID: "st-1", CNPJ: first}},
			second: {{StationID: "st-2", CNPJ: second}},
		},
	}
	a := Resolve(SourceRecord{
		RecordKey: "pmqc:A-1", CNPJ: first, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	b := Resolve(SourceRecord{
		RecordKey: "pmqc:A-2", CNPJ: second, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if a.Kind != ResolutionBound || b.Kind != ResolutionBound {
		t.Fatalf("both -> %+v / %+v, want two bound decisions", a, b)
	}
	if a.Link.StationID == b.Link.StationID {
		t.Error("same-address records merged into one station")
	}
}

func TestResolveUnmatchedAndMalformed(t *testing.T) {
	empty := fakeDirectory{}
	got := Resolve(SourceRecord{
		RecordKey: "pmqc:X", CNPJ: mustNormalized(t, "11.222.333/0001-81"),
		ObservedAt: "2026-09-04",
	}, "2026-10-14", empty)
	if got.Kind != ResolutionUnmatched || got.Link.Active {
		t.Errorf("unknown CNPJ -> %+v, want inactive unmatched", got)
	}
	for raw := range map[string]bool{"123": true, "": true, "ABC!": true} {
		got := Resolve(SourceRecord{
			RecordKey: "pmqc:X", CNPJ: raw, ObservedAt: "2026-09-04",
		}, "2026-10-14", empty)
		if got.Kind != ResolutionUnmatched || got.Reason != ReasonInvalidCNPJ {
			t.Errorf("CNPJ %q -> %+v, want invalid_cnpj quarantine", raw, got)
		}
	}
	got = Resolve(SourceRecord{
		RecordKey: "pmqc:X", CNPJ: "11222333000181", ObservedAt: "04/09/2026",
	}, "2026-10-14", empty)
	if got.Kind != ResolutionUnmatched {
		t.Errorf("malformed date -> %+v, want unmatched", got)
	}
}

func TestResolveIsReadOnly(t *testing.T) {
	cnpj := mustNormalized(t, "11.222.333/0001-81")
	directory := fakeDirectory{
		byCNPJ: map[string][]Binding{
			cnpj: {{StationID: "st-1", CNPJ: cnpj, Name: "Posto X"}},
		},
	}
	before := fakeDirectory{
		byCNPJ: map[string][]Binding{
			cnpj: {{StationID: "st-1", CNPJ: cnpj, Name: "Posto X"}},
		},
	}
	Resolve(SourceRecord{
		RecordKey: "pmqc:S-1", CNPJ: cnpj, ObservedAt: "2026-09-04",
	}, "2026-10-14", directory)
	if !reflect.DeepEqual(directory, before) {
		t.Error("Resolve mutated the Directory view: no implicit station write allowed")
	}
}

func TestLinkRevisionValidate(t *testing.T) {
	binding := Binding{
		StationID: "st-1", CNPJ: mustNormalized(t, "11.222.333/0001-81"),
		ValidFrom: "2020-01-01", ValidTo: "2026-12-31",
	}
	good := LinkRevision{
		Link:          StationLink{StationID: "st-1", RecordKey: "pmqc:S-1", Active: true},
		ObservedAt:    "2026-09-04",
		DecidedAt:     "2026-10-14",
		EffectiveFrom: "2026-09-04",
		EffectiveTo:   "2026-12-31",
	}
	if err := good.Validate(binding); err != nil {
		t.Errorf("ordered in-window revision refused: %v", err)
	}
	open := good
	open.EffectiveFrom, open.EffectiveTo = "", ""
	if err := open.Validate(binding); err != nil {
		t.Errorf("open effective window refused: %v", err)
	}
	inverted := good
	inverted.EffectiveFrom, inverted.EffectiveTo = "2026-12-31", "2026-09-04"
	if err := inverted.Validate(binding); err == nil {
		t.Error("inverted window accepted")
	}
	outside := good
	outside.EffectiveFrom, outside.EffectiveTo = "2027-01-01", "2027-06-01"
	if err := outside.Validate(binding); err == nil {
		t.Error("window outside binding validity accepted")
	}
	undated := good
	undated.ObservedAt = "setembro"
	if err := undated.Validate(binding); err == nil {
		t.Error("malformed clock accepted")
	}
}
