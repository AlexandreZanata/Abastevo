package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/jackc/pgx/v5"
)

// EMP-TRIP-008 missions (B-BR-EMP-353..358, EDR-228..231).
const (
	// MissionRespondOperationType is the assignee's signed response (EDR-229).
	MissionRespondOperationType = "trip.mission.respond"
	MissionEntityType           = "mission"
	MissionManagePermission     = "trip.mission.manage"
	MissionRespondPermission    = "trip.trip.start"
	// MaxMissionPage mirrors the shared private page bound (httpapi.MaxLimit).
	MaxMissionPage = 100
)

var (
	ErrMissionDisabled        = errors.New("trip: mission surface disabled")
	ErrMissionUnauthenticated = errors.New("trip: current company proof required")
	// ErrMissionIneligible is an assignee or asset that cannot be named.
	ErrMissionIneligible = errors.New("trip: mission target not eligible")
)

// MissionActor is the current company account and its web proof.
type MissionActor struct {
	private
	AccountID, SessionID string
}

func (a MissionActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrMissionUnauthenticated
	}
	return nil
}

// MissionAction is the closed set of mission policies.
type MissionAction uint8

const (
	// MissionRead lists and reads missions (trip.mission.manage).
	MissionRead MissionAction = iota + 1
	// MissionManage creates, assigns and cancels (fresh trip.mission.manage).
	MissionManage
	// MissionOwnRead lists the member's own missions (trip.trip.start).
	MissionOwnRead
	// MissionRespond is the signed response (trip.trip.start, operational write).
	MissionRespond
	// MissionReceipt re-reads a response receipt (trip.trip.start).
	MissionReceipt
)

