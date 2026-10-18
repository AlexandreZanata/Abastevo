package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// ProjectionPolicy is the frozen policy identity for official dimensions.
const ProjectionPolicy = "pmqc-dimensions-v2026-10a"

// ErrProjectionBasisStale is returned when the requested revision is not the
// current published revision, or stopped being it before the write committed.
// A stale basis is never applied and never rewritten.
var ErrProjectionBasisStale = errors.New("quality projections: basis is not current")

// ProjectionReport summarizes one recomputation in deterministic counts.
type ProjectionReport struct {
	RevisionID  string
	Stations    int
	Fuels       int
	Samples     int64
	Applied     int64
	InputDigest string
}

// ProjectionStore recomputes derived dimensions from frozen official facts.
// It reads only quality ownership tables; community reports, private signals
// and payments never participate.
type ProjectionStore struct {
	pool *pgxpool.Pool
}

func NewProjectionStore(pool *pgxpool.Pool) *ProjectionStore {
	return &ProjectionStore{pool: pool}
}

// RecomputeRevision judges every active linked sample of one published
// revision and upserts per-station/per-fuel dimensions atomically. The whole
// recomputation is one transaction: either every station row of this basis is
// visible together or none is. A basis that is not (or is no longer) the
// current published revision writes nothing.
func (s *ProjectionStore) RecomputeRevision(ctx context.Context, source, stream, revisionID string) (ProjectionReport, error) {
	report := ProjectionReport{RevisionID: revisionID}
	rev, err := mustUUID(revisionID)
	if err != nil {
		return report, errors.New("quality projections: malformed revision")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return report, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	q := quality.New(tx)
	current, err := q.LockProjectionBasis(ctx, quality.LockProjectionBasisParams{Source: source, Stream: stream})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return report, ErrProjectionBasisStale
		}
		return report, err
	}
	if current != rev {
		return report, ErrProjectionBasisStale
	}
	rows, err := q.ListLinkedAssayRows(ctx, rev)
	if err != nil {
		return report, err
	}
	samples := groupLinkedSamples(rows)
	fuels := map[string]bool{}
	for _, sample := range samples {
		fuels[string(sample.facts.Fuel)] = true
	}
	report.Samples = int64(len(samples))
	report.InputDigest = digestSamples(samples)
	dimensions := projectDimensions(samples)
	// Deterministic write order keeps concurrent replays and lock acquisition
	// stable across runs.
	keys := make([]projectionKey, 0, len(dimensions))
	for key := range dimensions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].station != keys[j].station {
			return keys[i].station < keys[j].station
		}
		return keys[i].fuel < keys[j].fuel
	})
	for _, key := range keys {
		dim := dimensions[key]
		station, terr := mustUUID(key.station)
		if terr != nil {
			return report, errors.New("quality projections: malformed station")
		}
		oldest, newest, derr := projectionDays(dim)
		if derr != nil {
			return report, derr
		}
		applied, aerr := q.StageStationFuelProjection(ctx, quality.StageStationFuelProjectionParams{
			StationID: station, Fuel: string(key.fuel),
			Methodology: dim.MethodVersion, Policy: ProjectionPolicy,
			BasisSource: source, BasisStream: stream, BasisRevision: rev,
			InputDigest: report.InputDigest,
			Samples:     dim.Samples, Visits: dim.Visits,
			Conforming: dim.Conforming, Nonconforming: dim.Nonconforming, Unresolved: dim.Unresolved,
			OldestObservedOn: oldest, NewestObservedOn: newest,
			ScoreState: string(domain.ScoreUnscored),
		})
		if aerr != nil {
			return report, aerr
		}
		report.Applied += applied
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	committed = true
	stations := map[string]bool{}
	for _, key := range keys {
		stations[key.station] = true
	}
	report.Stations = len(stations)
	report.Fuels = len(fuels)
	return report, nil
}

