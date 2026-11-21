//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

// The announced price and promotion stores run their real statements on
// PostGIS: a new announcement supersedes the active one of the same station,
// product and unit, a withdrawal is terminal, and the history can never be
// rewritten or deleted (EMP-STN-002, EMP-STN-004).
func TestStationCommerceStoreSupersedesWithdrawsAndKeepsHistory(t *testing.T) {
	pool, store, _, stationID := freshManageDB(t)
	ctx := context.Background()
	commerce := CommerceStore{Q: store.Q}
	const author = "00000000-0000-4000-8000-0000000000a1"
	from := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	price := application.StoredAnnouncedPrice{AuthorAccountID: author, Product: "DIESEL_S10", Unit: "L",
		PriceMilliBRL: 5999, Conditions: "synthetic", ValidFrom: from, ValidUntil: from.Add(24 * time.Hour)}

	first, err := commerce.InsertAnnouncedPrice(ctx, stationID, "00000000-0000-4000-8000-0000000000b1", price)
	if err != nil || first.State != "ACTIVE" || first.PriceMilliBRL != 5999 {
		t.Fatalf("first announcement was not stored: %+v %v", first, err)
	}
	price.PriceMilliBRL = 6099
	second, err := commerce.InsertAnnouncedPrice(ctx, stationID, "00000000-0000-4000-8000-0000000000b2", price)
	if err != nil || second.State != "ACTIVE" || second.PriceMilliBRL != 6099 {
		t.Fatalf("second announcement did not supersede the first: %+v %v", second, err)
	}
	other := price
	other.Product = "ETHANOL"
	if _, err := commerce.InsertAnnouncedPrice(ctx, stationID, "00000000-0000-4000-8000-0000000000b3", other); err != nil {
		t.Fatalf("another product could not be announced beside the first: %v", err)
	}
	states := func() map[string]string {
		t.Helper()
		listed, err := commerce.ListAnnouncedPrices(ctx, stationID)
		if err != nil {
			t.Fatal(err)
		}
		out := make(map[string]string, len(listed))
		for _, item := range listed {
			out[item.ID] = item.State
		}
		return out
	}
	if got := states(); len(got) != 3 || got[first.ID] != "SUPERSEDED" || got[second.ID] != "ACTIVE" {
		t.Fatalf("supersession left the wrong history: %v", got)
	}

	withdrawn, err := commerce.WithdrawAnnouncedPrice(ctx, stationID, second.ID)
	if err != nil || withdrawn.State != "WITHDRAWN" {
		t.Fatalf("active announcement was not withdrawn: %+v %v", withdrawn, err)
	}
	for _, terminal := range []string{second.ID, first.ID} {
		if _, err := commerce.WithdrawAnnouncedPrice(ctx, stationID, terminal); !errors.Is(err, application.ErrCommerceNotFound) {
			t.Fatalf("a withdrawn or superseded announcement was withdrawn again: %v", err)
		}
	}
	if _, err := commerce.WithdrawAnnouncedPrice(ctx, "00000000-0000-4000-8000-0000000000c1", second.ID); !errors.Is(err, application.ErrCommerceNotFound) {
		t.Fatalf("another station withdrew the announcement: %v", err)
	}

	refused := func(statement, code string) {
		t.Helper()
		_, err := pool.Exec(ctx, statement, first.ID)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != code {
			t.Fatalf("%q was not refused with %s: %v", statement, code, err)
		}
	}
	refused(`UPDATE station_announced_prices SET price_milli_brl=1 WHERE id=$1`, "23514")
	refused(`UPDATE station_announced_prices SET state='ACTIVE' WHERE id=$1`, "P0001")
	refused(`DELETE FROM station_announced_prices WHERE id=$1`, "P0001")
	if got := states(); got[first.ID] != "SUPERSEDED" || got[second.ID] != "WITHDRAWN" {
		t.Fatalf("a refused statement changed the history: %v", got)
	}

	promotion := application.StoredPromotion{AuthorAccountID: author, Label: "Synthetic week",
		Description: "Synthetic labelled promotion", ValidFrom: from, ValidUntil: from.Add(48 * time.Hour)}
	one, err := commerce.InsertPromotion(ctx, stationID, "00000000-0000-4000-8000-0000000000d1", promotion)
	if err != nil || one.State != "PUBLISHED" {
		t.Fatalf("promotion was not stored: %+v %v", one, err)
	}
	two, err := commerce.InsertPromotion(ctx, stationID, "00000000-0000-4000-8000-0000000000d2", promotion)
	if err != nil || two.State != "PUBLISHED" {
		t.Fatalf("the reused label did not replace the live promotion: %+v %v", two, err)
	}
	var live int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM station_promotions WHERE station_id=$1 AND state='PUBLISHED'`, stationID).Scan(&live); err != nil || live != 1 {
		t.Fatalf("label reuse left %d live promotions: %v", live, err)
	}
	if gone, err := commerce.WithdrawPromotion(ctx, stationID, two.ID); err != nil || gone.State != "WITHDRAWN" {
		t.Fatalf("promotion was not withdrawn: %+v %v", gone, err)
	}
	if _, err := commerce.WithdrawPromotion(ctx, stationID, one.ID); !errors.Is(err, application.ErrCommerceNotFound) {
		t.Fatalf("a replaced promotion was withdrawn again: %v", err)
	}
}