// MissionPolicy returns the fixed policy of an action (EDR-231).
func MissionPolicy(action MissionAction) access.Policy {
	switch action {
	case MissionRead:
		return access.Policy{Permission: MissionManagePermission, Capability: "organizations", Effect: access.Read}
	case MissionManage:
		return access.Policy{Permission: MissionManagePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case MissionOwnRead, MissionReceipt:
		return access.Policy{Permission: MissionRespondPermission, Capability: "organizations", Effect: access.Read}
	case MissionRespond:
		return access.Policy{Permission: MissionRespondPermission, Capability: "organizations", Effect: access.OperationalWrite}
	}
	return access.Policy{}
}

// MissionFacts are the stored structure of a mission (unit, optional asset).
type MissionFacts struct {
	private
	MissionID, UnitID, AssetID string
}

// MissionAuthority retains the company account/proof → capability → root →
// membership/role order in the caller's transaction. Organization-level
// actions target the organization; mission actions target the mission
// (structure: its stored unit and asset); the own list targets the member.
type MissionAuthority interface {
	CheckMissionTx(context.Context, pgx.Tx, kernel.OrgScope, MissionActor, string, string, MissionAction) error
	// EligibleAssigneeTx answers whether the membership currently holds
	// trip.trip.start covering the mission as its own record (B-BR-EMP-355).
	EligibleAssigneeTx(context.Context, pgx.Tx, kernel.OrgScope, string, MissionFacts) (bool, error)
}

// MissionCommandKind names one company command.
type MissionCommandKind string

const (
	MissionCreate        MissionCommandKind = "CREATE"
	MissionPlanCommand   MissionCommandKind = "PLAN"
	MissionAssignCommand MissionCommandKind = "ASSIGN"
	MissionCancelCommand MissionCommandKind = "CANCEL"
)

// MissionPlanFields is the content of one PLAN command (EMP-TRIP-009,
// B-BR-EMP-382): exactly one destination (text or unit), a reason, an
// optional cost centre and the load token with its only field.
type MissionPlanFields struct {
	private
	DestinationText   *string
	DestinationUnitID string
	Reason            string
	CostCentreID      string
	Load              string
	CargoDescription  *string
	Passengers        *int
}

func (p MissionPlanFields) copy() MissionPlanFields {
	p.DestinationText, p.CargoDescription, p.Passengers = copyPointer(p.DestinationText), copyPointer(p.CargoDescription), copyPointer(p.Passengers)
	return p
}

// validate checks the identities and the domain plan profile without a
// scope; the store re-proves the plan in its scope (trip.NewPlan).
func (p MissionPlanFields) validate() error {
	if (p.DestinationText == nil) == (p.DestinationUnitID == "") || p.DestinationUnitID != "" && !canonicalID(p.DestinationUnitID) || p.CostCentreID != "" && !canonicalID(p.CostCentreID) {
		return trip.ErrInvalid
	}
	if p.DestinationText != nil {
		if err := trip.ValidPlanText(*p.DestinationText); err != nil {
			return err
		}
	}
	if err := trip.ValidPlanText(p.Reason); err != nil {
		return err
	}
	load, err := trip.ParseMissionLoad(p.Load)
	if err != nil {
		return err
	}
	switch load {
	case trip.LoadNone:
		if p.CargoDescription != nil || p.Passengers != nil {
			return trip.ErrInvalid
		}
	case trip.LoadCargo:
		if p.CargoDescription == nil || p.Passengers != nil {
			return trip.ErrInvalid
		}
		return trip.ValidPlanText(*p.CargoDescription)
	case trip.LoadPassengers:
		if p.CargoDescription != nil || p.Passengers == nil || *p.Passengers < 1 || *p.Passengers > trip.MaxMissionPassengers {
			return trip.ErrInvalid
		}
	}
	return nil
}

// Domain builds the domain plan input in the scope.
func (p MissionPlanFields) Domain(scope kernel.OrgScope) (trip.PlanInput, error) {
	in := trip.PlanInput{DestinationText: copyPointer(p.DestinationText), Reason: p.Reason, Load: p.Load, CargoDescription: copyPointer(p.CargoDescription), Passengers: copyPointer(p.Passengers)}
	if p.DestinationUnitID != "" {
		unit, err := trip.NewUnitRef(scope, p.DestinationUnitID)
		if err != nil {
			return trip.PlanInput{}, err
		}
		in.DestinationUnit = &unit
	}
	if p.CostCentreID != "" {
		centre, err := trip.NewCostCentreRef(scope, p.CostCentreID)
		if err != nil {
			return trip.PlanInput{}, err
		}
		in.CostCentre = &centre
	}
	return in, nil
}

// MissionCommandInput is one company command: create carries the content;
// plan the expected revision and the plan; assign the expected revision and
// the employee; cancel the expected revision and the reason.
type MissionCommandInput struct {
	private
	Kind               MissionCommandKind
	OperationID        string
	MissionID          string
	ExpectedRevision   int64
	UnitID, AssetID    string
	Title              string
	StartsAt, EndsAt   time.Time
	AssigneeEmployeeID string
	Reason             string
	Plan               *MissionPlanFields
}

// MissionCommand is a validated, immutable command.
type MissionCommand struct {
	private
	value MissionCommandInput
}

func NewMissionCommand(in MissionCommandInput) (MissionCommand, error) {
	if !canonicalID(in.OperationID) {
		return MissionCommand{}, trip.ErrInvalid
	}
	out := MissionCommandInput{Kind: in.Kind, OperationID: in.OperationID}
	if in.Kind != MissionPlanCommand && in.Plan != nil {
		return MissionCommand{}, trip.ErrInvalid
	}
	switch in.Kind {
	case MissionPlanCommand:
		if !canonicalID(in.MissionID) || in.ExpectedRevision < 1 || in.UnitID != "" || in.AssetID != "" || in.Title != "" || !in.StartsAt.IsZero() || !in.EndsAt.IsZero() ||
			in.AssigneeEmployeeID != "" || in.Reason != "" || in.Plan == nil {
			return MissionCommand{}, trip.ErrInvalid
		}
		if err := in.Plan.validate(); err != nil {
			return MissionCommand{}, err
		}
		plan := in.Plan.copy()
		out.MissionID, out.ExpectedRevision, out.Plan = in.MissionID, in.ExpectedRevision, &plan
	case MissionCreate:
		if in.MissionID != "" || in.ExpectedRevision != 0 || in.AssigneeEmployeeID != "" || in.Reason != "" || !canonicalID(in.UnitID) || in.AssetID != "" && !canonicalID(in.AssetID) {
			return MissionCommand{}, trip.ErrInvalid
		}
		if err := trip.ValidMissionText(in.Title); err != nil {
			return MissionCommand{}, err
		}
		window, err := trip.NewMissionWindow(in.StartsAt, in.EndsAt)
		if err != nil {
			return MissionCommand{}, err
		}
		out.UnitID, out.AssetID, out.Title, out.StartsAt, out.EndsAt = in.UnitID, in.AssetID, in.Title, window.From, window.To
	case MissionAssignCommand, MissionCancelCommand:
		if !canonicalID(in.MissionID) || in.ExpectedRevision < 1 || in.UnitID != "" || in.AssetID != "" || in.Title != "" || !in.StartsAt.IsZero() || !in.EndsAt.IsZero() {
			return MissionCommand{}, trip.ErrInvalid
		}
		out.MissionID, out.ExpectedRevision = in.MissionID, in.ExpectedRevision
		if in.Kind == MissionAssignCommand {
			if !canonicalID(in.AssigneeEmployeeID) || in.Reason != "" {
				return MissionCommand{}, trip.ErrInvalid
			}
			out.AssigneeEmployeeID = in.AssigneeEmployeeID
		} else {
			if in.AssigneeEmployeeID != "" {
				return MissionCommand{}, trip.ErrInvalid
			}
			if err := trip.ValidMissionText(in.Reason); err != nil {
				return MissionCommand{}, err
			}
			out.Reason = in.Reason
		}
	default:
		return MissionCommand{}, trip.ErrInvalid
	}
	return MissionCommand{value: out}, nil
}

// Snapshot returns an independent copy of the validated command.
func (c MissionCommand) Snapshot() (MissionCommandInput, error) {
	again, err := NewMissionCommand(c.value)
	if err != nil {
		return MissionCommandInput{}, err
	}
	return again.value, nil
}

// planDigest is the canonical PLAN intent; CREATE, ASSIGN and CANCEL keep the
// EMP-TRIP-008 form so stored receipts replay unchanged.
func planDigest(scope kernel.OrgScope, in MissionCommandInput) []any {
	p := in.Plan
	return []any{"trip.mission.plan.v1", scope.OrgID().String(), in.Kind, in.OperationID, in.MissionID, in.ExpectedRevision, p.DestinationText, p.DestinationUnitID, p.Reason,
		p.CostCentreID, p.Load, p.CargoDescription, p.Passengers}
}

// Digest is the canonical request hash bound to the organization.
func (c MissionCommand) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	instant := func(at time.Time) string {
		if at.IsZero() {
			return ""
		}
		return orgtime.FormatInstant(at)
	}
	fields := []any{"trip.mission.command.v1", scope.OrgID().String(), in.Kind, in.OperationID, in.MissionID, in.ExpectedRevision, in.UnitID, in.AssetID,
		in.Title, instant(in.StartsAt), instant(in.EndsAt), in.AssigneeEmployeeID, in.Reason}
	if in.Kind == MissionPlanCommand {
		fields = planDigest(scope, in)
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// MissionWarning is the EMP-FLEET-007 start decision of the new assignee at
// the planned start, returned only as a warning (B-BR-EMP-355).
type MissionWarning struct {
	private
	Code, Availability string
}

// MissionAck is the recorded result of one accepted company command.
type MissionAck struct {
	private
	MissionID          string
	Revision           int64
	State              trip.MissionState
	AssigneeEmployeeID string
	Warning            *MissionWarning
}

// MissionPlanView is a mission's current planning revision (EMP-TRIP-009).
type MissionPlanView struct {
	private
	Revision  int64
	Fields    MissionPlanFields
	PlannedAt time.Time
}

// MissionView is one stored mission (head) as a reader sees it, with its
// current plan when it has one.
type MissionView struct {
	private
	MissionID, UnitID, AssetID string
	Title                      string
	StartsAt, EndsAt           time.Time
	State                      trip.MissionState
	Revision                   int64
	AssigneeEmployeeID         string
	Answer                     trip.MissionResponse
	AnswerReason               *string
	Plan                       *MissionPlanView
	CreatedAt, UpdatedAt       time.Time
}

func (v MissionView) copy() MissionView {
	v.AnswerReason = copyPointer(v.AnswerReason)
	if v.Plan != nil {
		p := *v.Plan
		p.Fields = p.Fields.copy()
		v.Plan = &p
	}
	return v
}

// MissionFilter is the identifier keyset page of missions.
type MissionFilter struct {
	private
	AfterID string
	Limit   int
}

func (f MissionFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxMissionPage || f.AfterID != "" && !canonicalID(f.AfterID) {
		return trip.ErrInvalid
	}
	return nil
}

type MissionPage struct {
	private
	Missions []MissionView
	NextID   string
}

// MissionRepository owns the company commands and reads; every call proves
// current authority inside its own transaction.
type MissionRepository interface {
	CommandMission(context.Context, kernel.OrgScope, MissionActor, string, string, MissionCommand) (MissionAck, bool, error)
	// ListMissions pages the organization's missions, or with own=true the
	// missions whose current assignee is the member.
	ListMissions(context.Context, kernel.OrgScope, MissionActor, string, bool, MissionFilter) (MissionPage, error)
	ReadMission(context.Context, kernel.OrgScope, MissionActor, string, string) (MissionView, error)
}

type MissionService struct {
	Enabled    bool
	Repository MissionRepository
	NewID      func() (string, error)
}

// missionCurrent binds the completed request decision to this actor, policy
// and target before and after the repository call.
func missionCurrent(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, action MissionAction, target string) error {
	if ctx == nil {
		return ErrMissionUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return trip.ErrInvalid
	}
	policy := MissionPolicy(action)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrMissionUnauthenticated
	}
	return nil
}

func (s MissionService) guarded(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, action MissionAction, target string, call func() error) error {
	if !s.Enabled {
		return ErrMissionDisabled
	}
	if err := missionCurrent(ctx, scope, actor, decision, action, target); err != nil {
		return err
	}
	if absentPort(s.Repository) {
		return ErrUnavailable
	}
	err := call()
	if guard := missionCurrent(ctx, scope, actor, decision, action, target); guard != nil {
		return guard
	}
	return err
}

func validMissionInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

func validMissionAck(v MissionAck, in MissionCommandInput) bool {
	if !canonicalID(v.MissionID) || in.Kind != MissionCreate && v.MissionID != in.MissionID {
		return false
	}
	if v.Warning != nil && (in.Kind != MissionAssignCommand || v.Warning.Code != "ASSET_BUSY" && v.Warning.Code != "ASSET_UNAVAILABLE" || v.Warning.Availability == "") {
		return false
	}
	switch in.Kind {
	case MissionCreate:
		return v.Revision == 1 && v.State == trip.MissionDraft && v.AssigneeEmployeeID == ""
	case MissionPlanCommand:
		return v.Revision == in.ExpectedRevision+1 && v.State == trip.MissionPlanned && v.AssigneeEmployeeID == ""
	case MissionAssignCommand:
		return v.Revision == in.ExpectedRevision+1 && v.State == trip.MissionAssigned && v.AssigneeEmployeeID == in.AssigneeEmployeeID
	case MissionCancelCommand:
		return v.Revision == in.ExpectedRevision+1 && v.State == trip.MissionCancelled && (v.AssigneeEmployeeID == "" || canonicalID(v.AssigneeEmployeeID))
	}
	return false
}

// Command runs one company command under fresh trip.mission.manage: create
// on the organization, plan, assign and cancel on the mission.
func (s MissionService) Command(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, command MissionCommand) (MissionAck, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return MissionAck{}, false, err
	}
	target := in.MissionID
	if in.Kind == MissionCreate {
		target = scope.OrgID().String()
	}
	var v MissionAck
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, MissionManage, target, func() error {
		newID := ""
		if in.Kind == MissionCreate {
			if s.NewID == nil {
				return ErrUnavailable
			}
			id, err := s.NewID()
			if err != nil || !canonicalID(id) {
				return ErrUnavailable
			}
			newID = id
		}
		var err error
		v, replayed, err = s.Repository.CommandMission(ctx, scope, actor, decision.Subject().MembershipID(), newID, command)
		return err
	})
	if err != nil {
		return MissionAck{}, false, err
	}
	if !validMissionAck(v, in) {
		return MissionAck{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func validMissionView(v MissionView) bool {
	if !canonicalID(v.MissionID) || !canonicalID(v.UnitID) || v.AssetID != "" && !canonicalID(v.AssetID) || v.AssigneeEmployeeID != "" && !canonicalID(v.AssigneeEmployeeID) ||
		trip.ValidMissionText(v.Title) != nil || v.Revision < 1 || !validMissionInstant(v.CreatedAt) || !validMissionInstant(v.UpdatedAt) {
		return false
	}
	if _, err := trip.NewMissionWindow(v.StartsAt, v.EndsAt); err != nil {
		return false
	}
	if _, err := trip.ParseMissionState(string(v.State)); err != nil {
		return false
	}
	if v.Answer != "" {
		if _, err := trip.ParseMissionResponse(string(v.Answer)); err != nil || v.AssigneeEmployeeID == "" {
			return false
		}
	}
	if v.Plan != nil && (v.Plan.Revision < 2 || v.Plan.Revision > v.Revision || v.Plan.Fields.validate() != nil || !validMissionInstant(v.Plan.PlannedAt)) ||
		v.State == trip.MissionPlanned && v.Plan == nil {
		return false
	}
	return (v.Answer == trip.ResponseDecline) == (v.AnswerReason != nil)
}

func (s MissionService) list(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, own bool, filter MissionFilter) (MissionPage, error) {
	if err := filter.Validate(); err != nil {
		return MissionPage{}, err
	}
	action, target := MissionRead, scope.OrgID().String()
	if own {
		action, target = MissionOwnRead, decision.Subject().MembershipID()
	}
	var v MissionPage
	err := s.guarded(ctx, scope, actor, decision, action, target, func() error {
		var err error
		v, err = s.Repository.ListMissions(ctx, scope, actor, decision.Subject().MembershipID(), own, filter)
		return err
	})
	if err != nil {
		return MissionPage{}, err
	}
	if len(v.Missions) > filter.Limit || v.NextID != "" && (len(v.Missions) != filter.Limit || v.Missions[len(v.Missions)-1].MissionID != v.NextID) {
		return MissionPage{}, ErrUnavailable
	}
	prior := filter.AfterID
	out := MissionPage{Missions: make([]MissionView, 0, len(v.Missions)), NextID: v.NextID}
	for _, m := range v.Missions {
		if !validMissionView(m) || m.MissionID <= prior {
			return MissionPage{}, ErrUnavailable
		}
		out.Missions = append(out.Missions, m.copy())
		prior = m.MissionID
	}
	return out, nil
}

// List pages the organization's missions (organization-level read).
func (s MissionService) List(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, filter MissionFilter) (MissionPage, error) {
	return s.list(ctx, scope, actor, decision, false, filter)
}

// Mine pages the missions currently assigned to the requesting member.
func (s MissionService) Mine(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, filter MissionFilter) (MissionPage, error) {
	return s.list(ctx, scope, actor, decision, true, filter)
}

// Read reads one mission in scope.
func (s MissionService) Read(ctx context.Context, scope kernel.OrgScope, actor MissionActor, decision access.Decision, mission string) (MissionView, error) {
	if !canonicalID(mission) {
		return MissionView{}, ErrNotFound
	}
	var v MissionView
	err := s.guarded(ctx, scope, actor, decision, MissionRead, mission, func() error {
		var err error
		v, err = s.Repository.ReadMission(ctx, scope, actor, decision.Subject().MembershipID(), mission)
		return err
	})
	if err != nil {
		return MissionView{}, err
	}
	if v.MissionID != mission || !validMissionView(v) {
		return MissionView{}, ErrUnavailable
	}
	return v.copy(), nil
}

// ---- trip.mission.respond (EDR-229) ----

// Exact schema 1 payload.
var respondKeys = []string{"response", "reason"}

// RespondInput is a parsed trip.mission.respond.
type RespondInput struct {
	private
	OperationID, PayloadDigest, MissionID, AuthorMembershipID string
	ExpectedRevision                                          int64
	Response                                                  trip.MissionResponse
	Reason                                                    *string
}

type RespondCommand struct {
	private
	value RespondInput
	valid bool
}

func (c RespondCommand) Snapshot() (RespondInput, error) {
	if !c.valid {
		return RespondInput{}, ErrUnavailable
	}
	v := c.value
	v.Reason = copyPointer(v.Reason)
	return v, nil
}

// ParseRespond decodes one trip.mission.respond schema 1 operation; every
// refusal is a final ErrMalformed (no future form exists).
func ParseRespond(op protocol.Operation) (RespondCommand, error) {
	v := op.Snapshot()
	if v.Type != MissionRespondOperationType || !op.SupportsSchema() || v.Entity.Type != MissionEntityType || !canonicalID(v.Entity.ID) ||
		v.Entity.ExpectedRevision == nil || *v.Entity.ExpectedRevision < 1 || len(v.DependsOn) != 0 || len(v.SoftReferences) != 0 || len(v.Attachments) != 0 {
		return RespondCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), respondKeys)
	if !ok {
		return RespondCommand{}, ErrMalformed
	}
	raw, okResponse := text(fields, "response")
	reason, okReason := optional(fields, "reason")
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if !okResponse || !okReason || err != nil {
		return RespondCommand{}, ErrMalformed
	}
	if _, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds}); err != nil {
		return RespondCommand{}, ErrMalformed
	}
	response, err := trip.ParseMissionResponse(raw)
	if err != nil {
		return RespondCommand{}, ErrMalformed
	}
	if response == trip.ResponseAccept && reason != nil || response == trip.ResponseDecline && (reason == nil || trip.ValidMissionText(*reason) != nil) {
		return RespondCommand{}, ErrMalformed
	}
	return RespondCommand{valid: true, value: RespondInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), MissionID: v.Entity.ID, AuthorMembershipID: v.MembershipID,
		ExpectedRevision: *v.Entity.ExpectedRevision, Response: response, Reason: reason}}, nil
}

