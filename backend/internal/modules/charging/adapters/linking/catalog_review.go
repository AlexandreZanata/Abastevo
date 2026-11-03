package linking

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CarryCatalogReviewTx retains an active human review for a new physical
// place. Prior places are locked in ID order against concurrent reviews.
func CarryCatalogReviewTx(ctx context.Context, tx pgx.Tx, placeID string, priorIDs []string) error {
	if len(priorIDs) == 0 {
		return nil
	}
	place, err := uuid(placeID)
	if err != nil {
		return err
	}
	prior := make([]pgtype.UUID, len(priorIDs))
	for i, raw := range priorIDs {
		prior[i], err = uuid(raw)
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, "SELECT id FROM charging_places WHERE id=ANY($1::uuid[]) ORDER BY id FOR UPDATE", prior); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id,station_id,state FROM charging_place_station_links
		WHERE place_id=ANY($1::uuid[]) AND superseded_by IS NULL AND reviewer_ref<>$2
		ORDER BY reviewed_at DESC,id DESC`, prior, sourceLinkReviewer)
	if err != nil {
		return err
	}
	defer rows.Close()
	var chosen, station pgtype.UUID
	var state State
	for rows.Next() {
		var id, candidateStation pgtype.UUID
		var candidateState State
		if err := rows.Scan(&id, &candidateStation, &candidateState); err != nil {
			return err
		}
		if !chosen.Valid {
			chosen, station, state = id, candidateStation, candidateState
		} else if station != candidateStation || state != candidateState {
			return errors.New("linking: catalog aliases have conflicting human reviews")
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !chosen.Valid {
		return nil
	}
	id, err := newID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO charging_place_station_links
		(id,place_id,station_id,state,method,evidence,reviewer_ref,reviewed_at)
		SELECT $1,$2,station_id,state,method,
			evidence || jsonb_build_object('carried_from_link_id',COALESCE(evidence->>'carried_from_link_id',id::text)),
			reviewer_ref,reviewed_at
		FROM charging_place_station_links WHERE id=$3`, id, place, chosen)
	return err
}
