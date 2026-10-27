package adapters

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	qcon "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/contributor"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func uid(raw string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(raw)
	return id
}

func idstr(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	raw := hex.EncodeToString(id.Bytes[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32]
}

func stamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}

func text(raw string) pgtype.Text { return pgtype.Text{String: raw, Valid: raw != ""} }

func claim(row qcon.ContributorClaim) domain.Claim {
	out := domain.Claim{
		ID: idstr(row.ID), AccountID: idstr(row.AccountID), OperationID: idstr(row.OperationID),
		GitHubLogin: row.GithubLogin, Code: row.Code,
		State: domain.ClaimState(row.State), ProofStatus: domain.ProofStatus(row.ProofStatus),
		GistID: row.GistID.String, Override: row.Override,
		DecisionReason: row.DecisionReason.String,
		CreatedAt:      row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
	}
	if row.DecidedAt.Valid {
		decided := row.DecidedAt.Time
		out.DecidedAt = &decided
	}
	return out
}

func profile(row qcon.ContributorProfile) domain.Profile {
	return domain.Profile{
		AccountID: idstr(row.AccountID), DisplayName: row.DisplayName,
		Visibility: row.Visibility, PublicConsent: row.PublicConsent,
		ShowRolesPublicly: row.ShowRolesPublicly, HiddenByAdmin: row.HiddenByAdmin,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func unique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Store) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if s == nil || s.Pool == nil {
		return domain.ErrUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.ErrUnavailable
	}
	defer rollback(tx)
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

func (s *Store) ExpireClaims(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) error {
	return qcon.New(tx).ExpireClaims(ctx, qcon.ExpireClaimsParams{AccountID: uid(accountID), Now: stamp(now)})
}

func (s *Store) ExpireAllClaims(ctx context.Context, tx pgx.Tx, now time.Time) error {
	return qcon.New(tx).ExpireAllClaims(ctx, stamp(now))
}

func (s *Store) CountClaimsSince(ctx context.Context, tx pgx.Tx, accountID string, since time.Time) (int, error) {
	n, err := qcon.New(tx).CountClaimsSince(ctx, qcon.CountClaimsSinceParams{AccountID: uid(accountID), Since: stamp(since)})
	return int(n), err
}

func (s *Store) GetOpenClaim(ctx context.Context, tx pgx.Tx, accountID string, now time.Time) (domain.Claim, bool, error) {
	row, err := qcon.New(tx).GetOpenClaim(ctx, qcon.GetOpenClaimParams{AccountID: uid(accountID), Now: stamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, false, nil
	}
	if err != nil {
		return domain.Claim{}, false, domain.ErrUnavailable
	}
	return claim(row), true, nil
}

func (s *Store) GetClaimByOperation(ctx context.Context, tx pgx.Tx, accountID, operationID string) (domain.Claim, bool, error) {
	row, err := qcon.New(tx).GetClaimByOperation(ctx, qcon.GetClaimByOperationParams{AccountID: uid(accountID), OperationID: uid(operationID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, false, nil
	}
	if err != nil {
		return domain.Claim{}, false, domain.ErrUnavailable
	}
	return claim(row), true, nil
}

func (s *Store) InsertClaim(ctx context.Context, tx pgx.Tx, in domain.Claim) (domain.Claim, error) {
	row, err := qcon.New(tx).InsertClaim(ctx, qcon.InsertClaimParams{
		ID: uid(in.ID), AccountID: uid(in.AccountID), OperationID: uid(in.OperationID),
		GithubLogin: in.GitHubLogin, Code: in.Code,
		CreatedAt: stamp(in.CreatedAt), ExpiresAt: stamp(in.ExpiresAt),
	})
	if unique(err) {
		return domain.Claim{}, domain.ErrConflict
	}
	if err != nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	return claim(row), nil
}

func (s *Store) ListOwnClaims(ctx context.Context, tx pgx.Tx, accountID string) ([]domain.Claim, error) {
	rows, err := qcon.New(tx).ListOwnClaims(ctx, uid(accountID))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Claim, 0, len(rows))
	for _, row := range rows {
		out = append(out, claim(row))
	}
	return out, nil
}

func (s *Store) LockClaim(ctx context.Context, tx pgx.Tx, id string) (domain.Claim, error) {
	row, err := qcon.New(tx).LockClaim(ctx, uid(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	return claim(row), nil
}

func (s *Store) SetClaimProof(ctx context.Context, tx pgx.Tx, id, accountID, gistID string, status domain.ProofStatus, now time.Time) (domain.Claim, error) {
	row, err := qcon.New(tx).SetClaimProof(ctx, qcon.SetClaimProofParams{
		ProofStatus: string(status), GistID: text(gistID), ID: uid(id), AccountID: uid(accountID), Now: stamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, domain.ErrClaimState
	}
	if err != nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	return claim(row), nil
}

func (s *Store) WithdrawClaim(ctx context.Context, tx pgx.Tx, id, accountID string, now time.Time) (domain.Claim, error) {
	row, err := qcon.New(tx).WithdrawClaim(ctx, qcon.WithdrawClaimParams{ID: uid(id), AccountID: uid(accountID), Now: stamp(now)})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, domain.ErrClaimState
	}
	if err != nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	return claim(row), nil
}

func (s *Store) DecideClaim(ctx context.Context, tx pgx.Tx, id string, state domain.ClaimState, reason string, override bool, now time.Time) (domain.Claim, error) {
	row, err := qcon.New(tx).DecideClaim(ctx, qcon.DecideClaimParams{
		State: string(state), Reason: text(reason), Override: override, ID: uid(id), Now: stamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Claim{}, domain.ErrClaimState
	}
	if err != nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	return claim(row), nil
}

func (s *Store) ListAdminClaims(ctx context.Context, tx pgx.Tx, filter domain.ClaimFilter) ([]domain.Claim, error) {
	limit := filter.Limit
	if limit < 1 || limit > 101 {
		limit = 50
	}
	state := pgtype.Text{}
	if filter.State != "" {
		state = pgtype.Text{String: string(filter.State), Valid: true}
	}
	rows, err := qcon.New(tx).ListAdminClaims(ctx, qcon.ListAdminClaimsParams{
		State: state, BeforeCreated: stamp(filter.BeforeCreated), BeforeID: uid(filter.BeforeID), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Claim, 0, len(rows))
	for _, row := range rows {
		out = append(out, claim(row))
	}
	return out, nil
}

func (s *Store) ListClaimsByAccount(ctx context.Context, tx pgx.Tx, accountID string) ([]domain.Claim, error) {
	rows, err := qcon.New(tx).ListClaimsByAccount(ctx, uid(accountID))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Claim, 0, len(rows))
	for _, row := range rows {
		out = append(out, claim(row))
	}
	return out, nil
}

func (s *Store) UpsertProfile(ctx context.Context, tx pgx.Tx, accountID, displayName, visibility string, consent, showRoles bool) (domain.Profile, error) {
	row, err := qcon.New(tx).UpsertProfile(ctx, qcon.UpsertProfileParams{
		AccountID: uid(accountID), DisplayName: displayName, Visibility: visibility,
		PublicConsent: consent, ShowRolesPublicly: showRoles,
	})
	if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	return profile(row), nil
}

func (s *Store) GetProfile(ctx context.Context, tx pgx.Tx, accountID string) (domain.Profile, bool, error) {
	row, err := qcon.New(tx).GetProfile(ctx, uid(accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, false, nil
	}
	if err != nil {
		return domain.Profile{}, false, domain.ErrUnavailable
	}
	return profile(row), true, nil
}

func (s *Store) ReplaceLinks(ctx context.Context, tx pgx.Tx, accountID string, links []domain.Link) error {
	q := qcon.New(tx)
	if err := q.DeleteProfileLinks(ctx, uid(accountID)); err != nil {
		return domain.ErrUnavailable
	}
	for _, link := range links {
		if err := q.InsertProfileLink(ctx, qcon.InsertProfileLinkParams{
			AccountID: uid(accountID), Platform: link.Platform, Handle: link.Handle,
		}); err != nil {
			return domain.ErrUnavailable
		}
	}
	return nil
}

func (s *Store) ListLinks(ctx context.Context, tx pgx.Tx, accountID string) ([]domain.Link, error) {
	rows, err := qcon.New(tx).ListProfileLinks(ctx, uid(accountID))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Link, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Link{Platform: row.Platform, Handle: row.Handle})
	}
	return out, nil
}

func (s *Store) CountProfileEditsSince(ctx context.Context, tx pgx.Tx, accountID string, since time.Time) (int, error) {
	n, err := qcon.New(tx).CountProfileEditsSince(ctx, qcon.CountProfileEditsSinceParams{AccountID: uid(accountID), Since: stamp(since)})
	return int(n), err
}

func (s *Store) InsertProfileEdit(ctx context.Context, tx pgx.Tx, accountID string, at time.Time) error {
	return qcon.New(tx).InsertProfileEdit(ctx, qcon.InsertProfileEditParams{AccountID: uid(accountID), EditedAt: stamp(at)})
}

func (s *Store) SetProfileHidden(ctx context.Context, tx pgx.Tx, accountID string, hidden bool, reason string) (domain.Profile, error) {
	row, err := qcon.New(tx).SetProfileHidden(ctx, qcon.SetProfileHiddenParams{
		Hidden: hidden, Reason: text(reason), AccountID: uid(accountID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	return profile(row), nil
}

func (s *Store) GetActiveLinkByLogin(ctx context.Context, tx pgx.Tx, login string) (string, bool, error) {
	row, err := qcon.New(tx).GetActiveLinkByLogin(ctx, login)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, domain.ErrUnavailable
	}
	return idstr(row.AccountID), true, nil
}

func (s *Store) GetActiveLinkByAccount(ctx context.Context, tx pgx.Tx, accountID string) (string, bool, error) {
	row, err := qcon.New(tx).GetActiveLinkByAccount(ctx, uid(accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, domain.ErrUnavailable
	}
	return row.GithubLogin, true, nil
}

func (s *Store) InsertGitHubLink(ctx context.Context, tx pgx.Tx, accountID, login string, at time.Time) (string, error) {
	row, err := qcon.New(tx).InsertGitHubLink(ctx, qcon.InsertGitHubLinkParams{
		AccountID: uid(accountID), GithubLogin: login, LinkedAt: stamp(at),
	})
	if unique(err) {
		return "", domain.ErrConflict
	}
	if err != nil {
		return "", domain.ErrUnavailable
	}
	return row.GithubLogin, nil
}

func (s *Store) ListActiveGitHubLinks(ctx context.Context, tx pgx.Tx) ([]domain.GitHubLink, error) {
	rows, err := qcon.New(tx).ListActiveGitHubLinks(ctx)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.GitHubLink, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.GitHubLink{AccountID: idstr(row.AccountID), Login: row.GithubLogin})
	}
	return out, nil
}

func (s *Store) ListOpenClaims(ctx context.Context, tx pgx.Tx, now time.Time) ([]domain.Claim, error) {
	rows, err := qcon.New(tx).ListOpenClaims(ctx, stamp(now))
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.Claim, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Claim{
			ID: idstr(row.ID), AccountID: idstr(row.AccountID), GitHubLogin: row.GithubLogin,
			State: domain.ClaimState(row.State), ProofStatus: domain.ProofStatus(row.ProofStatus),
			CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time,
		})
	}
	return out, nil
}

func (s *Store) GetSnapshot(ctx context.Context, repo string) (domain.Snapshot, bool, error) {
	row, err := qcon.New(s.Pool).GetSnapshot(ctx, repo)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Snapshot{}, false, nil
	}
	if err != nil {
		return domain.Snapshot{}, false, domain.ErrUnavailable
	}
	var logins []string
	if err := json.Unmarshal(row.Logins, &logins); err != nil {
		return domain.Snapshot{}, false, domain.ErrUnavailable
	}
	return domain.Snapshot{Repo: row.Repo, Logins: logins, ETag: row.Etag.String, FetchedAt: row.FetchedAt.Time}, true, nil
}

func (s *Store) SaveSnapshot(ctx context.Context, snap domain.Snapshot) error {
	raw, err := json.Marshal(snap.Logins)
	if err != nil {
		return domain.ErrUnavailable
	}
	_, err = qcon.New(s.Pool).UpsertSnapshot(ctx, qcon.UpsertSnapshotParams{
		Repo: snap.Repo, Logins: raw, Etag: text(snap.ETag), FetchedAt: stamp(snap.FetchedAt),
	})
	if err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (s *Store) PublicContributors(ctx context.Context, tx pgx.Tx, now time.Time, afterSince time.Time, afterID string, limit int) ([]domain.PublicContributor, error) {
	rows, err := qcon.New(tx).PublicContributors(ctx, qcon.PublicContributorsParams{
		Now: stamp(now), AfterSince: stamp(afterSince), AfterID: uid(afterID), RowLimit: int32(limit),
	})
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	out := make([]domain.PublicContributor, 0, len(rows))
	for _, row := range rows {
		var links []domain.Link
		if err := json.Unmarshal(row.Links, &links); err != nil {
			return nil, domain.ErrUnavailable
		}
		out = append(out, domain.PublicContributor{
			AccountID: idstr(row.AccountID), DisplayName: row.DisplayName,
			Links: domain.PublicLinks(links), GitHubLogin: row.GithubLogin.String,
			ContributorSince: row.ContributorSince.Time, Roles: []string{},
		})
	}
	return out, nil
}
