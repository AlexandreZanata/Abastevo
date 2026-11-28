package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// Service serves the owner API over the Inbox port.
type Service struct {
	Inbox Inbox
	Clock func() time.Time
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// List returns one page of the caller's unexpired notifications, newest
// first, and the caller's unread count.
func (s Service) List(ctx context.Context, accountID string, before *domain.Position, limit int) (Page, error) {
	if !domain.IsUUID(accountID) || limit < 1 || limit > domain.MaxListLimit {
		return Page{}, domain.ErrInvalid
	}
	if s.Inbox == nil {
		return Page{}, ErrUnavailable
	}
	rows, unread, err := s.Inbox.Page(ctx, accountID, s.now(), before, limit+1)
	if err != nil {
		return Page{}, unavailable(err)
	}
	page := Page{Items: make([]Item, 0, min(len(rows), limit)), Unread: unread}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		page.Next = &domain.Position{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	for _, r := range rows {
		page.Items = append(page.Items, Item{
			NotificationID: r.ID, Kind: r.Kind, Read: r.Read, Payload: r.Payload,
			CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return page, nil
}

// MarkRead marks the selection read. The answer counts the caller's
// unexpired notifications the selection covers, so a replay of the same
// request answers the same; other accounts' IDs are ignored silently.
func (s Service) MarkRead(ctx context.Context, accountID string, sel domain.Selection) (int64, error) {
	if !domain.IsUUID(accountID) || (!sel.All && (len(sel.IDs) < 1 || len(sel.IDs) > domain.MaxMarkIDs)) {
		return 0, domain.ErrInvalid
	}
	if s.Inbox == nil {
		return 0, ErrUnavailable
	}
	n, err := s.Inbox.MarkRead(ctx, accountID, sel, s.now())
	if err != nil {
		return 0, unavailable(err)
	}
	return n, nil
}

func unavailable(err error) error {
	if errors.Is(err, domain.ErrInvalid) || errors.Is(err, ErrUnavailable) {
		return err
	}
	return errors.Join(ErrUnavailable, err)
}
