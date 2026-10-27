package application

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	entdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/entitlement/domain"
	"github.com/jackc/pgx/v5"
)

type GrantView struct {
	ID            string     `json:"id"`
	AccountID     string     `json:"account_id"`
	Tier          string     `json:"tier"`
	PolicyVersion int        `json:"policy_version"`
	State         string     `json:"state"`
	StartAt       time.Time  `json:"start_at"`
	EndAt         *time.Time `json:"end_at"`
	Reason        string     `json:"reason"`
	CreatedAt     time.Time  `json:"created_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokeReason  string     `json:"revoke_reason,omitempty"`
}

type CreateGrantInput struct {
	AccountID    string
	Tier         string
	EndAt        *time.Time
	Reason       string
	Replace      bool
	Key          string
	RequestHash  string
	RequestID    string
	ClientIPHash string
}

type CreateGrantResult struct {
	Grant    GrantView  `json:"grant"`
	Replaced *GrantView `json:"replaced"`
	Replayed bool       `json:"-"`
}

func View(grant entdomain.Grant) GrantView {
	return GrantView{
		ID: grant.ID, AccountID: grant.AccountID, Tier: string(grant.Tier),
		PolicyVersion: grant.PolicyVersion, State: string(grant.State),
		StartAt: grant.StartAt, EndAt: grant.EndAt, Reason: grant.Reason,
		CreatedAt: grant.CreatedAt, RevokedAt: grant.RevokedAt,
		RevokeReason: grant.RevokeReason,
	}
}

func viewPtr(grant *entdomain.Grant) *GrantView {
	if grant == nil {
		return nil
	}
	view := View(*grant)
	return &view
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	return raw
}

// CreateGrant validates, optionally replaces and inserts one admin grant. The
// account lock, old revoke, new insert and both audit rows share one
// transaction (ENT-01/03/05).
func (s *Service) CreateGrant(ctx context.Context, session Session, in CreateGrantInput) (CreateGrantResult, error) {
	if s == nil || s.Store == nil || s.Grants == nil || s.NewID == nil {
		return CreateGrantResult{}, domain.ErrUnavailable
	}
	if !domain.ValidUUID(in.AccountID) || !domain.ValidReason(in.Reason) || !domain.ValidUUID(in.Key) || !domain.ValidHash(in.RequestHash) {
		return CreateGrantResult{}, domain.ErrInvalid
	}
	tier, ok := entdomain.ParseTier(in.Tier)
	if !ok {
		return CreateGrantResult{}, domain.ErrInvalid
	}
	now := s.now()
	if !entdomain.ValidEndAt(now, in.EndAt) {
		return CreateGrantResult{}, domain.ErrInvalid
	}
	m := Mutation{
		SessionID: session.ID, Action: "admin.grant.create", TargetType: "entitlement_grant",
		TargetID: in.AccountID, Reason: in.Reason, Key: in.Key,
		RequestHash: in.RequestHash, RequestID: in.RequestID, ClientIPHash: in.ClientIPHash, Now: now,
	}
	out, err := s.Store.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (MutationResult, []AuditEvent, error) {
		if _, err := s.Grants.LockEligibleAccount(ctx, tx, in.AccountID); err != nil {
			return MutationResult{}, nil, err
		}
		active, err := s.Grants.ActiveTx(ctx, tx, in.AccountID, now)
		if err != nil {
			return MutationResult{}, nil, err
		}
		var existing *entdomain.Grant
		for i := range active {
			if active[i].Tier == tier {
				existing = &active[i]
				break
			}
		}
		var events []AuditEvent
		var replaced *entdomain.Grant
		if existing != nil {
			if !in.Replace {
				return MutationResult{}, nil, entdomain.ErrExists
			}
			revoked, err := s.Grants.RevokeTx(ctx, tx, existing.ID, now, in.Reason)
			if err != nil {
				return MutationResult{}, nil, err
			}
			replaced = &revoked
			events = append(events, AuditEvent{
				Action: "admin.grant.revoke", TargetType: "entitlement_grant", TargetID: revoked.ID,
				Reason: in.Reason, Before: mustJSON(View(*existing)), After: mustJSON(View(revoked)),
			})
		}
		id, err := s.NewID()
		if err != nil || !domain.ValidUUID(id) {
			return MutationResult{}, nil, domain.ErrUnavailable
		}
		grant, err := s.Grants.CreateTx(ctx, tx, entdomain.NewGrant{
			ID: id, AccountID: in.AccountID, Tier: tier, EndAt: in.EndAt,
			Reason: in.Reason, CreatedSessionID: session.ID, Now: now,
		})
		if err != nil {
			return MutationResult{}, nil, err
		}
		events = append(events, AuditEvent{
			Action: "admin.grant.create", TargetType: "entitlement_grant", TargetID: grant.ID,
			Reason: in.Reason, After: mustJSON(View(grant)),
		})
		body, err := json.Marshal(CreateGrantResult{Grant: View(grant), Replaced: viewPtr(replaced)})
		if err != nil {
			return MutationResult{}, nil, domain.ErrUnavailable
		}
		return MutationResult{Status: 201, Body: body}, events, nil
	})
	if err != nil {
		return CreateGrantResult{}, err
	}
	var result CreateGrantResult
	if json.Unmarshal(out.Body, &result) != nil {
		return CreateGrantResult{}, domain.ErrUnavailable
	}
	result.Replayed = out.Replayed
	return result, nil
}

type RevokeGrantInput struct {
	GrantID      string
	Reason       string
	Key          string
	RequestHash  string
	RequestID    string
	ClientIPHash string
}

func (s *Service) RevokeGrant(ctx context.Context, session Session, in RevokeGrantInput) (GrantView, bool, error) {
	if s == nil || s.Store == nil || s.Grants == nil {
		return GrantView{}, false, domain.ErrUnavailable
	}
	if !domain.ValidUUID(in.GrantID) || !domain.ValidReason(in.Reason) || !domain.ValidUUID(in.Key) || !domain.ValidHash(in.RequestHash) {
		return GrantView{}, false, domain.ErrInvalid
	}
	now := s.now()
	m := Mutation{
		SessionID: session.ID, Action: "admin.grant.revoke", TargetType: "entitlement_grant",
		TargetID: in.GrantID, Reason: in.Reason, Key: in.Key,
		RequestHash: in.RequestHash, RequestID: in.RequestID, ClientIPHash: in.ClientIPHash, Now: now,
	}
	out, err := s.Store.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (MutationResult, []AuditEvent, error) {
		before, err := s.Grants.GetTx(ctx, tx, in.GrantID)
		if err != nil {
			return MutationResult{}, nil, err
		}
		grant, err := s.Grants.RevokeTx(ctx, tx, in.GrantID, now, in.Reason)
		if err != nil {
			return MutationResult{}, nil, err
		}
		body, err := json.Marshal(View(grant))
		if err != nil {
			return MutationResult{}, nil, domain.ErrUnavailable
		}
		return MutationResult{Status: 200, Body: body}, []AuditEvent{{
			Action: "admin.grant.revoke", TargetType: "entitlement_grant", TargetID: grant.ID,
			Reason: in.Reason, Before: mustJSON(View(before)), After: mustJSON(View(grant)),
		}}, nil
	})
	if err != nil {
		return GrantView{}, false, err
	}
	var view GrantView
	if json.Unmarshal(out.Body, &view) != nil {
		return GrantView{}, false, domain.ErrUnavailable
	}
	return view, out.Replayed, nil
}

func (s *Service) ListGrants(ctx context.Context, filter entdomain.GrantFilter) ([]entdomain.Grant, error) {
	if s == nil || s.Store == nil || s.Grants == nil {
		return nil, domain.ErrUnavailable
	}
	var grants []entdomain.Grant
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		grants, err = s.Grants.ListTx(ctx, tx, filter)
		return err
	})
	return grants, err
}

func (s *Service) SearchAccounts(ctx context.Context, query string) ([]Account, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrUnavailable
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 3 || len(query) > 254 {
		return nil, domain.ErrInvalid
	}
	return s.Store.SearchAccounts(ctx, query)
}

func (s *Service) AccountDetail(ctx context.Context, accountID string) (AccountDetail, error) {
	if s == nil || s.Store == nil || !domain.ValidUUID(accountID) {
		return AccountDetail{}, domain.ErrInvalid
	}
	var detail AccountDetail
	err := s.Store.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		account, err := s.Store.GetAccountTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		paid, err := s.Store.PaidSummaryTx(ctx, tx, accountID)
		if err != nil {
			return err
		}
		var grants []entdomain.Grant
		if s.Grants != nil {
			grants, err = s.Grants.ListByAccountTx(ctx, tx, accountID)
			if err != nil {
				return err
			}
		}
		detail = AccountDetail{Account: account, Grants: grants, Paid: paid}
		return nil
	})
	return detail, err
}
