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

type restoreRecord struct {
	Dataset  string          `json:"dataset"`
	ID       string          `json:"id"`
	Revision int64           `json:"revision"`
	Deleted  bool            `json:"deleted"`
	Payload  json.RawMessage `json:"payload"`
}

type restoreData struct {
	Version         int64           `json:"version"`
	Revision        int64           `json:"revision"`
	Epoch           int64           `json:"epoch"`
	ConsentRevision int64           `json:"consent_revision"`
	Records         []restoreRecord `json:"records"`
}

func (s *Store) PreviewRestore(ctx context.Context, p app.Principal, snapshotID string) (app.RestorePreview, error) {
	if !domain.ValidID(snapshotID) {
		return app.RestorePreview{}, domain.ErrInvalid
	}
	if s.Objects == nil {
		return app.RestorePreview{}, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.RestorePreview{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.RestorePreview{}, err
	}
	meta, err := query.GetSnapshot(ctx, q.GetSnapshotParams{AccountID: uid(p.AccountID), SnapshotID: uid(snapshotID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.RestorePreview{}, domain.ErrNotFound
		}
		return app.RestorePreview{}, domain.ErrUnavailable
	}
	if meta.Status != "READY" {
		return app.RestorePreview{}, domain.ErrNotFound
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.RestorePreview{}, domain.ErrUnavailable
	}
	rollback(tx)
	data, err := s.ReadSnapshot(ctx, p, snapshotID)
	if err != nil {
		return app.RestorePreview{}, err
	}
	if len(data) < 1 || len(data) > snapshotMaxBytes {
		return app.RestorePreview{}, domain.ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var parsed restoreData
	if err = dec.Decode(&parsed); err != nil {
		return app.RestorePreview{SnapshotID: snapshotID, SnapshotRevision: meta.Revision, CurrentRevision: state.Revision, Epoch: state.Epoch, ConsentRevision: state.ConsentRevision, Compatible: false, Issues: []string{"malformed-archive"}, ExpiresAt: time.Now().Add(15 * time.Minute).Unix()}, nil
	}
	preview := app.RestorePreview{SnapshotID: snapshotID, SnapshotRevision: meta.Revision, CurrentRevision: state.Revision, Epoch: state.Epoch, ConsentRevision: state.ConsentRevision, Compatible: true, ExpiresAt: time.Now().Add(15 * time.Minute).Unix()}
	if parsed.Version != 1 {
		preview.Compatible = false
		preview.Issues = append(preview.Issues, "old-schema")
		return preview, nil
	}
	if parsed.Revision != meta.Revision || parsed.Epoch != meta.Epoch || parsed.ConsentRevision != meta.ConsentRevision {
		preview.Compatible = false
		preview.Issues = append(preview.Issues, "metadata-mismatch")
		return preview, nil
	}
	if len(parsed.Records) > 2000 {
		preview.Compatible = false
		preview.Issues = append(preview.Issues, "oversize")
		return preview, nil
	}
	newVehicles := int64(0)
	seen := map[string]bool{}
	for _, r := range parsed.Records {
		key := r.Dataset + "\x00" + r.ID
		if seen[key] {
			preview.Compatible = false
			preview.Issues = append(preview.Issues, "duplicate-id")
			return preview, nil
		}
		seen[key] = true
		if datasetBit(r.Dataset) == 0 {
			preview.Compatible = false
			preview.Issues = append(preview.Issues, "unknown-dataset")
			return preview, nil
		}
		if !domain.ValidID(r.ID) || r.Revision < 1 {
			preview.Compatible = false
			preview.Issues = append(preview.Issues, "invalid-id")
			return preview, nil
		}
		if r.Deleted {
			if len(r.Payload) > 0 && string(r.Payload) != "null" {
				preview.Compatible = false
				preview.Issues = append(preview.Issues, "deleted-payload")
				return preview, nil
			}
			preview.ExcludedCount++
			continue
		}
		if _, err = NormalizePayload(r.Dataset, r.ID, r.Payload); err != nil {
			preview.Compatible = false
			preview.Issues = append(preview.Issues, "invalid-payload")
			return preview, nil
		}
		preview.RecordCount++
		erased, err := s.erasedForPreview(ctx, p.AccountID, r.Dataset, r.ID)
		if err != nil {
			return app.RestorePreview{}, domain.ErrUnavailable
		}
		if erased {
			preview.ExcludedCount++
			continue
		}
		current, err := s.recordForPreview(ctx, p.AccountID, r.Dataset, r.ID)
		if err != nil {
			return app.RestorePreview{}, domain.ErrUnavailable
		}
		if current == nil && r.Dataset == "vehicles" {
			newVehicles++
		}
		if current != nil {
			canonical, _ := NormalizePayload(r.Dataset, r.ID, current.Payload)
			if current.Revision != r.Revision || string(canonical) != string(r.Payload) {
				preview.ConflictCount++
			}
		}
	}
	if newVehicles > 0 {
		// Object reads release the first lock. Reacquire current authority and
		// the exact preview revision before reporting admission compatibility.
		checkTx, check, e := s.writeTx(ctx, p)
		if e != nil {
			return app.RestorePreview{}, e
		}
		defer rollback(checkTx)
		if e = s.device(ctx, checkTx, p); e != nil {
			return app.RestorePreview{}, e
		}
		current, e := check.GetState(ctx, uid(p.AccountID))
		if e != nil {
			return app.RestorePreview{}, domain.ErrUnavailable
		}
		if current.Epoch != state.Epoch || current.Revision != state.Revision || current.ConsentRevision != state.ConsentRevision {
			return app.RestorePreview{}, domain.ErrResnapshot
		}
		count, e := check.CountVehicles(ctx, uid(p.AccountID))
		if e != nil {
			return app.RestorePreview{}, domain.ErrUnavailable
		}
		admitted := vehicleAdmission{}
		for i := int64(0); i < newVehicles; i++ {
			e = s.admitRecord(ctx, checkTx, p.AccountID, "vehicles", count+i, &admitted)
			if errors.Is(e, domain.ErrQuota) || errors.Is(e, domain.ErrDenied) {
				preview.Compatible = false
				issue := "vehicle-quota"
				if errors.Is(e, domain.ErrDenied) {
					issue = "vehicle-capability-required"
				}
				preview.Issues = append(preview.Issues, issue)
				return preview, nil
			}
			if e != nil {
				return app.RestorePreview{}, e
			}
		}
		if e = s.checkPaid(ctx, checkTx, p.AccountID); e != nil {
			return app.RestorePreview{}, e
		}
		if e = s.recheckVehicleAdmission(ctx, checkTx, p.AccountID, admitted); e != nil {
			return app.RestorePreview{}, e
		}
	}
	return preview, nil
}

func (s *Store) erasedForPreview(ctx context.Context, accountID, dataset, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer rollback(tx)
	return q.New(tx).ErasedID(ctx, q.ErasedIDParams{AccountID: uid(accountID), Dataset: dataset, ID: uid(id)})
}

func (s *Store) recordForPreview(ctx context.Context, accountID, dataset, id string) (*q.GetRecordRow, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer rollback(tx)
	row, err := q.New(tx).GetRecord(ctx, q.GetRecordParams{AccountID: uid(accountID), Dataset: dataset, ID: uid(id)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

var _ app.RestoreStore = (*Store)(nil)
