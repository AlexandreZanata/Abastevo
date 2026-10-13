package adapters

import (
	"context"
	"errors"
	qsub "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/subscription"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

// RequireCapability joins the caller's transaction; composition must also
// require configured payment ports. No client flag is accepted.
func (s *Store) RequireCapability(ctx context.Context, tx pgx.Tx, owner, capability string) error {
	q := qsub.New(tx)
	if err := s.authorize(ctx, q, app.Principal{AccountID: owner}, false, false); err != nil {
		return err
	}
	row, err := q.ReadStatus(ctx, uid(owner))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDenied
		}
		return domain.ErrUnavailable
	}
	if !slices.Contains(domain.Evaluate(snapshot(row), time.Now(), true).Capabilities, capability) {
		return domain.ErrDenied
	}
	return nil
}
