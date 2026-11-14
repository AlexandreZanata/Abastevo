// Package adapters persists organization period locks through ordinary scoped
// transactions and serves the in-transaction period gate (EMP-APPR-006).
package adapters

import (
	"context"
	"errors"
	"reflect"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/review"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PeriodTargetType is the audit target vocabulary of a period identity.
const PeriodTargetType = "finance_period"

var auditActions = map[period.Kind]string{period.Lock: "fin.period.locked", period.Reopen: "fin.period.reopened", period.Relock: "fin.period.relocked"}

// PeriodStore owns the period commands and reads.
type PeriodStore struct {
	Pool      *pgxpool.Pool
	Authority app.PeriodAuthority
	Audit     auditing.TransactionRunner
}

var _ app.PeriodRepository = (*PeriodStore)(nil)

// PeriodAuditDefinitions registers one action per transition kind. The
// reopening reason is a business fact of the transition, not an audit code.
func PeriodAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: auditActions[period.Lock], TargetType: PeriodTargetType}, {Action: auditActions[period.Reopen], TargetType: PeriodTargetType}, {Action: auditActions[period.Relock], TargetType: PeriodTargetType}}
}

func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

func (s *PeriodStore) ready(write bool) bool {
	return s != nil && s.Pool != nil && !missing(s.Authority) && (!write || !missing(s.Audit))
}

// storeError keeps only registered refusals; provider details stay private.
func storeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrOperationReused
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound,
		app.ErrOperationReused, app.ErrNotFound, app.ErrUnavailable, app.ErrUnauthenticated, period.ErrConflict, period.ErrTransition, period.ErrInvalid, period.ErrReasonRequired, period.ErrReasonInvalid} {
		if errors.Is(err, known) {
			return err
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return app.ErrUnavailable
}

func validCall(scope kernel.OrgScope, actor app.PeriodActor, member string) error {
	if scope.Validate() != nil {
		return period.ErrInvalid
	}
	if actor.Validate() != nil {
		return app.ErrUnauthenticated
	}
	if !validID(member) {
		return period.ErrInvalid
	}
	return nil
}

func instant(v pgtype.Timestamptz) (time.Time, error) {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}, app.ErrUnavailable
	}
	return v.Time.UTC(), nil
}

