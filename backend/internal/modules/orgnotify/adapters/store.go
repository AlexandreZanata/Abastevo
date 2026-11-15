// Package adapters persists scoped notices and dispatches them on the queue.
package adapters

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgnotify"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	m "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	o "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration is the additive schema this store requires.
const Migration = "000204_org_notifications.sql"

const (
	actionRead       = "notice.read"
	actionPreference = "preference.set"
)

var (
	sourceName = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$`)
	sourceType = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
)

// AuthoritySource is AUTH-003's current authority over a borrowed scoped tx.
type AuthoritySource interface {
	LoadAuthorityTx(context.Context, access.AuthorityQueries, kernel.OrgScope, string) (access.Authority, error)
}

// Store owns the notification tables. Every member call reloads current
// authority in its own scoped transaction; dispatch borrows the orgevent tx.
type Store struct {
	Pool      *pgxpool.Pool
	Catalog   domain.Catalog
	Authority AuthoritySource
	NewID     func() (string, error)
}

var (
	_ app.Repository = (*Store)(nil)
	_ app.Producer   = (*Store)(nil)
)

func (s *Store) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.NewID != nil
}

// safe keeps only bounded module outcomes; SQL text never leaves the store.
func safe(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{domain.ErrInvalid, domain.ErrValidation, domain.ErrNotFound, domain.ErrSuspended, domain.ErrConflict, domain.ErrReused, domain.ErrDenied} {
		if errors.Is(err, known) {
			return known
		}
	}
	if ctx.Err() != nil {
		return domain.ErrUnavailable
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23503", "23514", "22P02":
			return domain.ErrInvalid
		case "42501":
			return domain.ErrDenied
		}
	}
	return domain.ErrUnavailable
}

func validSource(e event.Event) bool {
	return audit.ValidID(e.ID) && audit.ValidID(e.TargetID) && len(e.Name) <= 64 && sourceName.MatchString(e.Name) &&
		len(e.TargetType) <= 64 && sourceType.MatchString(e.TargetType) && e.Revision > 0
}

func scoped(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		if errors.Is(err, kernel.ErrInvalidOrgScope) {
			return domain.ErrDenied
		}
		return domain.ErrUnavailable
	}
	return nil
}

// RequestTx is the producer port. It runs inside the producer's own audited
// orgevent transaction, so the request commits or rolls back with the source
// change, its audit event and the notif.dispatch job.
func (s *Store) RequestTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, source event.Event, requests []domain.Request) ([]domain.Registered, error) {
	if s == nil || tx == nil || s.NewID == nil {
		return nil, domain.ErrUnavailable
	}
	if scope.Validate() != nil || !validSource(source) {
		return nil, domain.ErrInvalid
	}
	prepared, err := s.Catalog.PrepareAll(requests)
	if err != nil {
		return nil, err
	}
	if err := scoped(ctx, tx, scope); err != nil {
		return nil, err
	}
	queries, org := q.New(tx), scope.OrgID().String()
	out := make([]domain.Registered, 0, len(prepared))
	for _, p := range prepared {
		id, err := s.NewID()
		if err != nil || !audit.ValidID(id) {
			return nil, domain.ErrUnavailable
		}
		inserted, err := queries.InsertRequest(ctx, q.InsertRequestParams{OrgID: org, ID: id, IdempotencyKey: p.Key,
			SourceEventID: source.ID, SourceEventName: source.Name, SourceTargetType: source.TargetType, SourceTargetID: source.TargetID,
			Kind: p.Kind, Template: p.Template, Params: p.ParamsJSON(), LinkPath: p.Link, ContentHash: p.Hash, RecipientCount: int32(len(p.Recipients))})
		if errors.Is(err, pgx.ErrNoRows) {
			// The same key returns the first request; other content is refused.
			existing, err := queries.RequestByKey(ctx, q.RequestByKeyParams{OrgID: org, IdempotencyKey: p.Key})
			if err != nil {
				return nil, safe(ctx, err)
			}
			if existing.ContentHash != p.Hash {
				return nil, domain.ErrReused
			}
			out = append(out, domain.Registered{RequestID: existing.ID, Key: p.Key, Deduplicated: true})
			continue
		}
		if err != nil {
			return nil, safe(ctx, err)
		}
		for _, member := range p.Recipients {
			if err := queries.InsertRecipient(ctx, q.InsertRecipientParams{OrgID: org, RequestID: inserted, MembershipID: member}); err != nil {
				return nil, safe(ctx, err)
			}
		}
		out = append(out, domain.Registered{RequestID: inserted, Key: p.Key})
	}
	return out, nil
}

// Dispatch is the notif.dispatch orgevent effect. It claims the event's
// pending requests once and records exactly one IN_APP outcome per recipient.
func (s *Store) Dispatch(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx, e event.Event) error {
	if !s.ready() || tx == nil {
		return domain.ErrUnavailable
	}
	if scope.Validate() != nil || !validSource(e) {
		return domain.ErrInvalid
	}
	if err := scoped(ctx, tx, scope); err != nil {
		return err
	}
	queries, org := q.New(tx), scope.OrgID().String()
	claimed, err := queries.ClaimRequests(ctx, q.ClaimRequestsParams{OrgID: org, SourceEventID: e.ID})
	if err != nil {
		return safe(ctx, err)
	}
	if len(claimed) == 0 {
		return nil
	}
	for _, r := range claimed {
		if r.SourceEventName != e.Name || r.SourceTargetType != e.TargetType || r.SourceTargetID != e.TargetID {
			return domain.ErrDenied
		}
	}
	type item struct {
		request, member string
		kind            domain.Kind
	}
	var items []item
	for _, r := range claimed {
		kind, ok := s.Catalog.Lookup(r.Kind)
		if !ok {
			// Composition drift: fail closed and retry under the bounded budget.
			return domain.ErrUnavailable
		}
		recipients, err := queries.RequestRecipients(ctx, q.RequestRecipientsParams{OrgID: org, RequestID: r.ID})
		if err != nil {
			return safe(ctx, err)
		}
		for _, member := range recipients {
			items = append(items, item{request: r.ID, member: member, kind: kind})
		}
	}
	// One global (member, kind, request) order keeps budget-row locks
	// deadlock-free across concurrent dispatches of different events.
	slices.SortFunc(items, func(a, b item) int {
		return cmp.Or(strings.Compare(a.member, b.member), strings.Compare(a.kind.Name, b.kind.Name), strings.Compare(a.request, b.request))
	})
	stamp, err := queries.DatabaseNow(ctx)
	if err != nil || !stamp.Valid {
		return domain.ErrUnavailable
	}
	now := stamp.Time
	window := pgtype.Timestamptz{Time: now.UTC().Truncate(time.Hour), Valid: true}
	for _, it := range items {
		eligible, err := s.eligible(ctx, tx, scope, it.member, it.kind.Permission, now)
		if err != nil {
			return err
		}
		muted := false
		if eligible && !it.kind.Mandatory {
			if muted, err = queries.Muted(ctx, q.MutedParams{OrgID: org, MembershipID: it.member, Kind: it.kind.Name}); err != nil {
				return safe(ctx, err)
			}
		}
		reason := domain.Screen(it.kind, eligible, muted)
		if reason == "" {
			_, err := queries.ConsumeBudget(ctx, q.ConsumeBudgetParams{OrgID: org, MembershipID: it.member, Kind: it.kind.Name, WindowStart: window, HourlyLimit: domain.HourlyLimit})
			if errors.Is(err, pgx.ErrNoRows) {
				reason = domain.Limited
			} else if err != nil {
				return safe(ctx, err)
			}
		}
		delivery := q.InsertDeliveryParams{OrgID: org, RequestID: it.request, MembershipID: it.member, Outcome: string(domain.Suppressed), Reason: string(reason)}
		if reason == "" {
			id, err := s.NewID()
			if err != nil || !audit.ValidID(id) {
				return domain.ErrUnavailable
			}
			if err := queries.InsertNotice(ctx, q.InsertNoticeParams{OrgID: org, ID: id, RequestID: it.request, MembershipID: it.member}); err != nil {
				return safe(ctx, err)
			}
			delivery.Outcome, delivery.NoticeID = string(domain.Delivered), id
		}
		if err := queries.InsertDelivery(ctx, delivery); err != nil {
			return safe(ctx, err)
		}
	}
	return nil
}

// eligible evaluates current AUTH-003 authority at dispatch (BR-AUTH-014).
func (s *Store) eligible(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member, permission string, now time.Time) (bool, error) {
	account, err := q.New(tx).MembershipAccount(ctx, q.MembershipAccountParams{OrgID: scope.OrgID().String(), MembershipID: member})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, safe(ctx, err)
	}
	if account == "" {
		return false, nil
	}
	current, err := s.Authority.LoadAuthorityTx(ctx, tx, scope, account)
	if errors.Is(err, access.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, domain.ErrUnavailable
	}
	if current.MembershipID != member || current.MembershipState != m.MembershipActive || current.EmployeeState != m.EmployeeActive || current.OrganizationState == o.Closed {
		return false, nil
	}
	if permission == "" {
		return true, nil
	}
	for _, assignment := range current.Assignments {
		v, err := assignment.Snapshot(scope)
		if err != nil {
			return false, domain.ErrUnavailable
		}
		if v.MembershipID != member {
			continue
		}
		valid, err := assignment.ValidAt(scope, now)
		if err != nil {
			return false, domain.ErrUnavailable
		}
		if !valid {
			continue
		}
		rule, err := permissions.SystemCatalog().Lookup(v.Role, permissions.Permission(permission))
		if err != nil {
			return false, domain.ErrUnavailable
		}
		if rule.Coverage() != permissions.Denied {
			return true, nil
		}
	}
	return false, nil
}

// self admits only the account-bound current web-capable membership.
func (s *Store) self(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, v app.Self) error {
	current, err := s.Authority.LoadAuthorityTx(ctx, tx, scope, v.AccountID)
	if errors.Is(err, access.ErrNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	if current.MembershipID != v.MembershipID || current.AccountID != v.AccountID {
		return domain.ErrNotFound
	}
	switch current.MembershipState {
	case m.MembershipActive:
	case m.MembershipSuspended:
		return domain.ErrSuspended
	default:
		return domain.ErrNotFound
	}
	if current.EmployeeState != m.EmployeeActive || current.Model != m.ExistingAccount || current.OrganizationState == o.Closed {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) run(ctx context.Context, scope kernel.OrgScope, work func(pgx.Tx) error) error {
	if !s.ready() {
		return domain.ErrUnavailable
	}
	if scope.Validate() != nil {
		return domain.ErrNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return safe(ctx, database.WithOrgTx(ctx, s.Pool, scope, work))
}

func notice(id string, position pgtype.Int8, created, read pgtype.Timestamptz, kind, template string, params []byte, link string) (app.Notice, error) {
	n := app.Notice{ID: id, Kind: kind, Template: template, Link: link}
	if !position.Valid || position.Int64 < 1 || !created.Valid || json.Unmarshal(params, &n.Params) != nil || n.Params == nil {
		return app.Notice{}, domain.ErrUnavailable
	}
	n.Position, n.CreatedAt = position.Int64, created.Time
	if read.Valid {
		n.ReadAt = read.Time
	}
	return n, nil
}

func (s *Store) ListInbox(ctx context.Context, scope kernel.OrgScope, v app.Self, query app.InboxQuery) (app.InboxPage, error) {
	if query.Limit < 1 || query.Limit > app.MaxLimit || query.After < 0 {
		return app.InboxPage{}, domain.ErrInvalid
	}
	var page app.InboxPage
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		if err := s.self(ctx, tx, scope, v); err != nil {
			return err
		}
		rows, err := q.New(tx).ListInbox(ctx, q.ListInboxParams{OrgID: scope.OrgID().String(), MembershipID: v.MembershipID, UnreadOnly: query.UnreadOnly, AfterPosition: query.After, RowLimit: int32(query.Limit + 1)})
		if err != nil {
			return err
		}
		page = app.InboxPage{Notices: make([]app.Notice, 0, min(len(rows), query.Limit))}
		for i, row := range rows {
			if i == query.Limit {
				page.Next = page.Notices[i-1].Position
				break
			}
			n, err := notice(row.ID, row.Position, row.CreatedAt, row.ReadAt, row.Kind, row.Template, row.Params, row.LinkPath)
			if err != nil {
				return err
			}
			page.Notices = append(page.Notices, n)
		}
		// Fence: the membership is still current after the read.
		return s.self(ctx, tx, scope, v)
	})
	if err != nil {
		return app.InboxPage{}, err
	}
	return page, nil
}

// reserve records the command receipt first; a committed receipt with the
// same digest is a replay and any other content is a reused operation.
func reserve(ctx context.Context, queries *q.Queries, org, operation, member, action, target, hash string) (bool, error) {
	_, err := queries.ReserveCommand(ctx, q.ReserveCommandParams{OrgID: org, OperationID: operation, MembershipID: member, Action: action, Target: target, RequestHash: hash})
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	existing, err := queries.GetCommand(ctx, q.GetCommandParams{OrgID: org, OperationID: operation})
	if err != nil {
		return false, err
	}
	if existing.MembershipID != member || existing.Action != action || existing.Target != target || existing.RequestHash != hash {
		return false, domain.ErrReused
	}
	return true, nil
}

func (s *Store) MarkRead(ctx context.Context, scope kernel.OrgScope, v app.Self, c app.ReadCommand) (app.ReadResult, error) {
	if !audit.ValidID(c.OperationID) || !audit.ValidID(c.NoticeID) {
		return app.ReadResult{}, domain.ErrInvalid
	}
	var out app.ReadResult
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		if err := s.self(ctx, tx, scope, v); err != nil {
			return err
		}
		queries, org := q.New(tx), scope.OrgID().String()
		replayed, err := reserve(ctx, queries, org, c.OperationID, v.MembershipID, actionRead, c.NoticeID, domain.CommandDigest(actionRead, v.MembershipID, c.NoticeID))
		if err != nil {
			return err
		}
		if !replayed {
			if err := queries.MarkNoticeRead(ctx, q.MarkNoticeReadParams{OrgID: org, ID: c.NoticeID, MembershipID: v.MembershipID}); err != nil {
				return err
			}
		}
		row, err := queries.GetNotice(ctx, q.GetNoticeParams{OrgID: org, ID: c.NoticeID, MembershipID: v.MembershipID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		n, err := notice(row.ID, row.Position, row.CreatedAt, row.ReadAt, row.Kind, row.Template, row.Params, row.LinkPath)
		if err != nil || n.ReadAt.IsZero() {
			return domain.ErrUnavailable
		}
		out = app.ReadResult{Notice: n, Replayed: replayed}
		return nil
	})
	if err != nil {
		return app.ReadResult{}, err
	}
	return out, nil
}

func (s *Store) Preferences(ctx context.Context, scope kernel.OrgScope, v app.Self) ([]app.Preference, error) {
	var out []app.Preference
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		if err := s.self(ctx, tx, scope, v); err != nil {
			return err
		}
		rows, err := q.New(tx).ListPreferences(ctx, q.ListPreferencesParams{OrgID: scope.OrgID().String(), MembershipID: v.MembershipID})
		if err != nil {
			return err
		}
		out = make([]app.Preference, 0, len(rows))
		for _, row := range rows {
			out = append(out, app.Preference{Kind: row.Kind, Muted: row.Muted, Revision: row.Revision})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SetPreference(ctx context.Context, scope kernel.OrgScope, v app.Self, c app.PreferenceCommand) (app.PreferenceResult, error) {
	if !audit.ValidID(c.OperationID) || c.Revision < 0 {
		return app.PreferenceResult{}, domain.ErrInvalid
	}
	var out app.PreferenceResult
	err := s.run(ctx, scope, func(tx pgx.Tx) error {
		if err := s.self(ctx, tx, scope, v); err != nil {
			return err
		}
		if err := s.Catalog.Mutable(c.Kind); err != nil {
			return err
		}
		queries, org := q.New(tx), scope.OrgID().String()
		hash := domain.CommandDigest(actionPreference, v.MembershipID, c.Kind, strconv.FormatBool(c.Muted), strconv.FormatInt(c.Revision, 10))
		replayed, err := reserve(ctx, queries, org, c.OperationID, v.MembershipID, actionPreference, c.Kind, hash)
		if err != nil {
			return err
		}
		// The committed result is fully determined by the accepted command.
		out = app.PreferenceResult{Preference: app.Preference{Kind: c.Kind, Muted: c.Muted, Revision: c.Revision + 1}, Replayed: replayed}
		if replayed {
			return nil
		}
		current, err := queries.LockPreference(ctx, q.LockPreferenceParams{OrgID: org, MembershipID: v.MembershipID, Kind: c.Kind})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if c.Revision != 0 {
				return domain.ErrConflict
			}
			if _, err := queries.InsertPreference(ctx, q.InsertPreferenceParams{OrgID: org, MembershipID: v.MembershipID, Kind: c.Kind, Muted: c.Muted}); errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			} else if err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if current.Revision != c.Revision {
				return domain.ErrConflict
			}
			revision, err := queries.UpdatePreference(ctx, q.UpdatePreferenceParams{Muted: c.Muted, OrgID: org, MembershipID: v.MembershipID, Kind: c.Kind, Revision: c.Revision})
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			}
			if err != nil || revision != c.Revision+1 {
				return domain.ErrUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return app.PreferenceResult{}, err
	}
	return out, nil
}
