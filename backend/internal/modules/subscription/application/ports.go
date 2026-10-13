// Package application defines private subscription use-case ports.
package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"time"
)

type Principal struct{ AccountID, FamilyID, ContributorID, KeyID string }
type Command struct{ OperationID, Token, Kind string }
type Receipt struct {
	OperationID string        `json:"operation_id"`
	Status      domain.Status `json:"status"`
	Replayed    bool          `json:"replayed"`
}
type Attempt struct {
	Principal                                                  Principal
	OperationID, Digest, PayloadHash, Environment, PackageName string
	Generation                                                 int64
	CipherToken                                                []byte
	Replay                                                     bool
	Result, Kind                                               string
	Background                                                 bool
}
type VerifiedPurchase struct {
	Snapshot     domain.Snapshot
	LinkedDigest string
}
type Store interface {
	Begin(context.Context, Attempt) (Attempt, error)
	Commit(context.Context, Attempt, VerifiedPurchase) error
	Reject(context.Context, Attempt) error
	Status(context.Context, Principal) (*domain.Snapshot, error)
}
type ProviderRequest struct{ Token, ExpectedAccount, Environment, PackageName string }
type ProviderPurchase struct {
	Snapshot    domain.Snapshot
	LinkedToken string
}
type Verifier interface {
	Verify(context.Context, ProviderRequest) (ProviderPurchase, error)
}
type TokenCodec interface {
	Seal(string, string) ([]byte, error)
	Open([]byte, string) (string, error)
}
type Service struct {
	Store                    Store
	Provider                 Verifier
	Tokens                   TokenCodec
	Clock                    func() time.Time
	Environment, PackageName string
	AccountBinding           func(string) string
}

type StoredToken struct {
	AccountID, Environment, PackageName, Digest string
	CipherToken                                 []byte
	Snapshot                                    domain.Snapshot
	Revision                                    int64
}
type JobStore interface {
	ForJob(context.Context, string) (StoredToken, error)
	MarkAcknowledged(context.Context, string, int64) error
	ReceiveNotification(context.Context, Notification) (bool, error)
}
type Notification struct{ Environment, PackageName, MessageID, Digest, PayloadHash string }
type Acknowledger interface {
	Acknowledge(context.Context, ProviderRequest, string) error
}

type RefreshStore interface {
	BeginRefresh(context.Context, StoredToken, string) (Attempt, error)
}
