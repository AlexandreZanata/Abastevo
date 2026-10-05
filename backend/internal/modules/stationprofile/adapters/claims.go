package adapters

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

func pgTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

// ClaimStore implements application.ClaimStore over the generated
// stationprofile queries. Reads are owner-scoped at the service
// layer; state transitions stay pending-guarded in SQL.
type ClaimStore struct {
	Q *stationprofile.Queries
}

func mapClaimRow(row stationprofile.ProfileClaim) application.ClaimRow {
	var scopes []string
	if row.Scopes != "" {
		scopes = strings.Split(row.Scopes, ",")
	}
	return application.ClaimRow{
		ID: uuidString(row.ID), AccountID: uuidString(row.AccountID),
		StationID: uuidString(row.StationID), OperatorCNPJ: row.OperatorCnpj,
		OperatorSource: row.OperatorSource, Role: row.Role, Scopes: scopes,
		PolicyVersion: row.PolicyVersion, State: row.State, ClientKey: row.ClientKey,
	}
}

func mapDeclarationRow(row stationprofile.ClaimDeclaration) application.DeclarationRow {
	var expires time.Time
	if row.ExpiresAt.Valid {
		expires = row.ExpiresAt.Time
	}
	return application.DeclarationRow{
		ID: uuidString(row.ID), ClaimID: uuidString(row.ClaimID), Version: int(row.Version),
		NonceDigest: row.NonceDigest, ExpectedDigest: row.ExpectedDigest,
		Declaration: row.Declaration, State: row.State, ExpiresAt: expires,
	}
}

func (s ClaimStore) CreateClaim(ctx context.Context, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey string) (application.ClaimRow, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.ClaimRow{}, false, err
	}
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return application.ClaimRow{}, false, err
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return application.ClaimRow{}, false, err
	}
	row, err := s.Q.CreateClaim(ctx, stationprofile.CreateClaimParams{
		ID: uid, AccountID: ownerID, StationID: stationUID,
		OperatorCnpj: operatorCNPJ, OperatorSource: operatorSource,
		Role: role, Scopes: scopes, PolicyVersion: "profile-v1", ClientKey: clientKey,
	})
	if err != nil {
		if isNoRows(err) {
			return application.ClaimRow{}, false, nil
		}
		return application.ClaimRow{}, false, err
	}
	return mapClaimRow(row), true, nil
}

func (s ClaimStore) Claim(ctx context.Context, id string) (application.ClaimRow, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.ClaimRow{}, application.ErrClaimNotFound
	}
	row, err := s.Q.GetClaim(ctx, uid)
	if err != nil {
		if isNoRows(err) {
			return application.ClaimRow{}, application.ErrClaimNotFound
		}
		return application.ClaimRow{}, err
	}
	return mapClaimRow(row), nil
}

func (s ClaimStore) ClaimByKey(ctx context.Context, accountID, clientKey string) (application.ClaimRow, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return application.ClaimRow{}, application.ErrClaimNotFound
	}
	row, err := s.Q.GetClaimByKey(ctx, stationprofile.GetClaimByKeyParams{
		AccountID: ownerID, ClientKey: clientKey,
	})
	if err != nil {
		if isNoRows(err) {
			return application.ClaimRow{}, application.ErrClaimNotFound
		}
		return application.ClaimRow{}, err
	}
	return mapClaimRow(row), nil
}

func (s ClaimStore) ListOwnedClaims(ctx context.Context, accountID string, limit, offset int) ([]application.ClaimRow, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return nil, application.ErrClaimNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.Q.ListOwnedClaims(ctx, stationprofile.ListOwnedClaimsParams{
		AccountID: ownerID, PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]application.ClaimRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapClaimRow(row))
	}
	return out, nil
}

func (s ClaimStore) CountOpenClaims(ctx context.Context, accountID string) (int64, error) {
	ownerID, err := mustUUID(accountID)
	if err != nil {
		return 0, err
	}
	return s.Q.CountOpenClaims(ctx, ownerID)
}

func (s ClaimStore) SetClaimState(ctx context.Context, id, expected, state string) (int64, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return 0, err
	}
	return s.Q.SetClaimState(ctx, stationprofile.SetClaimStateParams{ID: uid, ExpectedState: expected, State: state})
}

func (s ClaimStore) CreateDeclaration(ctx context.Context, id, claimID string, version int, nonceDigest, expectedDigest, declaration string, expiresAt time.Time) (application.DeclarationRow, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.DeclarationRow{}, err
	}
	claimUID, err := mustUUID(claimID)
	if err != nil {
		return application.DeclarationRow{}, err
	}
	row, err := s.Q.CreateDeclaration(ctx, stationprofile.CreateDeclarationParams{
		ID: uid, ClaimID: claimUID, Version: int32(version),
		NonceDigest: nonceDigest, ExpectedDigest: expectedDigest,
		Declaration: declaration, ExpiresAt: pgTime(expiresAt),
	})
	if err != nil {
		return application.DeclarationRow{}, err
	}
	return mapDeclarationRow(row), nil
}

func (s ClaimStore) SupersedeDeclarations(ctx context.Context, claimID string) error {
	uid, err := mustUUID(claimID)
	if err != nil {
		return err
	}
	_, err = s.Q.SupersedeDeclarations(ctx, uid)
	return err
}

func (s ClaimStore) ActiveDeclaration(ctx context.Context, claimID string) (application.DeclarationRow, error) {
	uid, err := mustUUID(claimID)
	if err != nil {
		return application.DeclarationRow{}, application.ErrClaimNotFound
	}
	row, err := s.Q.ActiveDeclaration(ctx, uid)
	if err != nil {
		if isNoRows(err) {
			return application.DeclarationRow{}, application.ErrClaimNotFound
		}
		return application.DeclarationRow{}, err
	}
	return mapDeclarationRow(row), nil
}

func (s ClaimStore) GetDeclaration(ctx context.Context, id string) (application.DeclarationRow, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.DeclarationRow{}, application.ErrClaimNotFound
	}
	row, err := s.Q.GetDeclaration(ctx, uid)
	if err != nil {
		if isNoRows(err) {
			return application.DeclarationRow{}, application.ErrClaimNotFound
		}
		return application.DeclarationRow{}, err
	}
	return mapDeclarationRow(row), nil
}
