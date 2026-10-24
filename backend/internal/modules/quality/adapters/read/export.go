package read

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
)

// ExportReader implements application.ExportReader over the owned generated
// queries. It can run inside a caller-owned read-only transaction.
type ExportReader struct {
	db quality.DBTX
}

// NewExportReader wires the owned export queries to a pool or transaction.
func NewExportReader(db quality.DBTX) *ExportReader {
	return &ExportReader{db: db}
}

// ExportPin resolves the current published sanction revision and verifies its
// frozen provenance, complete company census and single source resource.
func (r *ExportReader) ExportPin(ctx context.Context) (application.OfficialExportPin, error) {
	q := quality.New(r.db)
	row, err := q.LeaseByStream(ctx, quality.LeaseByStreamParams{Source: application.OfficialExportSource, Stream: domain.StreamSanction})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.OfficialExportPin{}, fmt.Errorf("%w: no published revision", application.ErrOfficialExportUnavailable)
		}
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: lease", application.ErrOfficialExportUnavailable)
	}
	if row.Status != "published" || row.ProvenanceState != "VERIFIED" || !row.PublishedAt.Valid {
		return application.OfficialExportPin{}, fmt.Errorf("%w: frozen provenance", application.ErrOfficialExportUnavailable)
	}
	publicationKey, err := q.OfficialExportPublicationKey(ctx, row.RevisionID)
	if err != nil || !validSHA256(publicationKey.String) {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: publication key", application.ErrOfficialExportUnavailable)
	}
	resources, err := q.ReadRevisionResources(ctx, row.RevisionID)
	if err != nil {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: resource", application.ErrOfficialExportUnavailable)
	}
	if len(resources) != 1 || resources[0].MemberName != "" || resources[0].Url == "" || len(resources[0].Url) > 1024 || resources[0].Bytes <= 0 || !validSHA256(resources[0].Sha256) {
		return application.OfficialExportPin{}, fmt.Errorf("%w: source resource", application.ErrOfficialExportUnavailable)
	}
	censusContext, err := q.CompanyAssociationContext(ctx, row.RevisionID)
	if err != nil {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: association context", application.ErrOfficialExportUnavailable)
	}
	if censusContext.PolicyVersion != domain.CompanyObservationPolicyVersion || censusContext.AssociationScope != domain.CurrentCompanyOnly || censusContext.HistoricalBusinessWindowKnown || censusContext.ObservationCount < 0 || int(censusContext.ObservationCount) != int(row.Accepted) {
		return application.OfficialExportPin{}, fmt.Errorf("%w: association census", application.ErrOfficialExportUnavailable)
	}
	census, err := q.CompanyAssociationCounts(ctx, row.RevisionID)
	if err != nil {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: association counts", application.ErrOfficialExportUnavailable)
	}
	if census.Total != int64(row.Accepted) || census.Total != census.BoundCount+census.UnmatchedCount+census.AmbiguousCount+census.WrongBranchCount+census.SuccessionCount || census.Total < 0 || census.Total > 500000 {
		return application.OfficialExportPin{}, fmt.Errorf("%w: association totals", application.ErrOfficialExportUnavailable)
	}
	now := time.Now().UTC()
	stale, staleReason := application.Stale(true, row.PublishedAt.Time, now)
	pin := application.OfficialExportPin{
		SchemaVersion:     application.OfficialExportSchemaVersion,
		PublicFieldPolicy: application.OfficialExportPublicFieldPolicy,
		Source:            row.Source,
		Stream:            row.Stream,
		Edition:           row.Edition,
		RevisionID:        uuidString(row.RevisionID),
		LeaseID:           publicationKey.String,
		PublishedAt:       row.PublishedAt.Time.UTC().Format("2006-01-02T15:04:05Z"),
		Status:            row.Status,
		ProvenanceState:   row.ProvenanceState,
		SourceURL:         resources[0].Url,
		SourceChecksum:    resources[0].Sha256,
		SourceBytes:       resources[0].Bytes,
		SourcePeriod:      row.Edition,
		Counts: application.OfficialExportCounts{
			Input: int(row.Input), Accepted: int(row.Accepted),
			Duplicates: int(row.Duplicates), Quarantined: int(row.Quarantined),
		},
		AssociationPolicy:      censusContext.PolicyVersion,
		AssociationScope:       censusContext.AssociationScope,
		AssociationDecisionDay: dateString(censusContext.DecisionDay),
		AssociationCensus: application.AssociationCensus{
			Total: int(census.Total), Linked: int(census.BoundCount),
			Unmatched: int(census.UnmatchedCount), Ambiguous: int(census.AmbiguousCount),
			WrongBranch: int(census.WrongBranchCount), SuccessionReview: int(census.SuccessionCount),
		},
		Stale: stale, StaleReason: staleReason,
	}
	clock, err := q.NativeFineRevisionClock(ctx, row.RevisionID)
	if err == nil {
		pin.SourcePeriod = strconv.Itoa(int(clock.ReportYear))
		pin.SourceClockPrecision = "MONTH"
		pin.SourceClockValue = clock.ExtractionMonth
		if old, why := domain.FineMonthStale(clock.ExtractionMonth, now); old {
			pin.Stale = true
			pin.StaleReason = why
		}
		return pin, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: source clock", application.ErrOfficialExportUnavailable)
	}
	history, err := q.HistoricalFineContext(ctx, row.RevisionID)
	if err != nil {
		if ctx.Err() != nil {
			return application.OfficialExportPin{}, ctx.Err()
		}
		return application.OfficialExportPin{}, fmt.Errorf("%w: historical source", application.ErrOfficialExportUnavailable)
	}
	descriptor := application.FineHistoryContext{
		InputContract: history.InputContract, SourcePeriod: history.SourcePeriod,
		SourceFile: history.SourceFile, HeaderRecord: int(history.HeaderRecord),
		Clock:             application.FineHistoryClock{Precision: history.SourceClockPrecision, Value: history.SourceClockValue},
		SourceRecordCount: int(history.SourceRecordCount), MetadataRecords: int(history.MetadataCount),
	}
	if !descriptor.Valid() || descriptor.SourceRecordCount != pin.Counts.Input || descriptor.MetadataRecords > pin.Counts.Quarantined {
		return application.OfficialExportPin{}, fmt.Errorf("%w: historical clock", application.ErrOfficialExportUnavailable)
	}
	pin.SourcePeriod = descriptor.SourcePeriod
	pin.SourceClockPrecision = descriptor.Clock.Precision
	pin.SourceClockValue = descriptor.Clock.Value
	if old, why := domain.FineSourceClockStale(descriptor.Clock.Precision, descriptor.Clock.Value, now); old {
		pin.Stale = true
		pin.StaleReason = why
	}
	return pin, nil
}

