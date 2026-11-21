package domain

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	money "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

type StatementLine struct {
	SourceID, FuelingID string
	OccurredAt          time.Time
	PaidCentavos        int64
}

// ReadFuelCardStatement consumes the reviewed v1 mapping. It accepts only
// opaque identifiers and exact BRL centavos, without PANs or provider labels.
func ReadFuelCardStatement(raw string, maxBytes int64, maxRows int32) ([]StatementLine, error) {
	if int64(len(raw)) > maxBytes || maxRows < 1 {
		return nil, ErrInvalid
	}
	reader := csv.NewReader(strings.NewReader(raw))
	reader.FieldsPerRecord = 5
	header, err := reader.Read()
	if err != nil || strings.Join(header, ",") != "transaction_id,fueling_id,occurred_at,currency,paid_centavos" {
		return nil, ErrInvalid
	}
	var lines []StatementLine
	seen := make(map[string]struct{})
	candidates := make(map[string]struct{})
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(lines) == int(maxRows) ||
			!validID(row[0]) || row[1] != "" && !validID(row[1]) || row[3] != "BRL" {
			return nil, ErrInvalid
		}
		if _, duplicate := seen[row[0]]; duplicate {
			return nil, ErrConflict
		}
		if row[1] != "" {
			if _, duplicate := candidates[row[1]]; duplicate {
				return nil, ErrConflict
			}
			candidates[row[1]] = struct{}{}
		}
		instant, err := time.Parse(time.RFC3339Nano, row[2])
		if err != nil || instant.Year() < 1 || instant.UTC().Format(time.RFC3339Nano) != row[2] {
			return nil, ErrInvalid
		}
		amount, err := money.ParseCentavos(row[4])
		if err != nil || amount == 0 {
			return nil, ErrInvalid
		}
		seen[row[0]] = struct{}{}
		lines = append(lines, StatementLine{SourceID: row[0], FuelingID: row[1], OccurredAt: instant, PaidCentavos: amount})
	}
	if len(lines) == 0 {
		return nil, ErrInvalid
	}
	return lines, nil
}
