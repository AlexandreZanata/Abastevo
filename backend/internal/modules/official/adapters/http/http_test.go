package http

import (
	"encoding/json"
	"testing"
	"time"
)

func parseTime(t *testing.T, raw string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestWireCommunityPriceNulls(t *testing.T) {
	// DISPUTED projections carry no amount or representative: the wire
	// renders explicit nulls instead of zero values.
	amount := int64(5999)
	rep := "d6c74c23-63db-4c24-a2e5-408cb23bad27"
	anchor := "2026-09-30T11:00:00Z"
	expires := "2026-10-02T11:00:00Z"
	full := wireCommunityPrice(CommunityPrice{
		Availability: "AVAILABLE", Confidence: "MEDIUM", Freshness: "FRESH",
		AmountMilli: amount, HasAmount: true, Supporters: 2, Confirmations: 1,
		RepresentativeID: rep, AnchorReceivedAt: parseTime(t, anchor),
		HasAnchor: true, ExpiresAt: parseTime(t, expires), HasExpiry: true,
		AlgorithmVersion: "consensus-v1", ProjectionVersion: 4,
	})
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["amount_milli_brl"] != float64(5999) || decoded["source"] != "COMMUNITY" {
		t.Errorf("full = %s", raw)
	}
	if decoded["representative_observation_id"] != rep {
		t.Errorf("representative = %s", raw)
	}
	disputed := wireCommunityPrice(CommunityPrice{
		Availability: "DISPUTED", Confidence: "", Freshness: "FRESH",
		AlgorithmVersion: "consensus-v1", ProjectionVersion: 5,
	})
	raw, err = json.Marshal(disputed)
	if err != nil {
		t.Fatal(err)
	}
	decoded = map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["amount_milli_brl"] != nil || decoded["representative_observation_id"] != nil {
		t.Errorf("disputed leaks zeros: %s", raw)
	}
	if decoded["availability"] != "DISPUTED" {
		t.Errorf("disputed = %s", raw)
	}
}
