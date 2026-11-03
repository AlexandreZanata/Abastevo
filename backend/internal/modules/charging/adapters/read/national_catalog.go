package read

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNationalCursorStale   = errors.New("charging: national generation changed")
	ErrNationalPlaceNotFound = errors.New("charging: national place not active")
)

const nationalPlaceColumns = `primary_alias.public_id::text,p.id,p.entity_key,p.external_id,p.name,
	p.country,p.latitude,p.longitude,p.public_access,p.reported_points,p.catalog_details,l.station_id::text`

const nationalPlaceRelations = `
	JOIN charging_places p ON p.id=primary_alias.place_id AND p.batch_id=primary_alias.batch_id
	LEFT JOIN charging_place_station_links l ON l.place_id=p.id AND l.superseded_by IS NULL AND l.state='REVIEWED'`

type rowScanner interface{ Scan(...any) error }

func scanNationalPlace(row rowScanner) (Place, pgtype.UUID, error) {
	var place Place
	var physical pgtype.UUID
	var raw []byte
	if err := row.Scan(&place.ID, &physical, &place.EntityKey, &place.ExternalID, &place.Name, &place.Country,
		&place.Latitude, &place.Longitude, &place.PublicAccess, &place.ReportedPoints, &raw, &place.LinkedStationID); err != nil {
		return Place{}, physical, err
	}
	var details struct {
		Confidence string                     `json:"confidence"`
		UF         string                     `json:"uf"`
		Access     string                     `json:"access"`
		Sources    []domain.SourceAttribution `json:"sources"`
		Operator   string                     `json:"operator"`
		Address    PlaceAddress               `json:"address"`
	}
	if err := json.Unmarshal(raw, &details); err != nil || details.Confidence == "" || len(details.Sources) == 0 {
		return Place{}, physical, errors.New("charging: stored catalog metadata unavailable")
	}
	place.Confidence, place.UF, place.AccessClass, place.Sources = details.Confidence, details.UF, details.Access, details.Sources
	place.Operator, place.Address = details.Operator, details.Address
	return place, physical, nil
}

func (r Reader) nationalPlaces(ctx context.Context, revision, cursor string, limit int) ([]Place, string, error) {
	var generation string
	var after pgtype.UUID
	if cursor != "" {
		var afterText string
		var found bool
		generation, afterText, found = strings.Cut(cursor, ":")
		if !found {
			return nil, "", ErrNationalCursorStale
		}
		var err error
		after, err = mustUUID(afterText)
		if err != nil {
			return nil, "", ErrNationalCursorStale
		}
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var active pgtype.UUID
	var activeRevision string
	err = tx.QueryRow(ctx, `SELECT b.id,b.source_revision FROM charging_catalog_current c
		JOIN charging_batches b ON b.id=c.batch_id WHERE c.singleton`).Scan(&active, &activeRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		if cursor != "" {
			return nil, "", ErrNationalCursorStale
		}
		return []Place{}, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if cursor != "" && generation != active.String() {
		return nil, "", ErrNationalCursorStale
	}
	if revision != "" && revision != activeRevision {
		return []Place{}, "", nil
	}
	rows, err := tx.Query(ctx, `SELECT `+nationalPlaceColumns+`
		FROM charging_catalog_place_aliases primary_alias `+nationalPlaceRelations+`
		WHERE primary_alias.batch_id=$1 AND primary_alias.is_primary
		AND ($2::uuid IS NULL OR primary_alias.public_id>$2)
		ORDER BY primary_alias.public_id LIMIT $3`, active, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := make([]Place, 0, limit)
	physicalIDs := make([]pgtype.UUID, 0, limit)
	var next string
	for rows.Next() {
		if len(out) == limit {
			next = active.String() + ":" + out[limit-1].ID
			break
		}
		place, physical, err := scanNationalPlace(rows)
		if err != nil {
			return nil, "", err
		}
		out = append(out, place)
		physicalIDs = append(physicalIDs, physical)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	groups, err := nationalConnectionGroups(ctx, tx, physicalIDs)
	if err != nil {
		return nil, "", err
	}
	for i, physical := range physicalIDs {
		out[i].Groups = groups[physical]
	}
	return out, next, nil
}

func (r Reader) nationalPlaceDetail(ctx context.Context, id pgtype.UUID) (Place, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Place{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `SELECT `+nationalPlaceColumns+`
		FROM charging_catalog_current c
		JOIN charging_catalog_place_aliases requested ON requested.batch_id=c.batch_id AND requested.public_id=$1
		JOIN charging_catalog_place_aliases primary_alias ON primary_alias.batch_id=requested.batch_id
			AND primary_alias.place_id=requested.place_id AND primary_alias.is_primary
		`+nationalPlaceRelations+` WHERE c.singleton`, id)
	place, physical, err := scanNationalPlace(row)
	if errors.Is(err, pgx.ErrNoRows) {
		var known bool
		err = tx.QueryRow(ctx, `SELECT
			EXISTS(SELECT 1 FROM charging_catalog_place_aliases WHERE public_id=$1) OR
			EXISTS(SELECT 1 FROM charging_places WHERE id=$1 AND catalog_details IS NOT NULL)`, id).Scan(&known)
		if err != nil {
			return Place{}, false, err
		}
		if known {
			return Place{}, false, ErrNationalPlaceNotFound
		}
		return Place{}, false, nil
	}
	if err != nil {
		return Place{}, false, err
	}
	groups, err := nationalConnectionGroups(ctx, tx, []pgtype.UUID{physical})
	if err != nil {
		return Place{}, false, err
	}
	place.Groups = groups[physical]
	return place, true, nil
}

func nationalConnectionGroups(ctx context.Context, tx pgx.Tx, physicalIDs []pgtype.UUID) (map[pgtype.UUID][]ConnectionGroup, error) {
	groups := make(map[pgtype.UUID][]ConnectionGroup, len(physicalIDs))
	if len(physicalIDs) == 0 {
		return groups, nil
	}
	rows, err := tx.Query(ctx, `SELECT g.place_id,g.id::text,g.entity_key,g.external_id,g.connector_code,g.raw_connector_code,
		g.current,g.power_w,g.reported_quantity,g.catalog_source_key
		FROM charging_places p
		JOIN charging_connection_groups g ON g.batch_id=p.batch_id AND g.place_id=p.id
		WHERE p.id=ANY($1::uuid[]) ORDER BY g.place_id,g.entity_key`, physicalIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var physical pgtype.UUID
		var group ConnectionGroup
		if err := rows.Scan(&physical, &group.ID, &group.EntityKey, &group.ExternalID, &group.ConnectorCode,
			&group.RawConnectorCode, &group.Current, &group.PowerW, &group.ReportedQuantity, &group.SourceKey); err != nil {
			return nil, err
		}
		groups[physical] = append(groups[physical], group)
	}
	return groups, rows.Err()
}
