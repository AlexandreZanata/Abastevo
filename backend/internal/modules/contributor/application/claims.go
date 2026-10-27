package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/contributor/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/jackc/pgx/v5"
)

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

func (s *Service) CreateClaim(ctx context.Context, accountID, login, operationID string) (domain.Claim, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) || !domain.ValidUUID(operationID) {
		return domain.Claim{}, domain.ErrInvalid
	}
	login = domain.NormalizeGitHubLogin(login)
	if !domain.ValidGitHubLogin(login) {
		return domain.Claim{}, domain.ErrInvalid
	}
	now := s.now()
	var out domain.Claim
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.Store.ExpireClaims(ctx, tx, accountID, now); err != nil {
			return domain.ErrUnavailable
		}
		if existing, found, err := s.Store.GetClaimByOperation(ctx, tx, accountID, operationID); err != nil {
			return err
		} else if found {
			if existing.GitHubLogin != login {
				return domain.ErrConflict
			}
			out = existing
			return nil
		}
		count, err := s.Store.CountClaimsSince(ctx, tx, accountID, now.Add(-24*time.Hour))
		if err != nil {
			return domain.ErrUnavailable
		}
		if count >= domain.MaxClaimsPerDay {
			return domain.ErrQuota
		}
		if _, open, err := s.Store.GetOpenClaim(ctx, tx, accountID, now); err != nil {
			return err
		} else if open {
			return domain.ErrConflict
		}
		id, err := s.NewID()
		if err != nil || !domain.ValidUUID(id) {
			return domain.ErrUnavailable
		}
		code, err := domain.GenerateClaimCode(nil)
		if err != nil {
			return domain.ErrUnavailable
		}
		out, err = s.Store.InsertClaim(ctx, tx, domain.Claim{
			ID: id, AccountID: accountID, OperationID: operationID,
			GitHubLogin: login, Code: code, State: domain.ClaimPending,
			ProofStatus: domain.ProofUnverified, CreatedAt: now, ExpiresAt: now.Add(domain.ClaimLifetime),
		})
		return err
	})
	return out, err
}

func (s *Service) ListOwnClaims(ctx context.Context, accountID string) ([]domain.Claim, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) {
		return nil, domain.ErrInvalid
	}
	now := s.now()
	var out []domain.Claim
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.Store.ExpireClaims(ctx, tx, accountID, now); err != nil {
			return domain.ErrUnavailable
		}
		var err error
		out, err = s.Store.ListOwnClaims(ctx, tx, accountID)
		return err
	})
	return out, err
}

func (s *Service) SetClaimProof(ctx context.Context, accountID, claimID, gistID string) (domain.Claim, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) || !domain.ValidUUID(claimID) || strings.TrimSpace(gistID) == "" || len(gistID) > 128 {
		return domain.Claim{}, domain.ErrInvalid
	}
	var current domain.Claim
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		claim, err := s.Store.LockClaim(ctx, tx, claimID)
		if err != nil {
			return err
		}
		if claim.AccountID != accountID {
			return domain.ErrNotFound
		}
		if !domain.ClaimOpen(claim, s.now()) {
			return domain.ErrClaimState
		}
		current = claim
		return nil
	})
	if err != nil {
		return domain.Claim{}, err
	}
	status := domain.ProofUnverified
	if s.GitHub != nil {
		if gist, err := s.GitHub.Gist(ctx, gistID); err == nil {
			if gist.Owner == current.GitHubLogin {
				for _, content := range gist.Files {
					if strings.Contains(content, current.Code) {
						status = domain.ProofVerified
						break
					}
				}
			}
		}
	}
	var out domain.Claim
	err = s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = s.Store.SetClaimProof(ctx, tx, claimID, accountID, gistID, status, s.now())
		return err
	})
	return out, err
}

func (s *Service) WithdrawClaim(ctx context.Context, accountID, claimID string) (domain.Claim, error) {
	if !s.enabled() || !domain.ValidUUID(accountID) || !domain.ValidUUID(claimID) {
		return domain.Claim{}, domain.ErrInvalid
	}
	var out domain.Claim
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		claim, err := s.Store.LockClaim(ctx, tx, claimID)
		if err != nil {
			return err
		}
		if claim.AccountID != accountID {
			return domain.ErrNotFound
		}
		out, err = s.Store.WithdrawClaim(ctx, tx, claimID, accountID, s.now())
		return err
	})
	return out, err
}