func (s *PeriodStore) TransitionPeriod(ctx context.Context, scope kernel.OrgScope, actor app.PeriodActor, member, newID string, command app.PeriodCommand) (app.PeriodAck, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.PeriodAck{}, false, err
	}
	in, err := command.Snapshot()
	if err != nil {
		return app.PeriodAck{}, false, err
	}
	if !validID(newID) {
		return app.PeriodAck{}, false, period.ErrInvalid
	}
	action, err := app.ActionPolicy(in.Action)
	if err != nil {
		return app.PeriodAck{}, false, err
	}
	if !s.ready(true) {
		return app.PeriodAck{}, false, app.ErrUnavailable
	}
	org, month, digest := scope.OrgID().String(), command.Month(), command.Digest(scope)
	var ack app.PeriodAck
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckPeriodTx(ctx, tx, scope, actor, member, action); err != nil {
			return err
		}
		queries := q.New(tx)
		// The command holds the root exclusively until commit; record writes
		// take FOR KEY SHARE on it in the gate (B-BR-EMP-214).
		state, err := queries.PeriodRootLock(ctx, org)
		if err != nil {
			return err
		}
		if !state.Valid || state.String != "ACTIVE" {
			return access.ErrSetupIncomplete
		}
		prior, err := queries.PeriodTransitionByOperation(ctx, q.PeriodTransitionByOperationParams{OrgID: org, OperationID: in.OperationID})
		if err == nil {
			if prior.ActorAccountID != actor.AccountID || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			at, err := instant(prior.RecordedAt)
			if err != nil {
				return err
			}
			ack = app.PeriodAck{Month: prior.Month, PeriodID: prior.PeriodID, Kind: period.Kind(prior.Kind), State: period.State(prior.ToState), Revision: prior.Revision, RecordedAt: at}
			replayed = true
			return s.Authority.CheckPeriodTx(ctx, tx, scope, actor, member, action)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		head, periodID := period.Initial(), newID
		row, err := queries.PeriodHead(ctx, q.PeriodHeadParams{OrgID: org, PeriodMonth: month.Start()})
		switch {
		case err == nil:
			head, periodID = period.Head{State: period.State(row.State), Revision: row.Revision}, row.ID
			if head.Validate() != nil || head.State == period.Open || !validID(periodID) {
				return app.ErrUnavailable
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		next, err := period.Decide(head, command.Domain())
		if err != nil {
			return err
		}
		event := audit.Event{OperationID: in.OperationID, ActorID: actor.AccountID, Action: auditActions[next.Kind], TargetType: PeriodTargetType, TargetID: periodID, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			recorded, err := q.New(tx).InsertPeriodTransition(ctx, q.InsertPeriodTransitionParams{OrgID: org, PeriodMonth: month.Start(), PeriodID: periodID, Revision: next.Revision, Kind: string(next.Kind), FromState: string(next.From), ToState: string(next.To), Authority: next.Authority, Reason: next.Reason, ActorAccountID: actor.AccountID, ActorMembershipID: member, OperationID: in.OperationID, RequestHash: digest})
			if err != nil {
				return err
			}
			at, err := instant(recorded)
			if err != nil {
				return err
			}
			ack = app.PeriodAck{Month: month.String(), PeriodID: periodID, Kind: next.Kind, State: next.To, Revision: next.Revision, RecordedAt: at}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.Authority.CheckPeriodTx(ctx, tx, scope, actor, member, action)
	})
	if err != nil {
		return app.PeriodAck{}, false, storeError(ctx, err)
	}
	return ack, replayed, nil
}

func transitionView(r q.PeriodTransitionsRow) (app.PeriodTransitionView, error) {
	at, err := instant(r.RecordedAt)
	if err != nil {
		return app.PeriodTransitionView{}, err
	}
	return app.PeriodTransitionView{Revision: r.Revision, Kind: period.Kind(r.Kind), From: period.State(r.FromState), To: period.State(r.ToState), Authority: r.Authority, Reason: r.Reason, ActorAccountID: r.ActorAccountID, ActorMembershipID: r.ActorMembershipID, OperationID: r.OperationID, RecordedAt: at}, nil
}

func (s *PeriodStore) read(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.PeriodActor, member string, fn func(*q.Queries) error) error {
	if err := s.Authority.CheckPeriodTx(ctx, tx, scope, actor, member, app.PeriodRead); err != nil {
		return err
	}
	queries := q.New(tx)
	if _, err := queries.PeriodRootShare(ctx, scope.OrgID().String()); err != nil {
		return err
	}
	if err := fn(queries); err != nil {
		return err
	}
	return s.Authority.CheckPeriodTx(ctx, tx, scope, actor, member, app.PeriodRead)
}

func (s *PeriodStore) ReadPeriod(ctx context.Context, scope kernel.OrgScope, actor app.PeriodActor, member string, month period.Month) (app.PeriodView, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.PeriodView{}, err
	}
	if !month.Valid() {
		return app.PeriodView{}, app.ErrNotFound
	}
	if !s.ready(false) {
		return app.PeriodView{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	view := app.PeriodView{Month: month.String(), Start: month.Start(), End: month.End(), State: period.Open, Revision: 1, History: []app.PeriodTransitionView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		return s.read(ctx, tx, scope, actor, member, func(queries *q.Queries) error {
			row, err := queries.PeriodHead(ctx, q.PeriodHeadParams{OrgID: org, PeriodMonth: month.Start()})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			view.PeriodID, view.State, view.Revision = row.ID, period.State(row.State), row.Revision
			rows, err := queries.PeriodTransitions(ctx, q.PeriodTransitionsParams{OrgID: org, PeriodMonth: month.Start()})
			if err != nil {
				return err
			}
			for _, r := range rows {
				if r.PeriodID != row.ID {
					return app.ErrUnavailable
				}
				h, err := transitionView(r)
				if err != nil {
					return err
				}
				view.History = append(view.History, h)
			}
			return nil
		})
	})
	if err != nil {
		return app.PeriodView{}, storeError(ctx, err)
	}
	return view, nil
}

func (s *PeriodStore) ListPeriods(ctx context.Context, scope kernel.OrgScope, actor app.PeriodActor, member string, filter app.PeriodFilter) (app.PeriodPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.PeriodPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.PeriodPage{}, err
	}
	if !s.ready(false) {
		return app.PeriodPage{}, app.ErrUnavailable
	}
	page := app.PeriodPage{Periods: []app.PeriodView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		return s.read(ctx, tx, scope, actor, member, func(queries *q.Queries) error {
			rows, err := queries.ListPeriodHeads(ctx, q.ListPeriodHeadsParams{OrgID: scope.OrgID().String(), AfterMonth: filter.AfterMonth, State: string(filter.State), RowLimit: int32(filter.Limit + 1)})
			if err != nil {
				return err
			}
			more := len(rows) > filter.Limit
			if more {
				rows = rows[:filter.Limit]
			}
			for _, r := range rows {
				month, err := period.ParseMonth(r.Month)
				if err != nil {
					return app.ErrUnavailable
				}
				page.Periods = append(page.Periods, app.PeriodView{Month: r.Month, Start: month.Start(), End: month.End(), PeriodID: r.ID, State: period.State(r.State), Revision: r.Revision})
			}
			if more && len(rows) > 0 {
				page.NextMonth = rows[len(rows)-1].Month
			}
			return nil
		})
	})
	if err != nil {
		return app.PeriodPage{}, storeError(ctx, err)
	}
	return page, nil
}

