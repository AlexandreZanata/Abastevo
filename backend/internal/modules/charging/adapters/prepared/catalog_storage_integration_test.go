//go:build integration

package prepared

import (
	"context"
	"testing"
)

func TestNationalCatalogStoragePreservesLegacyConstraints(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	const legacyBatch = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const nationalBatch = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const legacyPlace = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const nationalPlace = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const alias = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	refuse := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err == nil {
			t.Fatal("invalid storage row accepted")
		}
	}
	const batchSQL = `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
		VALUES($1,'charging-prepared-v1',$2,'synthetic',repeat('a',64),1,1)`
	exec(batchSQL, legacyBatch, "ocm-br-20261021-v1")
	exec(batchSQL, nationalBatch, NationalCatalogProfile)
	exec(`INSERT INTO charging_catalog_generations(batch_id,rights_sha256,offer_sha256,uf_counts)
		VALUES($1,repeat('a',64),repeat('b',64),'{"SP":1}')`, nationalBatch)
	const placeSQL = `INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points,catalog_details)
		VALUES($1,$2,repeat('a',64),'synthetic',$3,'BR','-23.5','-46.6','UNKNOWN',0,$4)`
	refuse(placeSQL, legacyPlace, legacyBatch, "", nil)
	exec(placeSQL, legacyPlace, legacyBatch, "Synthetic legacy", nil)
	refuse(placeSQL, nationalPlace, nationalBatch, "", `{"confidence":null,"uf":"SP","sources":[]}`)
	exec(placeSQL, nationalPlace, nationalBatch, "", `{"confidence":"single-source","uf":"SP","sources":[{"source":"OSM"}]}`)
	const groupSQL = `INSERT INTO charging_connection_groups(id,batch_id,place_id,entity_key,external_id,connector_code,raw_connector_code,current,catalog_source_key)
		VALUES(gen_random_uuid(),$1,$2,repeat('b',64),'synthetic',$3,$4,'UNKNOWN',$5)`
	refuse(groupSQL, legacyBatch, legacyPlace, "TYPE1", "type1", nil)
	exec(groupSQL, legacyBatch, legacyPlace, "TYPE2", "25", nil)
	exec(groupSQL, nationalBatch, nationalPlace, "TYPE1", "", "osm:node:101")
	const aliasSQL = `INSERT INTO charging_catalog_place_aliases(batch_id,public_id,place_id,is_primary)
		VALUES($1,$2,$3,$4)`
	refuse(aliasSQL, nationalBatch, alias, legacyPlace, true)
	exec(aliasSQL, nationalBatch, alias, nationalPlace, true)
	refuse(aliasSQL, nationalBatch, alias, nationalPlace, false)
	refuse(aliasSQL, nationalBatch, legacyPlace, nationalPlace, true)
	exec(aliasSQL, nationalBatch, legacyPlace, nationalPlace, false)
}
