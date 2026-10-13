package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"time"
)

func validOrigin(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '.' && c != '-' {
			return false
		}
	}
	return true
}
func (s *Store) RegisterDevice(ctx context.Context, p app.Principal) (app.Device, error) {
	if !validOrigin(s.Origin) {
		return app.Device{}, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.Device{}, err
	}
	defer rollback(tx)
	if err = query.FenceRestoredDevices(ctx, uid(p.AccountID)); err != nil {
		return app.Device{}, domain.ErrUnavailable
	}
	old, err := query.ExistingDevice(ctx, q.ExistingDeviceParams{AccountID: uid(p.AccountID), FamilyID: uid(p.FamilyID), KeyID: uid(p.KeyID), Origin: s.Origin})
	if err == nil {
		var success *time.Time
		if old.LastSuccessAt.Valid {
			v := old.LastSuccessAt.Time
			success = &v
		}
		return app.Device{ID: idstr(old.ID), Origin: old.Origin, CreatedAt: old.CreatedAt.Time, LastSuccess: success, Revoked: old.RevokedAt.Valid}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return app.Device{}, domain.ErrUnavailable
	}
	count, err := query.CountDevices(ctx, uid(p.AccountID))
	if err != nil {
		return app.Device{}, domain.ErrUnavailable
	}
	if count >= 3 {
		return app.Device{}, domain.ErrQuota
	}
	id, err := jobs.NewUUIDv4()
	if err != nil {
		return app.Device{}, domain.ErrUnavailable
	}
	row, err := query.RegisterDevice(ctx, q.RegisterDeviceParams{ID: uid(id), AccountID: uid(p.AccountID), FamilyID: uid(p.FamilyID), ContributorID: uid(p.ContributorID), KeyID: uid(p.KeyID), Origin: s.Origin})
	if err != nil {
		return app.Device{}, domain.ErrUnavailable
	}
	if err = s.checkPaid(ctx, tx, p.AccountID); err != nil {
		return app.Device{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Device{}, domain.ErrUnavailable
	}
	return app.Device{ID: idstr(row.ID), Origin: row.Origin, CreatedAt: row.CreatedAt.Time}, nil
}
func (s *Store) device(ctx context.Context, tx pgx.Tx, p app.Principal) error {
	if !validOrigin(s.Origin) || !domain.ValidID(p.DeviceID) {
		return domain.ErrUnauthorized
	}
	_, err := q.New(tx).LockDevice(ctx, q.LockDeviceParams{ID: uid(p.DeviceID), AccountID: uid(p.AccountID), FamilyID: uid(p.FamilyID), ContributorID: uid(p.ContributorID), KeyID: uid(p.KeyID), Origin: s.Origin})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUnauthorized
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func (s *Store) CheckDevice(ctx context.Context, p app.Principal) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	if err = s.authorize(ctx, q.New(tx), p, true); err != nil {
		return err
	}
	return s.device(ctx, tx, p)
}
func (s *Store) RevokeDevice(ctx context.Context, p app.Principal, id string) error {
	if !domain.ValidID(id) {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = s.authorize(ctx, query, p, true); err != nil {
		return err
	}
	n, err := query.RevokeDevice(ctx, q.RevokeDeviceParams{ID: uid(id), AccountID: uid(p.AccountID), Origin: s.Origin})
	if err != nil {
		return domain.ErrUnavailable
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	if err = query.RecordDeviceRevocation(ctx, q.RecordDeviceRevocationParams{ID: uid(id), AccountID: uid(p.AccountID), Origin: s.Origin}); err != nil {
		return domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

var _ app.DeviceStore = (*Store)(nil)
