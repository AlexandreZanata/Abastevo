// Package application owns the member inbox use cases and the producer port.
package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/jackc/pgx/v5"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Self is the server-resolved account and the membership selected by path.
type Self struct{ AccountID, MembershipID string }

type InboxQuery struct {
	UnreadOnly bool
	After      int64
	Limit      int
}

type Notice struct {
	ID, Kind, Template string
	Params             map[string]string
	Link               string
	Position           int64
	CreatedAt, ReadAt  time.Time
}

type InboxPage struct {
	Notices []Notice
	Next    int64
}

type ReadCommand struct{ OperationID, NoticeID string }
type ReadResult struct {
	Notice   Notice
	Replayed bool
}

type Preference struct {
	Kind             string
	Muted, Mandatory bool
	Revision         int64
}

// PreferenceCommand.Revision is the expected current revision; 0 means none.
type PreferenceCommand struct {
	OperationID, Kind string
	Muted             bool
	Revision          int64
}
type PreferenceResult struct {
	Preference Preference
	Replayed   bool
}

// Repository rechecks current own membership inside every transaction.
type Repository interface {
	ListInbox(context.Context, kernel.OrgScope, Self, InboxQuery) (InboxPage, error)
	MarkRead(context.Context, kernel.OrgScope, Self, ReadCommand) (ReadResult, error)
	Preferences(context.Context, kernel.OrgScope, Self) ([]Preference, error)
	SetPreference(context.Context, kernel.OrgScope, Self, PreferenceCommand) (PreferenceResult, error)
}

// Producer is called inside the producer's own audited orgevent transaction.
type Producer interface {
	RequestTx(context.Context, pgx.Tx, kernel.OrgScope, event.Event, []domain.Request) ([]domain.Registered, error)
}

type Service struct {
	Enabled    bool
	Catalog    domain.Catalog
	Repository Repository
}

func validID(s string) bool {
	id, err := kernel.ParseOrgID(s)
	return err == nil && id.String() == s
}

// ready refuses a disabled or incomplete composition and a malformed selector
// before storage; an invalid selector is indistinguishable from a foreign one.
func (s Service) ready(scope kernel.OrgScope, self Self) error {
	if !s.Enabled || s.Repository == nil {
		return domain.ErrUnavailable
	}
	if scope.Validate() != nil || !validID(self.AccountID) || !validID(self.MembershipID) {
		return domain.ErrNotFound
	}
	return nil
}

func (s Service) List(ctx context.Context, scope kernel.OrgScope, self Self, q InboxQuery) (InboxPage, error) {
	if err := s.ready(scope, self); err != nil {
		return InboxPage{}, err
	}
	if q.Limit == 0 {
		q.Limit = DefaultLimit
	}
	if q.Limit < 1 || q.Limit > MaxLimit || q.After < 0 {
		return InboxPage{}, domain.ErrInvalid
	}
	return s.Repository.ListInbox(ctx, scope, self, q)
}

func (s Service) MarkRead(ctx context.Context, scope kernel.OrgScope, self Self, c ReadCommand) (ReadResult, error) {
	if err := s.ready(scope, self); err != nil {
		return ReadResult{}, err
	}
	if !validID(c.OperationID) || !validID(c.NoticeID) {
		return ReadResult{}, domain.ErrInvalid
	}
	return s.Repository.MarkRead(ctx, scope, self, c)
}

// Preferences lists every registered kind: stored IN_APP choice or the
// unmuted default. Mandatory kinds are always reported unmuted.
func (s Service) Preferences(ctx context.Context, scope kernel.OrgScope, self Self) ([]Preference, error) {
	if err := s.ready(scope, self); err != nil {
		return nil, err
	}
	stored, err := s.Repository.Preferences(ctx, scope, self)
	if err != nil {
		return nil, err
	}
	byKind := make(map[string]Preference, len(stored))
	for _, p := range stored {
		byKind[p.Kind] = p
	}
	kinds := s.Catalog.Kinds()
	out := make([]Preference, 0, len(kinds))
	for _, k := range kinds {
		p := byKind[k.Name]
		p.Kind, p.Mandatory = k.Name, k.Mandatory
		if k.Mandatory {
			p.Muted = false
		}
		out = append(out, p)
	}
	return out, nil
}

func (s Service) SetPreference(ctx context.Context, scope kernel.OrgScope, self Self, c PreferenceCommand) (PreferenceResult, error) {
	if err := s.ready(scope, self); err != nil {
		return PreferenceResult{}, err
	}
	if !validID(c.OperationID) || c.Revision < 0 {
		return PreferenceResult{}, domain.ErrInvalid
	}
	if err := s.Catalog.Mutable(c.Kind); err != nil {
		return PreferenceResult{}, err
	}
	return s.Repository.SetPreference(ctx, scope, self, c)
}
