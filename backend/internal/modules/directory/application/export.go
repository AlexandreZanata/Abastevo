package application

import (
	"context"
	"fmt"
)

const (
	MinExportLimit = 1
	MaxExportLimit = 1000
)

// ExportStation is the public Directory projection used to freeze an export
// snapshot. Status is the sourced registry state; it is not a trust score.
type ExportStation struct {
	ID                string
	DisplayName       string
	CNPJ              string
	Address           map[string]string
	MunicipalityCode  string
	State             string
	Status            string
	LocationQuality   string
	Coordinates       *LatLon
	CurrentRevisionID string
}

// ExportReader is the owned bounded snapshot port. Implementations must not
// write canonical data.
type ExportReader interface {
	ExportPage(ctx context.Context, afterID string, limit int) ([]ExportStation, string, error)
}

// ValidateExportPage bounds one page request.
func ValidateExportPage(afterID string, limit int) error {
	if limit < MinExportLimit || limit > MaxExportLimit {
		return fmt.Errorf("%w: export limit", ErrInvalidFilter)
	}
	if afterID != "" && !isUUID(afterID) {
		return fmt.Errorf("%w: export cursor", ErrInvalidFilter)
	}
	return nil
}
