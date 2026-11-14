// Package application owns the organization period command boundary
// (EMP-APPR-006). Repositories prove current authority inside their own
// transaction; owning record commands read period facts through PeriodGate.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strconv"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound        = errors.New("period: private period not found")
	ErrUnavailable     = errors.New("period: source unavailable")
	ErrUnauthenticated = errors.New("period: current company proof required")
	ErrOperationReused = errors.New("period: operation identifier reused")
	ErrSurfaceDisabled = errors.New("period: surface disabled")
)

type private struct{}

func (private) String() string                 { return "EnterprisePeriod(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return []byte(`"EnterprisePeriod(redacted)"`), nil }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }

// PeriodActor is the current company account and its web proof.
type PeriodActor struct {
	private
	AccountID, SessionID string
}

func canonicalID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func (a PeriodActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrUnauthenticated
	}
	return nil
}

// PeriodAction is the closed set of source-owned period policies.
type PeriodAction uint8

const (
	PeriodRead PeriodAction = iota + 1
	PeriodClose
	PeriodReopen
)

// PeriodPolicy returns the fixed policy of an action (EDR-149).
func PeriodPolicy(action PeriodAction) access.Policy {
	switch action {
	case PeriodRead:
		return access.Policy{Permission: period.ClosePermission, Capability: "organizations", Effect: access.Read}
	case PeriodClose:
		return access.Policy{Permission: period.ClosePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case PeriodReopen:
		return access.Policy{Permission: period.ReopenPermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{}
}

// ActionPolicy returns the write action that authorizes a period command.
func ActionPolicy(action period.Action) (PeriodAction, error) {
	switch action {
	case period.ActionClose:
		return PeriodClose, nil
	case period.ActionReopen:
		return PeriodReopen, nil
	}
	return 0, period.ErrInvalid
}

// PeriodAuthority retains the company account/proof -> capability -> root ->
// membership/role order in the caller's transaction for the organization
// target, and requires an ACTIVE root for writes.
type PeriodAuthority interface {
	CheckPeriodTx(context.Context, pgx.Tx, kernel.OrgScope, PeriodActor, string, PeriodAction) error
}

type PeriodCommandInput struct {
	private
	Action           period.Action
	OperationID      string
	Month            string
	ExpectedRevision int64
	Reason           string
}

// PeriodCommand is a validated, immutable command.
type PeriodCommand struct {
	private
	value PeriodCommandInput
	month period.Month
}

func NewPeriodCommand(in PeriodCommandInput) (PeriodCommand, error) {
	if _, err := ActionPolicy(in.Action); err != nil || !canonicalID(in.OperationID) || in.ExpectedRevision < 1 {
		return PeriodCommand{}, period.ErrInvalid
	}
	month, err := period.ParseMonth(in.Month)
	if err != nil {
		return PeriodCommand{}, err
	}
	switch in.Action {
	case period.ActionClose:
		if in.Reason != "" {
			return PeriodCommand{}, period.ErrInvalid
		}
	case period.ActionReopen:
		if err := period.CheckReason(in.Reason); err != nil {
			return PeriodCommand{}, err
		}
	}
	return PeriodCommand{value: PeriodCommandInput{Action: in.Action, OperationID: in.OperationID, Month: in.Month, ExpectedRevision: in.ExpectedRevision, Reason: in.Reason}, month: month}, nil
}
func (c PeriodCommand) Snapshot() (PeriodCommandInput, error) {
	if !c.month.Valid() {
		return PeriodCommandInput{}, period.ErrInvalid
	}
	return c.value, nil
}
func (c PeriodCommand) Month() period.Month { return c.month }

// Domain returns the pure command for period.Decide.
func (c PeriodCommand) Domain() period.Command {
	return period.Command{Action: c.value.Action, Expected: c.value.ExpectedRevision, Reason: c.value.Reason}
}

// Digest is the canonical request hash bound to the organization: a SHA-256
// of the versioned JSON array of every command field.
func (c PeriodCommand) Digest(scope kernel.OrgScope) string {
	if !c.month.Valid() || scope.Validate() != nil {
		return ""
	}
	v := c.value
	raw, err := json.Marshal([]string{"fin.period.command.v1", scope.OrgID().String(), string(v.Action), v.OperationID, v.Month, strconv.FormatInt(v.ExpectedRevision, 10), v.Reason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// PeriodTransitionView is one immutable history entry.
type PeriodTransitionView struct {
	private
	Revision                          int64
	Kind                              period.Kind
	From, To                          period.State
	Authority, Reason                 string
	ActorAccountID, ActorMembershipID string
	OperationID                       string
	RecordedAt                        time.Time
}

// PeriodView is a period with its current head. PeriodID is empty while the
// month is OPEN at the implicit revision 1. History is empty in list pages.
type PeriodView struct {
	private
	Month, Start, End string
	PeriodID          string
	State             period.State
	Revision          int64
	History           []PeriodTransitionView
}

// PeriodAck is the recorded result of one accepted command.
type PeriodAck struct {
	private
	Month, PeriodID string
	Kind            period.Kind
	State           period.State
	Revision        int64
	RecordedAt      time.Time
}

type PeriodFilter struct {
	private
	AfterMonth string
	State      period.State
	Limit      int
}

// MaxPeriodPage mirrors the shared private page bound (httpapi.MaxLimit).
const MaxPeriodPage = 100

func (f PeriodFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxPeriodPage || f.State != "" && f.State != period.Locked && f.State != period.Reopened {
		return period.ErrInvalid
	}
	if f.AfterMonth != "" {
		if _, err := period.ParseMonth(f.AfterMonth); err != nil {
			return period.ErrInvalid
		}
	}
	return nil
}

type PeriodPage struct {
	private
	Periods   []PeriodView
	NextMonth string
}

type PeriodRepository interface {
	TransitionPeriod(context.Context, kernel.OrgScope, PeriodActor, string, string, PeriodCommand) (PeriodAck, bool, error)
	ReadPeriod(context.Context, kernel.OrgScope, PeriodActor, string, period.Month) (PeriodView, error)
	ListPeriods(context.Context, kernel.OrgScope, PeriodActor, string, PeriodFilter) (PeriodPage, error)
}

type PeriodService struct {
	Enabled    bool
	Repository PeriodRepository
	NewID      func() (string, error)
}

func unavailable(port any) bool {
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

// current binds the completed request decision to this actor, policy and the
// organization target before and after the repository call.
func current(ctx context.Context, scope kernel.OrgScope, actor PeriodActor, decision access.Decision, action PeriodAction) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return period.ErrInvalid
	}
	policy := PeriodPolicy(action)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, scope.OrgID().String()) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

func validAck(v PeriodAck, c PeriodCommand) bool {
	in := c.value
	if v.Month != in.Month || !canonicalID(v.PeriodID) || !validInstant(v.RecordedAt) || v.Revision != in.ExpectedRevision+1 {
		return false
	}
	head := period.Head{State: v.State, Revision: v.Revision}
	if head.Validate() != nil {
		return false
	}
	switch v.Kind {
	case period.Lock:
		return in.Action == period.ActionClose && v.State == period.Locked && v.Revision == 2
	case period.Relock:
		return in.Action == period.ActionClose && v.State == period.Locked && v.Revision > 2
	case period.Reopen:
		return in.Action == period.ActionReopen && v.State == period.Reopened
	}
	return false
}

func validTransitionView(h PeriodTransitionView) bool {
	t := period.Transition{Kind: h.Kind, From: h.From, To: h.To, Revision: h.Revision, Authority: h.Authority, Reason: h.Reason}
	return t.Validate() == nil && canonicalID(h.ActorAccountID) && canonicalID(h.ActorMembershipID) && canonicalID(h.OperationID) && validInstant(h.RecordedAt)
}

// validView checks the head, the half-open range and, when present, that the
// history is the complete ordered chain ending at the head.
func validView(v PeriodView, withHistory bool) bool {
	month, err := period.ParseMonth(v.Month)
	if err != nil || v.Start != month.Start() || v.End != month.End() {
		return false
	}
	head := period.Head{State: v.State, Revision: v.Revision}
	if head.Validate() != nil || (v.State == period.Open) != (v.PeriodID == "") || v.PeriodID != "" && !canonicalID(v.PeriodID) {
		return false
	}
	if !withHistory {
		return len(v.History) == 0
	}
	if int64(len(v.History)) != v.Revision-1 {
		return false
	}
	prior := time.Time{}
	for i, h := range v.History {
		if h.Revision != int64(i+2) || !validTransitionView(h) || h.RecordedAt.Before(prior) {
			return false
		}
		prior = h.RecordedAt
	}
	return len(v.History) == 0 || v.History[len(v.History)-1].To == v.State
}

func cloneView(v PeriodView) PeriodView {
	out := v
	if v.History != nil {
		out.History = append([]PeriodTransitionView{}, v.History...)
	}
	return out
}

func (s PeriodService) Transition(ctx context.Context, scope kernel.OrgScope, actor PeriodActor, decision access.Decision, command PeriodCommand) (PeriodAck, bool, error) {
	if !s.Enabled {
		return PeriodAck{}, false, ErrSurfaceDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return PeriodAck{}, false, err
	}
	action, err := ActionPolicy(in.Action)
	if err != nil {
		return PeriodAck{}, false, err
	}
	if err := current(ctx, scope, actor, decision, action); err != nil {
		return PeriodAck{}, false, err
	}
	if unavailable(s.Repository) || s.NewID == nil {
		return PeriodAck{}, false, ErrUnavailable
	}
	newID, err := s.NewID()
	if err != nil || !canonicalID(newID) {
		return PeriodAck{}, false, ErrUnavailable
	}
	v, replayed, err := s.Repository.TransitionPeriod(ctx, scope, actor, decision.Subject().MembershipID(), newID, command)
	if guard := current(ctx, scope, actor, decision, action); guard != nil {
		return PeriodAck{}, false, guard
	}
	if err != nil {
		return PeriodAck{}, false, err
	}
	if !validAck(v, command) {
		return PeriodAck{}, false, ErrUnavailable
	}
	return v, replayed, nil
}
func (s PeriodService) Read(ctx context.Context, scope kernel.OrgScope, actor PeriodActor, decision access.Decision, month string) (PeriodView, error) {
	if !s.Enabled {
		return PeriodView{}, ErrSurfaceDisabled
	}
	m, err := period.ParseMonth(month)
	if err != nil {
		return PeriodView{}, ErrNotFound
	}
	if err := current(ctx, scope, actor, decision, PeriodRead); err != nil {
		return PeriodView{}, err
	}
	if unavailable(s.Repository) {
		return PeriodView{}, ErrUnavailable
	}
	v, err := s.Repository.ReadPeriod(ctx, scope, actor, decision.Subject().MembershipID(), m)
	if guard := current(ctx, scope, actor, decision, PeriodRead); guard != nil {
		return PeriodView{}, guard
	}
	if err != nil {
		return PeriodView{}, err
	}
	if v.Month != month || !validView(v, true) {
		return PeriodView{}, ErrUnavailable
	}
	return cloneView(v), nil
}
func (s PeriodService) List(ctx context.Context, scope kernel.OrgScope, actor PeriodActor, decision access.Decision, filter PeriodFilter) (PeriodPage, error) {
	if !s.Enabled {
		return PeriodPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return PeriodPage{}, err
	}
	if err := current(ctx, scope, actor, decision, PeriodRead); err != nil {
		return PeriodPage{}, err
	}
	if unavailable(s.Repository) {
		return PeriodPage{}, ErrUnavailable
	}
	v, err := s.Repository.ListPeriods(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := current(ctx, scope, actor, decision, PeriodRead); guard != nil {
		return PeriodPage{}, guard
	}
	if err != nil {
		return PeriodPage{}, err
	}
	if len(v.Periods) > filter.Limit {
		return PeriodPage{}, ErrUnavailable
	}
	out := PeriodPage{Periods: make([]PeriodView, 0, len(v.Periods)), NextMonth: v.NextMonth}
	prior := filter.AfterMonth
	for _, p := range v.Periods {
		// Canonical months order lexically; pages run newest first.
		if !validView(p, false) || p.State == period.Open || prior != "" && p.Month >= prior || filter.State != "" && p.State != filter.State {
			return PeriodPage{}, ErrUnavailable
		}
		out.Periods = append(out.Periods, cloneView(p))
		prior = p.Month
	}
	if out.NextMonth != "" && (len(out.Periods) != filter.Limit || out.NextMonth != prior) {
		return PeriodPage{}, ErrUnavailable
	}
	return out, nil
}

// ReopeningFact is the stored proof of the reopening in effect.
type ReopeningFact struct {
	private
	Revision                          int64
	Authority, Reason                 string
	ActorAccountID, ActorMembershipID string
	At                                time.Time
}

// PeriodFact is the period of one reporting day read at receipt inside the
// owning record transaction (B-BR-EMP-214).
type PeriodFact struct {
	private
	Day, Month, PeriodID string
	State                period.State
	Revision             int64
	Reopening            *ReopeningFact
	Zone, ZoneSource     string
	CheckedAt            time.Time
}

// Validate checks the fact is a consistent server fact. A REOPENED fact must
// carry the stored fin.period.reopen transition of its own revision, with an
// admissible reason, recorded no later than the check.
func (f PeriodFact) Validate() error {
	month, err := period.ParseMonth(f.Month)
	if err != nil || !month.Contains(f.Day) || !validInstant(f.CheckedAt) {
		return ErrUnavailable
	}
	if (period.Head{State: f.State, Revision: f.Revision}).Validate() != nil || (f.State == period.Open) != (f.PeriodID == "") || f.PeriodID != "" && !canonicalID(f.PeriodID) {
		return ErrUnavailable
	}
	switch {
	case f.Zone == "" && f.ZoneSource == "":
	case f.Zone != "" && (f.ZoneSource == "organization" || f.ZoneSource == "unit"):
		if _, ok := kernel.LoadTimeZone(f.Zone); !ok {
			return ErrUnavailable
		}
	default:
		return ErrUnavailable
	}
	if f.State != period.Reopened {
		if f.Reopening != nil {
			return ErrUnavailable
		}
		return nil
	}
	r := f.Reopening
	if r == nil || r.Revision != f.Revision || r.Authority != period.ReopenPermission || period.CheckReason(r.Reason) != nil ||
		!canonicalID(r.ActorAccountID) || !canonicalID(r.ActorMembershipID) || !validInstant(r.At) || r.At.After(f.CheckedAt) {
		return ErrUnavailable
	}
	return nil
}

// Writable reports whether records of the day may be written or corrected in
// place: OPEN, or REOPENED with a proven reasoned reopening.
func (f PeriodFact) Writable() bool { return f.Validate() == nil && f.State != period.Locked }

// Amendment maps the fact to the amendment kernel's input. AuthorityProven is
// true only for a validated stored reopening transition.
func (f PeriodFact) Amendment() (amend.Period, error) {
	if err := f.Validate(); err != nil {
		return amend.Period{}, err
	}
	switch f.State {
	case period.Open:
		return amend.Period{State: amend.PeriodOpen}, nil
	case period.Locked:
		return amend.Period{State: amend.PeriodLocked}, nil
	}
	return amend.Period{State: amend.PeriodReopened, Reopening: &amend.Reopening{AuthorityProven: true, Reason: f.Reopening.Reason}}, nil
}

// PeriodGate is called by an owning record command inside its own scoped
// transaction. It never authorizes the record command itself.
type PeriodGate interface {
	StateForDayTx(context.Context, pgx.Tx, kernel.OrgScope, string) (PeriodFact, error)
	StateForOccurrenceTx(context.Context, pgx.Tx, kernel.OrgScope, time.Time, string) (PeriodFact, error)
}
