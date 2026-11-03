//go:build integration

package registry

import (
	"context"
	"encoding/json"
	"testing"
)

func TestStationFactsSourceRoundTripAndImmutableReplay(t *testing.T) {
	store := freshStore(t)
	ctx := context.Background()
	run := newUUID()
	if _, _, err := store.CreateRun(ctx, run, SourceCSV, "sna-storage-synthetic", vectorChecksum("input")); err != nil {
		t.Fatal(err)
	}
	a := Assertion{Source: SourceCSV, SourceKey: "91000000000066", Checksum: vectorChecksum("source-row"), DisplayName: "[P25-TEST] Trade", LegalName: " [P25-TEST] Legal LTDA ", TradeName: "[P25-TEST] Trade", SourceDisplayName: "[P25-TEST] Source Value", AddressSource: "anp", Address: map[string]string{"street": "SYNTHETIC STREET", "municipio": "TEST CITY", "uf": "SP"}, AuthState: "unknown", Eligibility: "pending", LocationQuality: "unknown", TradeNameProvenance: &BatchNameProvenance{Source: "rfb", Profile: "rfb-station-names-v1", Reference: "https://example.invalid/rfb", Edition: "synthetic", SHA256: vectorChecksum("names"), ObservedAt: "2026-10-01"}}
	if accepted, err := store.StageAssertion(ctx, a.WithRun(run)); err != nil || !accepted {
		t.Fatal(accepted, err)
	}
	if err := store.FinishRun(ctx, run, "complete", 1, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListAssertions(ctx, run)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	check := func(got Assertion) {
		t.Helper()
		if got.LegalName != a.LegalName || got.TradeName != a.TradeName || got.SourceDisplayName != a.SourceDisplayName || got.AddressSource != "anp" {
			t.Fatalf("source facts lost: %+v", got)
		}
		want, _ := json.Marshal(a.TradeNameProvenance)
		raw, _ := json.Marshal(got.TradeNameProvenance)
		if string(raw) != string(want) {
			t.Fatal("name provenance lost")
		}
	}
	check(rows[0])
	got, state, err := store.GetAssertion(ctx, rows[0].ID)
	if err != nil || state != "complete" {
		t.Fatal(state, err)
	}
	check(got)
	a.LegalName = "[P25-TEST] Mutable Replay"
	if accepted, err := store.StageAssertion(ctx, a.WithRun(run)); err != nil || accepted {
		t.Fatal(accepted, err)
	}
	got, _, err = store.GetAssertion(ctx, rows[0].ID)
	if err != nil || got.LegalName == a.LegalName {
		t.Fatal("replay rewrote source", err)
	}
	a.Checksum = vectorChecksum("invalid-enum")
	a.AddressSource = "invented"
	if _, err := store.StageAssertion(ctx, a.WithRun(run)); err == nil {
		t.Fatal("unsupported address source stored")
	}
}
