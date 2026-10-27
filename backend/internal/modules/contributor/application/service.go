// Package application orchestrates contributor claims, profiles and the
// consented public list. GitHub and persistence are ports; no HTTP or SQL lives
// here.
package application

import (
	"context"
	"time"

	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	RunInTx(context.Context, func(context.Context, pgx.Tx) error) error
	ExpireClaims(context.Context, pgx.Tx, string, time.Time) error
	ExpireAllClaims(context.Context, pgx.Tx, time.Time) error
	CountClaimsSince(context.Context, pgx.Tx, string, time.Time) (int, error)
	GetOpenClaim(context.Context, pgx.Tx, string, time.Time) (domain.Claim, bool, error)
	GetClaimByOperation(context.Context, pgx.Tx, string, string) (domain.Claim, bool, error)
	InsertClaim(context.Context, pgx.Tx, domain.Claim) (domain.Claim, error)
	ListOwnClaims(context.Context, pgx.Tx, string) ([]domain.Claim, error)
	LockClaim(context.Context, pgx.Tx, string) (domain.Claim, error)
	SetClaimProof(context.Context, pgx.Tx, string, string, string, domain.ProofStatus, time.Time) (domain.Claim, error)
	WithdrawClaim(context.Context, pgx.Tx, string, string, time.Time) (domain.Claim, error)
	DecideClaim(context.Context, pgx.Tx, string, domain.ClaimState, string, bool, time.Time) (domain.Claim, error)
	ListAdminClaims(context.Context, pgx.Tx, domain.ClaimFilter) ([]domain.Claim, error)
	ListClaimsByAccount(context.Context, pgx.Tx, string) ([]domain.Claim, error)
	UpsertProfile(context.Context, pgx.Tx, string, string, string, bool, bool) (domain.Profile, error)
	GetProfile(context.Context, pgx.Tx, string) (domain.Profile, bool, error)
	ReplaceLinks(context.Context, pgx.Tx, string, []domain.Link) error
	ListLinks(context.Context, pgx.Tx, string) ([]domain.Link, error)
	CountProfileEditsSince(context.Context, pgx.Tx, string, time.Time) (int, error)
	InsertProfileEdit(context.Context, pgx.Tx, string, time.Time) error
	SetProfileHidden(context.Context, pgx.Tx, string, bool, string) (domain.Profile, error)
	GetActiveLinkByLogin(context.Context, pgx.Tx, string) (string, bool, error)
	GetActiveLinkByAccount(context.Context, pgx.Tx, string) (string, bool, error)
	InsertGitHubLink(context.Context, pgx.Tx, string, string, time.Time) (string, error)
	ListActiveGitHubLinks(context.Context, pgx.Tx) ([]domain.GitHubLink, error)
	ListOpenClaims(context.Context, pgx.Tx, time.Time) ([]domain.Claim, error)
	GetSnapshot(context.Context, string) (domain.Snapshot, bool, error)
	SaveSnapshot(context.Context, domain.Snapshot) error
	PublicContributors(context.Context, pgx.Tx, time.Time, time.Time, string, int) ([]domain.PublicContributor, error)
}

type GitHub interface {
	Contributors(context.Context) ([]string, string, error)
	Gist(context.Context, string) (domain.Gist, error)
}

type Grants interface {
	LockEligibleAccount(context.Context, pgx.Tx, string) (entdomain.Grant, error)
	ActiveTx(context.Context, pgx.Tx, string, time.Time) ([]entdomain.Grant, error)
	SupersedeExpiredTx(context.Context, pgx.Tx, string, entdomain.Tier, time.Time) (*entdomain.Grant, error)
	CreateTx(context.Context, pgx.Tx, entdomain.NewGrant) (entdomain.Grant, error)
}

type MutationRunner interface {
	Mutate(context.Context, adminapp.Mutation, func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error)
}

type Service struct {
	Store             Store
	GitHub            GitHub
	Grants            Grants
	Runner            MutationRunner
	Repo              string
	Clock             func() time.Time
	NewID             func() (string, error)
	RequireCapability func(context.Context, pgx.Tx, string) error
}

func (s *Service) now() time.Time {
	if s != nil && s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) enabled() bool {
	return s != nil && s.Store != nil && s.NewID != nil
}
