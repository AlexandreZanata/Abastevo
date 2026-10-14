//go:build integration

package adapters

import (
	"context"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"testing"
	"time"
)

func TestCommunityFirstMigrationRemovesOnlyGeometryAndSchedulesBoundedRebuild(t *testing.T) {
	s, pool, stationID := freshStore(t)
	ctx := context.Background()
	obs := validatedFixture(t, s, stationID, "policy-upgrade")
	if err := s.UpsertSignals(ctx, obs.ID, app.Bands{Proximity: app.ProximityNear, Recency: app.RecencyFresh, Capture: app.CaptureFresh, Photo: app.PhotoNone, Regional: app.RegionalUnknown, RiskCodes: []string{app.RiskTeleportSuspect, app.RiskNoPhoto}, NeedsReview: true, PolicyVersion: app.SignalsV1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	next := time.Now().Add(time.Hour)
	if err := s.SaveProjection(ctx, projectionKey(), priceResult(), obs.ID, time.Now(), time.Now().Add(-48*time.Hour), &next, "consensus-v1"); err != nil {
		t.Fatal(err)
	}
	raw, err := migrations.Files.ReadFile("000092_community_first_projections.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := s.LoadSignals(ctx, obs.ID)
	if err != nil || old.PolicyVersion != app.SignalsV1 {
		t.Fatal("rollback changed projection", err)
	}
	for range 2 {
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.LoadSignals(ctx, obs.ID)
	if err != nil || got.Proximity != app.ProximityUnknown || got.PolicyVersion != "signals-community-v2" || !got.NeedsReview || len(got.RiskCodes) != 1 || got.RiskCodes[0] != app.RiskNoPhoto {
		t.Fatalf("upgraded signals %+v %v", got, err)
	}
	var due bool
	if err := pool.QueryRow(ctx, `SELECT next_recompute_at<=now() FROM community_current_prices WHERE station_id=$1`, stationID).Scan(&due); err != nil || !due {
		t.Fatal("bounded sweeper not scheduled", err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_projection_inputs`).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("historical audit rewritten", err)
	}
}
