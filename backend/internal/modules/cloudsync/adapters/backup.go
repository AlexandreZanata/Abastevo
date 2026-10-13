package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/cloudsync"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"time"
)

var backupExcluded = []string{"credentials", "photos", "cache", "auth", "signing", "purchase", "location", "permissions", "professional", "advanced_rules"}

func (s *Store) Manifest(ctx context.Context, p app.Principal) (app.BackupManifest, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, query, err := s.writeTx(ctx, p)
	if err != nil {
		return app.BackupManifest{}, err
	}
	defer rollback(tx)
	if err = s.device(ctx, tx, p); err != nil {
		return app.BackupManifest{}, err
	}
	if err = query.EnsureState(ctx, uid(p.AccountID)); err != nil {
		return app.BackupManifest{}, domain.ErrUnavailable
	}
	state, err := query.GetState(ctx, uid(p.AccountID))
	if err != nil {
		return app.BackupManifest{}, domain.ErrUnavailable
	}
	counts := map[string]int64{}
	hash := sha256.New()
	for _, dataset := range []string{"vehicles", "favorites", "preferences"} {
		rows, err := query.ListRecords(ctx, q.ListRecordsParams{AccountID: uid(p.AccountID), Dataset: dataset, ID: uid(zeroID), Limit: 1001})
		if err != nil {
			return app.BackupManifest{}, domain.ErrUnavailable
		}
		counts[dataset] = int64(len(rows))
		for _, row := range rows {
			fmt.Fprintf(hash, "%s\x00%s\x00%d\x00", row.Dataset, idstr(row.ID), row.Revision)
			hash.Write(row.Payload)
			hash.Write([]byte{0})
		}
	}
	enabled := []string{}
	for _, d := range []string{"vehicles", "favorites", "preferences"} {
		if state.ConsentMask&datasetBit(d) != 0 {
			enabled = append(enabled, d)
		}
	}
	return app.BackupManifest{
		Version:         1,
		Revision:        state.Revision,
		Epoch:           state.Epoch,
		ConsentRevision: state.ConsentRevision,
		VehicleCount:    counts["vehicles"],
		FavoriteCount:   counts["favorites"],
		PreferenceCount: counts["preferences"],
		Checksum:        hex.EncodeToString(hash.Sum(nil)),
		Datasets:        enabled,
		Excluded:        append([]string(nil), backupExcluded...),
	}, nil
}

var _ app.BackupStore = (*Store)(nil)
