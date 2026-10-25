package portalgen

// Maintained regressions for the official-insights census: the breakdown
// follows the approved fiscalizacao grouping (wrong-branch and
// succession-review export as unlinked), always reconciles Rows, and an
// inconsistent census is refused at verification.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestESPFollowupOfficialInsightsReconcileAllStates(t *testing.T) {
	official := &officialDataset{Manifest: officialManifest{Edition: "2025", Rows: 3375, AssociationCensus: officialCensus{Total: 3375, Linked: 1669, Unmatched: 1656, WrongBranch: 50}}}
	cards := buildOfficialInsights(official)
	if len(cards) == 0 {
		t.Fatal("official insights absent")
	}
	if !strings.Contains(cards[0].Answer, "1706 sem vínculo") {
		t.Fatalf("association breakdown omits 50 wrong_branch records: %s", cards[0].Answer)
	}
}

func TestESPFollowupOfficialInsightsAllStatesNonzero(t *testing.T) {
	official := &officialDataset{Manifest: officialManifest{
		Edition: "2025", Rows: 10, SourcePeriod: "2025", SourceURL: "https://www.gov.br/anp/fines.csv",
		AssociationCensus: officialCensus{Total: 10, Linked: 3, Unmatched: 4, Ambiguous: 1, WrongBranch: 1, SuccessionReview: 1},
	}}
	cards := buildOfficialInsights(official)
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3", len(cards))
	}
	answer := cards[0].Answer
	if !strings.Contains(answer, "10 registros") || !strings.Contains(answer, "3 vinculados") ||
		!strings.Contains(answer, "6 sem vínculo") || !strings.Contains(answer, "1 com vínculo ambíguo") {
		t.Fatalf("breakdown does not reconcile rows: %s", answer)
	}
	for _, card := range cards {
		if strings.Contains(strings.ToLower(card.Question+card.Answer), "culpada") ||
			strings.Contains(strings.ToLower(card.Question+card.Answer), "pior posto") {
			t.Fatalf("verdict assigned: %+v", card)
		}
	}
}

func TestESPFollowupOfficialInsightsEmptySample(t *testing.T) {
	official := &officialDataset{Manifest: officialManifest{Edition: "2025", Rows: 0, SourcePeriod: "2025"}}
	cards := buildOfficialInsights(official)
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3 honest empty cards", len(cards))
	}
	if !strings.Contains(cards[0].Answer, "0 registros") {
		t.Fatalf("empty census misstated: %s", cards[0].Answer)
	}
	if buildOfficialInsights(nil) != nil {
		t.Fatal("nil dataset produced cards")
	}
}

func TestESPFollowupInconsistentCensusRefused(t *testing.T) {
	snapshot := writeFixture(t)
	writeOfficialFixture(t, snapshot)
	out := filepath.Join(t.TempDir(), "official")
	if _, err := Generate(Options{SnapshotDir: snapshot, OutputDir: out}); err != nil {
		t.Fatal(err)
	}
	manifest, err := readPortalManifest(out)
	if err != nil {
		t.Fatal(err)
	}
	manifest.OfficialRecords.AssociationCensus.Total++
	manifest.ContentSHA256, err = manifestContentDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "portal-manifest.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(out, VerifyOptions{}); err == nil {
		t.Fatal("inconsistent official census accepted")
	}
}
