package read

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

// ErrNoLease reports a stream without a published revision. Callers serve
// an explicit unavailable lease, never an invented one.
var ErrNoLease = errors.New("read: no published revision")

// ErrNotFound reports an unknown/cross-stream revision or a row outside
// the selected revision. Verified superseded history is explicitly selectable.
var ErrNotFound = errors.New("read: row outside the current lease")

// Reader serves frozen-lease quality reads.
type Reader struct {
	pool       *pgxpool.Pool
	revisionID string
}

// NewReader wires the owned generated queries to a pool.
func NewReader(pool *pgxpool.Pool) *Reader {
	return &Reader{pool: pool}
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("read: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func dateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

func timeString(t pgtype.Timestamptz) (time.Time, bool) {
	if !t.Valid {
		return time.Time{}, false
	}
	return t.Time, true
}

func pgText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// Period is one claimed coverage label of an edition.
type Period struct {
	Label string
	Start string
	End   string
}

// Lease is the frozen read context served with every response: stream
// identity, edition, revision, reconciled denominators, claimed periods
// and the staleness flag with its reason.
type Lease struct {
	SupersedesRevisionID string
	Status               string
	ProvenanceState      string
	IsCurrentStream      bool
	Source               string
	Stream               string
	Edition              string
	RevisionID           string
	PublishedAt          time.Time
	HasPublished         bool
	Counts               domain.Counts
	ManifestHash         string
	Periods              []Period
	Stale                bool
	StaleReason          string
	CoverageNote         string
}

// FuelCell is one fuel/conformity denominator of a station summary.
type FuelCell struct {
	Fuel         string
	Conformity   string
	Count        int64
	LastObserved string
}

// Assay is one published determination without source identifiers.
type Assay struct {
	ID               string
	SampleRef        string
	AssayRef         string
	ObservedOn       string
	SourceObservedAt string
	Fuel             string
	ResultText       string
	Unit             string
	Conformity       string
	EvidenceRef      string
}

// SampleLink is one persisted station association of a sample.
type SampleLink struct {
	StationID string
	Automatic bool
	Reason    string
	Active    bool
}

// SampleDetail carries a sample's assays, links and lease together.
type SampleDetail struct {
	SampleRef string
	Assays    []Assay
	Links     []SampleLink
	Lease     Lease
}

// EnforcementEvent is one published fiscal action without party identifiers.
type EnforcementEvent struct {
	ID          string
	NativeKey   string
	Scope       string
	Status      string
	OccurredOn  string
	AsOf        string
	EvidenceRef string
}

// SanctionEvent is one published snapshot without party identifiers.
type SanctionEvent struct {
	ID            string
	NativeKey     string
	ProceedingRef string
	AmountMinor   *int64
	Status        string
	DecidedOn     string
	AsOf          string
	EvidenceRef   string
}

func (r *Reader) lease(ctx context.Context, source, stream string, now time.Time) (Lease, pgtype.UUID, error) {
	q := quality.New(r.pool)
	var row quality.LeaseByStreamRow
	var err error
	if r.revisionID == "" {
		row, err = q.LeaseByStream(ctx, quality.LeaseByStreamParams{Source: source, Stream: stream})
	} else {
		id, e := mustUUID(r.revisionID)
		if e != nil {
			return Lease{}, pgtype.UUID{}, ErrNotFound
		}
		selected, e := q.RevisionLeaseByStream(ctx, quality.RevisionLeaseByStreamParams{Source: source, Stream: stream, RevisionID: id})
		err = e
		row = quality.LeaseByStreamRow(selected)
		if errors.Is(err, pgx.ErrNoRows) {
			return Lease{}, pgtype.UUID{}, ErrNotFound
		}
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Lease{Source: source, Stream: stream, Stale: true, StaleReason: "no published revision"}, pgtype.UUID{}, ErrNoLease
		}
		return Lease{}, pgtype.UUID{}, err
	}
	if row.ProvenanceState != "VERIFIED" {
		return Lease{PublishedAt: row.PublishedAt.Time, SupersedesRevisionID: uuidString(row.SupersedesRevisionID), Status: row.Status, ProvenanceState: row.ProvenanceState, IsCurrentStream: row.IsCurrentStream, Source: source, Stream: stream, Edition: row.Edition, RevisionID: uuidString(row.RevisionID), HasPublished: false, Stale: true, StaleReason: "revision provenance unverified", CoverageNote: "Frozen source provenance unavailable; verified reimport required."}, pgtype.UUID{}, ErrNoLease
	}
	publishedAt, hasPublished := timeString(row.PublishedAt)
	servable := row.Status == "published" || (r.revisionID != "" && row.Status == "superseded")
	if !servable || !hasPublished {
		return Lease{PublishedAt: row.PublishedAt.Time, SupersedesRevisionID: uuidString(row.SupersedesRevisionID), Status: row.Status, ProvenanceState: row.ProvenanceState, IsCurrentStream: row.IsCurrentStream, Source: source, Stream: stream, Edition: row.Edition, RevisionID: uuidString(row.RevisionID), Stale: true, StaleReason: "revision unavailable", CoverageNote: "Revision retained as metadata; no facts served."}, pgtype.UUID{}, ErrNoLease
	}
	stale, reason := application.Stale(true, publishedAt, now)
	lease := Lease{
		SupersedesRevisionID: uuidString(row.SupersedesRevisionID), Status: row.Status, ProvenanceState: row.ProvenanceState, IsCurrentStream: row.IsCurrentStream, Source: row.Source, Stream: row.Stream, Edition: row.Edition,
		RevisionID:  uuidString(row.RevisionID),
		PublishedAt: publishedAt, HasPublished: hasPublished,
		Counts: domain.Counts{
			Input: int(row.Input), Accepted: int(row.Accepted),
			Duplicates: int(row.Duplicates), Quarantined: int(row.Quarantined),
		},
		ManifestHash: row.ManifestHash,
		Stale:        stale, StaleReason: reason,
	}
	lease.CoverageNote = application.CoverageNote(row.Edition, lease.Counts)
	if row.Status == "superseded" {
		lease.CoverageNote = "Historical superseded revision; consult its correction. " + strings.Replace(lease.CoverageNote, "only published revisions serve reads", "completed verified historical revisions remain explicitly accessible", 1)
	}
	if periods, perr := q.ListRevisionPeriods(ctx, row.RevisionID); perr == nil {
		for _, p := range periods {
			lease.Periods = append(lease.Periods, Period{
				Label: p.PeriodLabel, Start: dateString(p.PeriodStart), End: dateString(p.PeriodEnd),
			})
		}
	} else {
		return Lease{}, pgtype.UUID{}, perr
	}
	return lease, row.RevisionID, nil
}

// LeaseBlock returns the servable lease envelope for a stream: unavailable
// with its reason when nothing is published, so responses distinguish
// empty leases from source outages.
func (r *Reader) LeaseBlock(ctx context.Context, now time.Time, source, stream string) (Lease, error) {
	lease, _, err := r.lease(ctx, source, stream, now)
	if errors.Is(err, ErrNoLease) {
		return lease, nil
	}
	if err != nil {
		return Lease{}, err
	}
	return lease, nil
}

// Summary returns per-fuel conformity denominators for one station.
func (r *Reader) Summary(ctx context.Context, now time.Time, source, stream, stationID, fuel string) ([]FuelCell, Lease, error) {
	lease, revisionID, err := r.lease(ctx, source, stream, now)
	if err != nil && !errors.Is(err, ErrNoLease) {
		return nil, lease, err
	}
	if errors.Is(err, ErrNoLease) {
		return nil, lease, nil
	}
	station, err := mustUUID(stationID)
	if err != nil {
		return nil, lease, err
	}
	rows, err := quality.New(r.pool).SummaryByRevision(ctx, quality.SummaryByRevisionParams{
		RevisionID: revisionID, StationID: station, Stream: pgText(stream), Fuel: fuel,
	})
	if err != nil {
		return nil, lease, err
	}
	cells := make([]FuelCell, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, FuelCell{
			Fuel: row.Fuel, Conformity: row.Conformity,
			Count: row.N, LastObserved: dateString(row.LastObserved),
		})
	}
	return cells, lease, nil
}

