package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func (s *Store) BeginRefresh(ctx context.Context, row app.StoredToken, operationID string) (app.Attempt, error) {
	current, err := s.ForJob(ctx, row.Digest)
	if err != nil {
		return app.Attempt{}, err
	}
	if current.AccountID != row.AccountID || current.Environment != row.Environment || current.PackageName != row.PackageName {
		return app.Attempt{}, domain.ErrDenied
	}
	hash := sha256.Sum256([]byte("refresh\x00" + row.Digest))
	return s.begin(ctx, app.Attempt{Principal: app.Principal{AccountID: current.AccountID}, OperationID: operationID, Digest: row.Digest, PayloadHash: hex.EncodeToString(hash[:]), Environment: row.Environment, PackageName: row.PackageName, CipherToken: current.CipherToken, Kind: "refresh"}, true)
}

var _ app.RefreshStore = (*Store)(nil)
