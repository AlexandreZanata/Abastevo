//go:build integration

package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestImportNationalCatalogAtomicallyStoresSourceFacts(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	raw := []byte(syntheticNationalCatalog())
	files, rights, manifest := nationalFixtureWithCatalog(t, raw)
	boundary := strings.Repeat("c", 64)
	geo, err := json.Marshal(map[string]any{
		"format":          "abastevo.ev-catalog-geography-v1",
		"catalog_sha256":  fmt.Sprintf("%x", sha256.Sum256(raw)),
		"boundary_sha256": boundary,
		"sites":           []map[string]string{{"key": "ev-site-v1:" + strings.Repeat("a", 64), "uf": "SP", "reason": "ibge_polygon"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := NationalImport{Files: files, RightsSHA256: rights, ManifestSHA256: manifest,
		Geography: geo, GeographySHA256: fmt.Sprintf("%x", sha256.Sum256(geo)), BoundarySHA256: boundary}
	const legacyID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err := pool.Exec(ctx, `WITH legacy AS (
		INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
		VALUES(gen_random_uuid(),'charging-prepared-v1','ocm-br-20261021-v1','legacy',repeat('b',64),1,1)
		RETURNING id
	) INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points)
	SELECT $1,id,repeat('b',64),'9','Synthetic legacy','BR','-23.5','-46.6','PUBLIC',0 FROM legacy`, legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE charging_catalog_current SET held=TRUE"); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportNationalCatalog(ctx, pool, input); err == nil {
		t.Fatal("held import accepted")
	}
	var total int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM charging_batches WHERE profile_id='charging-br-national-v1'").Scan(&total); err != nil || total != 0 {
		t.Fatalf("held import left batches: %d, %v", total, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE charging_catalog_current SET held=FALSE"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION synthetic_fail_group() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'synthetic group write failure'; END $$;
		CREATE TRIGGER synthetic_fail_group BEFORE INSERT ON charging_connection_groups
		FOR EACH ROW EXECUTE FUNCTION synthetic_fail_group()`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportNationalCatalog(ctx, pool, input); err == nil || !strings.Contains(err.Error(), "synthetic group write failure") {
		t.Fatalf("injected write failure did not reach the transaction: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM charging_batches WHERE profile_id='charging-br-national-v1'").Scan(&total); err != nil || total != 0 {
		t.Fatalf("failed import left batches: %d, %v", total, err)
	}
	if _, err := pool.Exec(ctx, "DROP TRIGGER synthetic_fail_group ON charging_connection_groups"); err != nil {
		t.Fatal(err)
	}
	id, err := ImportNationalCatalog(ctx, pool, input)
	if err != nil || id == "" {
		t.Fatalf("import id=%q err=%v", id, err)
	}
	var attempts sync.WaitGroup
	for range 5 {
		attempts.Go(func() {
			if again, err := ImportNationalCatalog(ctx, pool, input); err != nil || again != id {
				t.Errorf("replay id=%q err=%v", again, err)
			}
		})
	}
	attempts.Wait()
	var current, name, uf, publicID string
	var groups, aliases, activations int
	err = pool.QueryRow(ctx, `SELECT c.batch_id::text,p.name,p.catalog_details->>'uf',
		(SELECT count(DISTINCT catalog_source_key) FROM charging_connection_groups),
		(SELECT count(*) FROM charging_catalog_place_aliases),
		(SELECT count(*) FROM charging_catalog_activations),
		(SELECT public_id::text FROM charging_catalog_place_aliases WHERE place_id=p.id AND is_primary)
		FROM charging_catalog_current c JOIN charging_places p ON p.batch_id=c.batch_id`).
		Scan(&current, &name, &uf, &groups, &aliases, &activations, &publicID)
	if err != nil || current != id || name != "" || uf != "SP" || groups != 2 || aliases != 1 || activations != 1 || publicID != legacyID {
		t.Fatalf("current=%s name=%q UF=%s groups=%d aliases=%d activations=%d publicID=%s err=%v", current, name, uf, groups, aliases, activations, publicID, err)
	}
}