func (s *Service) AdminListClaims(ctx context.Context, filter domain.ClaimFilter) ([]domain.Claim, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrUnavailable
	}
	var out []domain.Claim
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.Store.ExpireAllClaims(ctx, tx, s.now()); err != nil {
			return domain.ErrUnavailable
		}
		var err error
		out, err = s.Store.ListAdminClaims(ctx, tx, filter)
		return err
	})
	return out, err
}

type ApproveResult struct {
	Claim domain.Claim        `json:"claim"`
	Grant *adminapp.GrantView `json:"grant"`
	Login string              `json:"login"`
}

// ApproveClaim creates the CONTRIBUTOR grant (when absent) and the GitHub link
// in the same transaction as the claim decision and audit rows.
func (s *Service) ApproveClaim(ctx context.Context, m adminapp.Mutation, claimID string, override bool) (ApproveResult, error) {
	if s == nil || s.Store == nil || s.Runner == nil || s.Grants == nil || s.NewID == nil {
		return ApproveResult{}, domain.ErrUnavailable
	}
	if !domain.ValidUUID(claimID) {
		return ApproveResult{}, domain.ErrInvalid
	}
	now := s.now()
	snap, snapErr := s.snapshot(ctx, false)
	known := map[string]bool{}
	if snapErr == nil {
		for _, login := range snap.Logins {
			known[login] = true
		}
	}
	var result ApproveResult
	out, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		claim, err := s.Store.LockClaim(ctx, tx, claimID)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		if claim.State != domain.ClaimPending || !domain.ClaimOpen(claim, now) {
			return adminapp.MutationResult{}, nil, domain.ErrClaimState
		}
		inSnapshot := known[claim.GitHubLogin]
		if (!inSnapshot || claim.ProofStatus != domain.ProofVerified) && !override {
			if snapErr != nil {
				return adminapp.MutationResult{}, nil, snapErr
			}
			return adminapp.MutationResult{}, nil, domain.ErrOverrideRequired
		}
		if _, err := s.Grants.LockEligibleAccount(ctx, tx, claim.AccountID); err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		decided, err := s.Store.DecideClaim(ctx, tx, claimID, domain.ClaimApproved, m.Reason, override, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		events := []adminapp.AuditEvent{{
			Action: "contributor.claim.approve", TargetType: "contributor_claim", TargetID: claimID,
			Reason: m.Reason, Before: mustJSON(claim), After: mustJSON(decided),
		}}
		var grant *entdomain.Grant
		active, err := s.Grants.ActiveTx(ctx, tx, claim.AccountID, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		for i := range active {
			if active[i].Tier == entdomain.TierContributor {
				grant = &active[i]
				break
			}
		}
		if grant == nil {
			id, err := s.NewID()
			if err != nil || !domain.ValidUUID(id) {
				return adminapp.MutationResult{}, nil, domain.ErrUnavailable
			}
			created, err := s.Grants.CreateTx(ctx, tx, entdomain.NewGrant{
				ID: id, AccountID: claim.AccountID, Tier: entdomain.TierContributor,
				Reason: m.Reason, CreatedSessionID: m.SessionID, Now: now,
			})
			if err != nil {
				return adminapp.MutationResult{}, nil, err
			}
			grant = &created
			events = append(events, adminapp.AuditEvent{
				Action: "admin.grant.create", TargetType: "entitlement_grant", TargetID: created.ID,
				Reason: m.Reason, After: mustJSON(created),
			})
		}
		login, err := s.Store.InsertGitHubLink(ctx, tx, claim.AccountID, claim.GitHubLogin, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		events = append(events, adminapp.AuditEvent{
			Action: "contributor.github.link", TargetType: "account", TargetID: claim.AccountID,
			Reason: m.Reason, After: mustJSON(map[string]string{"github_login": login}),
		})
		grantView := adminapp.View(*grant)
		result = ApproveResult{Claim: decided, Grant: &grantView, Login: login}
		body, err := json.Marshal(result)
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		return adminapp.MutationResult{Status: 200, Body: body}, events, nil
	})
	if err != nil {
		return ApproveResult{}, err
	}
	if len(out.Body) > 0 {
		if json.Unmarshal(out.Body, &result) != nil {
			return ApproveResult{}, domain.ErrUnavailable
		}
	}
	return result, nil
}

func (s *Service) RejectClaim(ctx context.Context, m adminapp.Mutation, claimID string) (domain.Claim, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return domain.Claim{}, domain.ErrUnavailable
	}
	if !domain.ValidUUID(claimID) {
		return domain.Claim{}, domain.ErrInvalid
	}
	now := s.now()
	var decided domain.Claim
	out, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		claim, err := s.Store.LockClaim(ctx, tx, claimID)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		if claim.State != domain.ClaimPending || !domain.ClaimOpen(claim, now) {
			return adminapp.MutationResult{}, nil, domain.ErrClaimState
		}
		decided, err = s.Store.DecideClaim(ctx, tx, claimID, domain.ClaimRejected, m.Reason, false, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		body, err := json.Marshal(decided)
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		return adminapp.MutationResult{Status: 200, Body: body}, []adminapp.AuditEvent{{
			Action: "contributor.claim.reject", TargetType: "contributor_claim", TargetID: claimID,
			Reason: m.Reason, Before: mustJSON(claim), After: mustJSON(decided),
		}}, nil
	})
	if err != nil {
		return domain.Claim{}, err
	}
	if len(out.Body) > 0 {
		if json.Unmarshal(out.Body, &decided) != nil {
			return domain.Claim{}, domain.ErrUnavailable
		}
	}
	return decided, nil
}