// projectionKey is the station/fuel identity of one derived row.
type projectionKey struct {
	station string
	fuel    domain.Fuel
}

type linkedSample struct {
	facts  domain.SampleFacts
	assays []domain.AssayObservation
}

// groupLinkedSamples collapses linked assay rows into distinct samples with
// their station identity and fuel; rows of one sample always share both.
func groupLinkedSamples(rows []quality.ListLinkedAssayRowsRow) []linkedSample {
	type key struct {
		station, sample string
	}
	order := []key{}
	byKey := map[key]*linkedSample{}
	for _, row := range rows {
		k := key{station: uuidString(row.StationID), sample: row.SampleRef}
		sample := byKey[k]
		if sample == nil {
			sample = &linkedSample{facts: domain.SampleFacts{
				SampleKey:  "pmqc-assay-v1:" + row.SampleRef,
				StationID:  k.station,
				ObservedOn: row.ObservedOn.Time.Format("2006-01-02"),
				Fuel:       domain.Fuel(row.Fuel),
			}}
			byKey[k] = sample
			order = append(order, k)
		}
		sample.assays = append(sample.assays, domain.AssayObservation{
			Ensaio:     row.AssayRef,
			Result:     row.ResultText,
			Conformity: domain.Conformity(row.Conformity),
		})
	}
	out := make([]linkedSample, 0, len(order))
	for _, k := range order {
		sample := byKey[k]
		sample.facts.Judgment = domain.JudgeSample(sample.facts, sample.assays)
		out = append(out, *sample)
	}
	return out
}

// digestSamples canonicalizes the exact judged input so identical facts
// always produce the identical digest, independent of row order.
func digestSamples(samples []linkedSample) string {
	lines := make([]string, 0, len(samples))
	for _, sample := range samples {
		assays := make([]string, 0, len(sample.assays))
		for _, assay := range sample.assays {
			encoded, _ := json.Marshal([]string{assay.Ensaio, assay.Result, string(assay.Conformity)})
			assays = append(assays, string(encoded))
		}
		sort.Strings(assays)
		encoded, _ := json.Marshal([]any{
			sample.facts.StationID, sample.facts.SampleKey, sample.facts.ObservedOn,
			string(sample.facts.Fuel), string(sample.facts.Judgment.Verdict),
			assays,
		})
		lines = append(lines, string(encoded))
	}
	sort.Strings(lines)
	encoded, _ := json.Marshal(lines)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func projectionDays(dim domain.FuelDimensions) (pgtype.Date, pgtype.Date, error) {
	conv := func(day string) (pgtype.Date, error) {
		if day == "" {
			return pgtype.Date{}, nil
		}
		parsed, err := time.Parse("2006-01-02", day)
		if err != nil {
			return pgtype.Date{}, errors.New("quality projections: malformed observation day")
		}
		return pgtype.Date{Time: parsed, Valid: true}, nil
	}
	oldest, err := conv(dim.OldestObservedOn)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	newest, err := conv(dim.NewestObservedOn)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, err
	}
	return oldest, newest, nil
}

// projectDimensions groups samples per (station, fuel) while retaining the
// station identity the domain reducer intentionally does not key on.
func projectDimensions(samples []linkedSample) map[projectionKey]domain.FuelDimensions {
	grouped := map[string][]domain.SampleFacts{}
	order := []string{}
	for _, sample := range samples {
		key := sample.facts.StationID + "\x1f" + string(sample.facts.Fuel)
		if _, seen := grouped[key]; !seen {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], sample.facts)
	}
	out := map[projectionKey]domain.FuelDimensions{}
	for _, key := range order {
		parts := strings.SplitN(key, "\x1f", 2)
		station, fuel := parts[0], domain.Fuel(parts[1])
		perFuel := domain.ProjectFuelDimensions(grouped[key])
		out[projectionKey{station: station, fuel: fuel}] = perFuel[fuel]
	}
	return out
}
