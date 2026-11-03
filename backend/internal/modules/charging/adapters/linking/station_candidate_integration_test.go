//go:build integration

package linking

import (
	"context"
	"testing"
)

func TestFindStationCandidate(t *testing.T) {
	pool := freshDB(t)
	seed(t, pool)
	store := Store{Pool: pool}
	ctx := context.Background()
	const run = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const assertion = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const identifier = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const mapGeneration = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const mapAssertion = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO registry_source_runs(id,source,snapshot_identity,checksum,parser_version,state,finished_at)
		VALUES($1,'registry-csv','synthetic','synthetic','test','complete',now())`, run)
	exec(`INSERT INTO directory_identifiers(id,station_id,kind,normalized_value)
		VALUES($1,$2,'CNPJ','91000000000066')`, identifier, stationA)
	exec(`INSERT INTO registry_assertions(id,run_id,source,source_key,checksum,station_id,display_name)
		VALUES($1,$2,'registry-csv','91000000000066','synthetic',$3,'Synthetic Fuel')`, assertion, run, stationA)
	point := func(id string, meters float64, quality string) {
		t.Helper()
		exec(`UPDATE directory_stations SET trade_name='Pôsto–Exemplo', current_quality=$3,
			current_point=ST_Project(ST_SetSRID(ST_MakePoint(-46.6333,-23.5505),4326)::geography,$2::double precision,0::double precision) WHERE id=$1`,
			id, meters, quality)
	}
	facts := StationSourceFacts{TradeName: "  POSTO   EXEMPLO "}
	check := func(facts StationSourceFacts, want string) {
		t.Helper()
		got, found, err := store.FindStationCandidate(ctx, placeID, facts)
		if err != nil {
			t.Fatal(err)
		}
		if found != (want != "") || (found && (got.StationID != stationA || got.Rule != want)) {
			t.Fatalf("candidate=%+v found=%v want station A with rule %q", got, found, want)
		}
	}

	point(stationA, 74.999, "reviewed")
	check(facts, "pin-trade-name")
	check(StationSourceFacts{Brand: "Posto Exemplo"}, "pin-trade-name")
	check(StationSourceFacts{TradeName: "Outro Posto"}, "")
	point(stationA, 75.001, "osm-mapped")
	check(facts, "")
	point(stationA, 40, "address-point")
	point(stationB, 10, "anp-reported")
	check(facts, "")
	point(stationA, 40, "anp-reported")
	check(facts, "pin-trade-name")
	const brandCorrection = "ffffffff-ffff-4fff-8fff-ffffffffffff"
	exec(`INSERT INTO station_corrections(id,station_id,account_id,field,proposed_value,
		client_submission_id,status,decided_at)
		VALUES($1,$2,gen_random_uuid(),'brand','{"value":"Rede Exemplo"}','test','accepted',now())`, brandCorrection, stationA)
	check(StationSourceFacts{Brand: "Rede Exemplo"}, "")
	exec(`INSERT INTO station_correction_decisions(id,correction_id,decision,reason,decider_kind)
		VALUES(gen_random_uuid(),$1,'accepted','test','staff')`, brandCorrection)
	check(StationSourceFacts{Brand: "Rêde-Exemplo"}, "pin-brand")
	check(StationSourceFacts{Brand: "Outra Rede"}, "")
	exec(`INSERT INTO directory_identifiers(id,station_id,kind,normalized_value)
		VALUES(gen_random_uuid(),$1,'CNPJ','92000000000066')`, stationB)
	exec(`INSERT INTO registry_assertions(id,run_id,source,source_key,checksum,station_id,display_name)
		VALUES(gen_random_uuid(),$1,'registry-csv','92000000000066','synthetic',$2,'Synthetic Fuel 2')`, run, stationB)
	check(facts, "") // Two distinct eligible stations must not be ranked by distance.
	point(stationB, 200, "reviewed")
	point(stationA, 200, "reviewed")
	check(facts, "")

	exec(`INSERT INTO map_generations(id,generation_key,format,product_class,visibility,profile_id,
		profile_sha256,rights_sha256,source_revision,assertions_sha256,content_sha256,
		input_count,accepted_count,duplicate_count,quarantined_count,status)
		VALUES($1,'synthetic','abastevo.map-prepared-v1','PUBLIC_OVERLAY','PUBLIC','test',
		repeat('a',64),repeat('b',64),'test',repeat('c',64),repeat('d',64),1,1,0,0,'ACTIVE')`, mapGeneration)
	exec(`INSERT INTO map_assertions(generation_id,assertion_id,source,kind,name,station_id,
		osm_type,osm_id,quality,services,clocks,rights,capabilities,assertion_sha256)
		VALUES($1,$2,'OSM','FUEL_STATION','Synthetic Fuel',$3,'way',101,'osm-mapped','{}','{}','{}','{}',repeat('e',64))`,
		mapGeneration, mapAssertion, stationA)
	check(StationSourceFacts{Combined: &OSMObject{Type: "node", ID: 101}}, "")
	combined := StationSourceFacts{Combined: &OSMObject{Type: "way", ID: 101}}
	check(combined, "combined-osm")
	exec(`INSERT INTO map_assertion_revocations(id,assertion_id,reason,source_reference)
		VALUES(gen_random_uuid(),$1,'synthetic test','test')`, mapAssertion)
	check(combined, "")
}