// MissionTarget names the server-resolved facts of a response: the stored
// mission's current assignee membership, unit and asset, and (receipt reads)
// the member whose stored response it is. An empty MissionID is the root.
type MissionTarget struct {
	private
	MissionID, AssigneeMembershipID, ResponderMembershipID, UnitID, AssetID string
}

// MissionResponseAuthority proves current trip.trip.start on the enrolled
// device over the mission as the member's own record.
type MissionResponseAuthority interface {
	CheckMissionResponseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, MissionTarget, MissionAction) error
}

// MissionRecord is the stored head with its current assignee membership.
type MissionRecord struct {
	private
	View                 MissionView
	AssigneeMembershipID string
}

// Domain restores the stored head through the pure EMP-TRIP-008 rules.
func (r MissionRecord) Domain(scope kernel.OrgScope) (trip.Mission, error) {
	v := r.View
	s := trip.MissionSnapshot{Title: v.Title, Window: trip.MissionWindow{From: v.StartsAt, To: v.EndsAt}, State: v.State, Revision: v.Revision}
	var err error
	if s.Reference, err = trip.NewMissionRef(scope, v.MissionID); err != nil {
		return trip.Mission{}, ErrUnavailable
	}
	if s.Unit, err = trip.NewUnitRef(scope, v.UnitID); err != nil {
		return trip.Mission{}, ErrUnavailable
	}
	if v.AssetID != "" {
		asset, err := trip.NewMissionAssetRef(scope, v.AssetID)
		if err != nil {
			return trip.Mission{}, ErrUnavailable
		}
		s.Asset = &asset
	}
	if v.AssigneeEmployeeID != "" || r.AssigneeMembershipID != "" {
		var a trip.MissionAssignee
		if a.Employee, err = trip.NewEmployeeRef(scope, v.AssigneeEmployeeID); err != nil {
			return trip.Mission{}, ErrUnavailable
		}
		if a.Member, err = trip.NewMembershipRef(scope, r.AssigneeMembershipID); err != nil {
			return trip.Mission{}, ErrUnavailable
		}
		s.Assignee = &a
		if v.Answer != "" {
			s.Answer = &trip.MissionAnswer{Kind: v.Answer, Member: a.Member, Reason: copyPointer(v.AnswerReason)}
		}
	}
	m, err := trip.RestoreMission(scope, s)
	if err != nil {
		return trip.Mission{}, ErrUnavailable
	}
	return m, nil
}

// MissionResponseRecord is one stored response transition.
type MissionResponseRecord struct {
	private
	MissionID, OperationID, ActorMembershipID, ActorAccountID, EnrollmentID string
	Revision                                                                int64
	Response                                                                trip.MissionResponse
	Reason                                                                  *string
}

// MissionResponseStore reads and records responses in the intake
// transaction.
type MissionResponseStore interface {
	// MissionTx reads the stored head, locking it when lock is true;
	// ErrNotFound when no mission of the scope has the identity.
	MissionTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, bool) (MissionRecord, error)
	// RecordResponseTx appends the response transition and moves the head.
	RecordResponseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, RespondCommand, MissionRecord, trip.Mission) error
	// ResponseTx reads the stored response of an operation; ErrNotFound when none.
	ResponseTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (MissionResponseRecord, error)
}
