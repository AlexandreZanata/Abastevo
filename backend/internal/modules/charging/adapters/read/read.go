// Package read serves bounded public charging/reference facts from
// stored batches. Reads are explicit (profile plus optional revision);
// latest-stored is a documented fallback, never a freshness claim. No
// ingestion, provider fan-out or private fields occur here.
package read

import (
	"context"
	"fmt"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reader composes stored-batch reads without sharing SQL across modules.
type Reader struct {
	pool *pgxpool.Pool
}

// NewReader wires a pool-local reader; queries stay in owned sqlc.
func NewReader(pool *pgxpool.Pool) Reader {
	return Reader{pool: pool}
}

// Place is one public charging location.
type Place struct {
	ID             string
	EntityKey      string
	ExternalID     string
	Name           string
	Country        string
	Latitude       string
	Longitude      string
	PublicAccess   string
	ReportedPoints int64
	Groups         []ConnectionGroup
}

// ConnectionGroup is one public connector group under a place.
type ConnectionGroup struct {
	ID               string
	EntityKey        string
	ExternalID       string
	ConnectorCode    string
	RawConnectorCode string
	Current          string
	PowerW           *int64
	ReportedQuantity *int64
}

// BatchCoverage reports one batch's valid counts with its identity.
type BatchCoverage struct {
	Profile        string
	SourceRevision string
	Places         int64
	Groups         int64
	Vehicles       int64
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	b := id.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func mustUUID(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(text); err != nil {
		return pgtype.UUID{}, err
	}
	if !id.Valid {
		return pgtype.UUID{}, fmt.Errorf("read: invalid UUID")
	}
	return id, nil
}

// Places pages stored places with keyset for an explicit profile and
// optional revision. Empty revision matches any revision for the profile;
// callers prefer explicit revisions when exactness matters.
func (r Reader) Places(ctx context.Context, profile, revision, afterID string, limit int) ([]Place, string, error) {
	if (profile != "ocm-br-20261021-v1" && profile != "pbe-2026-pdf-v1") || limit < 1 || limit > 100 {
		return nil, "", fmt.Errorf("read: invalid page")
	}
	rows, err := charging.New(r.pool).ChargingPlacePage(ctx, charging.ChargingPlacePageParams{
		Profile: profile, Revision: revision, AfterID: afterID, LimitPlusOne: int32(limit + 1),
	})
	if err != nil {
		return nil, "", err
	}
	out := make([]Place, 0, min(limit, len(rows)))
	for i, row := range rows {
		if i == limit {
			return out, uuidString(rows[limit-1].ID), nil
		}
		out = append(out, Place{
			ID: uuidString(row.ID), EntityKey: row.EntityKey, ExternalID: row.ExternalID,
			Name: row.Name, Country: row.Country, Latitude: row.Latitude, Longitude: row.Longitude,
			PublicAccess: row.PublicAccess, ReportedPoints: int64(row.ReportedPoints),
		})
	}
	return out, "", nil
}

// PlaceDetail returns one place with its connection groups. Unknown IDs
// yield nothing; groups order by entity key.
func (r Reader) PlaceDetail(ctx context.Context, id string) (Place, error) {
	placeID, err := mustUUID(id)
	if err != nil {
		return Place{}, err
	}
	row, err := charging.New(r.pool).ChargingPlaceDetail(ctx, placeID)
	if err != nil {
		return Place{}, err
	}
	place := Place{
		ID: uuidString(row.ID), EntityKey: row.EntityKey, ExternalID: row.ExternalID,
		Name: row.Name, Country: row.Country, Latitude: row.Latitude, Longitude: row.Longitude,
		PublicAccess: row.PublicAccess, ReportedPoints: int64(row.ReportedPoints),
	}
	groupRows, err := charging.New(r.pool).ChargingGroupsForPlace(ctx, placeID)
	if err != nil {
		return Place{}, err
	}
	for _, group := range groupRows {
		item := ConnectionGroup{
			ID: uuidString(group.ID), EntityKey: group.EntityKey, ExternalID: group.ExternalID,
			ConnectorCode: group.ConnectorCode, RawConnectorCode: group.RawConnectorCode, Current: group.Current,
		}
		if group.PowerW.Valid {
			power := int64(group.PowerW.Int32)
			item.PowerW = &power
		}
		if group.ReportedQuantity.Valid {
			quantity := int64(group.ReportedQuantity.Int32)
			item.ReportedQuantity = &quantity
		}
		place.Groups = append(place.Groups, item)
	}
	return place, nil
}

// Coverage reports one batch's valid counts. The batch must already be
// resolved by the caller (explicit ID, no guessing).
func (r Reader) Coverage(ctx context.Context, batchID pgtype.UUID) (BatchCoverage, error) {
	row, err := charging.New(r.pool).ChargingBatchCoverage(ctx, batchID)
	if err != nil {
		return BatchCoverage{}, err
	}
	return BatchCoverage{
		Profile: row.ProfileID, SourceRevision: row.SourceRevision,
		Places: row.Places, Groups: row.Groups, Vehicles: row.Vehicles,
	}, nil
}

// LatestBatch returns the latest stored batch ID for a profile, for the
// documented latest-stored fallback. Callers prefer explicit revisions.
func (r Reader) LatestBatch(ctx context.Context, profile string) (pgtype.UUID, error) {
	if profile == "" {
		return pgtype.UUID{}, fmt.Errorf("read: profile required")
	}
	row, err := charging.New(r.pool).LatestChargingBatch(ctx, profile)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return row.ID, nil
}
