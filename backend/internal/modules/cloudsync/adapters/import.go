package adapters

import (
	"bytes"
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) SetConsent(ctx context.Context, p app.Principal, datasets []string, enabled bool) (app.Status, error) {
	if len(datasets) < 1 || len(datasets) > len(domain.Datasets()) {
		return app.Status{}, domain.ErrInvalid
	}
	var mask int32
	seen := map[string]bool{}
	for _, d := range datasets {
		bit := datasetBit(d)
		if bit == 0 || seen[d] {
			return app.Status{}, domain.ErrInvalid
		}
		seen[d] = true
		mask |= bit
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, true)
	if err != nil {
		return app.Status{}, err
	}
	defer rollback(tx)
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	row, err := query.SetConsent(ctx, q.SetConsentParams{AccountID: uid(p.AccountID), Mask: mask, Enabled: enabled})
	if err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	enabledDatasets := []string{}
	for _, d := range domain.Datasets() {
		if row.ConsentMask&datasetBit(d) != 0 {
			enabledDatasets = append(enabledDatasets, d)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return app.Status{}, domain.ErrUnavailable
	}
	return app.Status{Epoch: row.Epoch, Revision: row.Revision, ConsentRevision: row.ConsentRevision, EnabledDatasets: enabledDatasets}, nil
}

func (s *Store) Preview(ctx context.Context, p app.Principal, b app.Batch) (app.PreviewResult, error) {
	b, err := normalizeBatch(b)
	if err != nil {
		return app.PreviewResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.readTx(ctx, p, true)
	if err != nil {
		return app.PreviewResult{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.PreviewResult{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.PreviewResult{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.PreviewResult{}, domain.ErrUnavailable
	}
	if b.Epoch != state.Epoch {
		return app.PreviewResult{}, domain.ErrResnapshot
	}
	if b.ConsentRevision != state.ConsentRevision {
		return app.PreviewResult{}, domain.ErrDenied
	}
	result := app.PreviewResult{Epoch: state.Epoch, Revision: state.Revision, ConsentRevision: state.ConsentRevision, Items: []app.PreviewItem{}}
	pending := map[string]int64{}
	pendingIDs := map[string]app.Command{}
	admission := vehicleAdmission{}
	for _, c := range b.Commands {
		if state.ConsentMask&datasetBit(c.Dataset) == 0 {
			return app.PreviewResult{}, domain.ErrDenied
		}
		item := app.PreviewItem{OperationID: c.OperationID, Dataset: c.Dataset, ID: c.ID, ExpectedRevision: c.ExpectedRevision}
		key := c.Dataset + "\x00" + c.ID
		if prior, ok := pendingIDs[key]; ok {
			item.Action = "conflict"
			if prior.OperationID == c.OperationID && prior.ExpectedRevision == c.ExpectedRevision && prior.Delete == c.Delete && bytes.Equal(prior.Payload, c.Payload) {
				item.Action = "converge"
			}
			result.Items = append(result.Items, item)
			continue
		}
		erased, err := query.ErasedID(ctx, q.ErasedIDParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)})
		if err != nil {
			return app.PreviewResult{}, domain.ErrUnavailable
		}
		if erased {
			item.Action = "erased"
			item.Erased = true
			result.Items = append(result.Items, item)
			continue
		}
		old, err := query.GetRecord(ctx, q.GetRecordParams{AccountID: uid(p.AccountID), Dataset: c.Dataset, ID: uid(c.ID)})
		if err == nil {
			if old.Revision != c.ExpectedRevision {
				if c.ExpectedRevision == 0 && !old.Deleted && !c.Delete {
					priorBody, _ := NormalizePayload(c.Dataset, c.ID, old.Payload)
					if bytes.Equal(priorBody, c.Payload) {
						item.Action = "converge"
						item.CurrentRevision = old.Revision
					} else {
						item.Action = "conflict"
						item.CurrentRevision = old.Revision
					}
				} else {
					item.Action = "conflict"
					item.CurrentRevision = old.Revision
					item.CurrentDeleted = old.Deleted
				}
			} else if old.Deleted {
				item.Action = "conflict"
				item.CurrentRevision = old.Revision
				item.CurrentDeleted = true
			} else if c.Delete {
				item.Action = "delete"
				item.CurrentRevision = old.Revision
			} else {
				priorBody, _ := NormalizePayload(c.Dataset, c.ID, old.Payload)
				if bytes.Equal(priorBody, c.Payload) {
					item.Action = "converge"
				} else {
					item.Action = "update"
				}
				item.CurrentRevision = old.Revision
			}
		} else if errors.Is(err, pgx.ErrNoRows) {
			if c.ExpectedRevision != 0 || c.Delete {
				item.Action = "conflict"
			} else {
				if c.Dataset == "favorites" && s.Station != nil {
					available, e := s.Station(ctx, tx, c.ID)
					if e != nil {
						return app.PreviewResult{}, domain.ErrUnavailable
					}
					if !available {
						item.Action = "invalid"
						result.Items = append(result.Items, item)
						continue
					}
				}

				var count int64
				if _, ok := pending[c.Dataset]; !ok {
					n, e := query.CountDataset(ctx, q.CountDatasetParams{AccountID: uid(p.AccountID), Dataset: c.Dataset})
					if e != nil {
						return app.PreviewResult{}, domain.ErrUnavailable
					}
					pending[c.Dataset] = n
				}
				count = pending[c.Dataset]
				admissionError := s.admitRecord(ctx, tx, p.AccountID, c.Dataset, count, &admission)
				if errors.Is(admissionError, domain.ErrQuota) || errors.Is(admissionError, domain.ErrDenied) {
					item.Action = "quota"
				} else if admissionError != nil {
					return app.PreviewResult{}, admissionError
				} else {
					item.Action = "create"
					pending[c.Dataset] = count + 1
					pendingIDs[key] = c
				}
			}
		} else {
			return app.PreviewResult{}, domain.ErrUnavailable
		}
		result.Items = append(result.Items, item)
	}
	if err = s.recheckVehicleAdmission(ctx, tx, p.AccountID, admission); err != nil {
		return app.PreviewResult{}, err
	}
	return result, nil
}

var _ app.ImportStore = (*Store)(nil)
