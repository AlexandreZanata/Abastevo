package read

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/official"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

// Reader implements application.PriceReader.
type Reader struct {
	pool *pgxpool.Pool
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

// Groups returns current price groups: the latest published revision
// holding the station, one group per product/unit with the newest collected
// row winning. Community stays null until P04.
func (r *Reader) Groups(ctx context.Context, stationID, fuel string) ([]application.PriceGroup, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return nil, err
	}
	rows, err := official.New(r.pool).StationCurrentPrices(ctx, official.StationCurrentPricesParams{
		StationID: uid,
		Fuel:      fuel,
	})
	if err != nil {
		return nil, err
	}
	byKey := map[string]*application.PriceGroup{}
	order := []string{}
	for _, row := range rows {
		key := row.FuelProduct + "\x00" + row.Unit
		g, ok := byKey[key]
		if !ok {
			g = &application.PriceGroup{
				StationID: stationID,
				Product:   row.FuelProduct,
				Unit:      row.Unit,
				Condition: application.Condition{Kind: "STANDARD"},
			}
			byKey[key] = g
			order = append(order, key)
		}
		collected := dateString(row.CollectedOn)
		if g.Official == nil || collected > g.Official.CollectedOn {
			g.Official = &application.OfficialSection{
				Source:      "ANP",
				AmountMilli: row.AmountMilliBrl,
				Currency:    "BRL",
				CollectedOn: collected,
				SurveyStart: dateString(row.SurveyStart),
				SurveyEnd:   dateString(row.SurveyEnd),
				RevisionID:  uuidString(row.RevisionID),
				SourceURL:   row.SourceUrl,
				SourceSha:   row.SourceChecksum,
				RawText:     row.RawPriceText,
			}
		}
	}
	out := make([]application.PriceGroup, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, nil
}

// History returns published prices newest-first with keyset pagination over
// (collected_on, id).
func (r *Reader) History(ctx context.Context, f application.HistoryFilter) ([]application.HistoryEntry, string, error) {
	uid, err := mustUUID(f.StationID)
	if err != nil {
		return nil, "", err
	}
	var revID pgtype.UUID
	revNull := true
	if f.RevisionID != "" {
		revID, err = mustUUID(f.RevisionID)
		if err != nil {
			return nil, "", err
		}
		revNull = false
	}
	var afterDate pgtype.Date
	if f.HasCursor {
		day, derr := time.Parse("2006-01-02", f.AfterDate)
		if derr != nil {
			return nil, "", derr
		}
		afterDate = pgtype.Date{Time: day, Valid: true}
	}
	rows, err := official.New(r.pool).StationPriceHistory(ctx, official.StationPriceHistoryParams{
		StationID:    uid,
		Fuel:         f.Fuel,
		RevisionNull: revNull,
		RevisionID:   revID,
		HasCursor:    f.HasCursor,
		AfterDate:    afterDate,
		AfterID:      orZeroUUID(f.AfterID),
		LimitPlusOne: int32(f.Limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	var out []application.HistoryEntry
	var rowIDs []string
	for _, row := range rows {
		rowIDs = append(rowIDs, uuidString(row.ID))
		out = append(out, application.HistoryEntry{
			RevisionID:  uuidString(row.RevisionID),
			WeekStart:   dateString(row.SurveyStart),
			WeekEnd:     dateString(row.SurveyEnd),
			CollectedOn: dateString(row.CollectedOn),
			Product:     row.FuelProduct,
			Unit:        row.Unit,
			AmountMilli: row.AmountMilliBrl,
			Currency:    "BRL",
			SourceURL:   row.SourceUrl,
			SourceSha:   row.SourceChecksum,
			RawText:     row.RawPriceText,
		})
	}
	if len(out) <= f.Limit {
		if out == nil {
			out = []application.HistoryEntry{}
		}
		return out, "", nil
	}
	out = out[:f.Limit]
	return out, out[len(out)-1].CollectedOn + ":" + rowIDs[f.Limit-1], nil
}

// orZeroUUID maps the empty cursor position to the zero UUID, which sorts
// below every real id in the keyset comparison.
func orZeroUUID(s string) string {
	if s == "" {
		return "00000000-0000-0000-0000-000000000000"
	}
	return s
}
