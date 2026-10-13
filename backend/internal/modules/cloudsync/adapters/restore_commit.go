package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) RestoreCommit(ctx context.Context, p app.Principal, snapshotID, restoreID string, previewRevision, previewExpiresAt int64) (app.RestoreResult, error) {
	if !domain.ValidID(snapshotID) || !domain.ValidID(restoreID) || previewRevision < 0 || previewExpiresAt <= 0 {
		return app.RestoreResult{}, domain.ErrInvalid
	}
	if time.Now().Unix() > previewExpiresAt {
		return app.RestoreResult{}, domain.ErrResnapshot
	}
	if s.Objects == nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	data, err := s.ReadSnapshot(ctx, p, snapshotID)
	if err != nil {
		return app.RestoreResult{}, err
	}
	var parsed restoreData
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&parsed); err != nil || parsed.Version != 1 || len(parsed.Records) > 2000 {
		return app.RestoreResult{}, domain.ErrConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.RestoreResult{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.RestoreResult{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	if old, err := query.GetRestore(ctx, q.GetRestoreParams{AccountID: uid(p.AccountID), RestoreID: uid(restoreID)}); err == nil {
		var prior app.RestoreResult
		if json.Unmarshal(old.Receipt, &prior) != nil {
			return app.RestoreResult{}, domain.ErrUnavailable
		}
		if old.SnapshotID != uid(snapshotID) {
			return app.RestoreResult{}, domain.ErrConflict
		}
		prior.Replayed = true
		if err = tx.Commit(ctx); err != nil {
			return app.RestoreResult{}, domain.ErrUnavailable
		}
		return prior, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	if state.Revision != previewRevision {
		return app.RestoreResult{}, domain.ErrResnapshot
	}
	meta, err := query.GetSnapshot(ctx, q.GetSnapshotParams{AccountID: uid(p.AccountID), SnapshotID: uid(snapshotID)})
	if err != nil || meta.Status != "READY" {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.RestoreResult{}, domain.ErrNotFound
		}
		if err == nil {
			return app.RestoreResult{}, domain.ErrNotFound
		}
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	if meta.Epoch != state.Epoch {
		return app.RestoreResult{}, domain.ErrResnapshot
	}
	pre := state.Revision
	post := state.Revision
	results := []app.CommandResult{}
	seen := map[string]bool{}
	for _, r := range parsed.Records {
		key := r.Dataset + "\x00" + r.ID
		if seen[key] || datasetBit(r.Dataset) == 0 || !domain.ValidID(r.ID) || r.Revision < 1 {
			return app.RestoreResult{}, domain.ErrConflict
		}
		seen[key] = true
		if state.ConsentMask&datasetBit(r.Dataset) == 0 {
			return app.RestoreResult{}, domain.ErrDenied
		}
		if r.Deleted {
			continue
		}
		payload, err := NormalizePayload(r.Dataset, r.ID, r.Payload)
		if err != nil {
			return app.RestoreResult{}, domain.ErrConflict
		}
		erased, err := query.ErasedID(ctx, q.ErasedIDParams{AccountID: uid(p.AccountID), Dataset: r.Dataset, ID: uid(r.ID)})
		if err != nil {
			return app.RestoreResult{}, domain.ErrUnavailable
		}
		if erased {
			continue
		}
		old, err := query.GetRecord(ctx, q.GetRecordParams{AccountID: uid(p.AccountID), Dataset: r.Dataset, ID: uid(r.ID)})
		if err == nil {
			if old.Deleted {
				continue
			}
			priorBody, _ := NormalizePayload(r.Dataset, r.ID, old.Payload)
			if old.Revision == r.Revision && bytes.Equal(priorBody, payload) {
				continue
			}
			var current json.RawMessage
			if len(old.Payload) > 0 {
				current = append([]byte(nil), old.Payload...)
			}
			return app.RestoreResult{}, &app.ConflictError{Conflict: app.Conflict{OperationID: restoreID, Dataset: r.Dataset, ID: r.ID, ExpectedRevision: r.Revision, CurrentRevision: old.Revision, CurrentPayload: current, ProposedPayload: append([]byte(nil), payload...), Erased: false}, Epoch: state.Epoch, Revision: state.Revision}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return app.RestoreResult{}, domain.ErrUnavailable
		} else {
			count, err := query.CountDataset(ctx, q.CountDatasetParams{AccountID: uid(p.AccountID), Dataset: r.Dataset})
			if err != nil {
				return app.RestoreResult{}, domain.ErrUnavailable
			}
			maximum := int64(1000)
			if r.Dataset == "vehicles" {
				maximum = 3
			}
			if r.Dataset == "preferences" {
				maximum = 1
			}
			if count >= maximum {
				return app.RestoreResult{}, domain.ErrQuota
			}
			row, err := query.SaveRecord(ctx, q.SaveRecordParams{AccountID: uid(p.AccountID), Dataset: r.Dataset, ID: uid(r.ID), Payload: payload})
			if err != nil {
				return app.RestoreResult{}, domain.ErrUnavailable
			}
			sequence, err := query.NextRevision(ctx, uid(p.AccountID))
			if err != nil {
				return app.RestoreResult{}, domain.ErrUnavailable
			}
			if err = query.SaveChange(ctx, q.SaveChangeParams{AccountID: uid(p.AccountID), Sequence: sequence, Epoch: state.Epoch, Dataset: r.Dataset, ID: uid(r.ID), RecordRevision: row.Revision, Deleted: false, Payload: payload}); err != nil {
				return app.RestoreResult{}, domain.ErrUnavailable
			}
			post = sequence
			results = append(results, app.CommandResult{OperationID: restoreID, Dataset: r.Dataset, ID: r.ID, Revision: row.Revision, Status: "accepted"})
		}
	}
	result := app.RestoreResult{RestoreID: restoreID, SnapshotID: snapshotID, PreRevision: pre, PostRevision: post, Results: results}
	receipt, _ := json.Marshal(result)
	if err = query.SaveRestore(ctx, q.SaveRestoreParams{AccountID: uid(p.AccountID), RestoreID: uid(restoreID), SnapshotID: uid(snapshotID), PreRevision: pre, PostRevision: post, Receipt: receipt}); err != nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	if err = s.checkPaid(ctx, tx, p.AccountID); err != nil {
		return app.RestoreResult{}, err
	}
	if _, err = query.DeviceSuccess(ctx, q.DeviceSuccessParams{ID: uid(p.DeviceID), AccountID: uid(p.AccountID), Origin: s.Origin}); err != nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return app.RestoreResult{}, domain.ErrUnavailable
	}
	return result, nil
}

var _ app.RestoreCommitStore = (*Store)(nil)
