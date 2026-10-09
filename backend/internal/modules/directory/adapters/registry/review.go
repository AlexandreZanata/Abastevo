package registry

// Location review gate (RST-06). Corroborated candidate points promote to
// reviewed assertions through the existing staging ownership; rejection
// stages nothing and returns a typed error, so uncorroborated points can
// never reach the 150m capture rule. The CRS transform stays inside
// PostGIS/PROJ behind the Transformer port: Go plumbs coordinates and
// corroboration policy, never datum math.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
)

// SourceReview is the frozen source key for audited location-review runs.
const SourceReview = "review"

// ReviewParserVersion stamps review runs.
const ReviewParserVersion = "review-v1"

// ReviewCorroborationM bounds primary/corroborator agreement: the same
// 150 metres the photo-capture gate enforces, applied here as an
// independent-sample proximity requirement before any promotion.
const ReviewCorroborationM = 150.0

// ReviewWindowDays bounds corroborating observation dates. Samples
// farther apart belong to different site states until a reviewer proves
// otherwise outside this gate.
const ReviewWindowDays = 90

// crsToSRID maps staged CRS labels to PostGIS SRIDs. Anything else fails
// at the Go gate before reaching SQL; coordinates are never guessed
// across reference systems.
func crsToSRID(crs string) (int32, error) {
	switch crs {
	case "EPSG:4674", "SIRGAS2000":
		return 4674, nil
	case "EPSG:4326", "WGS84":
		return 4326, nil
	default:
		return 0, fmt.Errorf("registry: unsupported CRS %q", crs)
	}
}

// Transformer converts staged coordinates to EPSG:4326. Production runs
// through PostGIS/PROJ; tests use a fake that only passes 4326 through.
type Transformer interface {
	Transform(ctx context.Context, lat, lon float64, crs string) (lat4326, lon4326 float64, err error)
}

// Transform converts one staged point to EPSG:4326 through PostGIS/PROJ.
func (s *PGStore) Transform(ctx context.Context, lat, lon float64, crs string) (float64, float64, error) {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return 0, 0, fmt.Errorf("registry: non-finite coordinates")
	}
	srid, err := crsToSRID(crs)
	if err != nil {
		return 0, 0, err
	}
	row, err := s.Q.TransformPoint(ctx, directory.TransformPointParams{Lon: lon, Lat: lat, Srid: srid})
	if err != nil {
		return 0, 0, err
	}
	return row.Latitude, row.Longitude, nil
}

// haversineM is the corroboration distance in metres (spherical earth).
// Integration pins it against PostGIS ST_Distance; policy only needs the
// 150m bound, never survey precision.
func haversineM(latA, lonA, latB, lonB float64) float64 {
	const radius = 6371000.0
	toRad := math.Pi / 180
	latARad, latBRad := latA*toRad, latB*toRad
	half := math.Sin((latB-latA)*toRad/2)*math.Sin((latB-latA)*toRad/2) +
		math.Cos(latARad)*math.Cos(latBRad)*math.Sin((lonB-lonA)*toRad/2)*math.Sin((lonB-lonA)*toRad/2)
	return 2 * radius * math.Asin(math.Sqrt(half))
}

// ReviewDecision is one evidence-backed promotion.
type ReviewDecision struct {
	RunID          string
	AssertionID    string
	CorroboratorID string
	DistanceM      float64
}