func (s *Service) LinkGitHub(ctx context.Context, m adminapp.Mutation, accountID, login string, override bool) (string, error) {
	if s == nil || s.Store == nil || s.Runner == nil || s.Grants == nil {
		return "", domain.ErrUnavailable
	}
	login = domain.NormalizeGitHubLogin(login)
	if !domain.ValidUUID(accountID) || !domain.ValidGitHubLogin(login) {
		return "", domain.ErrInvalid
	}
	now := s.now()
	snap, snapErr := s.snapshot(ctx, false)
	inSnapshot := false
	if snapErr == nil {
		for _, candidate := range snap.Logins {
			if candidate == login {
				inSnapshot = true
				break
			}
		}
	}
	var linked string
	var mutationErr error
	var out adminapp.MutationResult
	out, mutationErr = s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		if _, err := s.Grants.LockEligibleAccount(ctx, tx, accountID); err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		if !inSnapshot && !override {
			if snapErr != nil {
				return adminapp.MutationResult{}, nil, snapErr
			}
			return adminapp.MutationResult{}, nil, domain.ErrOverrideRequired
		}
		var err error
		linked, err = s.Store.InsertGitHubLink(ctx, tx, accountID, login, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		body, err := json.Marshal(map[string]string{"account_id": accountID, "github_login": linked})
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		return adminapp.MutationResult{Status: 201, Body: body}, []adminapp.AuditEvent{{
			Action: "contributor.github.link", TargetType: "account", TargetID: accountID,
			Reason: m.Reason, After: mustJSON(map[string]string{"github_login": linked}),
		}}, nil
	})
	if mutationErr != nil {
		return "", mutationErr
	}
	if len(out.Body) > 0 {
		var payload struct {
			GitHubLogin string `json:"github_login"`
		}
		if json.Unmarshal(out.Body, &payload) != nil {
			return "", domain.ErrUnavailable
		}
		linked = payload.GitHubLogin
	}
	return linked, nil
}

func (s *Service) HideProfile(ctx context.Context, m adminapp.Mutation, accountID string, hidden bool) (domain.Profile, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return domain.Profile{}, domain.ErrUnavailable
	}
	if !domain.ValidUUID(accountID) {
		return domain.Profile{}, domain.ErrInvalid
	}
	reason := ""
	if hidden {
		reason = m.Reason
	}
	var out domain.Profile
	result, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		var err error
		out, err = s.Store.SetProfileHidden(ctx, tx, accountID, hidden, reason)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		out.Links = []domain.Link{}
		body, err := json.Marshal(out)
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		action := "contributor.profile.unhide"
		if hidden {
			action = "contributor.profile.hide"
		}
		return adminapp.MutationResult{Status: 200, Body: body}, []adminapp.AuditEvent{{
			Action: action, TargetType: "contributor_profile", TargetID: accountID,
			Reason: m.Reason, After: mustJSON(out),
		}}, nil
	})
	if err != nil {
		return domain.Profile{}, err
	}
	if len(result.Body) > 0 {
		if json.Unmarshal(result.Body, &out) != nil {
			return domain.Profile{}, domain.ErrUnavailable
		}
	}
	return out, nil
}

