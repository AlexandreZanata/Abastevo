package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

const snapshotMaxBytes = 10485760

func snapshotKey(accountID string, revision int64, snapshotID string) string {
	return fmt.Sprintf("snapshots/%s/%d/%s.json", accountID, revision, snapshotID)
}

func validSnapshotKey(key, accountID string) bool {
	if len(key) == 0 || len(key) > 1024 || !stringsHasPrefix(key, "snapshots/") {
		return false
	}
	if stringsContains(key, "..") || stringsContains(key, "\\") {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] == 0x7f {
			return false
		}
	}
	return stringsContains(key, accountID)
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func stringsContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}

func storedView(row q.GetSnapshotRow) app.StoredSnapshot {
	return app.StoredSnapshot{SnapshotID: idstr(row.SnapshotID), Revision: row.Revision, Epoch: row.Epoch, ConsentRevision: row.ConsentRevision, ByteSize: row.ByteSize, Checksum: row.Checksum, Status: row.Status, ObjectKey: row.ObjectKey}
}

func (s *Store) StoreSnapshot(ctx context.Context, p app.Principal, snapshotID string, revision, epoch, consentRevision int64, data []byte) (app.StoredSnapshot, error) {
	if !domain.ValidID(snapshotID) || revision < 0 || epoch < 1 || consentRevision < 0 {
		return app.StoredSnapshot{}, domain.ErrInvalid
	}
	if len(data) < 1 || len(data) > snapshotMaxBytes {
		return app.StoredSnapshot{}, domain.ErrInvalid
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(data) {
		return app.StoredSnapshot{}, domain.ErrInvalid
	}
	if s.Objects == nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	key := snapshotKey(p.AccountID, revision, snapshotID)
	if !validSnapshotKey(key, p.AccountID) {
		return app.StoredSnapshot{}, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.StoredSnapshot{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.StoredSnapshot{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if epoch != state.Epoch || consentRevision != state.ConsentRevision || revision > state.Revision {
		rollback(tx)
		return app.StoredSnapshot{}, domain.ErrResnapshot
	}
	row, err := query.InsertSnapshot(ctx, q.InsertSnapshotParams{AccountID: uid(p.AccountID), SnapshotID: uid(snapshotID), Revision: revision, Epoch: epoch, ConsentRevision: consentRevision, ByteSize: int64(len(data)), Checksum: checksum, ObjectKey: key})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			rollback(tx)
			return app.StoredSnapshot{}, domain.ErrUnavailable
		}
		if err = tx.Commit(ctx); err != nil {
			return app.StoredSnapshot{}, domain.ErrUnavailable
		}
		return s.repairSnapshot(ctx, p, snapshotID, data, checksum, key)
	}
	_ = row
	if err = tx.Commit(ctx); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if err = s.Objects.Put(ctx, key, data); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	got, err := s.Objects.Get(ctx, key)
	if err != nil || int64(len(got)) != int64(len(data)) {
		_ = s.Objects.Delete(ctx, key)
		_ = s.markFailed(ctx, p.AccountID, snapshotID)
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	readSum := sha256.Sum256(got)
	if hex.EncodeToString(readSum[:]) != checksum {
		_ = s.Objects.Delete(ctx, key)
		_ = s.markFailed(ctx, p.AccountID, snapshotID)
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if err = s.markReady(ctx, p.AccountID, snapshotID, checksum); err != nil {
		return app.StoredSnapshot{}, err
	}
	return s.fetchSnapshot(ctx, p.AccountID, snapshotID)
}

func (s *Store) repairSnapshot(ctx context.Context, p app.Principal, snapshotID string, data []byte, checksum, key string) (app.StoredSnapshot, error) {
	existing, err := s.fetchSnapshot(ctx, p.AccountID, snapshotID)
	if err != nil {
		return app.StoredSnapshot{}, err
	}
	if existing.Status == "READY" {
		if existing.Checksum != checksum {
			return app.StoredSnapshot{}, domain.ErrConflict
		}
		size, err := s.Objects.Head(ctx, existing.ObjectKey)
		if err == nil && size == existing.ByteSize {
			return existing, nil
		}
	}
	if existing.Status == "FAILED" {
		if err = s.deleteFailed(ctx, p.AccountID, snapshotID); err != nil {
			return app.StoredSnapshot{}, err
		}
		return app.StoredSnapshot{}, domain.ErrConflict
	}
	size, err := s.Objects.Head(ctx, existing.ObjectKey)
	if err == nil && size == existing.ByteSize {
		got, err := s.Objects.Get(ctx, existing.ObjectKey)
		if err == nil {
			readSum := sha256.Sum256(got)
			if hex.EncodeToString(readSum[:]) == existing.Checksum {
				if err = s.markReady(ctx, p.AccountID, snapshotID, existing.Checksum); err != nil {
					return app.StoredSnapshot{}, err
				}
				return s.fetchSnapshot(ctx, p.AccountID, snapshotID)
			}
		}
	}
	if err = s.Objects.Put(ctx, existing.ObjectKey, data); err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	got, err := s.Objects.Get(ctx, existing.ObjectKey)
	if err != nil {
		_ = s.markFailed(ctx, p.AccountID, snapshotID)
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	readSum := sha256.Sum256(got)
	if hex.EncodeToString(readSum[:]) != existing.Checksum {
		_ = s.Objects.Delete(ctx, existing.ObjectKey)
		_ = s.markFailed(ctx, p.AccountID, snapshotID)
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	if err = s.markReady(ctx, p.AccountID, snapshotID, existing.Checksum); err != nil {
		return app.StoredSnapshot{}, err
	}
	return s.fetchSnapshot(ctx, p.AccountID, snapshotID)
}

func (s *Store) markReady(ctx context.Context, accountID, snapshotID, checksum string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	n, err := q.New(tx).MarkSnapshotReady(ctx, q.MarkSnapshotReadyParams{AccountID: uid(accountID), SnapshotID: uid(snapshotID), Checksum: checksum})
	if err != nil {
		return domain.ErrUnavailable
	}
	if n == 0 {
		return domain.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) markFailed(ctx context.Context, accountID, snapshotID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	if _, err = q.New(tx).MarkSnapshotFailed(ctx, q.MarkSnapshotFailedParams{AccountID: uid(accountID), SnapshotID: uid(snapshotID)}); err != nil {
		return domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) deleteFailed(ctx context.Context, accountID, snapshotID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	if err = q.New(tx).DeleteSnapshotObject(ctx, q.DeleteSnapshotObjectParams{AccountID: uid(accountID), SnapshotID: uid(snapshotID)}); err != nil {
		return domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) fetchSnapshot(ctx context.Context, accountID, snapshotID string) (app.StoredSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	defer rollback(tx)
	row, err := q.New(tx).GetSnapshot(ctx, q.GetSnapshotParams{AccountID: uid(accountID), SnapshotID: uid(snapshotID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.StoredSnapshot{}, domain.ErrNotFound
		}
		return app.StoredSnapshot{}, domain.ErrUnavailable
	}
	return storedView(row), nil
}

func (s *Store) ReadSnapshot(ctx context.Context, p app.Principal, snapshotID string) ([]byte, error) {
	if !domain.ValidID(snapshotID) {
		return nil, domain.ErrInvalid
	}
	if s.Objects == nil {
		return nil, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return nil, err
	}
	row, err := query.GetSnapshot(ctx, q.GetSnapshotParams{AccountID: uid(p.AccountID), SnapshotID: uid(snapshotID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, domain.ErrUnavailable
	}
	if row.Status != "READY" {
		return nil, domain.ErrNotFound
	}
	rollback(tx)
	got, err := s.Objects.Get(ctx, row.ObjectKey)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	readSum := sha256.Sum256(got)
	if hex.EncodeToString(readSum[:]) != row.Checksum || int64(len(got)) != row.ByteSize {
		return nil, domain.ErrUnavailable
	}
	return got, nil
}

func (s *Store) ListSnapshots(ctx context.Context, p app.Principal, limit int) ([]app.StoredSnapshot, error) {
	if limit < 1 || limit > 100 {
		return nil, domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return nil, err
	}
	rows, err := query.ListReadySnapshots(ctx, q.ListReadySnapshotsParams{AccountID: uid(p.AccountID), Limit: int32(limit)})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	result := make([]app.StoredSnapshot, 0, len(rows))
	for _, row := range rows {
		result = append(result, app.StoredSnapshot{SnapshotID: idstr(row.SnapshotID), Revision: row.Revision, Epoch: row.Epoch, ConsentRevision: row.ConsentRevision, ByteSize: row.ByteSize, Checksum: row.Checksum, Status: row.Status, ObjectKey: row.ObjectKey})
	}
	return result, nil
}

var _ app.SnapshotStorageStore = (*Store)(nil)
