// Package application defines private subscription use-case ports.
package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
	"time"
)

type Principal struct{ AccountID, FamilyID, ContributorID, KeyID string }
type Command struct{ OperationID, Token, Kind, Provider string }
type Receipt struct {
	OperationID string        `json:"operation_id"`
	Status      domain.Status `json:"status"`
	Replayed    bool          `json:"replayed"`
}
type Attempt struct {
	Principal                                                  Principal
	Provider                                                   string
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

// Service verifies purchases for one store app identity. Store is the store
// name (domain.ProviderPlay when empty); Provider is its canonical lookup.
type Service struct {
	Store                    Store
	Provider                 Verifier
	Tokens                   TokenCodec
	Clock                    func() time.Time
	Environment, PackageName string
	AccountBinding           func(string) string
	StoreName                string
	Catalog                  []Product
	ManagementURL            string
}

// Product is one sellable store product and its plans, owned by the server
// catalog (B-BR-PAY-01). Plan IDs are Play base plans; App Store products
// have none.
type Product struct {
	ProductID string   `json:"product_id"`
	PlanIDs   []string `json:"plan_ids"`
}

// Offer tells a signed-in owner which store products the app may sell and
// the binding the store must carry (B-BR-STORE-04).
type Offer struct {
	Provider       string    `json:"provider"`
	Products       []Product `json:"products"`
	AccountBinding string    `json:"account_binding"`
	ManagementURL  string    `json:"management_url"`
}

type StoredToken struct {
	Provider                                    string
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
type Notification struct{ Provider, Environment, PackageName, MessageID, Digest, PayloadHash string }
type Acknowledger interface {
	Acknowledge(context.Context, ProviderRequest, string) error
}

type RefreshStore interface {
	BeginRefresh(context.Context, StoredToken, string) (Attempt, error)
}

type ExportRow struct {
	Provider     string       `json:"provider"`
	ProductID    string       `json:"product_id"`
	BasePlanID   string       `json:"base_plan_id"`
	State        domain.State `json:"state"`
	ValidUntil   *time.Time   `json:"valid_until"`
	VerifiedAt   time.Time    `json:"verified_at"`
	Retired      bool         `json:"retired"`
	Acknowledged bool         `json:"acknowledged"`
	Digest       string       `json:"-"`
}
type ExportStore interface {
	ExportRows(context.Context, Principal, int, time.Time, string) ([]ExportRow, error)
}

type ReconciliationStore interface {
	ScheduleReconciliation(ctx context.Context, provider, environment, packageName string, limit int) (int, error)
	CleanupReceipts(context.Context) error
}
