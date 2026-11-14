package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// ErrAssignmentOverlap is the B-BR-EMP-244 refusal of an overlapping primary.
var ErrAssignmentOverlap = errors.New("fleet: overlapping primary assignment")

// Assignment actions of the fleet-owned authority port (EMP-FLEET-006).
const (
	// FleetAssign is fresh fleet.asset.assign on one asset.
	FleetAssign FleetAction = 3
	// FleetReadOwnAssignments is current fleet.asset.read on the member's own
	// assignment list: the target is the requesting membership itself.
	FleetReadOwnAssignments FleetAction = 4
)

// AssignPolicy is the source-owned policy of every assignment command.
func AssignPolicy() access.Policy {
	return access.Policy{Permission: "fleet.asset.assign", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
}

// validEndCause accepts only the B-BR-EMP-244/245 causes.
func validEndCause(cause AssignmentEndCause) bool {
	switch cause {
	case EndedByCommand, EndedByReplacement, EndedByTermination, EndedByDisposal:
		return true
	}
	return false
}

// exactInstant is a whole-microsecond instant of years 1..9999 (B-BR-EMP-243).
func exactInstant(at *time.Time) bool {
	if at == nil {
		return true
	}
	floor, err := fleet.AllocationInstant(*at)
	return err == nil && floor.Equal(*at)
}

func copyInstant(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	v := at.UTC()
	return &v
}

func validOptionalReason(raw string) bool {
	return utf8.ValidString(raw) && (raw == "" || fleet.ValidReason(raw))
}

// AssignmentEndCause records why an assignment ended before its planned end.
type AssignmentEndCause string

const (
	EndedByCommand     AssignmentEndCause = "ENDED"
	EndedByReplacement AssignmentEndCause = "REPLACED"
	EndedByTermination AssignmentEndCause = "EMPLOYEE_TERMINATED"
	EndedByDisposal    AssignmentEndCause = "ASSET_DISPOSED"
)

type AssignmentCreateInput struct {
	private
	OperationID, AssetID string
	Kind                 fleet.AssignmentKind
	EmployeeIDs          []string
	// Start nil means the server instant of the command.
	Start, End     *time.Time
	Reason         string
	ReplacePrimary bool
}
type AssignmentCreate struct {
	private
	value AssignmentCreateInput
}

// NewAssignmentCreate validates the exact command shape of B-BR-EMP-243/244:
// canonical identifiers, a known kind, 1..100 distinct employees (one for a
// primary), whole-microsecond instants with an end after the start, an end
// for TEMPORARY, an optional reason and "replace" only for a primary. The
// start against the server clock is checked by the repository.
func NewAssignmentCreate(v AssignmentCreateInput) (AssignmentCreate, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !validOptionalReason(v.Reason) {
		return AssignmentCreate{}, fleet.ErrInvalid
	}
	if err := fleet.ValidAssignees(v.Kind, v.EmployeeIDs); err != nil {
		return AssignmentCreate{}, fleet.ErrInvalid
	}
	for _, id := range v.EmployeeIDs {
		if !canonicalID(id) {
			return AssignmentCreate{}, fleet.ErrInvalid
		}
	}
	if !exactInstant(v.Start) || !exactInstant(v.End) || v.Start != nil && v.End != nil && !v.End.After(*v.Start) {
		return AssignmentCreate{}, fleet.ErrInvalid
	}
	if v.Kind == fleet.TemporaryAssignment && v.End == nil || v.ReplacePrimary && v.Kind != fleet.PrimaryAssignment {
		return AssignmentCreate{}, fleet.ErrInvalid
	}
	v.EmployeeIDs = append([]string(nil), v.EmployeeIDs...)
	v.Start, v.End = copyInstant(v.Start), copyInstant(v.End)
	return AssignmentCreate{value: v}, nil
}
func (c AssignmentCreate) Snapshot() (AssignmentCreateInput, error) {
	copy, err := NewAssignmentCreate(c.value)
	if err != nil {
		return AssignmentCreateInput{}, err
	}
	return copy.value, nil
}

func instantText(at *time.Time) string {
	if at == nil {
		return ""
	}
	return orgtime.FormatInstant(*at)
}

// Digest binds the scoped canonical intent; the same instant from another
// zone is the same intent, the employee order is part of it.
func (c AssignmentCreate) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "fleet.asset.assignment.create", scope.OrgID().String(), in.OperationID, in.AssetID, in.Kind, in.EmployeeIDs, instantText(in.Start), instantText(in.End), in.Reason, in.ReplacePrimary})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type AssignmentEndInput struct {
	private
	OperationID, AssetID, AssignmentID, Reason string
}
type AssignmentEnd struct {
	private
	value AssignmentEndInput
}

