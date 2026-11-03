//go:build integration

package prepared

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCatalogActivationRetainsLastGoodAndBindsOverride(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	seed := func(n int, digest string, rights bool) pgtype.UUID {
		t.Helper()
		id, err := newUUID()
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count)
			VALUES($1,'charging-prepared-v1',$2,$3,$3,$4,$4)`, id, NationalCatalogProfile, digest, n)
		if err != nil {
			t.Fatal(err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points)
			SELECT gen_random_uuid(),$1,lpad(i::text,64,'0'),i::text,'Synthetic Charge','BR','-23.5','-46.6','PUBLIC',0
			FROM generate_series(1,$2::integer) i`, id, n)
		if err != nil {
			t.Fatal(err)
		}
		if rights {
			_, err = pool.Exec(ctx, `INSERT INTO charging_catalog_generations(batch_id,rights_sha256,offer_sha256,uf_counts)
				VALUES($1,repeat('a',64),repeat('b',64),$2::jsonb)`, id, fmt.Sprintf(`{"SP":%d}`, n))
			if err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	activate := func(batch pgtype.UUID, override *CatalogCoverageOverride, commit bool) error {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := ActivateCatalog(ctx, tx, batch, override); err != nil {
			return err
		}
		if commit {
			return tx.Commit(ctx)
		}
		return nil
	}
	assertCurrent := func(want pgtype.UUID, activations int) {
		t.Helper()
		var got pgtype.UUID
		var count int
		err := pool.QueryRow(ctx, `SELECT batch_id,(SELECT count(*) FROM charging_catalog_activations)
			FROM charging_catalog_current WHERE singleton`).Scan(&got, &count)
		if err != nil || got != want || count != activations {
			t.Fatalf("pointer=%v activations=%d error=%v, want %v/%d", got, count, err, want, activations)
		}
	}
	firstHash, nextHash := strings.Repeat("1", 64), strings.Repeat("2", 64)
	first, next := seed(20, firstHash, true), seed(16, nextHash, true)
	missing := seed(30, strings.Repeat("3", 64), false)
	var attempts sync.WaitGroup
	for range 5 {
		attempts.Go(func() {
			if err := activate(first, nil, true); err != nil {
				t.Errorf("concurrent activation: %v", err)
			}
		})
	}
	attempts.Wait()
	assertCurrent(first, 1)
	stale := &CatalogCoverageOverride{strings.Repeat("0", 64), nextHash, "synthetic-owner", "verified synthetic exception"}
	approved := &CatalogCoverageOverride{firstHash, nextHash, "synthetic-owner", "verified synthetic exception"}
	for _, tc := range []struct {
		batch    pgtype.UUID
		override *CatalogCoverageOverride
	}{{missing, nil}, {next, nil}, {next, stale}} {
		if err := activate(tc.batch, tc.override, true); err == nil {
			t.Fatal("unapproved publication accepted")
		}
		assertCurrent(first, 1)
	}
	if _, err := pool.Exec(ctx, "UPDATE charging_catalog_current SET held=TRUE WHERE singleton"); err != nil {
		t.Fatal(err)
	}
	if err := activate(next, approved, true); err == nil {
		t.Fatal("held publication accepted")
	}
	assertCurrent(first, 1)
	if _, err := pool.Exec(ctx, "UPDATE charging_catalog_current SET held=FALSE WHERE singleton"); err != nil {
		t.Fatal(err)
	}
	if err := activate(next, approved, false); err != nil {
		t.Fatal(err)
	}
	assertCurrent(first, 1)
	if err := activate(next, approved, true); err != nil {
		t.Fatal(err)
	}
	if err := activate(next, nil, true); err != nil {
		t.Fatal(err)
	}
	assertCurrent(next, 2)
	if err := activate(first, nil, true); err == nil {
		t.Fatal("superseded batch unexpectedly accepted")
	}
	assertCurrent(next, 2)
	var actor, reason string
	err := pool.QueryRow(ctx, "SELECT override_actor,override_reason FROM charging_catalog_activations WHERE batch_id=$1", next).
		Scan(&actor, &reason)
	if err != nil || actor != approved.Actor || reason != approved.Reason {
		t.Fatalf("override audit actor=%q reason=%q err=%v", actor, reason, err)
	}

}
