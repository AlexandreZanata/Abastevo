package jobs

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/geocoder"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Geocode resolves one bounded batch of unprojected stations per run.
type Geocode struct {
	Pool    *pgxpool.Pool
	Service *geocoder.Service
	Batch   int
}

// Kind implements jobs.Handler.
func (Geocode) Kind() string { return "geocode-station" }

// Version implements jobs.Handler.
func (Geocode) Version() int { return 1 }

// Handle resolves up to Batch stations missing a projection. Stations with
// any recorded revision stay out: misses remain explicitly unknown until
// an operator re-queues them, instead of spamming the provider every tick.
// A nil service refuses: coordinates are never fabricated, and D05 selects
// the live provider before this runs in production.
func (g Geocode) Handle(ctx context.Context, job jobs.Job) error {
	if g.Service == nil {
		return errors.New("geocode: no service configured (D05 pending)")
	}
	batch := g.Batch
	if batch <= 0 {
		batch = 25
	}
	rows, err := directory.New(g.Pool).StationsMissingProjection(ctx, int32(batch))
	if err != nil {
		return err
	}
	for _, row := range rows {
		address := ""
		if len(row.Address) > 0 {
			var m map[string]any
			if jerr := json.Unmarshal(row.Address, &m); jerr == nil {
				if s, ok := m["municipio"].(string); ok {
					address = s
				}
			}
		}
		if _, err := g.Service.Resolve(ctx, geocoder.Query{
			StationID: uuidString(row.ID), Address: address,
		}); err != nil {
			return fmt.Errorf("geocode %s: %w", uuidString(row.ID), err)
		}
	}
	return nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}