// PeriodGate reads period facts inside the caller's scoped transaction. It
// first takes FOR KEY SHARE on the organization root, which a period command
// holds FOR UPDATE until it commits, then reads the committed head (READ
// COMMITTED). It never authorizes the record command itself.
type PeriodGate struct{}

var _ app.PeriodGate = PeriodGate{}

func (PeriodGate) StateForDayTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, day string) (app.PeriodFact, error) {
	if missing(tx) {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	month, err := period.MonthOf(day)
	if err != nil {
		return app.PeriodFact{}, period.ErrInvalid
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	queries := q.New(tx)
	if _, err := queries.PeriodGateRoot(ctx, scope.OrgID().String()); err != nil {
		return app.PeriodFact{}, gateError(ctx, err)
	}
	return gateRead(ctx, queries, scope, day, month)
}

func (PeriodGate) StateForOccurrenceTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, at time.Time, unitID string) (app.PeriodFact, error) {
	if missing(tx) {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	if year := at.UTC().Year(); at.IsZero() || year < 1 || year > 9999 || unitID != "" && !validID(unitID) {
		return app.PeriodFact{}, period.ErrInvalid
	}
	if database.ValidateOrgDBTX(ctx, tx, scope) != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	queries := q.New(tx)
	org := scope.OrgID().String()
	zone, err := queries.PeriodGateRoot(ctx, org)
	if err != nil {
		return app.PeriodFact{}, gateError(ctx, err)
	}
	zones := orgtime.ReportingZones{Organization: zone}
	if unitID != "" {
		unitZone, err := queries.PeriodUnitZone(ctx, q.PeriodUnitZoneParams{OrgID: org, ID: unitID})
		if err != nil {
			return app.PeriodFact{}, gateError(ctx, err)
		}
		zones.Unit = &unitZone
	}
	attributed, err := orgtime.AttributeReportingDay(at, zones)
	if err != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	month, err := period.MonthOf(attributed.Day)
	if err != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	fact, err := gateRead(ctx, queries, scope, attributed.Day, month)
	if err != nil {
		return app.PeriodFact{}, err
	}
	fact.Zone, fact.ZoneSource = attributed.Zone, attributed.Source
	if fact.Validate() != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	return fact, nil
}

func gateError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return app.ErrUnavailable
}

func gateRead(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, day string, month period.Month) (app.PeriodFact, error) {
	org := scope.OrgID().String()
	fact := app.PeriodFact{Day: day, Month: month.String(), State: period.Open, Revision: 1}
	row, err := queries.PeriodHead(ctx, q.PeriodHeadParams{OrgID: org, PeriodMonth: month.Start()})
	switch {
	case err == nil:
		fact.PeriodID, fact.State, fact.Revision = row.ID, period.State(row.State), row.Revision
		if fact.State == period.Reopened {
			r, err := queries.PeriodTransitionAt(ctx, q.PeriodTransitionAtParams{OrgID: org, PeriodMonth: month.Start(), Revision: row.Revision})
			if err != nil {
				return app.PeriodFact{}, gateError(ctx, err)
			}
			at, err := instant(r.RecordedAt)
			if err != nil || r.Kind != string(period.Reopen) || r.PeriodID != row.ID {
				return app.PeriodFact{}, app.ErrUnavailable
			}
			fact.Reopening = &app.ReopeningFact{Revision: r.Revision, Authority: r.Authority, Reason: r.Reason, ActorAccountID: r.ActorAccountID, ActorMembershipID: r.ActorMembershipID, At: at}
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return app.PeriodFact{}, gateError(ctx, err)
	}
	// The check instant is taken after the root lock and the head read.
	checked, err := queries.PeriodClock(ctx)
	if err != nil {
		return app.PeriodFact{}, gateError(ctx, err)
	}
	if fact.CheckedAt, err = instant(checked); err != nil {
		return app.PeriodFact{}, err
	}
	if fact.Validate() != nil {
		return app.PeriodFact{}, app.ErrUnavailable
	}
	return fact, nil
}
