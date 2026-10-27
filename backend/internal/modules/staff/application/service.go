// Package application orchestrates staff catalog reads, assignments and
// revocations. Roles grant no permission by themselves.
package application

import (
	"context"
	"encoding/json"
	"time"

	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/staff/domain"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	LockEligibleAccount(context.Context, pgx.Tx, string) error
	GetActiveAssignment(context.Context, pgx.Tx, string, string) (domain.Assignment, bool, error)
	Insert(context.Context, pgx.Tx, domain.Assignment) (domain.Assignment, error)
	Get(context.Context, pgx.Tx, string) (domain.Assignment, error)
	Revoke(context.Context, pgx.Tx, string, time.Time, string) (domain.Assignment, error)
	Active(context.Context, pgx.Tx, string, time.Time) ([]domain.Assignment, error)
	ActiveLabels(context.Context, pgx.Tx, string, time.Time) ([]string, error)
}

type MutationRunner interface {
	Mutate(context.Context, adminapp.Mutation, func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error)
}

type Service struct {
	Store  Store
	Runner MutationRunner
	Clock  func() time.Time
	NewID  func() (string, error)
}

func (s *Service) now() time.Time {
	if s != nil && s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) Catalog() []domain.RoleInfo { return domain.Catalog() }

type AssignInput struct {
	AccountID string
	Role      string
	EndAt     *time.Time
}

func (s *Service) Assign(ctx context.Context, m adminapp.Mutation, in AssignInput) (domain.Assignment, error) {
	if s == nil || s.Store == nil || s.Runner == nil || s.NewID == nil {
		return domain.Assignment{}, domain.ErrUnavailable
	}
	if !validUUID(in.AccountID) || !domain.ValidRole(in.Role) || !domain.ValidEndAt(s.now(), in.EndAt) {
		return domain.Assignment{}, domain.ErrInvalid
	}
	now := s.now()
	var out domain.Assignment
	result, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		if err := s.Store.LockEligibleAccount(ctx, tx, in.AccountID); err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		if _, found, err := s.Store.GetActiveAssignment(ctx, tx, in.AccountID, in.Role); err != nil {
			return adminapp.MutationResult{}, nil, err
		} else if found {
			return adminapp.MutationResult{}, nil, domain.ErrConflict
		}
		id, err := s.NewID()
		if err != nil || !validUUID(id) {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		assignment, err := s.Store.Insert(ctx, tx, domain.Assignment{
			ID: id, AccountID: in.AccountID, Role: in.Role, StartAt: now,
			EndAt: in.EndAt, Reason: m.Reason, CreatedSessionID: m.SessionID,
		})
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		body, err := json.Marshal(assignment)
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		return adminapp.MutationResult{Status: 201, Body: body}, []adminapp.AuditEvent{{
			Action: "admin.staff.assign", TargetType: "staff_assignment", TargetID: assignment.ID,
			Reason: m.Reason, After: body,
		}}, nil
	})
	if err != nil {
		return domain.Assignment{}, err
	}
	if len(result.Body) > 0 {
		if json.Unmarshal(result.Body, &out) != nil {
			return domain.Assignment{}, domain.ErrUnavailable
		}
	}
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, m adminapp.Mutation, assignmentID string) (domain.Assignment, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return domain.Assignment{}, domain.ErrUnavailable
	}
	if !validUUID(assignmentID) {
		return domain.Assignment{}, domain.ErrInvalid
	}
	now := s.now()
	var out domain.Assignment
	result, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		before, err := s.Store.Get(ctx, tx, assignmentID)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		assignment, err := s.Store.Revoke(ctx, tx, assignmentID, now, m.Reason)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		body, err := json.Marshal(assignment)
		if err != nil {
			return adminapp.MutationResult{}, nil, domain.ErrUnavailable
		}
		beforeBody, _ := json.Marshal(before)
		return adminapp.MutationResult{Status: 200, Body: body}, []adminapp.AuditEvent{{
			Action: "admin.staff.revoke", TargetType: "staff_assignment", TargetID: assignment.ID,
			Reason: m.Reason, Before: beforeBody, After: body,
		}}, nil
	})
	if err != nil {
		return domain.Assignment{}, err
	}
	if len(result.Body) > 0 {
		if json.Unmarshal(result.Body, &out) != nil {
			return domain.Assignment{}, domain.ErrUnavailable
		}
	}
	return out, nil
}

func validUUID(raw string) bool {
	if len(raw) != 36 || raw[8] != '-' || raw[13] != '-' || raw[18] != '-' || raw[23] != '-' {
		return false
	}
	for i, c := range raw {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
