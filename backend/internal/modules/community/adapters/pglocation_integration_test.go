//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

const (
	e2eStationA = "aaaaaaaa-1111-4111-8111-000000000001"
	e2eStationB = "aaaaaaaa-1111-4111-8111-000000000002"
	// Station A: São Paulo Sé (lon, lat). Station B: ~50 km north.
	e2eLonA, e2eLatA = -46.633309, -23.55052
	e2eLonB, e2eLatB = -46.633309, -23.10052
)

func seedPreciseStation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, lon, lat float64) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto E2E')`, id); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE directory_stations SET current_point = ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography,
			current_quality = 'reviewed' WHERE id = $3`, lon, lat, id); err != nil {
		t.Fatalf("seed point: %v", err)
	}
}

// locateViaPostGIS mirrors the api composition root: precise-site
// mapping plus the directory distance query over the transient fix.
func locateViaPostGIS(pool *pgxpool.Pool) func(context.Context, string, float64, float64) (float64, communityapp.StationSite, error) {
	return func(ctx context.Context, stationID string, lat, lon float64) (float64, communityapp.StationSite, error) {
		var distanceM float64
		var quality string
		err := pool.QueryRow(ctx,
			`SELECT ST_Distance(current_point, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography),
				current_quality FROM directory_stations WHERE id = $3 AND current_point IS NOT NULL`,
			lon, lat, stationID).Scan(&distanceM, &quality)
		if err != nil {
			return 0, communityapp.SiteUnknown, nil
		}
		if quality != "reviewed" {
			return 0, communityapp.SiteUnknown, nil
		}
		return distanceM, communityapp.SitePrecise, nil
	}
}

func pairDistanceViaPostGIS(pool *pgxpool.Pool) func(context.Context, string, string) (float64, error) {
	return func(ctx context.Context, a, b string) (float64, error) {
		var distanceM float64
		err := pool.QueryRow(ctx,
			`SELECT ST_Distance(x.current_point, y.current_point)
			 FROM directory_stations x JOIN directory_stations y ON y.id = $2
			 WHERE x.id = $1 AND x.current_point IS NOT NULL AND y.current_point IS NOT NULL`,
			a, b).Scan(&distanceM)
		if err != nil {
			return 0, err
		}
		return distanceM, nil
	}
}

func e2eSubmitPorts(s *Store, pool *pgxpool.Pool, now time.Time) communityapp.Ports {
	return communityapp.Ports{
		Clock: func() time.Time { return now },
		NewID: newUUIDv4,
		Attribution: func(_ context.Context, id string) (string, error) {
			return "tok-" + id, nil
		},
		CheckQuota: func(context.Context, string, string) (time.Duration, error) { return 0, nil },
		Idempotent: func(_ context.Context, _ communityapp.IdempotencyKey, _ []byte, run func(context.Context) (communityapp.Outcome, error)) (communityapp.Outcome, error) {
			return run(context.Background())
		},
		EnqueueJob: func(context.Context, pgx.Tx, string, []byte, string) error { return nil },
		Store:      s,
		Locate:     locateViaPostGIS(pool),
		LastSite: func(ctx context.Context, ref string) (string, time.Time, bool, error) {
			return s.LastObservationSite(ctx, ref)
		},
		StationDistance: pairDistanceViaPostGIS(pool),
	}
}

func e2eLocationDTO(clientID, stationID string, captured time.Time, lat, lon float64) communityapp.SubmitDTO {
	acc := 25.0
	return communityapp.SubmitDTO{
		ClientSubmissionID: clientID, StationID: stationID,
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		Location: &communityapp.LocationEvidence{
			ClaimedVerdict: "VERIFIED", PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, AccuracyMeters: &acc,
			CapturedAt: &captured, Latitude: &lat, Longitude: &lon,
		},
	}
}

func e2eCaller(id string) communityapp.Caller {
	return communityapp.Caller{ContributorID: id, Fingerprint: "fp-" + id, Token: "tok-" + id}
}

// TestSubmitLocationEndToEnd drives verified intake on real PostGIS:
// a fix at station A stores VERIFIED/NEAR, derives NEAR signals, and
// a 50 km jump a minute later flags teleport-suspect for review.
func TestSubmitLocationEndToEnd(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	seedPreciseStation(t, ctx, pool, e2eStationA, e2eLonA, e2eLatA)
	seedPreciseStation(t, ctx, pool, e2eStationB, e2eLonB, e2eLatB)

	t0 := time.Now().Truncate(time.Second)
	ports := e2eSubmitPorts(s, pool, t0)
	res, err := communityapp.Submit(ctx, ports, e2eCaller("c1"), "POST", "/v1/observations",
		"k-1", []byte(`{"sub":"1"}`),
		e2eLocationDTO("sub-1", e2eStationA, t0.Add(-30*time.Second), e2eLatA, e2eLonA))
	if err != nil {
		t.Fatalf("submit A: %v", err)
	}
	stored, err := s.Observation(ctx, res.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LocationVerdict != "VERIFIED" || stored.LocationProximity != "NEAR" {
		t.Fatalf("stored bands = %q/%q, want VERIFIED/NEAR", stored.LocationVerdict, stored.LocationProximity)
	}
	if stored.LocationReason != "" {
		t.Fatalf("clean submit must carry no reason, got %q", stored.LocationReason)
	}

	bands, err := communityapp.DeriveSignals(ctx, communityapp.SignalPorts{
		Clock: func() time.Time { return t0 },
		Store: s,
		Station: func(context.Context, string) (communityapp.StationSite, error) {
			return communityapp.SitePrecise, nil
		},
		Photo:    func(context.Context, string) (bool, int, error) { return false, 0, nil },
		Regional: func(context.Context, string, string, string) (int64, bool, error) { return 0, false, nil },
	}, res.ObservationID)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if bands.Proximity != communityapp.ProximityNear {
		t.Errorf("derived proximity = %q, want NEAR", bands.Proximity)
	}

	// 50 km jump a minute later: accepted fact, teleport review flag.
	t1 := t0.Add(time.Minute)
	ports.Clock = func() time.Time { return t1 }
	res2, err := communityapp.Submit(ctx, ports, e2eCaller("c1"), "POST", "/v1/observations",
		"k-2", []byte(`{"sub":"2"}`),
		e2eLocationDTO("sub-2", e2eStationB, t1.Add(-30*time.Second), e2eLatB, e2eLonB))
	if err != nil {
		t.Fatalf("submit B: %v", err)
	}
	stored2, err := s.Observation(ctx, res2.ObservationID)
	if err != nil {
		t.Fatal(err)
	}
	if stored2.LocationReason != "teleport-suspect" {
		t.Errorf("jump must flag teleport-suspect, got %q", stored2.LocationReason)
	}
	bands2, err := communityapp.DeriveSignals(ctx, communityapp.SignalPorts{
		Clock: func() time.Time { return t1 },
		Store: s,
		Station: func(context.Context, string) (communityapp.StationSite, error) {
			return communityapp.SitePrecise, nil
		},
		Photo:    func(context.Context, string) (bool, int, error) { return false, 0, nil },
		Regional: func(context.Context, string, string, string) (int64, bool, error) { return 0, false, nil },
	}, res2.ObservationID)
	if err != nil {
		t.Fatalf("derive B: %v", err)
	}
	found := false
	for _, r := range bands2.RiskCodes {
		if r == communityapp.RiskTeleportSuspect {
			found = true
		}
	}
	if !found || !bands2.NeedsReview {
		t.Errorf("teleport must route to review: %+v", bands2)
	}
}

// TestObservationRowsCarryNoCoordinates asserts the acceptance rule
// directly against the schema: no long-lived precise GPS means no
// latitude/longitude/coordinate column may exist on the facts table.
func TestObservationRowsCarryNoCoordinates(t *testing.T) {
	s, pool, _ := freshStore(t)
	_ = s
	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'community_observations'
		  AND (column_name ILIKE '%lat%' OR column_name ILIKE '%lon%'
		   OR column_name ILIKE '%coord%' OR column_name ILIKE '%gps%'
		   OR column_name ILIKE '%fix%')`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d coordinate-like columns on community_observations", count)
	}
}