// Assays pages linked determinations for one station, keyed by
// observed date plus row id. nextCursor is empty at the frozen end.
func (r *Reader) Assays(ctx context.Context, now time.Time, source, stream, stationID, fuel, afterObserved, afterID string, limit int) ([]Assay, string, Lease, error) {
	lease, _, err := r.lease(ctx, source, stream, now)
	if err != nil && !errors.Is(err, ErrNoLease) {
		return nil, "", lease, err
	}
	return r.AssaysAtLease(ctx, lease, stationID, fuel, afterObserved, afterID, limit)
}

// AssaysAtLease consumes the already resolved lease used to validate the
// cursor. A concurrent publication cannot substitute a second revision.
func (r *Reader) AssaysAtLease(ctx context.Context, lease Lease, stationID, fuel, afterObserved, afterID string, limit int) ([]Assay, string, Lease, error) {
	if !lease.HasPublished {
		return nil, "", lease, nil
	}
	revisionID, err := mustUUID(lease.RevisionID)
	if err != nil {
		return nil, "", lease, err
	}
	station, err := mustUUID(stationID)
	if err != nil {
		return nil, "", lease, err
	}
	var afterDate pgtype.Date
	if afterObserved != "" {
		day, derr := time.Parse("2006-01-02", afterObserved)
		if derr != nil {
			return nil, "", lease, derr
		}
		afterDate = pgtype.Date{Time: day, Valid: true}
	}
	rows, err := quality.New(r.pool).AssayPageByRevision(ctx, quality.AssayPageByRevisionParams{
		RevisionID: revisionID, StationID: station, Stream: pgText(lease.Stream), Fuel: fuel,
		AfterDate: afterDate, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", lease, err
	}
	out := make([]Assay, 0, len(rows))
	for i, row := range rows {
		if i == limit {
			last := rows[limit-1]
			return out, dateString(last.ObservedOn) + "|" + uuidString(last.ID), lease, nil
		}
		out = append(out, wireAssay(row.SampleRef, row.AssayRef, row.ObservedOn, row.Fuel,
			row.ResultText, row.Unit, row.Conformity, row.EvidenceRef, uuidString(row.ID), row.SourceObservedAt.String))
	}
	return out, "", lease, nil
}

func wireAssay(sampleRef, assayRef string, observed pgtype.Date, fuel, result, unit, conformity, evidence, id, sourceObservedAt string) Assay {
	return Assay{
		ID: id, SampleRef: sampleRef, AssayRef: assayRef,
		ObservedOn: dateString(observed), SourceObservedAt: sourceObservedAt, Fuel: fuel, ResultText: result,
		Unit: unit, Conformity: conformity, EvidenceRef: evidence,
	}
}

// Sample returns one sample's assays, links and lease. Samples outside
// the selected servable revision report no rows with that lease.
func (r *Reader) Sample(ctx context.Context, now time.Time, source, stream, sampleRef string) (SampleDetail, error) {
	lease, revisionID, err := r.lease(ctx, source, stream, now)
	if err != nil && !errors.Is(err, ErrNoLease) {
		return SampleDetail{}, err
	}
	detail := SampleDetail{SampleRef: sampleRef, Lease: lease}
	if errors.Is(err, ErrNoLease) {
		return detail, nil
	}
	q := quality.New(r.pool)
	rows, err := q.SampleAssaysByRevision(ctx, quality.SampleAssaysByRevisionParams{
		RevisionID: revisionID, SampleRef: sampleRef,
	})
	if err != nil {
		return SampleDetail{}, err
	}
	for _, row := range rows {
		detail.Assays = append(detail.Assays, wireAssay(row.SampleRef, row.AssayRef,
			row.ObservedOn, row.Fuel, row.ResultText, row.Unit, row.Conformity,
			row.EvidenceRef, uuidString(row.ID), row.SourceObservedAt.String))
	}
	links, err := q.SampleLinksByRevision(ctx, quality.SampleLinksByRevisionParams{
		RevisionID: revisionID, RecordKey: stream + ":" + sampleRef,
	})
	if err != nil {
		return SampleDetail{}, err
	}
	for _, link := range links {
		detail.Links = append(detail.Links, SampleLink{
			StationID: uuidString(link.StationID), Automatic: link.Automatic,
			Reason: link.Reason, Active: link.Active,
		})
	}
	return detail, nil
}

// Enforcement pages one stream's published actions, keyed by native key
// plus row id for complete frozen-edition traversal.
func (r *Reader) Enforcement(ctx context.Context, now time.Time, source, stream, status, scope, afterNative, afterID string, limit int) ([]EnforcementEvent, string, Lease, error) {
	lease, _, err := r.lease(ctx, source, stream, now)
	if err != nil && !errors.Is(err, ErrNoLease) {
		return nil, "", lease, err
	}
	return r.EnforcementAtLease(ctx, lease, status, scope, afterNative, afterID, limit)
}

// EnforcementAtLease consumes the already resolved lease used to validate the
// cursor. A concurrent publication cannot substitute a second revision.
func (r *Reader) EnforcementAtLease(ctx context.Context, lease Lease, status, scope, afterNative, afterID string, limit int) ([]EnforcementEvent, string, Lease, error) {
	if !lease.HasPublished {
		return nil, "", lease, nil
	}
	revisionID, err := mustUUID(lease.RevisionID)
	if err != nil {
		return nil, "", lease, err
	}
	rows, err := quality.New(r.pool).ActionPageByRevision(ctx, quality.ActionPageByRevisionParams{
		RevisionID: revisionID, Status: status, Scope: scope,
		AfterNative: afterNative, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", lease, err
	}
	out := make([]EnforcementEvent, 0, len(rows))
	for i, row := range rows {
		if i == limit {
			last := rows[limit-1]
			return out, last.NativeKey + "|" + uuidString(last.ID), lease, nil
		}
		out = append(out, EnforcementEvent{
			ID: uuidString(row.ID), NativeKey: row.NativeKey,
			Scope: row.Scope, Status: row.Status,
			OccurredOn: dateString(row.OccurredOn), AsOf: dateString(row.AsOf),
			EvidenceRef: row.EvidenceRef,
		})
	}
	return out, "", lease, nil
}

// Sanctions pages one stream's published snapshots with the same keyset.
func (r *Reader) Sanctions(ctx context.Context, now time.Time, source, stream, status, afterNative, afterID string, limit int) ([]SanctionEvent, string, Lease, error) {
	lease, _, err := r.lease(ctx, source, stream, now)
	if err != nil && !errors.Is(err, ErrNoLease) {
		return nil, "", lease, err
	}
	return r.SanctionsAtLease(ctx, lease, status, afterNative, afterID, limit)
}

// SanctionsAtLease consumes the already resolved lease used to validate the
// cursor. A concurrent publication cannot substitute a second revision.
func (r *Reader) SanctionsAtLease(ctx context.Context, lease Lease, status, afterNative, afterID string, limit int) ([]SanctionEvent, string, Lease, error) {
	if !lease.HasPublished {
		return nil, "", lease, nil
	}
	revisionID, err := mustUUID(lease.RevisionID)
	if err != nil {
		return nil, "", lease, err
	}
	rows, err := quality.New(r.pool).SanctionPageByRevision(ctx, quality.SanctionPageByRevisionParams{
		RevisionID: revisionID, Status: status,
		AfterNative: afterNative, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", lease, err
	}
	out := make([]SanctionEvent, 0, len(rows))
	for i, row := range rows {
		if i == limit {
			last := rows[limit-1]
			return out, last.NativeKey + "|" + uuidString(last.ID), lease, nil
		}
		var amount *int64
		if row.AmountMinor.Valid {
			v := row.AmountMinor.Int64
			amount = &v
		}
		out = append(out, SanctionEvent{
			ID: uuidString(row.ID), NativeKey: row.NativeKey,
			ProceedingRef: row.ProceedingRef, AmountMinor: amount,
			Status: row.Status, DecidedOn: dateString(row.DecidedOn),
			AsOf: dateString(row.AsOf), EvidenceRef: row.EvidenceRef,
		})
	}
	return out, "", lease, nil
}

// EnforcementOne returns one action by id within the selected revision.
func (r *Reader) EnforcementOne(ctx context.Context, now time.Time, source, stream, id string) (EnforcementEvent, Lease, error) {
	lease, revisionID, err := r.lease(ctx, source, stream, now)
	if err != nil {
		return EnforcementEvent{}, lease, err
	}
	rowID, err := mustUUID(id)
	if err != nil {
		return EnforcementEvent{}, lease, err
	}
	row, err := quality.New(r.pool).EnforcementByID(ctx, quality.EnforcementByIDParams{
		ID: rowID, RevisionID: revisionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EnforcementEvent{}, lease, ErrNotFound
		}
		return EnforcementEvent{}, lease, err
	}
	return EnforcementEvent{
		ID: uuidString(row.ID), NativeKey: row.NativeKey,
		Scope: row.Scope, Status: row.Status,
		OccurredOn: dateString(row.OccurredOn), AsOf: dateString(row.AsOf),
		EvidenceRef: row.EvidenceRef,
	}, lease, nil
}

// SanctionOne returns one snapshot by id within the selected revision.
func (r *Reader) SanctionOne(ctx context.Context, now time.Time, source, stream, id string) (SanctionEvent, Lease, error) {
	lease, revisionID, err := r.lease(ctx, source, stream, now)
	if err != nil {
		return SanctionEvent{}, lease, err
	}
	rowID, err := mustUUID(id)
	if err != nil {
		return SanctionEvent{}, lease, err
	}
	row, err := quality.New(r.pool).SanctionByID(ctx, quality.SanctionByIDParams{
		ID: rowID, RevisionID: revisionID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SanctionEvent{}, lease, ErrNotFound
		}
		return SanctionEvent{}, lease, err
	}
	var amount *int64
	if row.AmountMinor.Valid {
		v := row.AmountMinor.Int64
		amount = &v
	}
	return SanctionEvent{
		ID: uuidString(row.ID), NativeKey: row.NativeKey,
		ProceedingRef: row.ProceedingRef, AmountMinor: amount,
		Status: row.Status, DecidedOn: dateString(row.DecidedOn),
		AsOf: dateString(row.AsOf), EvidenceRef: row.EvidenceRef,
	}, lease, nil
}