// ReviewCandidate promotes one staged candidate to a reviewed assertion
// when a second independent sample corroborates it: same exact CNPJ,
// both from complete runs, both unknown quality with supported CRS,
// transformed agreement within 150m and observation dates within 90
// days. Every rule is checked before any write; rejection stages
// nothing. Replay converges on the deterministic snapshot key.
func ReviewCandidate(ctx context.Context, store Store, tx Transformer, candidateID, corroboratorID, reviewer string) (ReviewDecision, error) {
	fail := func(format string, args ...any) (ReviewDecision, error) {
		return ReviewDecision{}, fmt.Errorf("registry: "+format, args...)
	}
	if strings.TrimSpace(reviewer) == "" {
		return fail("review without reviewer")
	}
	if candidateID == "" || corroboratorID == "" || candidateID == corroboratorID {
		return fail("review needs two distinct evidence ids")
	}
	candidate, candidateState, err := store.GetAssertion(ctx, candidateID)
	if err != nil {
		return fail("review candidate lookup: %v", err)
	}
	corroborator, corroboratorState, err := store.GetAssertion(ctx, corroboratorID)
	if err != nil {
		return fail("review corroborator lookup: %v", err)
	}
	if candidateState != "complete" || corroboratorState != "complete" {
		return fail("review evidence must come from complete runs")
	}
	for name, assertion := range map[string]Assertion{"candidate": candidate, "corroborator": corroborator} {
		if !assertion.HasCoords {
			return fail("review %s carries no point", name)
		}
		if assertion.LocationQuality != "unknown" {
			return fail("only unknown-quality rows promote, %s is %q", name, assertion.LocationQuality)
		}
		if _, err := crsToSRID(assertion.CRS); err != nil {
			return fail("review %s: %v", name, err)
		}
	}
	if candidate.SourceKey != corroborator.SourceKey {
		return fail("review evidence spans two establishments")
	}
	latA, lonA, err := tx.Transform(ctx, candidate.Latitude, candidate.Longitude, candidate.CRS)
	if err != nil {
		return fail("review transform: %v", err)
	}
	latB, lonB, err := tx.Transform(ctx, corroborator.Latitude, corroborator.Longitude, corroborator.CRS)
	if err != nil {
		return fail("review transform: %v", err)
	}
	distance := haversineM(latA, lonA, latB, lonB)
	if distance > ReviewCorroborationM {
		return fail("review samples disagree by %.0f m", distance)
	}
	if !candidate.EffectiveDate.Valid || !corroborator.EffectiveDate.Valid {
		return fail("review evidence without observation dates")
	}
	window := candidate.EffectiveDate.Time.Sub(corroborator.EffectiveDate.Time)
	if window < 0 {
		window = -window
	}
	if window > time.Duration(ReviewWindowDays)*24*time.Hour {
		return fail("review samples %d days apart", int(window.Hours()/24))
	}

	sum := sha256.Sum256([]byte("review-v1|" + candidate.Checksum + "|" + corroborator.Checksum + "|" + reviewer))
	checksum := hex.EncodeToString(sum[:])
	snapshot := "review:" + candidateID
	runID := newUUID()
	if _, created, err := store.CreateRun(ctx, runID, SourceReview, snapshot, checksum); err != nil {
		return fail("review run: %v", err)
	} else if !created {
		existing, err := store.GetRun(ctx, SourceReview, snapshot)
		if err != nil {
			return fail("review replay lookup: %v", err)
		}
		assertions, err := store.ListAssertions(ctx, existing.RunID)
		if err != nil {
			return fail("review replay list: %v", err)
		}
		for _, assertion := range assertions {
			if assertion.Checksum == checksum {
				return ReviewDecision{RunID: existing.RunID, AssertionID: assertion.ID, CorroboratorID: corroboratorID, DistanceM: distance}, nil
			}
		}
		return fail("review replay diverged")
	}
	staged, err := store.StageAssertion(ctx, Assertion{
		Source:           SourceReview,
		SourceKey:        candidate.SourceKey,
		Checksum:         checksum,
		DisplayName:      candidate.DisplayName,
		Address:          candidate.Address,
		MunicipalityCode: candidate.MunicipalityCode,
		State:            candidate.State,
		AuthState:        candidate.AuthState,
		Eligibility:      candidate.Eligibility,
		LocationQuality:  "reviewed",
		SourceReference:  "review by " + reviewer + " of " + candidate.SourceReference + " corroborated by " + corroborator.SourceReference,
		EffectiveDate:    candidate.EffectiveDate,
		Latitude:         latA,
		Longitude:        lonA,
		HasCoords:        true,
		CRS:              "EPSG:4326",
	}.WithRun(runID))
	if err != nil {
		_ = store.FinishRun(ctx, runID, "failed", 0, 0, 0, "stage_error")
		return fail("review stage: %v", err)
	}
	if !staged {
		_ = store.FinishRun(ctx, runID, "failed", 0, 0, 0, "duplicate_review")
		return fail("review content already staged elsewhere")
	}
	if err := store.FinishRun(ctx, runID, "complete", 1, 0, 0, ""); err != nil {
		return fail("review finish: %v", err)
	}
	assertions, err := store.ListAssertions(ctx, runID)
	if err != nil {
		return fail("review list: %v", err)
	}
	for _, assertion := range assertions {
		if assertion.Checksum == checksum {
			return ReviewDecision{RunID: runID, AssertionID: assertion.ID, CorroboratorID: corroboratorID, DistanceM: distance}, nil
		}
	}
	return fail("review staged row not found")
}
