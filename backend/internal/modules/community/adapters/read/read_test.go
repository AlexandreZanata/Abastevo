package read

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
)

func expiresAt(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func storedProjection(exp pgtype.Timestamptz, availability, confidence string, amount int64) community.CommunityCurrentPrice {
	return community.CommunityCurrentPrice{
		AmountMilliBrl: pgtype.Int8{Int64: amount, Valid: true},
		Availability:   availability, Confidence: confidence,
		IndependentSupporters: 2, ConfirmationCount: 1,
		AnchorReceivedAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC), Valid: true},
		ExpiresAt:        exp, ComputedAt: pgtype.Timestamptz{Time: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), Valid: true},
		ProjectionVersion: 3, AlgorithmVersion: "consensus-v1",
	}
}

func TestMapViewAppliesQueryTimeExpiry(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	fresh := storedProjection(expiresAt(now.Add(time.Hour)), "AVAILABLE", "LOW", 5999)
	view := mapView(fresh, now)
	if view.Availability != "AVAILABLE" || !view.HasAmount || view.AmountMilliBrl != 5999 {
		t.Errorf("fresh = %+v", view)
	}
	if view.Freshness != "FRESH" {
		t.Errorf("freshness = %q", view.Freshness)
	}
	stale := storedProjection(expiresAt(now.Add(-time.Hour)), "AVAILABLE", "LOW", 5999)
	view = mapView(stale, now)
	// B-BR-008: expired projections surface as UNKNOWN with STALE
	// freshness and no amount, excluded from current-price ranking.
	if view.Availability != "UNKNOWN" || view.HasAmount || view.Freshness != "STALE" {
		t.Errorf("expired = %+v", view)
	}
}

func TestMapViewDecaysHighWithoutFreshAnchor(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	// Anchor 7 h old: HIGH no longer holds its 6 h photo requirement.
	old := storedProjection(expiresAt(now.Add(40*time.Hour)), "AVAILABLE", "HIGH", 5999)
	old.AnchorReceivedAt = pgTime(now.Add(-7 * time.Hour))
	view := mapView(old, now)
	if view.Confidence != "MEDIUM" {
		t.Errorf("decayed confidence = %q, want MEDIUM", view.Confidence)
	}
	if view.Availability != "AVAILABLE" || !view.HasAmount {
		t.Errorf("decayed view lost the price: %+v", view)
	}
	recent := storedProjection(expiresAt(now.Add(40*time.Hour)), "AVAILABLE", "HIGH", 5999)
	recent.AnchorReceivedAt = pgTime(now.Add(-time.Hour))
	view = mapView(recent, now)
	if view.Confidence != "HIGH" {
		t.Errorf("fresh HIGH decayed: %+v", view)
	}
}

func TestMapViewKeepsDisputedWithoutAmount(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	row := storedProjection(expiresAt(now.Add(time.Hour)), "DISPUTED", "", 0)
	row.AmountMilliBrl.Valid = false
	row.RepresentativeObservationID.Valid = false
	view := mapView(row, now)
	if view.Availability != "DISPUTED" || view.HasAmount {
		t.Errorf("disputed = %+v", view)
	}
}
