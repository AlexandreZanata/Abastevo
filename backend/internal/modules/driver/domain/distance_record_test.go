package domain

import (
	"errors"
	"testing"
	"time"
)

func recordBase() DistanceRecord {
	return DistanceRecord{
		ID: "11111111-1111-4111-8111-111111111111", Origin: "android", VehicleID: "22222222-2222-4222-8222-222222222222",
		Segment: "segment-a", Basis: "ODOMETER", From: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC),
		To: time.Date(2026, 10, 26, 13, 0, 0, 0, time.UTC), Timezone: "America/Cuiaba", Complete: true,
		Quality: "MEASURED",
	}
}

func TestDistanceOdometerEndpointsAndExactHash(t *testing.T) {
	start, end := int64(10_000_000), int64(10_500_000)
	r := recordBase()
	r.Metres = new(int64)
	*r.Metres = 500_000
	r.StartOdometerMetres = &start
	r.EndOdometerMetres = &end
	normalized, e := r.Normalize()
	if e != nil {
		t.Fatal(e)
	}
	hash, e := normalized.ContentHash()
	if e != nil || len(hash) != 64 {
		t.Fatal(hash, e)
	}
	again, _ := r.ContentHash()
	if again != hash {
		t.Fatal("unstable hash")
	}
	bad := r
	bad.Metres = new(int64)
	*bad.Metres = 500_001
	if _, e = bad.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("distance must equal endpoints", e)
	}
	reversed := r
	reversed.StartOdometerMetres = &end
	reversed.EndOdometerMetres = &start
	if _, e = reversed.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("reversed endpoints", e)
	}
}

func TestDistanceCoverageQualityAndGpsPolicy(t *testing.T) {
	gps := recordBase()
	gps.Basis = "GPS_ESTIMATE"
	gps.Quality = "ESTIMATED"
	gps.Complete = false
	gps.HasGap = true
	gps.QualityNote = "gps-gap"
	if _, e := gps.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("gps requires a versioned policy", e)
	}
	version := int32(2)
	gps.QualityVersion = &version
	if _, e := gps.Normalize(); e != nil {
		t.Fatal(e)
	}
	impossible := gps
	impossible.Complete = true
	impossible.HasGap = true
	impossible.Metres = new(int64)
	if _, e := impossible.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("complete with gap", e)
	}
	completeWithoutMetres := gps
	completeWithoutMetres.Complete = true
	completeWithoutMetres.HasGap = false
	completeWithoutMetres.QualityVersion = &version
	if _, e := completeWithoutMetres.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("complete without metres", e)
	}
	mismatch := recordBase()
	mismatch.Basis = "PAID_REPORTED"
	mismatch.Quality = "MEASURED"
	if _, e := mismatch.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("basis/quality mismatch", e)
	}
	nanos := recordBase()
	nanos.From = nanos.From.Add(500 * time.Nanosecond)
	if _, e := nanos.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("sub-microsecond instant", e)
	}
}
