//go:build integration

package read

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

func TestStationMetadataPublicPrivacySourceSeparationAndLegacyNull(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	repo := parent.NewRepository(pool)
	station, err := repo.ResolveCNPJ(ctx, "91000000000066", "[P25-TEST] Approved", map[string]string{"street": "[P25-TEST] Street", "number": "100", "municipio": "TEST CITY", "uf": "SP"})
	if err != nil {
		t.Fatal(err)
	}
	const legal = " [P25-TEST] Legal   LTDA "
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET legal_name=$2,address_source='anp' WHERE id=$1::uuid", station.ID, legal); err != nil {
		t.Fatal(err)
	}
	rev, err := repo.RecordLocation(ctx, domain.LocationRevision{StationID: station.ID, PointWKT: "POINT(-46 -23)", Quality: domain.QualityReviewed, Provider: "SNA-TEST"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ProjectLocation(ctx, station.ID, rev.ID); err != nil {
		t.Fatal(err)
	}
	run, _ := parent.NewUUIDv4()
	assertion, _ := parent.NewUUIDv4()
	if _, err := pool.Exec(ctx, "INSERT INTO registry_source_runs(id,source,snapshot_identity,checksum,parser_version,state,finished_at) VALUES($1::uuid,'registry-api','sna-api-synthetic','synthetic','registry-api-v3','complete',now())", run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO registry_assertions(id,run_id,source,source_key,checksum,display_name,source_display_name,station_id) VALUES($1::uuid,$2::uuid,'registry-api','91000000000066','synthetic','[P25-TEST] Derived label','[P25-TEST] Own ANP value',$3::uuid)", assertion, run, station.ID); err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.ResolveCNPJ(ctx, "12ABC34501DE35", "[P25-TEST] Legacy", map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewReader(pool)
	check := func(wantLegal string) {
		t.Helper()
		detail, err := reader.Detail(ctx, station.ID)
		if err != nil {
			t.Fatal(err)
		}
		search, _, err := reader.Search(ctx, application.SearchFilter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		nearby, _, err := reader.Nearby(ctx, application.NearbyFilter{Lat: -23, Lon: -46, RadiusM: 1000, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		views := []application.Station{detail}
		for _, item := range search {
			if item.ID == station.ID {
				views = append(views, item)
			}
		}
		for _, item := range nearby {
			views = append(views, item.Station)
		}
		if len(views) != 3 {
			t.Fatal("missing public path", len(views))
		}
		for _, got := range views {
			if got.LegalName == nil || *got.LegalName != wantLegal || got.AddressSource == nil || *got.AddressSource != "anp" || got.DisplayName != "[P25-TEST] Approved" || got.Official == nil || got.Official.DisplayName != "[P25-TEST] Own ANP value" {
				t.Fatalf("metadata/source separation %+v", got)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), "000.000.000-00") {
				t.Fatal("CPF public")
			}
			// A client reading only the prior field set can still decode the response.
			var old struct {
				ID      string `json:"station_id"`
				Display string `json:"display_name"`
			}
			if err := json.Unmarshal(raw, &old); err != nil || old.ID != station.ID || old.Display != got.DisplayName {
				t.Fatal("legacy JSON compatibility", old, err)
			}
		}
	}
	check(legal)
	const private = "[P25-TEST] Person 000.000.000-00"
	if _, err := pool.Exec(ctx, "UPDATE directory_stations SET legal_name=$2 WHERE id=$1::uuid", station.ID, private); err != nil {
		t.Fatal(err)
	}
	check("[P25-TEST] Person ")
	var retained string
	if err := pool.QueryRow(ctx, "SELECT legal_name FROM directory_stations WHERE id=$1::uuid", station.ID).Scan(&retained); err != nil || retained != private {
		t.Fatal("private source changed", retained, err)
	}
	got, err := reader.Detail(ctx, legacy.ID)
	if err != nil || got.LegalName != nil || got.AddressSource != nil {
		t.Fatal("legacy metadata invented", got, err)
	}
	raw, _ := json.Marshal(got)
	var object map[string]any
	_ = json.Unmarshal(raw, &object)
	for _, key := range []string{"legal_name", "address_source"} {
		value, ok := object[key]
		if !ok || value != nil {
			t.Fatal("nullable additive contract", key, object)
		}
	}
}

func TestStationSearchMatchesBothNamesWithoutCPF(t *testing.T) {
	pool := freshPool(t)
	ctx := context.Background()
	repo := parent.NewRepository(pool)
	station, err := repo.ResolveCNPJ(ctx, "91000000000066", "[P25-TEST] Trade 00000000000", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "UPDATE directory_stations SET legal_name=$2, municipality_code='3550308' WHERE id=$1::uuid",
		station.ID, "[P25-TEST] Razão 111.111.111-11 22222222222 LTDA")
	if err != nil {
		t.Fatal(err)
	}
	reader := NewReader(pool)
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"trade", 1}, {"razão", 1}, {"00000000000", 0}, {"111.111.111-11", 0}, {"22222222222", 0},
	} {
		rows, _, err := reader.Search(ctx, application.SearchFilter{Q: tc.query, Municipality: "3550308", Limit: 10})
		if err != nil || len(rows) != tc.want || (tc.want == 1 && rows[0].ID != station.ID) {
			t.Fatalf("search %q: got %d, want %d, err %v", tc.query, len(rows), tc.want, err)
		}
	}
}