func NewAssignmentEnd(v AssignmentEndInput) (AssignmentEnd, error) {
	if !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || !canonicalID(v.AssignmentID) || !validOptionalReason(v.Reason) {
		return AssignmentEnd{}, fleet.ErrInvalid
	}
	return AssignmentEnd{value: v}, nil
}
func (c AssignmentEnd) Snapshot() (AssignmentEndInput, error) {
	copy, err := NewAssignmentEnd(c.value)
	if err != nil {
		return AssignmentEndInput{}, err
	}
	return copy.value, nil
}
func (c AssignmentEnd) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]any{1, "fleet.asset.assignment.end", scope.OrgID().String(), in.OperationID, in.AssetID, in.AssignmentID, in.Reason})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// AssignmentResult is the acknowledgement of one command: the created (or
// ended) assignments in command order and, for a replace, the ended primary.
type AssignmentResult struct {
	private
	AssetID       string
	AssignmentIDs []string
	ReplacedID    string
}

// AssignmentView is one stored assignment with its state derived at the
// page's server instant.
type AssignmentView struct {
	private
	ID, AssetID, EmployeeID string
	Kind                    fleet.AssignmentKind
	Period                  fleet.AssignmentPeriod
	State                   fleet.AssignmentState
	Reason                  string
	RecordedAt              time.Time
	EndCause                AssignmentEndCause
	EndedAt                 *time.Time
}

type AssignmentFilter struct {
	private
	AfterFrom *time.Time
	AfterID   string
	Limit     int
}

func (f AssignmentFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || (f.AfterFrom == nil) != (f.AfterID == "") || f.AfterID != "" && !canonicalID(f.AfterID) || !exactInstant(f.AfterFrom) {
		return fleet.ErrInvalid
	}
	return nil
}

type OwnAssignmentFilter struct {
	private
	AfterID string
	Limit   int
}

func (f OwnAssignmentFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) {
		return fleet.ErrInvalid
	}
	return nil
}

// AssignmentPage is one keyset page; At is the server instant of the states.
type AssignmentPage struct {
	private
	Assignments []AssignmentView
	At          time.Time
	NextFrom    *time.Time
	NextID      string
}

type AssignmentRepository interface {
	CreateAssignments(context.Context, kernel.OrgScope, FleetActor, string, []string, AssignmentCreate) (AssignmentResult, bool, error)
	EndAssignment(context.Context, kernel.OrgScope, FleetActor, string, AssignmentEnd) (AssignmentResult, bool, error)
	ListAssetAssignments(context.Context, kernel.OrgScope, FleetActor, string, string, AssignmentFilter) (AssignmentPage, error)
	ListOwnAssignments(context.Context, kernel.OrgScope, FleetActor, string, OwnAssignmentFilter) (AssignmentPage, error)
}

type AssignmentService struct {
	Enabled    bool
	Repository AssignmentRepository
	NewID      func() (string, error)
}

func (v AssignmentView) clone() AssignmentView {
	if v.Period.To != nil {
		to := *v.Period.To
		v.Period.To = &to
	}
	if v.EndedAt != nil {
		at := *v.EndedAt
		v.EndedAt = &at
	}
	return v
}

