package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"time"
)

func checksumHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

const (
	snapshotCadence   = 24 * time.Hour
	snapshotTickLimit = 25
	snapshotMaxCount  = 7
)

type SnapshotScheduleStatus struct {
	LastRevision       int64 `json:"last_revision"`
	UnprotectedChanges int64 `json:"unprotected_changes"`
	ReadyCount         int64 `json:"ready_count"`
	Enabled            bool  `json:"enabled"`
}

func (s *Store) SnapshotStatus(ctx context.Context, p app.Principal) (SnapshotScheduleStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return SnapshotScheduleStatus{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return SnapshotScheduleStatus{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return SnapshotScheduleStatus{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return SnapshotScheduleStatus{}, domain.ErrUnavailable
	}
	rows, err := query.ListReadySnapshots(ctx, q.ListReadySnapshotsParams{AccountID: uid(p.AccountID), Limit: 1})
	if err != nil {
		return SnapshotScheduleStatus{}, domain.ErrUnavailable
	}
	var last int64
	var count int64
	if len(rows) > 0 {
		last = rows[0].Revision
	}
	all, err := query.ListReadySnapshots(ctx, q.ListReadySnapshotsParams{AccountID: uid(p.AccountID), Limit: 100})
	if err != nil {
		return SnapshotScheduleStatus{}, domain.ErrUnavailable
	}
	count = int64(len(all))
	unprotected := state.Revision - last
	if unprotected < 0 {
		unprotected = 0
	}
	return SnapshotScheduleStatus{LastRevision: last, UnprotectedChanges: unprotected, ReadyCount: count, Enabled: state.ConsentMask != 0}, nil
}

func (s *Store) TickSnapshots(ctx context.Context, now time.Time, limit int) (int, error) {
	if s == nil || s.Pool == nil || s.Objects == nil {
		return 0, domain.ErrUnavailable
	}
	if now.IsZero() || limit < 1 || limit > 100 {
		return 0, domain.ErrInvalid
	}
	if limit > snapshotTickLimit {
		limit = snapshotTickLimit
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cutoff := pgtype.Timestamptz{Time: now.Add(-snapshotCadence), Valid: true}
	owners, err := q.New(s.Pool).DueSnapshotAccounts(ctx, q.DueSnapshotAccountsParams{CreatedAt: cutoff, Limit: int32(limit)})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	done := 0
	for _, owner := range owners {
		accountID := idstr(owner)
		ok, err := s.snapshotOneAccount(ctx, accountID, now)
		if err != nil {
			return done, err
		}
		if ok {
			done++
		}
	}
	return done, nil
}

func (s *Store) snapshotOneAccount(ctx context.Context, accountID string, now time.Time) (bool, error) {
	// Keep the background admission/upload/receipt sequence serialized across
	// workers. The owner row is released before object I/O, so it cannot be
	// the lease. A separate bounded connection avoids consuming a publication
	// pool slot while another transaction or upload is running.
	cfg := s.Pool.Config().ConnConfig.Copy()
	cfg.ConnectTimeout = 5 * time.Second
	lease, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lease.Close(closeCtx) // Closing releases the session lock, including on cancellation.
	}()
	var admitted bool
	if err = lease.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1, 0))", "cloud-snapshot-tick:"+accountID).Scan(&admitted); err != nil {
		return false, domain.ErrUnavailable
	}
	if !admitted {
		return false, nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if _, err = query.LockOwner(ctx, uid(accountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, domain.ErrUnavailable
	}
	if err = query.EnsureState(ctx, uid(accountID)); err != nil {
		return false, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(accountID))
	if err != nil {
		return false, domain.ErrUnavailable
	}
	if state.ConsentMask == 0 {
		return false, nil
	}
	if s.Paid == nil {
		return false, nil
	}
	if err = s.Paid(ctx, tx, accountID); err != nil {
		if errors.Is(err, domain.ErrDenied) || errors.Is(err, domain.ErrUnauthorized) {
			return false, nil
		}
		return false, domain.ErrUnavailable
	}
	ready, err := query.ListReadySnapshots(ctx, q.ListReadySnapshotsParams{AccountID: uid(accountID), Limit: 1})
	if err != nil {
		return false, domain.ErrUnavailable
	}
	if len(ready) > 0 && ready[0].Revision >= state.Revision {
		return false, nil
	}
	if err = tx.Commit(ctx); err != nil {
		return false, domain.ErrUnavailable
	}
	data, _ := json.Marshal(map[string]any{"version": 1, "revision": state.Revision, "epoch": state.Epoch, "consent_revision": state.ConsentRevision})
	snapshotID, err := jobs.NewUUIDv4()
	if err != nil {
		return false, domain.ErrUnavailable
	}
	stored, err := s.storeSnapshotForAccount(ctx, accountID, snapshotID, state.Revision, state.Epoch, state.ConsentRevision, data)
	if err != nil {
		return false, err
	}
	_ = stored
	if err = s.trimSnapshots(ctx, accountID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) storeSnapshotForAccount(ctx context.Context, accountID, snapshotID string, revision, epoch, consent int64, data []byte) (app.StoredSnapshot, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if _, err = query.LockOwner(ctx, uid(accountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.StoredSnapshot{}, domain.ErrUnauthorized
		}
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(accountID))
	if err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if epoch != state.Epoch || consent != state.ConsentRevision || revision > state.Revision {
		return app.StoredSnapshot{}, domain.ErrResnapshot
	}
	key := snapshotKey(accountID, revision, snapshotID)
	sum := checksumHex(data)
	row, err := query.InsertSnapshot(ctx, q.InsertSnapshotParams{AccountID: uid(accountID), SnapshotID: uid(snapshotID), Revision: revision, Epoch: epoch, ConsentRevision: consent, ByteSize: int64(len(data)), Checksum: sum, ObjectKey: key})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return app.StoredSnapshot{}, domain.ErrUnavailable
		}
		return app.StoredSnapshot{}, domain.ErrConflict
	}
	_ = row
	if err = tx.Commit(ctx); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if err = s.Objects.Put(ctx, key, data); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	got, err := s.Objects.Get(ctx, key)
	if err != nil || checksumHex(got) != sum {
		_ = s.Objects.Delete(ctx, key)
		_ = s.markFailed(ctx, accountID, snapshotID)
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if err = s.markReady(ctx, accountID, snapshotID, sum); err != nil {
		return app.StoredSnapshot{}, err
	}
	return s.fetchSnapshot(ctx, accountID, snapshotID)
}

func (s *Store) trimSnapshots(ctx context.Context, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if _, err = query.LockOwner(ctx, uid(accountID)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return domain.ErrUnavailable
	}
	rows, err := query.ListReadySnapshots(ctx, q.ListReadySnapshotsParams{AccountID: uid(accountID), Limit: 100})
	if err != nil {
		return domain.ErrUnavailable
	}
	if len(rows) <= snapshotMaxCount {
		return nil
	}
	victims := rows[snapshotMaxCount:]
	for _, victim := range victims {
		if err = query.DeleteSnapshot(ctx, q.DeleteSnapshotParams{AccountID: uid(accountID), SnapshotID: victim.SnapshotID}); err != nil {
			return domain.ErrUnavailable
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	for _, victim := range victims {
		_ = s.Objects.Delete(ctx, victim.ObjectKey)
	}
	return nil
}
