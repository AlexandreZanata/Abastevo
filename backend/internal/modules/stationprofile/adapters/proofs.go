package adapters

import (
	"context"
	"time"

	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

// ProofStore implements application.ProofStore over the generated
// stationprofile queries. Metadata is hash-bound and immutable; rows
// stay as audit while bytes purge physically.
type ProofStore struct {
	Q *stationprofile.Queries
}

func mapProofRow(row stationprofile.ClaimProof) application.ProofRow {
	var expires time.Time
	if row.ExpiresAt.Valid {
		expires = row.ExpiresAt.Time
	}
	return application.ProofRow{
		ID: uuidString(row.ID), ClaimID: uuidString(row.ClaimID),
		SHA256: row.Sha256, BytesSize: row.BytesSize, Format: row.Format,
		Kind: row.EvidenceKind, ObjectKey: row.ObjectKey,
		Status: row.Status, ExpiresAt: expires,
	}
}

func (s ProofStore) CreateProof(ctx context.Context, id, claimID, declarationID, sha, format, kind, objectKey string, bytesSize int64, expiresAt time.Time) (application.ProofRow, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.ProofRow{}, false, err
	}
	claimUID, err := mustUUID(claimID)
	if err != nil {
		return application.ProofRow{}, false, err
	}
	declarationUID, err := mustUUID(declarationID)
	if err != nil {
		return application.ProofRow{}, false, err
	}
	row, err := s.Q.CreateProof(ctx, stationprofile.CreateProofParams{
		ID: uid, ClaimID: claimUID, DeclarationID: declarationUID,
		Sha256: sha, BytesSize: bytesSize, Format: format,
		EvidenceKind: kind, ObjectKey: objectKey, ExpiresAt: pgTime(expiresAt),
	})
	if err != nil {
		if isNoRows(err) {
			return application.ProofRow{}, false, nil
		}
		return application.ProofRow{}, false, err
	}
	return mapProofRow(row), true, nil
}

func (s ProofStore) ProofByHash(ctx context.Context, claimID, sha string) (application.ProofRow, error) {
	claimUID, err := mustUUID(claimID)
	if err != nil {
		return application.ProofRow{}, application.ErrClaimNotFound
	}
	row, err := s.Q.GetProofByHash(ctx, stationprofile.GetProofByHashParams{
		ClaimID: claimUID, Sha256: sha,
	})
	if err != nil {
		if isNoRows(err) {
			return application.ProofRow{}, application.ErrClaimNotFound
		}
		return application.ProofRow{}, err
	}
	return mapProofRow(row), nil
}

func (s ProofStore) ListExpiredProofs(ctx context.Context, limit int) ([]application.ProofRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.Q.ListExpiredProofs(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	out := make([]application.ProofRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapProofRow(row))
	}
	return out, nil
}

func (s ProofStore) MarkProofExpired(ctx context.Context, id string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	_, err = s.Q.MarkProofExpired(ctx, uid)
	return err
}

func (s ProofStore) MarkProofDeleted(ctx context.Context, id string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	_, err = s.Q.MarkProofDeleted(ctx, uid)
	return err
}