// valid checks one stored row against B-BR-EMP-243 and its state at the
// page instant.
func (v AssignmentView) valid(at time.Time) bool {
	if !canonicalID(v.ID) || !canonicalID(v.AssetID) || !canonicalID(v.EmployeeID) || !validOptionalReason(v.Reason) || !validInstant(v.RecordedAt) {
		return false
	}
	if _, err := fleet.ParseAssignmentKind(string(v.Kind)); err != nil {
		return false
	}
	if _, err := fleet.NewAssignmentPeriod(v.Period.From, v.Period.To); err != nil || v.Kind == fleet.TemporaryAssignment && v.Period.To == nil {
		return false
	}
	if v.EndCause == "" {
		if v.EndedAt != nil || v.Period.Empty() {
			return false
		}
	} else if !validEndCause(v.EndCause) || v.EndedAt == nil || !validInstant(*v.EndedAt) || v.Period.To == nil {
		return false
	}
	return v.State == v.Period.StateAt(at)
}

func validAssignmentResult(v AssignmentResult, asset string, count int) bool {
	if v.AssetID != asset || len(v.AssignmentIDs) != count {
		return false
	}
	seen := map[string]bool{}
	for _, id := range v.AssignmentIDs {
		if !canonicalID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return v.ReplacedID == "" || canonicalID(v.ReplacedID) && !seen[v.ReplacedID]
}

func cloneResult(v AssignmentResult) AssignmentResult {
	v.AssignmentIDs = append([]string(nil), v.AssignmentIDs...)
	return v
}

func (s AssignmentService) Create(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AssignmentCreate) (AssignmentResult, bool, error) {
	if !s.Enabled {
		return AssignmentResult{}, false, ErrSurfaceDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return AssignmentResult{}, false, err
	}
	if err := authorized(ctx, scope, actor, decision, in.AssetID, AssignPolicy()); err != nil {
		return AssignmentResult{}, false, err
	}
	if unavailable(s.Repository) || s.NewID == nil {
		return AssignmentResult{}, false, ErrUnavailable
	}
	ids := make([]string, len(in.EmployeeIDs))
	for i := range ids {
		id, err := s.NewID()
		if err != nil || !canonicalID(id) {
			return AssignmentResult{}, false, ErrUnavailable
		}
		ids[i] = id
	}
	v, replayed, err := s.Repository.CreateAssignments(ctx, scope, actor, decision.Subject().MembershipID(), ids, command)
	if guard := authorized(ctx, scope, actor, decision, in.AssetID, AssignPolicy()); guard != nil {
		return AssignmentResult{}, false, guard
	}
	if err != nil {
		return AssignmentResult{}, false, err
	}
	if !validAssignmentResult(v, in.AssetID, len(in.EmployeeIDs)) || v.ReplacedID != "" && !in.ReplacePrimary {
		return AssignmentResult{}, false, ErrUnavailable
	}
	return cloneResult(v), replayed, nil
}

func (s AssignmentService) End(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AssignmentEnd) (AssignmentResult, bool, error) {
	if !s.Enabled {
		return AssignmentResult{}, false, ErrSurfaceDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return AssignmentResult{}, false, err
	}
	if err := authorized(ctx, scope, actor, decision, in.AssetID, AssignPolicy()); err != nil {
		return AssignmentResult{}, false, err
	}
	if unavailable(s.Repository) {
		return AssignmentResult{}, false, ErrUnavailable
	}
	v, replayed, err := s.Repository.EndAssignment(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := authorized(ctx, scope, actor, decision, in.AssetID, AssignPolicy()); guard != nil {
		return AssignmentResult{}, false, guard
	}
	if err != nil {
		return AssignmentResult{}, false, err
	}
	if !validAssignmentResult(v, in.AssetID, 1) || v.AssignmentIDs[0] != in.AssignmentID || v.ReplacedID != "" {
		return AssignmentResult{}, false, ErrUnavailable
	}
	return cloneResult(v), replayed, nil
}

// validAssignmentPage checks the bound, every row, the order and the cursor of one
// page; own pages hold only ACTIVE rows in identifier order.
func validAssignmentPage(v AssignmentPage, limit int, asset string, own bool) bool {
	if len(v.Assignments) > limit || !validInstant(v.At) {
		return false
	}
	for i, row := range v.Assignments {
		if !row.valid(v.At) || asset != "" && row.AssetID != asset || own && row.State != fleet.AssignmentActive {
			return false
		}
		if i > 0 {
			prior := v.Assignments[i-1]
			if own && row.ID <= prior.ID || !own && (row.Period.From.Before(prior.Period.From) || row.Period.From.Equal(prior.Period.From) && row.ID <= prior.ID) {
				return false
			}
		}
	}
	if v.NextID == "" {
		return v.NextFrom == nil
	}
	last := v.Assignments
	if len(last) != limit || last[len(last)-1].ID != v.NextID {
		return false
	}
	if own {
		return v.NextFrom == nil
	}
	return v.NextFrom != nil && v.NextFrom.Equal(last[len(last)-1].Period.From)
}

func clonePage(v AssignmentPage) AssignmentPage {
	out := AssignmentPage{Assignments: make([]AssignmentView, 0, len(v.Assignments)), At: v.At, NextID: v.NextID, NextFrom: copyInstant(v.NextFrom)}
	for _, row := range v.Assignments {
		out.Assignments = append(out.Assignments, row.clone())
	}
	return out
}

func (s AssignmentService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, asset string, filter AssignmentFilter) (AssignmentPage, error) {
	if !s.Enabled {
		return AssignmentPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(asset) {
		return AssignmentPage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return AssignmentPage{}, err
	}
	if err := authorized(ctx, scope, actor, decision, asset, Policy(false)); err != nil {
		return AssignmentPage{}, err
	}
	if unavailable(s.Repository) {
		return AssignmentPage{}, ErrUnavailable
	}
	v, err := s.Repository.ListAssetAssignments(ctx, scope, actor, decision.Subject().MembershipID(), asset, filter)
	if guard := authorized(ctx, scope, actor, decision, asset, Policy(false)); guard != nil {
		return AssignmentPage{}, guard
	}
	if err != nil {
		return AssignmentPage{}, err
	}
	if !validAssignmentPage(v, filter.Limit, asset, false) {
		return AssignmentPage{}, ErrUnavailable
	}
	return clonePage(v), nil
}

// Mine lists the requesting member's ACTIVE assignments; the decision target
// is the requesting membership itself (B-BR-EMP-246).
func (s AssignmentService) Mine(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, filter OwnAssignmentFilter) (AssignmentPage, error) {
	if !s.Enabled {
		return AssignmentPage{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return AssignmentPage{}, err
	}
	member := decision.Subject().MembershipID()
	if err := authorized(ctx, scope, actor, decision, member, Policy(false)); err != nil {
		return AssignmentPage{}, err
	}
	if unavailable(s.Repository) {
		return AssignmentPage{}, ErrUnavailable
	}
	v, err := s.Repository.ListOwnAssignments(ctx, scope, actor, member, filter)
	if guard := authorized(ctx, scope, actor, decision, member, Policy(false)); guard != nil {
		return AssignmentPage{}, guard
	}
	if err != nil {
		return AssignmentPage{}, err
	}
	if !validAssignmentPage(v, filter.Limit, "", true) {
		return AssignmentPage{}, ErrUnavailable
	}
	return clonePage(v), nil
}

// AssignedAssetsPort answers B-BR-EMP-246 inside the caller's organization
// transaction (a pgx.Tx or the borrowed intake port): whether one asset is
// assigned to a membership at an instant, and every asset assigned to it.
// The membership period and the assignment period must both contain the
// instant. It is the query of the SELF scope on assets, of EMP-METER-002
// intake and of the EMP-OFFL-006 reference feed. It takes no locks.
type AssignedAssetsPort interface {
	AssignedAtDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, string, time.Time) (bool, error)
	AssetsAssignedAtDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) ([]string, error)
}
