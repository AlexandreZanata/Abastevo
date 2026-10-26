package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validRow() reviewRow {
	return reviewRow{
		SchemaVersion:  reviewSchema,
		CNPJ:           "01122233300000",
		StationName:    "POSTO TESTE",
		Municipality:   "CURITIBA",
		State:          "PR",
		Latitude:       -25.4,
		Longitude:      -49.2,
		Method:         reviewMethod,
		MatchDistanceM: 12.3,
		Reviewer:       "tester",
		Evidence:       "anp:2026-10-10;osm:node:1",
	}
}

func TestValidateReviewAcceptsEvidenceBoundRow(t *testing.T) {
	if err := validateReview(validRow()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReviewRejectsMissingEvidenceAndUnsupportedMethod(t *testing.T) {
	row := validRow()
	row.Evidence = ""
	if err := validateReview(row); err == nil {
		t.Fatal("missing evidence accepted")
	}
	row = validRow()
	row.Method = "NEAREST"
	if err := validateReview(row); err == nil {
		t.Fatal("proximity method accepted")
	}
	row = validRow()
	row.Latitude = 91
	if err := validateReview(row); err == nil {
		t.Fatal("out-of-range latitude accepted")
	}
}

func TestReadReviewsIsBoundedAndRequiresRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reviews.ndjson")
	row := validRow()
	raw := `{"schema_version":"station-location-review-v1","cnpj":"01122233300000","station_name":"POSTO TESTE","municipality":"CURITIBA","state":"PR","latitude":-25.4,"longitude":-49.2,"method":"ANP_REGISTRY_API+OSM_ADDRESS_NAME","match_distance_m":12.3,"reviewer":"tester","evidence":"anp:2026-10-10;osm:node:1"}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	rows, err := readReviews(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CNPJ != row.CNPJ {
		t.Fatalf("rows = %+v", rows)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", 4<<20)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReviews(path); err == nil {
		t.Fatal("oversized review file accepted")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReviews(path); err == nil {
		t.Fatal("empty review file accepted")
	}
}
