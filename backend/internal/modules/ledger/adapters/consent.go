package adapters

import (
	"context"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/ledger"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/ledger/application"
)

func ledgerBit(dataset string) int32 {
	switch dataset {
	case "refuel":
		return 1
	case "expense":
		return 2
	}
	return 0
}

func (s *Store) SetConsent(ctx context.Context, accountID string, datasets []string, enabled bool) (application.ConsentStatus, error) {
	if len(datasets) < 1 || len(datasets) > 2 {
		return application.ConsentStatus{}, errInvalid()
	}
	var mask int32
	seen := map[string]bool{}
	for _, d := range datasets {
		bit := ledgerBit(d)
		if bit == 0 || seen[d] {
			return application.ConsentStatus{}, errInvalid()
		}
		seen[d] = true
		mask |= bit
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.ConsentStatus{}, errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = query.EnsureLedgerConsent(ctx, uid(accountID)); err != nil {
		return application.ConsentStatus{}, errUnavailable()
	}
	row, err := query.SetLedgerConsent(ctx, q.SetLedgerConsentParams{AccountID: uid(accountID), Enabled: enabled, Mask: mask})
	if err != nil {
		return application.ConsentStatus{}, errUnavailable()
	}
	enabledDatasets := []string{}
	for _, d := range []string{"refuel", "expense"} {
		if row.Mask&ledgerBit(d) != 0 {
			enabledDatasets = append(enabledDatasets, d)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.ConsentStatus{}, errUnavailable()
	}
	return application.ConsentStatus{Revision: row.Revision, Datasets: enabledDatasets}, nil
}

func (s *Store) EraseAll(ctx context.Context, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = query.EnsureLedgerConsent(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.EraseLedgerRecords(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.ClearLedgerGaps(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.ClearLedgerSegments(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.PurgeLedgerOperations(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if _, err = query.SetLedgerConsent(ctx, q.SetLedgerConsentParams{AccountID: uid(accountID), Enabled: false, Mask: 3}); err != nil {
		return errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return errUnavailable()
	}
	return nil
}

var _ application.ConsentStore = (*Store)(nil)