func (s *Service) snapshot(ctx context.Context, refresh bool) (domain.Snapshot, error) {
	if s == nil || s.Store == nil || s.GitHub == nil || s.Repo == "" {
		return domain.Snapshot{}, domain.ErrUnavailable
	}
	now := s.now()
	cached, found, err := s.Store.GetSnapshot(ctx, s.Repo)
	if err != nil {
		return domain.Snapshot{}, err
	}
	freshEnough := found && now.Sub(cached.FetchedAt) < time.Hour
	refreshAllowed := !found || now.Sub(cached.FetchedAt) >= time.Minute
	if freshEnough && (!refresh || !refreshAllowed) {
		return cached, nil
	}
	logins, etag, fetchErr := s.GitHub.Contributors(ctx)
	if fetchErr != nil {
		if found {
			return cached, nil
		}
		return domain.Snapshot{}, domain.ErrUnavailable
	}
	snap := domain.Snapshot{Repo: s.Repo, Logins: logins, ETag: etag, FetchedAt: now}
	if err := s.Store.SaveSnapshot(ctx, snap); err != nil {
		return domain.Snapshot{}, err
	}
	return snap, nil
}

type GitHubContributorState struct {
	Login            string
	LinkedAccountID  string
	ClaimID          string
	ClaimState       string
	ClaimProofStatus string
}

type GitHubSnapshotView struct {
	Repo      string
	FetchedAt time.Time
	Stale     bool
	Logins    []GitHubContributorState
}

// AdminGitHubContributors returns the cached snapshot plus link/claim state.
// GitHub failures fall back to the last snapshot (stale=true).
func (s *Service) AdminGitHubContributors(ctx context.Context, refresh bool) (GitHubSnapshotView, error) {
	if s == nil || s.Store == nil || s.GitHub == nil {
		return GitHubSnapshotView{}, domain.ErrUnavailable
	}
	cached, found, err := s.Store.GetSnapshot(ctx, s.Repo)
	if err != nil {
		return GitHubSnapshotView{}, err
	}
	snap, stale := cached, false
	if !found || s.now().Sub(cached.FetchedAt) >= time.Hour || (refresh && s.now().Sub(cached.FetchedAt) >= time.Minute) {
		logins, etag, fetchErr := s.GitHub.Contributors(ctx)
		if fetchErr != nil {
			if !found {
				return GitHubSnapshotView{}, domain.ErrUnavailable
			}
			stale = true
		} else {
			snap = domain.Snapshot{Repo: s.Repo, Logins: logins, ETag: etag, FetchedAt: s.now()}
			if err := s.Store.SaveSnapshot(ctx, snap); err != nil {
				return GitHubSnapshotView{}, err
			}
		}
	}
	view := GitHubSnapshotView{Repo: snap.Repo, FetchedAt: snap.FetchedAt, Stale: stale}
	err = s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		links, err := s.Store.ListActiveGitHubLinks(ctx, tx)
		if err != nil {
			return err
		}
		claims, err := s.Store.ListOpenClaims(ctx, tx, s.now())
		if err != nil {
			return err
		}
		byLogin := map[string]GitHubContributorState{}
		for _, link := range links {
			byLogin[link.Login] = GitHubContributorState{Login: link.Login, LinkedAccountID: link.AccountID}
		}
		for _, claim := range claims {
			state := byLogin[claim.GitHubLogin]
			state.Login = claim.GitHubLogin
			state.ClaimID, state.ClaimState, state.ClaimProofStatus = claim.ID, string(claim.State), string(claim.ProofStatus)
			byLogin[claim.GitHubLogin] = state
		}
		for _, login := range snap.Logins {
			state := byLogin[login]
			state.Login = login
			view.Logins = append(view.Logins, state)
		}
		return nil
	})
	return view, err
}
