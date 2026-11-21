package adapters

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	export "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/jackc/pgx/v5"
)

const accountingConnector = "accounting-csv-v1"

type accountingFiles interface {
	AccountingFile(context.Context, kernel.OrgScope, string) (export.Object, error)
}

type AccountingDownloads struct {
	Access       Access
	Keys         *os.Root
	Files        accountingFiles
	FilesContent exportapp.Objects
}

// Download returns one approved private artifact or a matching validator.
// The proof is bound to the exact company, connection and file and carries no
// company session, export grant or permission to retrieve another file.
func (s AccountingDownloads) Download(ctx context.Context, scope kernel.OrgScope, connectionID, exportID, proof, knownETag string) (body []byte, etag string, err error) {
	if s.Keys == nil || s.Files == nil || s.FilesContent == nil || scope.Validate() != nil ||
		!audit.ValidID(connectionID) || !audit.ValidID(exportID) {
		return nil, "", domain.ErrDenied
	}
	ref, ok := s.verify(scope.OrgID().String(), connectionID, exportID, proof)
	if !ok {
		return nil, "", exportapp.ErrUnauthenticated
	}
	object, err := s.Files.AccountingFile(ctx, scope, exportID)
	if err != nil {
		return nil, "", err
	}
	err = s.Access.WithActive(ctx, scope, domain.AccountingExport, connectionID, exportID, func(_ context.Context, _ pgx.Tx, review domain.Approval) error {
		if review.Provider != accountingConnector || ref != review.Credential.Reference() ||
			object.Bytes > review.MaxBytes || object.Rows > int64(review.MaxItems) {
			return domain.ErrDenied
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	etag = `"` + object.SHA256 + `"`
	if knownETag == etag {
		return nil, etag, nil
	}
	body, err = s.FilesContent.Get(ctx, scope, object)
	return body, etag, err
}

func (s AccountingDownloads) verify(org, connection, exportID, proof string) (string, bool) {
	ref, digest, ok := strings.Cut(proof, ".")
	if !ok || !audit.ValidID(ref) || len(digest) != sha256.Size*2 {
		return "", false
	}
	received, err := hex.DecodeString(digest)
	if err != nil {
		return "", false
	}
	key, err := readConnectorKey(s.Keys, ref)
	if err != nil {
		return "", false
	}
	defer clear(key[:])
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte("abastevo-accounting-v1\n" + org + "\n" + connection + "\n" + exportID))
	return ref, hmac.Equal(mac.Sum(nil), received)
}
