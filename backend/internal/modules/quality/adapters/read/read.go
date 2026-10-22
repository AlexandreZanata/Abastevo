package read

import (
	"context"
	"encoding/hex"
	"errors"
	"strconv"
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
	companyProbe CompanyIdentityProbe
	companyGuard CompanyIdentityGuard
	pool         *pgxpool.Pool
	revisionID   string
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

func readDay(s string) (pgtype.Date, error) {
	day, err := time.Parse("2006-01-02", s)
	if err != nil || day.Format("2006-01-02") != s {
		return pgtype.Date{}, errors.New("read: invalid calendar day")
	}
	return pgtype.Date{Time: day, Valid: true}, nil
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
type SourceNote struct {
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	EvidenceRef string `json:"evidence_ref"`
}

type Lease struct {
	SourceInputContract   string
	SourcePeriod          string
	SourceClockValue      string
	SourceRecordCount     *int
	SourceMetadataRecords *int
	SourceNotes           []SourceNote
	SourceClockPrecision  string
	SourceExtractionMonth string
	SourceReportYear      *int32
	SupersedesRevisionID  string
	Status                string
	ProvenanceState       string
	IsCurrentStream       bool
	Source                string
	Stream                string
	Edition               string
	RevisionID            string
	PublishedAt           time.Time
	HasPublished          bool
	Counts                domain.Counts
	ManifestHash          string
	Periods               []Period
	Stale                 bool
	StaleReason           string
	CoverageNote          string
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
	SourceProfile, SubjectKind, UF, Municipality, Segment, DocumentDay, DocumentNumber, Procedure, Result string
	ID                                                                                                    string
	NativeKey                                                                                             string
	Scope                                                                                                 string
	Status                                                                                                string
	OccurredOn                                                                                            string
	AsOf                                                                                                  string
	EvidenceRef                                                                                           string
}

// SanctionEvent is one published snapshot without party identifiers.
type SanctionEvent struct {
	InputContract    string
	SourcePeriod     string
	SourceClockValue string
	ReferenceLabel   string
	ReferenceRef     string
	SourceTransitOn  string
	UnlabelledField  string
	IdentityKind     string
	ClockPrecision   string
	ExtractionMonth  string
	ReportYear       *int32
	DUFRef           string
	Department       string
	DueOn            string
	AmountText       string
	PaidText         string
	PaidMinor        *int64
	ID               string
	NativeKey        string
	ProceedingRef    string
	AmountMinor      *int64
	Status           string
	DecidedOn        string
	AsOf             string
	EvidenceRef      string
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

	if source == "enforcement" && stream == domain.StreamEnforcement {
		c, e := q.EnforcementSourceContext(ctx, row.RevisionID)
		if e == nil {
			if int(c.InputCount) != lease.Counts.Input || int(c.AcceptedCount) != lease.Counts.Accepted || int(c.DuplicateCount) != lease.Counts.Duplicates || lease.Counts.Quarantined != 0 {
				return Lease{}, pgtype.UUID{}, errors.New("read: enforcement census unavailable")
			}
			lease.SourceInputContract = c.SourceProfile
			lease.SourceClockPrecision = "UNAVAILABLE"
			records, metadata := int(c.InputCount), 0
			lease.SourceRecordCount = &records
			lease.SourceMetadataRecords = &metadata
			lease.Stale = true
			lease.StaleReason = "source extraction clock unavailable"
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return Lease{}, pgtype.UUID{}, e
		}
	}
	if source == "sanctions" && stream == domain.StreamSanction {
		clock, cerr := q.NativeFineRevisionClock(ctx, row.RevisionID)
		if cerr == nil {
			lease.SourceClockPrecision = "MONTH"
			lease.SourceInputContract = "anp-sicom-cp850-9-v1"
			lease.SourcePeriod = strconv.Itoa(int(clock.ReportYear))
			lease.SourceClockValue = clock.ExtractionMonth
			records, metadata := lease.Counts.Input, 0
			lease.SourceRecordCount = &records
			lease.SourceMetadataRecords = &metadata
			lease.SourceExtractionMonth = clock.ExtractionMonth
			year := clock.ReportYear
			lease.SourceReportYear = &year
			if old, why := domain.FineMonthStale(clock.ExtractionMonth, now); old {
				lease.Stale = true
				lease.StaleReason = why
			}
		} else if !errors.Is(cerr, pgx.ErrNoRows) {
			return Lease{}, pgtype.UUID{}, cerr
		} else {
			history, herr := q.HistoricalFineContext(ctx, row.RevisionID)
			if herr == nil {
				descriptor := application.FineHistoryContext{InputContract: history.InputContract, SourcePeriod: history.SourcePeriod, SourceFile: history.SourceFile, HeaderRecord: int(history.HeaderRecord), Clock: application.FineHistoryClock{Precision: history.SourceClockPrecision, Value: history.SourceClockValue}, SourceRecordCount: int(history.SourceRecordCount), MetadataRecords: int(history.MetadataCount)}
				if !descriptor.Valid() || descriptor.SourceRecordCount != lease.Counts.Input || descriptor.MetadataRecords > lease.Counts.Quarantined {
					return Lease{}, pgtype.UUID{}, errors.New("read: historical source context unavailable")
				}
				lease.SourceInputContract = descriptor.InputContract
				lease.SourcePeriod = descriptor.SourcePeriod
				lease.SourceClockValue = descriptor.Clock.Value
				lease.SourceClockPrecision = descriptor.Clock.Precision
				records, metadata := descriptor.SourceRecordCount, descriptor.MetadataRecords
				lease.SourceRecordCount = &records
				lease.SourceMetadataRecords = &metadata
				if old, why := domain.FineSourceClockStale(descriptor.Clock.Precision, descriptor.Clock.Value, now); old {
					lease.Stale = true
					lease.StaleReason = why
				}
				notes, nerr := q.PublicHistoricalFineNotes(ctx, row.RevisionID)
				if nerr != nil {
					return Lease{}, pgtype.UUID{}, nerr
				}
				if len(notes) > 2 {
					return Lease{}, pgtype.UUID{}, errors.New("read: historical source notes unavailable")
				}
				for _, note := range notes {
					lease.SourceNotes = append(lease.SourceNotes, SourceNote{Kind: note.Kind, Text: note.NoteText, EvidenceRef: note.EvidenceRef})
				}
			} else if !errors.Is(herr, pgx.ErrNoRows) {
				return Lease{}, pgtype.UUID{}, herr
			}
		}
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

	if lease.SourceInputContract == application.EnforcementSourceProfile {
		if status != "" || scope != "" {
			return nil, "", lease, errors.New("read: legacy filters unavailable for native source")
		}
		rows, e := quality.New(r.pool).EnforcementSourcePage(ctx, quality.EnforcementSourcePageParams{RevisionID: revisionID, AfterNative: afterNative, AfterID: afterID, LimitPlusOne: int32(limit + 1)})
		if e != nil {
			return nil, "", lease, e
		}
		out := make([]EnforcementEvent, 0, min(limit, len(rows)))
		for i, row := range rows {
			if i == limit {
				last := rows[limit-1]
				return out, last.NativeKey + "|" + uuidString(last.ID), lease, nil
			}
			out = append(out, EnforcementEvent{ID: uuidString(row.ID), NativeKey: row.NativeKey, SourceProfile: application.EnforcementSourceProfile, SubjectKind: row.SubjectKind, UF: row.Uf, Municipality: row.Municipality, Segment: row.Segment, DocumentDay: dateString(row.DocumentDay), DocumentNumber: row.DocumentNumber, Procedure: row.Procedure, Result: row.Result, EvidenceRef: row.EvidenceRef})
		}
		return out, "", lease, nil
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

// EnforcementFilter carries the supported native-source facets. Empty
// fields mean no filter; every set field must already be validated by the
// caller (UF shape, known subject kind, calendar days, from <= to).
type EnforcementFilter struct {
	UF, Municipality, Segment, SubjectKind, FromDay, ToDay string
}

// EnforcementCoverage reports the leased revision's valid counts: distinct
// retained observations versus distinct source-stated cases (documents),
// with the document-day bounds. Counts only, never subject identifiers.
type EnforcementCoverage struct {
	Observations int64
	Cases        int64
	FirstDay     string
	LastDay      string
	DateBasis    string
}

// EnforcementFilteredAtLease pages native source facts under the leased
// revision with exact facet matches. Unmatched and unknown records stay
// included unless a facet excludes them; an absent facet never excludes.
func (r *Reader) EnforcementFilteredAtLease(ctx context.Context, lease Lease, filter EnforcementFilter, afterNative, afterID string, limit int) ([]EnforcementEvent, string, Lease, error) {
	if !lease.HasPublished || lease.SourceInputContract != application.EnforcementSourceProfile {
		return nil, "", lease, nil
	}
	revisionID, err := mustUUID(lease.RevisionID)
	if err != nil {
		return nil, "", lease, err
	}
	params := quality.EnforcementSourceFilteredPageParams{RevisionID: revisionID, AfterNative: afterNative, AfterID: afterID, LimitPlusOne: int32(limit + 1)}
	if filter.UF != "" {
		params.Uf = pgtype.Text{String: filter.UF, Valid: true}
	}
	if filter.Municipality != "" {
		params.Municipality = pgtype.Text{String: filter.Municipality, Valid: true}
	}
	if filter.Segment != "" {
		params.Segment = pgtype.Text{String: filter.Segment, Valid: true}
	}
	if filter.SubjectKind != "" {
		params.SubjectKind = pgtype.Text{String: filter.SubjectKind, Valid: true}
	}
	if filter.FromDay != "" {
		from, err := readDay(filter.FromDay)
		if err != nil {
			return nil, "", lease, err
		}
		params.FromDay = from
	}
	if filter.ToDay != "" {
		to, err := readDay(filter.ToDay)
		if err != nil {
			return nil, "", lease, err
		}
		params.ToDay = to
	}
	rows, e := quality.New(r.pool).EnforcementSourceFilteredPage(ctx, params)
	if e != nil {
		return nil, "", lease, e
	}
	out := make([]EnforcementEvent, 0, min(limit, len(rows)))
	for i, row := range rows {
		if i == limit {
			last := rows[limit-1]
			return out, last.NativeKey + "|" + uuidString(last.ID), lease, nil
		}
		out = append(out, EnforcementEvent{ID: uuidString(row.ID), NativeKey: row.NativeKey, SourceProfile: application.EnforcementSourceProfile, SubjectKind: row.SubjectKind, UF: row.Uf, Municipality: row.Municipality, Segment: row.Segment, DocumentDay: dateString(row.DocumentDay), DocumentNumber: row.DocumentNumber, Procedure: row.Procedure, Result: row.Result, EvidenceRef: row.EvidenceRef})
	}
	return out, "", lease, nil
}

// EnforcementCoverageAtLease reports the leased revision's observation and
// case counts with the document-day bounds and basis. A concurrent
// publication cannot substitute a second revision.
func (r *Reader) EnforcementCoverageAtLease(ctx context.Context, lease Lease) (EnforcementCoverage, Lease, error) {
	if !lease.HasPublished || lease.SourceInputContract != application.EnforcementSourceProfile {
		return EnforcementCoverage{}, lease, nil
	}
	revisionID, err := mustUUID(lease.RevisionID)
	if err != nil {
		return EnforcementCoverage{}, lease, err
	}
	row, e := quality.New(r.pool).EnforcementSourceCoverage(ctx, revisionID)
	if e != nil {
		return EnforcementCoverage{}, lease, e
	}
	out := EnforcementCoverage{Observations: row.Observations, Cases: row.Cases, DateBasis: domain.DateBasisDocumentDay}
	out.FirstDay = coverageDay(row.FirstDay)
	out.LastDay = coverageDay(row.LastDay)
	return out, lease, nil
}

func coverageDay(value any) string {
	switch day := value.(type) {
	case pgtype.Date:
		return dateString(day)
	case time.Time:
		return day.Format("2006-01-02")
	default:
		return ""
	}
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
	out, next, e := sanctionPageRows(rows, lease, limit)
	return out, next, lease, e
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

	if lease.SourceInputContract == application.EnforcementSourceProfile {
		row, e := quality.New(r.pool).EnforcementSourceByID(ctx, quality.EnforcementSourceByIDParams{RevisionID: revisionID, ID: rowID})
		if errors.Is(e, pgx.ErrNoRows) {
			e = ErrNotFound
		}
		if e != nil {
			return EnforcementEvent{}, lease, e
		}
		return EnforcementEvent{ID: uuidString(row.ID), NativeKey: row.NativeKey, SourceProfile: application.EnforcementSourceProfile, SubjectKind: row.SubjectKind, UF: row.Uf, Municipality: row.Municipality, Segment: row.Segment, DocumentDay: dateString(row.DocumentDay), DocumentNumber: row.DocumentNumber, Procedure: row.Procedure, Result: row.Result, EvidenceRef: row.EvidenceRef}, lease, nil
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
	event, e := sanctionDetailRow(row, lease)
	return event, lease, e
}

func fineFacts(event SanctionEvent, contract string, month pgtype.Text, year pgtype.Int4, duf, department pgtype.Text, due pgtype.Date, amount, paid pgtype.Text, paidMinor pgtype.Int8) SanctionEvent {
	event.IdentityKind = "LEGACY_REFERENCE"
	event.ClockPrecision = "DAY"
	if contract == "anp-sicom-cp850-9-v1" {
		event.IdentityKind = "CONTENT_OBSERVATION"
		event.InputContract = contract
		event.SourceClockValue = month.String
		event.ReferenceLabel = "Número do DUF"
		event.ReferenceRef = duf.String
		if year.Valid {
			event.SourcePeriod = strconv.Itoa(int(year.Int32))
		}
		event.ClockPrecision = "MONTH"
		event.ExtractionMonth = month.String
		if year.Valid {
			value := year.Int32
			event.ReportYear = &value
		}
		event.DUFRef = duf.String
		event.Department = department.String
		event.DueOn = dateString(due)
		event.AmountText = amount.String
		event.PaidText = paid.String
		if paidMinor.Valid {
			value := paidMinor.Int64
			event.PaidMinor = &value
		}
	}
	return event
}

func historicalFineFacts(event SanctionEvent, lease Lease, label, reference pgtype.Text, transit pgtype.Date, unlabelled, department pgtype.Text, due pgtype.Date, amount, paid pgtype.Text, paidMinor pgtype.Int8) SanctionEvent {
	event.IdentityKind = "CONTENT_OBSERVATION"
	event.ClockPrecision = lease.SourceClockPrecision
	event.SourceClockValue = lease.SourceClockValue
	event.InputContract = lease.SourceInputContract
	event.SourcePeriod = lease.SourcePeriod
	event.ReferenceLabel = label.String
	event.ReferenceRef = reference.String
	event.SourceTransitOn = dateString(transit)
	event.UnlabelledField = unlabelled.String
	event.Department = department.String
	event.DueOn = dateString(due)
	event.AmountText = amount.String
	event.PaidText = paid.String
	if paidMinor.Valid {
		v := paidMinor.Int64
		event.PaidMinor = &v
	}
	return event
}

func sanctionPageRows(rows []quality.SanctionPageByRevisionRow, lease Lease, limit int) ([]SanctionEvent, string, error) {
	out := make([]SanctionEvent, 0, len(rows))
	for i, row := range rows {
		if i == limit {
			last := rows[limit-1]
			return out, last.NativeKey + "|" + uuidString(last.ID), nil
		}
		var amount *int64
		if row.AmountMinor.Valid {
			v := row.AmountMinor.Int64
			amount = &v
		}
		event := SanctionEvent{ID: uuidString(row.ID), NativeKey: row.NativeKey, ProceedingRef: row.ProceedingRef, AmountMinor: amount, Status: row.Status, DecidedOn: dateString(row.DecidedOn), AsOf: dateString(row.AsOf), EvidenceRef: row.EvidenceRef}
		event = fineFacts(event, row.SourceContract, row.ExtractionMonth, row.ReportYear, row.DufRef, row.Department, row.DueOn, row.AmountText, row.PaidText, row.PaidMinor)
		if row.HistoryRevisionID.Valid {
			if row.SourceContract != lease.SourceInputContract || lease.SourcePeriod == "" {
				return nil, "", errors.New("read: historical source mismatch")
			}
			event = historicalFineFacts(event, lease, row.ReferenceLabel, row.ReferenceRef, row.SourceTransitOn, row.UnlabelledField, row.Department, row.DueOn, row.AmountText, row.PaidText, row.PaidMinor)
		}
		out = append(out, event)
	}
	return out, "", nil
}
func sanctionDetailRow(row quality.QualitySanctionEvent, lease Lease) (SanctionEvent, error) {
	var amount *int64
	if row.AmountMinor.Valid {
		v := row.AmountMinor.Int64
		amount = &v
	}
	event := SanctionEvent{ID: uuidString(row.ID), NativeKey: row.NativeKey, ProceedingRef: row.ProceedingRef, AmountMinor: amount, Status: row.Status, DecidedOn: dateString(row.DecidedOn), AsOf: dateString(row.AsOf), EvidenceRef: row.EvidenceRef}
	event = fineFacts(event, row.SourceContract, row.ExtractionMonth, row.ReportYear, row.DufRef, row.Department, row.DueOn, row.AmountText, row.PaidText, row.PaidMinor)
	if row.HistoryRevisionID.Valid {
		if row.SourceContract != lease.SourceInputContract || lease.SourcePeriod == "" {
			return SanctionEvent{}, errors.New("read: historical source mismatch")
		}
		event = historicalFineFacts(event, lease, row.ReferenceLabel, row.ReferenceRef, row.SourceTransitOn, row.UnlabelledField, row.Department, row.DueOn, row.AmountText, row.PaidText, row.PaidMinor)
	}
	return event, nil
}
