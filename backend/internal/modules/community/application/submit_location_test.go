package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

func acc(v float64) *float64 { return &v }

func locationDTO(
	verdict string,
	accuracy *float64,
	captured *time.Time,
	lat, lon *float64,
) *LocationEvidence {
	return &LocationEvidence{
		ClaimedVerdict: verdict, PermissionGranted: true, HasFix: true,
		SourceInfoPresent: true, AccuracyMeters: accuracy,
		CapturedAt: captured, Latitude: lat, Longitude: lon,
	}
}

func locationPorts() Ports {
	p := testPorts()
	p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 50, SitePrecise, nil
	}
	p.LastSite = func(context.Context, string) (string, time.Time, bool, error) {
		return "", time.Time{}, false, nil
	}
	p.StationDistance = func(context.Context, string, string) (float64, error) {
		return 0, nil
	}
	return p
}

func locationObs() domain.Observation {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	obs, _, err := domain.NewObservation(domain.Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", ContributorRef: "tok-c1",
		ClientSubmissionID: "sub-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD", ReceivedAt: now,
	})
	if err != nil {
		panic(err)
	}
	return obs
}

func TestAttachLocationSkipsWithoutEvidence(t *testing.T) {
	// Nil ports + nil evidence preserves pre-location behavior exactly.
	obs, err := attachLocation(context.Background(), testPorts(), "tok-c1",
		SubmitDTO{}, locationObs(), time.Now())
	if err != nil {
		t.Fatalf("nil evidence refused: %v", err)
	}
	if obs.LocationVerdict != "" || obs.LocationProximity != "" || obs.LocationReason != "" {
		t.Errorf("nil evidence must leave bands empty: %+v", obs)
	}
}

func TestAttachLocationFailsClosedWithoutPorts(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	if _, err := attachLocation(context.Background(), testPorts(), "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second)); err == nil {
		t.Error("unconfigured intake accepted location evidence")
	}
}

func TestAttachLocationRefusesForgedVerdict(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	dto := testDTO()
	// Accuracy 500 m can never verify: the VERIFIED claim is forged.
	dto.Location = locationDTO(FixVerified, acc(500), &captured, acc(-23.5), acc(-46.6))
	_, err := attachLocation(context.Background(), locationPorts(), "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if !errors.Is(err, ErrLocationForged) {
		t.Errorf("forged claim = %v, want location-forged", err)
	}
}

func TestAttachLocationStoresVerifiedNear(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	got, err := attachLocation(context.Background(), locationPorts(), "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if err != nil {
		t.Fatalf("verified = %v", err)
	}
	if got.LocationVerdict != FixVerified || got.LocationProximity != ProximityNear {
		t.Errorf("bands = %q/%q, want VERIFIED/NEAR", got.LocationVerdict, got.LocationProximity)
	}
	if got.LocationReason != "" {
		t.Errorf("clean verify must carry no reason, got %q", got.LocationReason)
	}
}

func TestAttachLocationFarStaysVerified(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	p := locationPorts()
	p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 5000, SitePrecise, nil
	}
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	got, err := attachLocation(context.Background(), p, "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if err != nil {
		t.Fatalf("far fix = %v", err)
	}
	if got.LocationVerdict != FixVerified || got.LocationProximity != ProximityFar {
		t.Errorf("bands = %q/%q, want VERIFIED/FAR", got.LocationVerdict, got.LocationProximity)
	}
}

func TestAttachLocationUnknownWithoutFixOrSite(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	// No coordinates: verified fix, unknown proximity.
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, nil, nil)
	got, err := attachLocation(context.Background(), locationPorts(), "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationVerdict != FixVerified || got.LocationProximity != ProximityUnknown {
		t.Errorf("bands = %q/%q, want VERIFIED/UNKNOWN", got.LocationVerdict, got.LocationProximity)
	}
	// Imprecise station: same outcome, no proximity from centroids.
	p := locationPorts()
	p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 10, SiteCentroid, nil
	}
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	got, err = attachLocation(context.Background(), p, "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationProximity != ProximityUnknown {
		t.Errorf("centroid site must stay UNKNOWN, got %q", got.LocationProximity)
	}
}

