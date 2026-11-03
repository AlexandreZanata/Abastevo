//go:build integration

package linking

import (
	"context"
	"strings"
	"testing"
)

func TestCarryCatalogReviewRefusesConflictingHumanDecisions(t *testing.T) {
	pool := freshDB(t)
	seed(t, pool)
	ctx := context.Background()
	const other = "55555555-5555-4555-8555-555555555555"
	const target = "66666666-6666-4666-8666-666666666666"
	for i, id := range []string{other, target} {
		key := strings.Repeat(string(rune('c'+i)), 64)
		if _, err := pool.Exec(ctx, `INSERT INTO charging_places
			(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points)
			VALUES($1,$2,$3,$3,'Synthetic Charge','BR','-23.5','-46.6','PUBLIC',0)`, id, batchID, key); err != nil {
			t.Fatal(err)
		}
	}
	store := Store{Pool: pool}
	for _, decision := range []Link{
		{PlaceID: placeID, StationID: stationA, State: Reviewed, Method: MethodManual, ReviewerRef: "synthetic-reviewer"},
		{PlaceID: other, StationID: stationB, State: Rejected, Method: MethodManual, ReviewerRef: "synthetic-reviewer"},
	} {
		if _, err := store.Review(ctx, decision); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := CarryCatalogReviewTx(ctx, tx, target, []string{other, placeID}); err == nil ||
		!strings.Contains(err.Error(), "conflicting human reviews") {
		t.Fatalf("expected review conflict, got %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Active(ctx, target); err != nil || found {
		t.Fatalf("conflicting link was written: %v %v", found, err)
	}
}
