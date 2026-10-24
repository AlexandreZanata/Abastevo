package read

import (
	"context"
	"fmt"
	"time"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	"github.com/jackc/pgx/v5/pgtype"
)

// ExportReader implements application.ExportReader over the owned generated
// queries. It can run inside a caller-owned read-only transaction.
type ExportReader struct {
	db official.DBTX
}

// NewExportReader wires the owned queries to a pool or transaction.
func NewExportReader(db official.DBTX) *ExportReader {
	return &ExportReader{db: db}
}

// ExportRevisions returns the current published revision of each survey window
// inside the range. Superseded, staging, failed and under-review revisions are
// not current pointers and never appear.
func (r *ExportReader) ExportRevisions(ctx context.Context, fromSurvey, toSurvey, afterID string, limit int) ([]application.ExportRevision, string, error) {
	if err := application.ValidateExportRevisions(fromSurvey, toSurvey, afterID, limit); err != nil {
		return nil, "", err
	}
	from, err := pgDateString(fromSurvey)
	if err != nil {
		return nil, "", err
	}
	to, err := pgDateString(toSurvey)
	if err != nil {
		return nil, "", err
	}
	rows, err := official.New(r.db).ExportPublishedRevisions(ctx, official.ExportPublishedRevisionsParams{
		FromSurvey: from, ToSurvey: to, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]application.ExportRevision, 0, len(rows))
	for _, row := range rows {
		out = append(out, application.ExportRevision{
			ID:                   uuidString(row.ID),
			SurveyStart:          dateString(row.SurveyStart),
			SurveyEnd:            dateString(row.SurveyEnd),
			PublishedAt:          timestampString(row.PublishedAt),
			SupersedesRevisionID: uuidString(row.SupersedesRevisionID),
			SourceURL:            row.SourceUrl,
			SourceChecksum:       row.SourceChecksum,
			ParserVersion:        row.ParserVersion,
		})
	}
	next := ""
	if hasMore && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

// ExportPrices returns allowlisted rows for one current published revision.
func (r *ExportReader) ExportPrices(ctx context.Context, revisionID, afterID string, limit int) ([]application.ExportPrice, string, error) {
	if err := application.ValidateExportPrices(revisionID, afterID, limit); err != nil {
		return nil, "", err
	}
	rev, err := mustUUID(revisionID)
	if err != nil {
		return nil, "", fmt.Errorf("%w: revision", application.ErrInvalidFilter)
	}
	rows, err := official.New(r.db).ExportRevisionPrices(ctx, official.ExportRevisionPricesParams{
		RevisionID: rev, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]application.ExportPrice, 0, len(rows))
	for _, row := range rows {
		out = append(out, application.ExportPrice{
			ID:             uuidString(row.ID),
			RevisionID:     uuidString(row.RevisionID),
			StationID:      uuidString(row.StationID),
			FuelProduct:    row.FuelProduct,
			Unit:           row.Unit,
			AmountMilliBRL: row.AmountMilliBrl,
			CollectedOn:    dateString(row.CollectedOn),
		})
	}
	next := ""
	if hasMore && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}

func pgDateString(value string) (pgtype.Date, error) {
	day, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: day, Valid: true}, nil
}

func timestampString(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format("2006-01-02T15:04:05Z")
}