func TestAttachLocationKeepsHonestStatesSilent(t *testing.T) {
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	dto := testDTO()
	dto.Location = &LocationEvidence{
		ClaimedVerdict: FixSimulated, PermissionGranted: true, HasFix: true,
		SourceInfoPresent: true, Simulated: true,
		AccuracyMeters: acc(10), CapturedAt: &captured,
	}
	got, err := attachLocation(context.Background(), locationPorts(), "tok-c1",
		dto, locationObs(), captured.Add(30*time.Second))
	if err != nil {
		t.Fatalf("honest simulated refused: %v", err)
	}
	if got.LocationVerdict != FixSimulated || got.LocationProximity != ProximityUnknown {
		t.Errorf("bands = %q/%q, want SIMULATED/UNKNOWN", got.LocationVerdict, got.LocationProximity)
	}
	if got.LocationReason != FixReasonSimulatedSource {
		t.Errorf("reason = %q", got.LocationReason)
	}
}

func TestAttachLocationFlagsTeleport(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	captured := now.Add(-30 * time.Second)
	p := locationPorts()
	p.LastSite = func(context.Context, string) (string, time.Time, bool, error) {
		return "far-station-id", now.Add(-time.Minute), true, nil
	}
	p.StationDistance = func(context.Context, string, string) (float64, error) {
		return 50000, nil
	}
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	got, err := attachLocation(context.Background(), p, "tok-c1",
		dto, locationObs(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationReason != RiskTeleportSuspect {
		t.Errorf("reason = %q, want teleport-suspect", got.LocationReason)
	}
	if got.LocationVerdict != FixVerified || got.LocationProximity != ProximityNear {
		t.Errorf("teleport flags review without dropping bands: %+v", got)
	}
}

func TestAttachLocationTeleportNeedsBaseline(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	captured := now.Add(-30 * time.Second)
	// No baseline: first observations never teleport.
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	got, err := attachLocation(context.Background(), locationPorts(), "tok-c1",
		dto, locationObs(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationReason != "" {
		t.Errorf("first observation flagged: %q", got.LocationReason)
	}
	// Same station: staying put never teleports.
	p := locationPorts()
	p.LastSite = func(context.Context, string) (string, time.Time, bool, error) {
		return "d6c74c23-63db-4c24-a2e5-408cb23bad26", now.Add(-time.Minute), true, nil
	}
	got, err = attachLocation(context.Background(), p, "tok-c1", dto, locationObs(), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationReason != "" {
		t.Errorf("same station flagged: %q", got.LocationReason)
	}
}

func TestAttachLocationDegradesOnInfraErrors(t *testing.T) {
	// Infrastructure failures degrade to safe defaults instead of
	// refusing the price fact: UNKNOWN proximity can never become
	// verified, and a missing baseline never teleports.
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	captured := now.Add(-30 * time.Second)
	dto := testDTO()
	dto.Location = locationDTO(FixVerified, acc(25), &captured, acc(-23.5), acc(-46.6))
	p := locationPorts()
	p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 0, "", errors.New("postgis down")
	}
	got, err := attachLocation(context.Background(), p, "tok-c1",
		dto, locationObs(), now)
	if err != nil {
		t.Fatalf("locate outage refused the fact: %v", err)
	}
	if got.LocationVerdict != FixVerified || got.LocationProximity != ProximityUnknown {
		t.Errorf("outage must degrade to VERIFIED/UNKNOWN, got %q/%q",
			got.LocationVerdict, got.LocationProximity)
	}
	p = locationPorts()
	p.LastSite = func(context.Context, string) (string, time.Time, bool, error) {
		return "", time.Time{}, false, errors.New("db down")
	}
	p.StationDistance = func(context.Context, string, string) (float64, error) {
		return 0, errors.New("postgis down")
	}
	got, err = attachLocation(context.Background(), p, "tok-c1",
		dto, locationObs(), now)
	if err != nil {
		t.Fatalf("baseline outage refused the fact: %v", err)
	}
	if got.LocationReason != "" {
		t.Errorf("unmeasurable teleport must not flag, got %q", got.LocationReason)
	}
}
