package prepared

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const offerChunkBytes = 256 << 10

func stageCatalogOffer(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, bundle NationalBundle) error {
	for _, artifact := range []struct {
		kind string
		hash string
		data []byte
	}{{"data", bundle.OfferSHA256, bundle.Catalog}, {"notice", bundle.NoticeSHA256, bundle.Notice}} {
		if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_offer_artifacts
			(batch_id,kind,sha256,bytes) VALUES($1,$2,$3,$4)`, batch, artifact.kind, artifact.hash, len(artifact.data)); err != nil {
			return err
		}
		for position, offset := 0, 0; offset < len(artifact.data); position, offset = position+1, offset+offerChunkBytes {
			end := min(offset+offerChunkBytes, len(artifact.data))
			if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_offer_chunks
				(batch_id,kind,position,data) VALUES($1,$2,$3,$4)`, batch, artifact.kind, position, artifact.data[offset:end]); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireCatalogOffer(ctx context.Context, tx pgx.Tx, batch pgtype.UUID) error {
	var complete int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT a.kind FROM charging_catalog_offer_artifacts a
		JOIN charging_catalog_generations g ON g.batch_id=a.batch_id
		JOIN charging_catalog_offer_chunks c ON c.batch_id=a.batch_id AND c.kind=a.kind
		WHERE a.batch_id=$1 AND (a.kind='notice' OR a.sha256=g.offer_sha256)
		GROUP BY a.kind,a.bytes
		HAVING sum(octet_length(c.data))=a.bytes AND min(c.position)=0
			AND max(c.position)+1=count(*) AND count(*)=(a.bytes+262143)/262144
	) ready`, batch).Scan(&complete)
	if err != nil {
		return err
	}
	if complete != 2 {
		return errors.New("prepared: complete national offer required")
	}
	return nil
}
