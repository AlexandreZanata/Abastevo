package application

import (
	"strings"
	"testing"
)

func TestPlanBackfillMatrix(t *testing.T) {
	entries := []InventoryEntry{
		{Label: "2026-07", URL: "u/07.zip", SHA256: strings.Repeat("7", 64)},
		{Label: "2026-08", URL: "", SHA256: ""},
		{Label: "2026-02", URL: "u/02-as-07.zip", SHA256: strings.Repeat("7", 64)},
		{Label: "2026-09", URL: "u/09.zip", SHA256: strings.Repeat("9", 64)},
		{Label: "2026-10", URL: "u/10.zip", SHA256: strings.Repeat("a", 64)},
		{Label: "2026-11", URL: "u/11.zip", SHA256: ""},
	}
	recorded := []RecordedEdition{
		{Label: "2026-07", ManifestHash: strings.Repeat("m", 64), ResourceHashes: []string{strings.Repeat("7", 64)}},
		{Label: "2026-09", ManifestHash: strings.Repeat("n", 64), ResourceHashes: []string{strings.Repeat("0", 64)}},
	}
	got := PlanBackfill(entries, recorded)
	want := map[string]PlanAction{
		// Oldest first regardless of input order.
		"2026-02": PlanAlias,        // duplicated July bytes: no double import.
		"2026-07": PlanReplayStable, // same content: publisher converges.
		"2026-08": PlanSkipMissing,  // unpublished: later runs retry.
		"2026-09": PlanCorrect,      // same label, new bytes: supersede.
		"2026-10": PlanImportNew,    // unknown label: import.
		"2026-11": PlanImportNew,    // unfetched hash: import, never skip.
	}
	if len(got) != len(want) {
		t.Fatalf("planned %d editions, want %d: %+v", len(got), len(want), got)
	}
	for i, p := range got {
		if w := want[p.Label]; w != p.Action {
			t.Errorf("label %s -> %s, want %s (%s)", p.Label, p.Action, w, p.Reason)
		}
		if i > 0 && got[i-1].Label > p.Label {
			t.Errorf("plan not oldest-first: %+v", got)
		}
	}
	alias := got[0]
	if alias.AliasOf != "2026-07" || alias.URL == "" {
		t.Errorf("alias must name its owner and keep the URL: %+v", alias)
	}
}

func TestPlanBackfillLateResourceImports(t *testing.T) {
	// A February resource appearing late was never recorded: it imports
	// under its own label instead of aliasing or skipping.
	got := PlanBackfill([]InventoryEntry{
		{Label: "2026-02", URL: "u/02.zip", SHA256: strings.Repeat("2", 64)},
	}, []RecordedEdition{
		{Label: "2026-07", ResourceHashes: []string{strings.Repeat("7", 64)}},
	})
	if len(got) != 1 || got[0].Action != PlanImportNew {
		t.Fatalf("late resource -> %+v, want single import_new", got)
	}
}

func TestPlanBackfillEmpty(t *testing.T) {
	if got := PlanBackfill(nil, nil); len(got) != 0 {
		t.Fatalf("empty inventory planned %+v", got)
	}
}
