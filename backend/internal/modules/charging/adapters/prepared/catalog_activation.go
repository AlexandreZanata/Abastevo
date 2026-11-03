package prepared

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const NationalCatalogProfile = domain.NationalCatalogProfile

// CatalogCoverageOverride is created by the owner-controlled publication
// path. Both digests bind the approval to one comparison.
type CatalogCoverageOverride struct {
	PreviousSHA256, CandidateSHA256 string
	Actor, Reason                   string
}

type catalogCensus struct {
	sha256 string
	counts map[string]int64
}

func readCatalogCensus(ctx context.Context, tx pgx.Tx, batch pgtype.UUID) (catalogCensus, error) {
	var census catalogCensus
	var raw []byte
	var places int64
	err := tx.QueryRow(ctx, `SELECT b.manifest_sha256,g.uf_counts,
		(SELECT count(*) FROM charging_places p WHERE p.batch_id=b.id)
		FROM charging_catalog_generations g JOIN charging_batches b ON b.id=g.batch_id
		WHERE b.id=$1 AND b.profile_id=$2`, batch, NationalCatalogProfile).Scan(&census.sha256, &raw, &places)
	if errors.Is(err, pgx.ErrNoRows) {
		return census, errors.New("prepared: admitted catalog rights and offer required")
	}
	if err != nil {
		return census, err
	}
	var fields map[string]*int64
	if err := json.Unmarshal(raw, &fields); err != nil || places == 0 {
		return census, errors.New("prepared: invalid catalog census")
	}
	census.counts = make(map[string]int64, len(fields))
	remaining := places
	for uf, count := range fields {
		if count == nil || *count < 0 || *count > remaining {
			return census, errors.New("prepared: invalid catalog census")
		}
		census.counts[uf] = *count
		remaining -= *count
	}
	if remaining != 0 {
		return census, errors.New("prepared: catalog census differs from stored sites")
	}
	return census, nil
}

func (o *CatalogCoverageOverride) matches(previous, next catalogCensus) bool {
	return o != nil && o.PreviousSHA256 == previous.sha256 && o.CandidateSHA256 == next.sha256 &&
		validOverrideText(o.Actor, 128) && validOverrideText(o.Reason, 500)
}

func validOverrideText(text string, maximum int) bool {
	return len(text) <= maximum && strings.TrimSpace(text) != "" && !strings.ContainsFunc(text, unicode.IsControl)
}

// ActivateCatalog gates the caller's verified generation, source links and
// rights receipt. The caller must roll back the transaction on error.
func ActivateCatalog(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, override *CatalogCoverageOverride) error {
	var current pgtype.UUID
	var held bool
	if err := tx.QueryRow(ctx, `SELECT batch_id,held FROM charging_catalog_current
		WHERE singleton FOR UPDATE`).Scan(&current, &held); err != nil {
		return err
	}
	if held {
		return errors.New("prepared: national catalog publication is held")
	}
	if current.Valid && current == batch {
		return nil
	}
	next, err := readCatalogCensus(ctx, tx, batch)
	if err != nil {
		return err
	}
	if err := requireCatalogOffer(ctx, tx, batch); err != nil {
		return err
	}
	var alreadyPublished bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM charging_catalog_activations WHERE batch_id=$1)`,
		batch).Scan(&alreadyPublished); err != nil {
		return err
	}
	if alreadyPublished {
		return errors.New("prepared: superseded national catalog cannot be replayed")
	}
	var previous catalogCensus
	if current.Valid {
		previous, err = readCatalogCensus(ctx, tx, current)
		if err != nil {
			return err
		}
	}
	drops, err := domain.CoverageDrops(previous.counts, next.counts)
	if err != nil {
		return err
	}
	var actor, reason *string
	if len(drops) != 0 {
		if !override.matches(previous, next) {
			return fmt.Errorf("prepared: catalog exceeds state coverage loss limit: %+v", drops)
		}
		actor, reason = &override.Actor, &override.Reason
	}
	if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_activations
		(batch_id,previous_batch_id,override_actor,override_reason) VALUES($1,$2,$3,$4)`,
		batch, current, actor, reason); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE charging_catalog_current SET batch_id=$1 WHERE singleton", batch)
	return err
}
