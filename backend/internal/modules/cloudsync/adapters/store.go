package adapters

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	qcloud "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Store struct {
	Pool *pgxpool.Pool
	Paid func(context.Context, pgx.Tx, string) error
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }
func uid(s string) pgtype.UUID           { var id pgtype.UUID; _ = id.Scan(s); return id }
func idstr(id pgtype.UUID) string {
	s := hex.EncodeToString(id.Bytes[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (s *Store) authorize(ctx context.Context, q *qcloud.Queries, p app.Principal, signing bool) error {
	if !domain.ValidID(p.AccountID) || !domain.ValidID(p.FamilyID) {
		return domain.ErrUnauthorized
	}
	if _, err := q.LockOwner(ctx, uid(p.AccountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if _, err := q.LockSession(ctx, qcloud.LockSessionParams{FamilyID: uid(p.FamilyID), AccountID: uid(p.AccountID)}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUnauthorized
		}
		return domain.ErrUnavailable
	}
	if signing {
		if !domain.ValidID(p.ContributorID) || !domain.ValidID(p.KeyID) {
			return domain.ErrUnauthorized
		}
		if _, err := q.LockBinding(ctx, qcloud.LockBindingParams{AccountID: uid(p.AccountID), ContributorID: p.ContributorID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrUnauthorized
			}
			return domain.ErrUnavailable
		}
		if _, err := q.LockSigningKey(ctx, qcloud.LockSigningKeyParams{KeyID: uid(p.KeyID), ContributorID: uid(p.ContributorID)}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrUnauthorized
			}
			return domain.ErrUnavailable
		}
	}
	return nil
}
func (s *Store) writeTx(ctx context.Context, p app.Principal) (pgx.Tx, *qcloud.Queries, error) {
	if s == nil || s.Pool == nil {
		return nil, nil, domain.ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, nil, domain.ErrUnavailable
	}
	q := qcloud.New(tx)
	if err = s.authorize(ctx, q, p, true); err != nil {
		rollback(tx)
		return nil, nil, err
	}
	if s.Paid == nil {
		rollback(tx)
		return nil, nil, domain.ErrDenied
	}
	if err = s.checkPaid(ctx, tx, p.AccountID); err != nil {
		rollback(tx)
		return nil, nil, err
	}
	return tx, q, nil
}
func (s *Store) PutVehicle(ctx context.Context, p app.Principal, v domain.Vehicle, expected int64) (app.Record, error) {
	v, err := domain.NormalizeVehicle(v)
	if err != nil || expected < 0 {
		return app.Record{}, domain.ErrInvalid
	}
	body, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, q, err := s.writeTx(ctx, p)
	if err != nil {
		return app.Record{}, err
	}
	defer rollback(tx)
	old, err := q.GetRecord(ctx, qcloud.GetRecordParams{AccountID: uid(p.AccountID), Dataset: "vehicles", ID: uid(v.ID)})
	if err == nil {
		// JSONB reorders fields; compare typed canonical bytes.
		var prior domain.Vehicle
		_ = json.Unmarshal(old.Payload, &prior)
		priorBody, _ := json.Marshal(prior)
		if !old.Deleted && expected == 0 && bytes.Equal(priorBody, body) {
			return app.Record{Dataset: "vehicles", ID: v.ID, Revision: old.Revision, Payload: body}, nil
		}
		if old.Deleted || expected != old.Revision {
			return app.Record{}, domain.ErrConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.Record{}, domain.ErrUnavailable
	} else {
		if expected != 0 {
			return app.Record{}, domain.ErrConflict
		}
		n, e := q.CountVehicles(ctx, uid(p.AccountID))
		if e != nil {
			return app.Record{}, domain.ErrUnavailable
		}
		if n >= 3 {
			return app.Record{}, domain.ErrQuota
		}
	}
	row, err := q.SaveRecord(ctx, qcloud.SaveRecordParams{AccountID: uid(p.AccountID), Dataset: "vehicles", ID: uid(v.ID), Payload: body})
	if err != nil {
		return app.Record{}, domain.ErrUnavailable
	}
	if err = s.checkPaid(ctx, tx, p.AccountID); err != nil {
		return app.Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Record{}, domain.ErrUnavailable
	}
	return app.Record{Dataset: row.Dataset, ID: idstr(row.ID), Revision: row.Revision, Deleted: row.Deleted, Payload: row.Payload}, nil
}

var _ app.VehicleStore = (*Store)(nil)

func (s *Store) checkPaid(ctx context.Context, tx pgx.Tx, owner string) error {
	if s.Paid == nil {
		return domain.ErrDenied
	}
	err := s.Paid(ctx, tx, owner)
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrDenied) || errors.Is(err, domain.ErrUnauthorized) {
		return err
	}
	return domain.ErrUnavailable
}
