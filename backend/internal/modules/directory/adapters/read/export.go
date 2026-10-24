package read

import (
	"context"
	"encoding/json"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// ExportReader implements application.ExportReader over the owned generated
// queries. It can run inside a caller-owned read-only transaction.
type ExportReader struct {
	db directory.DBTX
}

// NewExportReader wires the owned queries to a pool or transaction.
func NewExportReader(db directory.DBTX) *ExportReader {
	return &ExportReader{db: db}
}

var exportAddressKeys = []string{"street", "number", "complement", "district", "postal_code", "municipio", "uf"}

func exportAddress(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	address := make(map[string]string, len(exportAddressKeys))
	for _, key := range exportAddressKeys {
		text, ok := values[key].(string)
		if ok && text != "" {
			address[key] = text
		}
	}
	if len(address) == 0 {
		return nil
	}
	return address
}

// ExportPage walks stations in canonical ID order. Identifiers are resolved
// only while valid; a suspended or revoked station remains in the snapshot
// with its sourced status instead of disappearing.
func (r *ExportReader) ExportPage(ctx context.Context, afterID string, limit int) ([]application.ExportStation, string, error) {
	if err := application.ValidateExportPage(afterID, limit); err != nil {
		return nil, "", err
	}
	rows, err := directory.New(r.db).ExportStations(ctx, directory.ExportStationsParams{
		AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]application.ExportStation, 0, len(rows))
	for _, row := range rows {
		coords, err := parseWKT(wktString(row.CurrentPointWkt))
		if err != nil {
			return nil, "", err
		}
		quality := row.CurrentQuality.String
		if coords == nil || quality != domain.QualityReviewed {
			coords = nil
			if quality == "" {
				quality = domain.QualityUnknown
			}
		}
		out = append(out, application.ExportStation{
			ID:                uuidString(row.ID),
			DisplayName:       row.DisplayName,
			CNPJ:              row.Cnpj,
			Address:           exportAddress(row.Address),
			MunicipalityCode:  row.MunicipalityCode.String,
			State:             row.State.String,
			Status:            row.Status,
			LocationQuality:   quality,
			Coordinates:       coords,
			CurrentRevisionID: uuidString(row.CurrentRevisionID),
		})
	}
	next := ""
	if hasMore && len(out) > 0 {
		next = out[len(out)-1].ID
	}
	return out, next, nil
}
