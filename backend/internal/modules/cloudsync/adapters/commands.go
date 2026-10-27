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
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) EnsureState(ctx context.Context, p app.Principal) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = s.authorize(ctx, query, p, false); err != nil {
		return err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func datasetBit(dataset string) int32 {
	switch dataset {
	case "vehicles":
		return 1
	case "favorites":
		return 2
	case "preferences":
		return 4
	case "driver_earnings":
		return 8
	case "driver_distance":
		return 16
	case "driver_costs":
		return 32
	case "driver_maintenance":
		return 64
	}
	return 0
}
func allDatasetMask() int32 {
	var mask int32
	for _, dataset := range domain.Datasets() {
		mask |= datasetBit(dataset)
	}
	return mask
}

func normalizeBatch(b app.Batch) (app.Batch, error) {
	if len(b.Commands) < 1 || len(b.Commands) > 50 || b.Epoch < 1 || b.ConsentRevision < 0 {
		return b, domain.ErrInvalid
	}
	raw, _ := json.Marshal(b)
	if len(raw) > 65536 {
		return b, domain.ErrInvalid
	}
	b.Commands = append([]app.Command{}, b.Commands...)
	for i, c := range b.Commands {
		if !domain.ValidID(c.OperationID) || !domain.ValidID(c.ID) || c.ExpectedRevision < 0 || datasetBit(c.Dataset) == 0 {
			return b, domain.ErrInvalid
		}
		if c.Delete {
			if len(c.Payload) > 0 && string(c.Payload) != "null" {
				return b, domain.ErrInvalid
			}
			// Ledger dependency/cascade preview is not implemented: archive vehicles.
			if c.Dataset == "vehicles" {
				return b, domain.ErrInvalid
			}
			c.Payload = nil
		} else {
			payload, e := NormalizePayload(c.Dataset, c.ID, c.Payload)
			if e != nil {
				return b, e
			}
			c.Payload = payload
		}
		b.Commands[i] = c
	}
	return b, nil
}
func (s *Store) Apply(ctx context.Context, p app.Principal, b app.Batch) (app.BatchResult, error) {
	b, err := normalizeBatch(b)
	if err != nil {
		return app.BatchResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.BatchResult{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.BatchResult{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.BatchResult{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.BatchResult{}, domain.ErrUnavailable
	}
	if b.Epoch != state.Epoch {
		return app.BatchResult{}, domain.ErrResnapshot
	}
	if b.ConsentRevision != state.ConsentRevision {
		return app.BatchResult{}, domain.ErrDenied
	}
	vehicleSlots := vehicleAdmission{}
	result := app.BatchResult{Epoch: state.Epoch, Revision: state.Revision, Results: []app.CommandResult{}}
	for _, c := range b.Commands {
		if state.ConsentMask&datasetBit(c.Dataset) == 0 {
			return app.BatchResult{}, domain.ErrDenied
		}
		raw, _ := json.Marshal(struct {
			Epoch   int64
			Consent int64
			Command app.Command
		}{b.Epoch, b.ConsentRevision, c})
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		old, e := query.GetCloudOperation(ctx, q.GetCloudOperationParams{AccountID: uid(p.AccountID), OperationID: uid(c.OperationID)})
		if e == nil {
			if old.PayloadHash != hash {
				return app.BatchResult{}, domain.ErrConflict
			}
			var receipt app.CommandResult
			if json.Unmarshal(old.Receipt, &receipt) != nil {
				return app.BatchResult{}, domain.ErrUnavailable
			}
			receipt.Replayed = true
			result.Results = append(result.Results, receipt)
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return app.BatchResult{}, domain.ErrUnavailable
		}
		admission, e := query.CloudAdmission(ctx, uid(p.AccountID))
		if e != nil {
			return app.BatchResult{}, domain.ErrUnavailable
		}
		if admission.Total >= 10000 || admission.Hourly >= 600 {
			return app.BatchResult{}, domain.ErrQuota
		}
		revision, e := s.applyOne(ctx, tx, query, p, c, &vehicleSlots)
		if e != nil {
			var conflict *app.ConflictError
			if errors.As(e, &conflict) {
				conflict.Epoch = state.Epoch
				conflict.Revision = state.Revision
				return app.BatchResult{}, conflict
			}
			return app.BatchResult{}, e
		}
		sequence, e := query.NextRevision(ctx, uid(p.AccountID))
		if e != nil {
			return app.BatchResult{}, domain.ErrUnavailable
		}
		if e = query.SaveChange(ctx, q.SaveChangeParams{AccountID: uid(p.AccountID), Sequence: sequence, Epoch: state.Epoch, Dataset: c.Dataset, ID: uid(c.ID), RecordRevision: revision, Deleted: c.Delete, Payload: c.Payload}); e != nil {
			return app.BatchResult{}, domain.ErrUnavailable
		}
		receipt := app.CommandResult{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, Revision: revision, Status: "accepted"}
		receiptBytes, _ := json.Marshal(receipt)
		if e = query.SaveCloudOperation(ctx, q.SaveCloudOperationParams{AccountID: uid(p.AccountID), OperationID: uid(c.OperationID), PayloadHash: hash, Receipt: receiptBytes}); e != nil {
			return app.BatchResult{}, domain.ErrUnavailable
		}
		result.Results = append(result.Results, receipt)
		result.Revision = sequence
	}
	if err = s.checkPaid(ctx, tx, p.AccountID); err != nil {
		return app.BatchResult{}, err
	}
	if err = s.recheckVehicleAdmission(ctx, tx, p.AccountID, vehicleSlots); err != nil {
		return app.BatchResult{}, err
	}
	if _, err = query.DeviceSuccess(ctx, q.DeviceSuccessParams{ID: uid(p.DeviceID), AccountID: uid(p.AccountID), Origin: s.Origin}); err != nil {
		return app.BatchResult{}, domain.ErrUnavailable
	}
	if err = tx.Commit(ctx); err != nil {
		return app.BatchResult{}, domain.ErrUnavailable
	}
	return result, nil
}
func proposedForConflict(c app.Command) json.RawMessage {
	if c.Delete || len(c.Payload) == 0 {
		return nil
	}
	return json.RawMessage(append([]byte(nil), c.Payload...))
}

func (s *Store) applyOne(ctx context.Context, tx pgx.Tx, query *q.Queries, p app.Principal, c app.Command, admission *vehicleAdmission) (int64, error) {
	erased, err := query.ErasedID(ctx, q.ErasedIDParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	if erased {
		return 0, &app.ConflictError{Conflict: app.Conflict{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, ExpectedRevision: c.ExpectedRevision, Erased: true, ProposedPayload: proposedForConflict(c)}}
	}
	var storedVehicle []byte
	old, err := query.GetRecord(ctx, q.GetRecordParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)})
	if err == nil {
		if !old.Deleted && c.Dataset == "vehicles" && len(old.Payload) > 0 {
			storedVehicle = old.Payload
		}
		if old.Deleted || old.Revision != c.ExpectedRevision {
			var current json.RawMessage
			if !old.Deleted && len(old.Payload) > 0 {
				current = json.RawMessage(append([]byte(nil), old.Payload...))
			}
			return 0, &app.ConflictError{Conflict: app.Conflict{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, ExpectedRevision: c.ExpectedRevision, CurrentRevision: old.Revision, CurrentDeleted: old.Deleted, CurrentPayload: current, ProposedPayload: proposedForConflict(c)}}
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if c.ExpectedRevision != 0 || c.Delete {
			return 0, &app.ConflictError{Conflict: app.Conflict{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, ExpectedRevision: c.ExpectedRevision, ProposedPayload: proposedForConflict(c)}}
		}
		count, e := query.CountDataset(ctx, q.CountDatasetParams{AccountID: uid(p.AccountID), Dataset: c.Dataset})
		if e != nil {
			return 0, domain.ErrUnavailable
		}
		if e = s.admitRecord(ctx, tx, p.AccountID, c.Dataset, count, admission); e != nil {
			return 0, e
		}
	} else {
		return 0, domain.ErrUnavailable
	}
	if c.Delete {
		if err = query.EraseID(ctx, q.EraseIDParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)}); err != nil {
			return 0, domain.ErrUnavailable
		}
		if err = query.ClearChangePayloads(ctx, q.ClearChangePayloadsParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)}); err != nil {
			return 0, domain.ErrUnavailable
		}
		revision, e := query.DeleteRecord(ctx, q.DeleteRecordParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)})
		if e != nil {
			return 0, domain.ErrUnavailable
		}
		return revision, nil
	}
	if c.Dataset == "favorites" {
		if s.Station == nil {
			return 0, domain.ErrUnavailable
		}
		available, e := s.Station(ctx, tx, c.ID)
		if e != nil {
			return 0, domain.ErrUnavailable
		}
		if !available {
			return 0, domain.ErrInvalid
		}
	}
	payload := c.Payload
	if storedVehicle != nil {
		payload, err = domain.MergeStoredVehiclePayload(storedVehicle, c.Payload)
		if err != nil {
			return 0, err
		}
	}
	row, err := query.SaveRecord(ctx, q.SaveRecordParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID), Payload: payload})
	if err != nil {
		return 0, domain.ErrUnavailable
	}
	return row.Revision, nil
}

var _ app.BatchStore = (*Store)(nil)
