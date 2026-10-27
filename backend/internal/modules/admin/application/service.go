// Package application orchestrates administrative login, session checks and
// audited mutations. Credentials are hashed before crossing the store port;
// the raw key, session token and CSRF token never enter persistence.
package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/jackc/pgx/v5"
)

// Session is the authenticated administrative principal. CSRF fields stay
// server-side and never serialize into a response.
type Session struct {
	ID             string
	KeyFingerprint string
	CSRFSalt       string
	CSRFHash       string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
}

type LoginResult struct {
	Token       string
	CSRFToken   string
	Fingerprint string
	ExpiresAt   time.Time
}

type LoginInput struct {
	PresentedHash string
	IPHash        string
	RequestID     string
	Now           time.Time
	Verify        func() (fingerprint string, ok bool)
}

type Mutation struct {
	SessionID    string
	Action       string
	TargetType   string
	TargetID     string
	Reason       string
	Key          string
	RequestHash  string
	RequestID    string
	ClientIPHash string
	Now          time.Time
}

type MutationResult struct {
	Status   int
	Body     []byte
	Replayed bool
}

// AuditEvent is one append-only row written by the mutation helper.
type AuditEvent struct {
	Action, TargetType, TargetID, Reason string
	Before, After                        []byte
}

type Account struct {
	ID        string
	Alias     string
	Status    string
	CreatedAt time.Time
}

type PaidSummary struct {
	State      string
	ValidUntil *time.Time
}

type AccountDetail struct {
	Account Account
	Grants  []entdomain.Grant
	Paid    PaidSummary
}

// GrantsPort is the entitlement-owned persistence surface. Every method joins
// the caller transaction.
type GrantsPort interface {
	LockEligibleAccount(context.Context, pgx.Tx, string) (entdomain.Grant, error)
	ActiveTx(context.Context, pgx.Tx, string, time.Time) ([]entdomain.Grant, error)
	SupersedeExpiredTx(context.Context, pgx.Tx, string, entdomain.Tier, time.Time) (*entdomain.Grant, error)
	CreateTx(context.Context, pgx.Tx, entdomain.NewGrant) (entdomain.Grant, error)
	RevokeTx(context.Context, pgx.Tx, string, time.Time, string) (entdomain.Grant, error)
	GetTx(context.Context, pgx.Tx, string) (entdomain.Grant, error)
	ListTx(context.Context, pgx.Tx, entdomain.GrantFilter) ([]entdomain.Grant, error)
	ListByAccountTx(context.Context, pgx.Tx, string) ([]entdomain.Grant, error)
}

type Store interface {
	Login(context.Context, LoginInput) (LoginResult, error)
	Authenticate(context.Context, string, time.Time) (Session, error)
	Logout(context.Context, Session, Mutation) (MutationResult, error)
	RevokeAll(context.Context, Session, Mutation) (MutationResult, error)
	Mutate(context.Context, Mutation, func(context.Context, pgx.Tx) (MutationResult, []AuditEvent, error)) (MutationResult, error)
	RunInTx(context.Context, func(context.Context, pgx.Tx) error) error
	SearchAccounts(context.Context, string) ([]Account, error)
	GetAccountTx(context.Context, pgx.Tx, string) (Account, error)
	PaidSummaryTx(context.Context, pgx.Tx, string) (PaidSummary, error)
}

type Service struct {
	Store  Store
	Grants GrantsPort
	Keys   []domain.ConfiguredKey
	Clock  func() time.Time
	NewID  func() (string, error)
}

func (s *Service) Enabled() bool { return s != nil && s.Store != nil && len(s.Keys) > 0 }

func (s *Service) now() time.Time {
	if s != nil && s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// Login validates the presented key against the configured hashes. The store
// enforces limits before invoking Verify, so even a malformed key consumes
// the same failure accounting.
func (s *Service) Login(ctx context.Context, rawKey, ipHash, requestID string) (LoginResult, error) {
	if !s.Enabled() {
		return LoginResult{}, domain.ErrDenied
	}
	if !domain.ValidHash(ipHash) {
		return LoginResult{}, domain.ErrInvalid
	}
	presented := domain.HashKey(rawKey)
	return s.Store.Login(ctx, LoginInput{
		PresentedHash: presented,
		IPHash:        ipHash,
		RequestID:     requestID,
		Now:           s.now(),
		Verify: func() (string, bool) {
			key, ok := domain.MatchKey(presented, s.Keys)
			return key.Fingerprint, ok
		},
	})
}

// Authenticate resolves one cookie token and rejects sessions whose key hash
// was removed by rotation.
func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	if !s.Enabled() || !domain.ValidToken(token) {
		return Session{}, domain.ErrUnauthorized
	}
	session, err := s.Store.Authenticate(ctx, token, s.now())
	if err != nil {
		return Session{}, err
	}
	if !domain.FingerprintAllowed(session.KeyFingerprint, s.Keys) {
		return Session{}, domain.ErrUnauthorized
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, session Session, m Mutation) (MutationResult, error) {
	if err := validateMutation(m); err != nil {
		return MutationResult{}, err
	}
	if session.ID == "" || session.ID != m.SessionID {
		return MutationResult{}, domain.ErrUnauthorized
	}
	m.Now = s.now()
	return s.Store.Logout(ctx, session, m)
}

func (s *Service) RevokeAll(ctx context.Context, session Session, m Mutation) (MutationResult, error) {
	if err := validateMutation(m); err != nil {
		return MutationResult{}, err
	}
	if session.ID == "" || session.ID != m.SessionID {
		return MutationResult{}, domain.ErrUnauthorized
	}
	m.Now = s.now()
	return s.Store.RevokeAll(ctx, session, m)
}

func validateMutation(m Mutation) error {
	if !domain.ValidReason(m.Reason) || !domain.ValidUUID(m.Key) || !domain.ValidHash(m.RequestHash) {
		return domain.ErrInvalid
	}
	if m.Action == "" || m.TargetType == "" || m.SessionID == "" {
		return domain.ErrInvalid
	}
	return nil
}
