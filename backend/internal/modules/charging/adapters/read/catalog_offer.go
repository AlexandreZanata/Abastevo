package read

import (
	"context"
	"errors"
	"io"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

// CatalogOffer describes one immutable file of a published generation.
type CatalogOffer struct {
	batch  pgtype.UUID
	kind   string
	SHA256 string
	Bytes  int64
}

func (r Reader) CatalogOffer(ctx context.Context, revision, kind string) (CatalogOffer, error) {
	offer := CatalogOffer{kind: kind}
	err := r.pool.QueryRow(ctx, `SELECT a.batch_id,a.sha256,a.bytes
		FROM charging_catalog_offer_artifacts a
		JOIN charging_batches b ON b.id=a.batch_id
		WHERE b.profile_id=$3 AND b.source_revision=$1 AND a.kind=$2
		AND EXISTS (SELECT 1 FROM charging_catalog_activations p WHERE p.batch_id=a.batch_id)`,
		revision, kind, domain.NationalCatalogProfile).Scan(&offer.batch, &offer.SHA256, &offer.Bytes)
	return offer, err
}

// WriteCatalogOffer streams chunks of the admitted immutable file.
func (r Reader) WriteCatalogOffer(ctx context.Context, output io.Writer, offer CatalogOffer) error {
	rows, err := r.pool.Query(ctx, `SELECT position,data FROM charging_catalog_offer_chunks
		WHERE batch_id=$1 AND kind=$2 ORDER BY position`, offer.batch, offer.kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	var written int64
	for expected := 0; rows.Next(); expected++ {
		var position int
		var data []byte
		if err := rows.Scan(&position, &data); err != nil {
			return err
		}
		if position != expected || len(data) == 0 || int64(len(data)) > offer.Bytes-written {
			return errors.New("charging: corrupt stored offer")
		}
		n, err := output.Write(data)
		written += int64(n)
		if err != nil {
			return err
		}
		if n != len(data) {
			return io.ErrShortWrite
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if written != offer.Bytes {
		return io.ErrUnexpectedEOF
	}
	return nil
}
