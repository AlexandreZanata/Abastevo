package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Location quality levels. Unknown is the default for missing or unverified
// positions; city-centroid results are recorded for transparency but must
// never establish a precise station position (B-BR-014).
const (
	QualityUnknown      = "unknown"
	QualityCityCentroid = "city-centroid"
	QualityReviewed     = "reviewed"
)

var (
	ErrUnknownQuality      = errors.New("directory: unknown location quality")
	ErrLocationNotReviewed = errors.New("directory: only reviewed points project as current")
	ErrMissingPoint        = errors.New("directory: reviewed location needs a point")
	ErrUnknownStation      = errors.New("directory: unknown station")
	ErrUnknownIdentifier   = errors.New("directory: unknown identifier")
)

// ParseQuality accepts exactly the stored quality levels.
func ParseQuality(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case QualityUnknown:
		return QualityUnknown, nil
	case QualityCityCentroid:
		return QualityCityCentroid, nil
	case QualityReviewed:
		return QualityReviewed, nil
	default:
		return "", ErrUnknownQuality
	}
}

// Station is the canonical identity of a fuel station. Location lives in
// revisions plus one reviewed projection; it is never inferred from an
// address string (merging unrelated businesses by address is the P02-T03
// risk to avoid).
type Station struct {
	ID               string
	DisplayName      string
	MunicipalityCode string
	State            string
	Status           string
	CurrentPointWKT  string
	CurrentQuality   string
	CreatedAt        time.Time
}

// Identifier binds one normalized CNPJ to a station over a validity window.
// Retiring (valid_to set) preserves history; rows are never deleted.
type Identifier struct {
	ID               string
	StationID        string
	Kind             string
	NormalizedValue  string
	ValidFrom        time.Time
	ValidTo          *time.Time
	SourceRevisionID string
}

// LocationRevision is one append-only position record. PointWKT is empty
// when the position is missing or unverified; Quality records why.
type LocationRevision struct {
	ID              string
	StationID       string
	PointWKT        string
	Quality         string
	Provider        string
	SourceReference string
	ObtainedAt      time.Time
	SupersedesID    string
}

// Projected checks whether a revision may become the current projection:
// reviewed quality with a concrete point. Anything else stays recorded
// history and fails here instead of silently projecting.
func (r LocationRevision) Projected() error {
	if r.Quality != QualityReviewed {
		return ErrLocationNotReviewed
	}
	if strings.TrimSpace(r.PointWKT) == "" {
		return ErrMissingPoint
	}
	return nil
}

// Resolver maps source identifiers to canonical stations. Concurrent calls
// with the same CNPJ converge on one station; unknown identifiers create
// exactly one station and bind the identifier atomically.
type Resolver interface {
	ResolveCNPJ(ctx context.Context, cnpjNormalized, displayName string, address map[string]string) (Station, error)
}

// Store is the owned persistence port behind the adapters.
type Store interface {
	Resolver
	Station(ctx context.Context, id string) (Station, error)
	RecordLocation(ctx context.Context, rev LocationRevision) (LocationRevision, error)
	ProjectLocation(ctx context.Context, stationID, revisionID string) (Station, error)
	RetireIdentifier(ctx context.Context, kind, normalizedValue string) error
	Revisions(ctx context.Context, stationID string) ([]LocationRevision, error)
}
