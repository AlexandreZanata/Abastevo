package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// webGrants reads only verified, currently active web payment grants. It is
// deliberately independent of Play proof, Play credentials or provider clients;
// capability checks still join the caller transaction.
func webGrants(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) ([]string, time.Time, error) {
	rows, err := tx.Query(ctx, `SELECT capabilities,end_at FROM payment_grants
WHERE account_id=$1 AND state='ACTIVE' AND policy_version=1
  AND start_at<=$2 AND end_at>$2
ORDER BY end_at`, uid(accountID), now.UTC())
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var capabilities []string
	var latest time.Time
	for rows.Next() {
		var values []string
		var end time.Time
		if err := rows.Scan(&values, &end); err != nil {
			return nil, time.Time{}, err
		}
		if end.After(latest) {
			latest = end
		}
		for _, value := range values {
			if !seen[value] {
				seen[value] = true
				capabilities = append(capabilities, value)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return capabilities, latest, nil
}
