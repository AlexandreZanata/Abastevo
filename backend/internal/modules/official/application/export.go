package application

import (
	"context"
	"fmt"
	"time"
)

const (
	MinExportLimit = 1
	MaxExportLimit = 1000
)

// ExportRevision pins one current published survey revision.
type ExportRevision struct {
	ID                   string
	SurveyStart          string
	SurveyEnd            string
	PublishedAt          string
	SupersedesRevisionID string
	SourceURL            string
	SourceChecksum       string
	ParserVersion        string
}

// ExportPrice is one allowlisted station price from a pinned revision.
type ExportPrice struct {
	ID             string
	RevisionID     string
	StationID      string
	FuelProduct    string
	Unit           string
	AmountMilliBRL int64
	CollectedOn    string
}

// ExportReader is the owned bounded snapshot port. It never includes staging,
// failed or superseded revisions.
type ExportReader interface {
	ExportRevisions(ctx context.Context, fromSurvey, toSurvey, afterID string, limit int) ([]ExportRevision, string, error)
	ExportPrices(ctx context.Context, revisionID, afterID string, limit int) ([]ExportPrice, string, error)
}

// ValidateExportRevisions bounds one revision page and the survey range.
func ValidateExportRevisions(fromSurvey, toSurvey, afterID string, limit int) error {
	if limit < MinExportLimit || limit > MaxExportLimit {
		return fmt.Errorf("%w: export limit", ErrInvalidFilter)
	}
	from, err := time.Parse("2006-01-02", fromSurvey)
	if err != nil {
		return fmt.Errorf("%w: from_survey", ErrInvalidFilter)
	}
	to, err := time.Parse("2006-01-02", toSurvey)
	if err != nil {
		return fmt.Errorf("%w: to_survey", ErrInvalidFilter)
	}
	if from.After(to) {
		return fmt.Errorf("%w: survey range", ErrInvalidFilter)
	}
	if afterID != "" && !isUUID(afterID) {
		return fmt.Errorf("%w: export cursor", ErrInvalidFilter)
	}
	return nil
}

// ValidateExportPrices bounds one price page for a current revision.
func ValidateExportPrices(revisionID, afterID string, limit int) error {
	if limit < MinExportLimit || limit > MaxExportLimit {
		return fmt.Errorf("%w: export limit", ErrInvalidFilter)
	}
	if !isUUID(revisionID) {
		return fmt.Errorf("%w: revision", ErrInvalidFilter)
	}
	if afterID != "" && !isUUID(afterID) {
		return fmt.Errorf("%w: export cursor", ErrInvalidFilter)
	}
	return nil
}
