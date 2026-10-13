package adapters

import (
	"context"
	qsub "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/subscription"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"time"
)

func (s *Store) ExportRows(ctx context.Context, p app.Principal, limit int, after time.Time, key string) ([]app.ExportRow, error) {
	if s == nil || s.Pool == nil {
		return nil, domain.ErrUnavailable
	}
	if limit < 1 || limit > 101 || ((key != "") != (!after.IsZero())) || (key != "" && !digestOK(key)) {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer rollback(tx)
	q := qsub.New(tx)
	if err = s.authorize(ctx, q, p, true, false); err != nil {
		return nil, err
	}
	rows, err := q.ExportPurchases(ctx, qsub.ExportPurchasesParams{AccountID: uid(p.AccountID), HasCursor: key != "", AfterTime: stamp(after), AfterDigest: key, LimitCount: int32(limit)})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	result := make([]app.ExportRow, 0, len(rows))
	for _, r := range rows {
		row := app.ExportRow{ProductID: r.ProductID, BasePlanID: r.BasePlanID, State: domain.State(r.State), VerifiedAt: r.VerifiedAt.Time, Retired: r.Retired, Acknowledged: r.Acknowledged, Digest: r.TokenDigest}
		if r.ValidUntil.Valid {
			until := r.ValidUntil.Time
			row.ValidUntil = &until
		}
		result = append(result, row)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, domain.ErrUnavailable
	}
	return result, nil
}

var _ app.ExportStore = (*Store)(nil)
