package prepared

import (
	"context"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const noCatalog = "none"

var catalogDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CatalogControl binds an owner action to the active generation. The literal
// "none" denotes the state before any national catalog is visible.
type CatalogControl struct {
	Action, ExpectedSHA256, RestoreSHA256 string
	Actor, Reason                         string
}

func (c CatalogControl) Validate() error {
	validDigest := func(value string) bool { return value == noCatalog || catalogDigest.MatchString(value) }
	if !validDigest(c.ExpectedSHA256) || !validOverrideText(c.Actor, 128) || !validOverrideText(c.Reason, 500) {
		return errors.New("prepared: expected digest, actor and reason required")
	}
	switch c.Action {
	case "hold", "resume":
		if c.RestoreSHA256 == "" {
			return nil
		}
	case "rollback":
		if c.ExpectedSHA256 != noCatalog && validDigest(c.RestoreSHA256) && c.RestoreSHA256 != c.ExpectedSHA256 {
			return nil
		}
	}
	return errors.New("prepared: invalid publication control action or restore digest")
}

// ControlCatalog serializes owner controls with imports and atomically records
// the decision. Rollback follows the recorded predecessor and freezes updates.
func ControlCatalog(ctx context.Context, pool *pgxpool.Pool, control CatalogControl) error {
	if err := control.Validate(); err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current pgtype.UUID
	var held bool
	if err := tx.QueryRow(ctx, `SELECT batch_id, held FROM charging_catalog_current
		WHERE singleton FOR UPDATE`).Scan(&current, &held); err != nil {
		return err
	}
	digest := noCatalog
	if current.Valid {
		if err := tx.QueryRow(ctx, "SELECT manifest_sha256 FROM charging_batches WHERE id=$1", current).Scan(&digest); err != nil {
			return err
		}
	}
	if digest != control.ExpectedSHA256 {
		return errors.New("prepared: stale expected national catalog digest")
	}

	next, nextHeld := current, control.Action != "resume"
	if control.Action == "rollback" {
		var restoreDigest string
		err := tx.QueryRow(ctx, `SELECT a.previous_batch_id, COALESCE(b.manifest_sha256, 'none')
			FROM charging_catalog_activations a
			LEFT JOIN charging_batches b ON b.id=a.previous_batch_id
			WHERE a.batch_id=$1`, current).Scan(&next, &restoreDigest)
		if err != nil {
			return err
		}
		if restoreDigest != control.RestoreSHA256 {
			return errors.New("prepared: restore digest differs from recorded predecessor")
		}
		if next.Valid {
			if err := requireCatalogOffer(ctx, tx, next); err != nil {
				return err
			}
		}
	} else if held == nextHeld {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_control_events
		(action,previous_batch_id,batch_id,actor,reason) VALUES($1,$2,$3,$4,$5)`,
		control.Action, current, next, control.Actor, control.Reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE charging_catalog_current SET batch_id=$1,held=$2 WHERE singleton", next, nextHeld); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
