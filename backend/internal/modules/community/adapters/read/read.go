package read

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
)

// Reader serves current prices from the indexed projection row: one
// lookup per exact key, never a history replay.
type Reader struct {
	pool  *pgxpool.Pool
	clock func() time.Time
}

// NewReader wires the owned generated queries to a pool.
func NewReader(pool *pgxpool.Pool) *Reader {
	return &Reader{pool: pool, clock: time.Now}
}

// PriceView is one query-time price projection. Amount and reference
// fields travel only with a live AVAILABLE verdict; expired rows
// surface as UNKNOWN with STALE freshness (B-BR-008).
type PriceView struct {
	Found             bool
	Availability      string
	Confidence        string
	Freshness         string
	AmountMilliBrl    int64
	HasAmount         bool
	Supporters        int
	Confirmations     int
	RepresentativeID  string
	AnchorReceivedAt  time.Time
	HasAnchor         bool
	ExpiresAt         time.Time
	HasExpiry         bool
	AlgorithmVersion  string
	ProjectionVersion int64
	GeneratedAt       time.Time
}

// CurrentPrice loads one exact key and applies query-time expiry and
// HIGH decay at read, independent of worker health. Missing keys
// report Found=false: callers render null/UNKNOWN, never an official
// substitution.
func (r *Reader) CurrentPrice(ctx context.Context, stationID, product, unit, condition, qualifier string) (PriceView, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return PriceView{}, err
	}
	row, err := community.New(r.pool).GetProjection(ctx, community.GetProjectionParams{
		StationID: uid, FuelProduct: product, Unit: unit,
		ConditionKind: condition, QualifierKey: qualifier,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PriceView{}, nil
		}
		return PriceView{}, err
	}
	now := time.Now()
	if r.clock != nil {
		now = r.clock()
	}
	return mapView(row, now), nil
}

// mapView applies the read-time policy over one stored row: expiry
// demotes to UNKNOWN without amount, and HIGH decays to MEDIUM once
// its anchor photo requirement fails. Pure and unit-tested.
func mapView(row community.CommunityCurrentPrice, now time.Time) PriceView {
	view := PriceView{
		Found: true, Availability: row.Availability, Confidence: row.Confidence,
		Freshness:  freshnessOf(row, now),
		Supporters: int(row.IndependentSupporters), Confirmations: int(row.ConfirmationCount),
		AlgorithmVersion: row.AlgorithmVersion, ProjectionVersion: row.ProjectionVersion,
	}
	if row.ComputedAt.Valid {
		view.GeneratedAt = row.ComputedAt.Time
	}
	if row.AnchorReceivedAt.Valid {
		view.AnchorReceivedAt, view.HasAnchor = row.AnchorReceivedAt.Time, true
	}
	if row.ExpiresAt.Valid {
		view.ExpiresAt, view.HasExpiry = row.ExpiresAt.Time, true
	}
	if row.AmountMilliBrl.Valid {
		view.AmountMilliBrl, view.HasAmount = row.AmountMilliBrl.Int64, true
	}
	if row.RepresentativeObservationID.Valid {
		view.RepresentativeID = uuidString(row.RepresentativeObservationID)
	}
	if view.HasExpiry && !now.Before(view.ExpiresAt) {
		view.Availability, view.Confidence = "UNKNOWN", ""
		view.HasAmount, view.RepresentativeID = false, ""
		view.Freshness = "STALE"
		return view
	}
	if view.Confidence == "HIGH" && (!view.HasAnchor || now.Sub(view.AnchorReceivedAt) > 6*time.Hour) {
		view.Confidence = "MEDIUM"
	}
	return view
}

func freshnessOf(row community.CommunityCurrentPrice, now time.Time) string {
	if !row.AnchorReceivedAt.Valid {
		return "UNKNOWN"
	}
	age := now.Sub(row.AnchorReceivedAt.Time)
	if age < 0 {
		age = 0
	}
	switch {
	case age <= 6*time.Hour:
		return "FRESH"
	case age <= 48*time.Hour:
		return "AGING"
	default:
		return "STALE"
	}
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("read: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func pgTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}