// ExportSanctions pages one frozen revision by native key. A non-empty cursor
// is sealed to the same revision; a concurrent publication refuses instead of
// mixing two editions.
func (r *ExportReader) ExportSanctions(ctx context.Context, after string, limit int) ([]application.OfficialRecord, string, error) {
	if err := application.ValidateOfficialExportLimit(limit); err != nil {
		return nil, "", err
	}
	sealedRevision, afterNative, err := application.OpenOfficialExportCursor(after)
	if err != nil {
		return nil, "", err
	}
	pin, err := r.ExportPin(ctx)
	if err != nil {
		return nil, "", err
	}
	if sealedRevision != "" && sealedRevision != pin.RevisionID {
		return nil, "", fmt.Errorf("%w: revision changed", application.ErrOfficialExportCursor)
	}
	revisionID, err := mustUUID(pin.RevisionID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: revision", application.ErrOfficialExportUnavailable)
	}
	rows, err := quality.New(r.db).OfficialSanctionExportPage(ctx, quality.OfficialSanctionExportPageParams{
		RevisionID: revisionID, AfterNative: afterNative, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", fmt.Errorf("%w: page", application.ErrOfficialExportUnavailable)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	records := make([]application.OfficialRecord, 0, len(rows))
	for _, row := range rows {
		state, err := application.AssociationStateFor(row.ResolutionKind)
		if err != nil {
			return nil, "", err
		}
		stationID := ""
		if state == application.AssociationLinked {
			stationID = uuidString(row.StationID)
			if stationID == "" {
				return nil, "", fmt.Errorf("%w: station", application.ErrOfficialExportIncomplete)
			}
		}
		var amount *int64
		if row.AmountMinor.Valid {
			value := row.AmountMinor.Int64
			amount = &value
		}
		var reportYear *int32
		if row.ReportYear.Valid {
			value := row.ReportYear.Int32
			reportYear = &value
		}
		record := application.OfficialRecord{
			NativeKey: row.NativeKey, SubjectCNPJ: row.PartyCnpj,
			Scope: application.OfficialExportScope, Status: row.Status,
			AsOf: dateString(row.AsOf), Evidence: row.EvidenceRef,
			StationID: stationID, AssociationState: string(state), AssociationReason: row.Reason,
			RevisionID: pin.RevisionID, LeaseID: pin.LeaseID,
			SourceURL: pin.SourceURL, SourceChecksum: pin.SourceChecksum,
			ProceedingRef: row.ProceedingRef, AmountMinor: amount,
			AmountText: row.AmountText.String, DecidedOn: dateString(row.DecidedOn),
			DueOn: dateString(row.DueOn), SourceContract: row.SourceContract,
			SourcePeriod: pin.SourcePeriod, ExtractionMonth: row.ExtractionMonth.String,
			ReportYear: reportYear,
		}
		if err := application.ValidateOfficialRecord(record); err != nil {
			return nil, "", err
		}
		records = append(records, record)
	}
	next := ""
	if hasMore && len(records) > 0 {
		next = application.SealOfficialExportCursor(pin.RevisionID, records[len(records)-1].NativeKey)
	}
	return records, next, nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

var _ application.ExportReader = (*ExportReader)(nil)
