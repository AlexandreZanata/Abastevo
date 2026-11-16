package application

// EMP-CHECK-003 damage and incident reports (B-BR-EMP-369..374,
// EDR-238..241): the signed check.incident.report codec and owner ports, and
// the company handling and read boundary. Repositories prove current
// authority inside their own transaction; the service binds the completed
// request decision before and after every repository call.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/jackc/pgx/v5"
)

const (
	// ReportOperationType records one damage or incident (EDR-238).
	ReportOperationType = "check.incident.report"
	ReportEntityType    = "incident_report"
	// ReportPermission reports (S); ManagePermission handles (O, v5).
	ReportPermission = "check.incident.report"
	ManagePermission = "check.incident.manage"
	// MaxReportPage mirrors the shared private page bound (httpapi.MaxLimit).
	MaxReportPage = 100
)

var (
	// ErrMalformed is a final REJECTED MALFORMED operation.
	ErrMalformed = errors.New("incident: malformed operation")
	// ErrUnsupported keeps an envelope part (an attachment declaration or a
	// soft reference) unavailable until its owner exists.
	ErrUnsupported = errors.New("incident: operation part unavailable")
	// ErrSelfApproval is the reporter resolving or dismissing a blocking report.
	ErrSelfApproval = errors.New("incident: self approval")
)

// ---- signed check.incident.report (B-BR-EMP-369..371) ----

// ReportAction selects the intake write or the receipt read policy.
type ReportAction uint8

const (
	ReportWrite ReportAction = iota + 1
	ReportReceipt
)

// ReportPolicy is check.incident.report: an operational write for a new
// report and a read for a receipt re-read (the member's own record).
func ReportPolicy(action ReportAction) access.Policy {
	switch action {
	case ReportWrite:
		return access.Policy{Permission: ReportPermission, Capability: "organizations", Effect: access.OperationalWrite}
	case ReportReceipt:
		return access.Policy{Permission: ReportPermission, Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Exact schema 1 payload (B-BR-EMP-369).
var reportKeys = []string{"report_type", "incident_kind", "asset_id", "trip_id", "description", "location", "blocks_use"}

// ReportInput is a parsed check.incident.report.
type ReportInput struct {
	private
	OperationID, PayloadDigest, ReportID, AuthorMembershipID, OwnerAccountID string
	AssetID, TripID                                                          string
	Report                                                                   incident.Report
	Declared                                                                 orgtime.Declaration
	DependsOn                                                                []string
}

type ReportCommand struct {
	private
	value ReportInput
	valid bool
}

func copyText(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

// Snapshot returns an independent copy of the parsed operation.
func (c ReportCommand) Snapshot() (ReportInput, error) {
	if !c.valid {
		return ReportInput{}, ErrUnavailable
	}
	v := c.value
	v.Report.Location = copyText(v.Report.Location)
	v.DependsOn = slices.Clone(v.DependsOn)
	return v, nil
}

func payloadFields(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != len(reportKeys) {
		return nil, false
	}
	for _, key := range reportKeys {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	return fields, true
}

func isNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func textField(fields map[string]json.RawMessage, key string) (string, bool) {
	var v string
	raw := fields[key]
	if isNull(raw) || json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}

func optionalField(fields map[string]json.RawMessage, key string) (*string, bool) {
	if isNull(fields[key]) {
		return nil, true
	}
	v, ok := textField(fields, key)
	return &v, ok
}

// ParseReport decodes one check.incident.report schema 1 operation.
// ErrMalformed is final; ErrUnsupported keeps an attachment declaration or a
// soft reference unavailable (EDR-238/239).
func ParseReport(op protocol.Operation) (ReportCommand, error) {
	v := op.Snapshot()
	if v.Type != ReportOperationType || !op.SupportsSchema() || v.Entity.Type != ReportEntityType || v.Entity.ExpectedRevision != nil || !incident.CanonicalID(v.Entity.ID) || len(v.DependsOn) > 1 {
		return ReportCommand{}, ErrMalformed
	}
	fields, ok := payloadFields(op.PayloadBytes())
	if !ok {
		return ReportCommand{}, ErrMalformed
	}
	kind := ""
	if !isNull(fields["incident_kind"]) {
		raw, ok := textField(fields, "incident_kind")
		if !ok {
			return ReportCommand{}, ErrMalformed
		}
		kind = raw
		if kind == "" {
			// An empty kind is never the absent kind.
			return ReportCommand{}, ErrMalformed
		}
	}
	reportType, okType := textField(fields, "report_type")
	asset, okAsset := textField(fields, "asset_id")
	trip, okTrip := optionalField(fields, "trip_id")
	description, okDescription := textField(fields, "description")
	location, okLocation := optionalField(fields, "location")
	var blocks bool
	okBlocks := !isNull(fields["blocks_use"]) && json.Unmarshal(fields["blocks_use"], &blocks) == nil
	if !okType || !okAsset || !okTrip || !okDescription || !okLocation || !okBlocks || !incident.CanonicalID(asset) || trip != nil && !incident.CanonicalID(*trip) {
		return ReportCommand{}, ErrMalformed
	}
	report, err := incident.NewReport(incident.Draft{Type: incident.ReportType(reportType), Kind: incident.Kind(kind), Description: description, Location: location, BlocksUse: blocks})
	if err != nil {
		return ReportCommand{}, ErrMalformed
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return ReportCommand{}, ErrMalformed
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil {
		return ReportCommand{}, ErrMalformed
	}
	if len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return ReportCommand{}, ErrUnsupported
	}
	in := ReportInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), ReportID: v.Entity.ID, AuthorMembershipID: v.MembershipID, OwnerAccountID: v.OwnerAccountID,
		AssetID: asset, Report: report, Declared: declared, DependsOn: slices.Clone(v.DependsOn)}
	if trip != nil {
		in.TripID = *trip
	}
	return ReportCommand{valid: true, value: in}, nil
}

// ReportDisposition is the EDR-118 history answer for a new report.
type ReportDisposition string

const (
	ReportApply           ReportDisposition = "APPLY"
	ReportPermissionMoved ReportDisposition = "HOLD_PERMISSION_CHANGED"
	ReportDisposalEnded   ReportDisposition = "HOLD_DISPOSAL_ENDED"
)

// ReportTarget names server-resolved record facts; an empty ReportID is the
// root selector, a ReportID without an asset a report not stored here.
type ReportTarget struct {
	private
	ReportID, AssetID, UnitID, CostCentreID, AuthorMembershipID string
}

// ReportAuthority is the signed-intake authority (organization adapters):
// the current enrolled device, current coverage over the record and the
// EDR-118 history classification on the asset at the declared instant.
type ReportAuthority interface {
	CheckReportTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, ReportTarget, ReportAction) error
	ClassifyReportTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, time.Time) (ReportDisposition, error)
}

// AssetLocks takes the organization root and asset row locks (the fleet
// lifecycle lookup the trip owners use) and returns the disposal instant.
type AssetLocks interface {
	MeterAssetDisposalTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (*time.Time, error)
}

// Allocations is the FLEET-005 snapshot port at the declared instant.
type Allocations interface {
	UnitAndCostCentreAtDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (fleet.AllocationSnapshot, error)
}

// Trips reads a stored trip (EMP-TRIP-002 store).
type Trips interface {
	ReadTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (tripapp.Record, error)
}

// ReportFacts are the server facts a new report stores.
type ReportFacts struct {
	private
	EmployeeID, UnitID, CostCentreID string
}

// ReportRecord is one stored report.
type ReportRecord struct {
	private
	ID, OperationID, PayloadDigest, AssetID, UnitID, CostCentreID, TripID string
	ReporterAccountID, ReporterMembershipID, ReporterEmployeeID           string
	EnrollmentID                                                          string
	Report                                                                incident.Report
	Declared                                                              orgtime.Declaration
	ReceivedAt, UpdatedAt                                                 time.Time
}

// ReportStore persists reports in the intake transaction (EDR-238).
type ReportStore interface {
	ReportOperationTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	ReporterEmployeeTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	AssetStateTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	InsertReportTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, ReportCommand, ReportFacts) error
	ReadReportTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (ReportRecord, error)
}

// TripWindow checks a named trip against the report (B-BR-EMP-371): the
// same asset (else ErrMalformed), the reporter as the member responsible for
// the trip at the declared instant (its author, or the receiver of a
// handover, EMP-TRIP-012; else ErrForbiddenTrip) and the declared instant
// inside the trip.
func TripWindow(record tripapp.Record, in ReportInput) error {
	if record.ID != in.TripID {
		return ErrUnavailable
	}
	if record.AssetID != in.AssetID {
		return ErrMalformed
	}
	at := in.Declared.OccurredAt
	if responsible, _ := record.ResponsibleAt(at); responsible != in.AuthorMembershipID {
		return ErrForbiddenTrip
	}
	if at.Before(record.Declared.OccurredAt) || record.Finish != nil && at.After(record.Finish.Declared.OccurredAt) {
		return ErrMalformed
	}
	return nil
}

// ErrForbiddenTrip is a report naming another member's trip.
var ErrForbiddenTrip = errors.New("incident: another member's trip")

// ---- company handling and reads (B-BR-EMP-373/374) ----

// IncidentActor is the current company account and its web proof.
type IncidentActor struct {
	private
	AccountID, SessionID string
}

func (a IncidentActor) Validate() error {
	if !incident.CanonicalID(a.AccountID) || !incident.CanonicalID(a.SessionID) {
		return ErrUnauthenticated
	}
	return nil
}

// IncidentAction is the closed set of company policies (EDR-241).
type IncidentAction uint8

const (
	// IncidentQueue lists the organization's reports (ORG grants only).
	IncidentQueue IncidentAction = iota + 1
	// IncidentRead reads one report under the stored structure.
	IncidentRead
	// IncidentDecide applies one handling step (fresh proof).
	IncidentDecide
	// IncidentOwn lists the member's own reports.
	IncidentOwn
)

// IncidentPolicy returns the fixed policy of a company action.
func IncidentPolicy(action IncidentAction) access.Policy {
	switch action {
	case IncidentQueue, IncidentRead:
		return access.Policy{Permission: ManagePermission, Capability: "organizations", Effect: access.Read}
	case IncidentDecide:
		return access.Policy{Permission: ManagePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case IncidentOwn:
		return access.Policy{Permission: ReportPermission, Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Handler is the actual handler resolved at receipt.
type Handler struct {
	private
	AccountID, MembershipID, EmployeeID string
}

// IncidentAuthority retains the company account/proof → capability → root →
// membership/role order in the caller's transaction and returns the handler.
type IncidentAuthority interface {
	CheckIncidentTx(context.Context, pgx.Tx, kernel.OrgScope, IncidentActor, string, string, IncidentAction) (Handler, error)
}

// DecisionInput is one handling command.
type DecisionInput struct {
	private
	OperationID, ReportID string
	ExpectedRevision      int64
	Action                incident.Action
	Note                  *string
	DuplicateOf           string
}

// DecisionCommand is a validated handling command.
type DecisionCommand struct {
	private
	value DecisionInput
}

// NewDecisionCommand validates the command shape with the pure note and
// duplicate policy (B-BR-EMP-373); state, revision and separation are
// decided against the stored report.
func NewDecisionCommand(in DecisionInput) (DecisionCommand, error) {
	if !incident.CanonicalID(in.OperationID) || !incident.CanonicalID(in.ReportID) || in.ExpectedRevision < 1 {
		return DecisionCommand{}, incident.ErrInvalid
	}
	action, err := incident.ParseAction(string(in.Action))
	if err != nil {
		return DecisionCommand{}, err
	}
	if action.NeedsNote() != (in.Note != nil) || in.Note != nil && !incident.ValidText(*in.Note, incident.MaxNoteScalars) {
		return DecisionCommand{}, incident.ErrInvalid
	}
	if in.DuplicateOf != "" && (action != incident.Dismiss || !incident.CanonicalID(in.DuplicateOf) || in.DuplicateOf == in.ReportID) {
		return DecisionCommand{}, incident.ErrInvalid
	}
	in.Note = copyText(in.Note)
	return DecisionCommand{value: in}, nil
}

func (c DecisionCommand) Snapshot() (DecisionInput, error) {
	checked, err := NewDecisionCommand(c.value)
	return checked.value, err
}

// Digest binds every field; the note travels only inside it.
func (c DecisionCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	note := ""
	if v.Note != nil {
		note = "+" + *v.Note
	}
	raw, err := json.Marshal([]any{1, "check.incident.decide", scope.OrgID().String(), v.OperationID, v.ReportID, v.ExpectedRevision, v.Action, note, v.DuplicateOf})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// DecisionResult is rebuilt from the stored history row.
type DecisionResult struct {
	private
	ReportID   string
	Step       incident.Step
	RecordedAt time.Time
}

// ReportSummary is one queue or own-list row: no description or location.
type ReportSummary struct {
	private
	ReportID, AssetID, TripID, ReporterMembershipID string
	Type                                            incident.ReportType
	Kind                                            incident.Kind
	BlocksUse                                       bool
	State                                           incident.State
	Revision                                        int64
	OccurredAt, ReceivedAt, UpdatedAt               time.Time
}

// TransitionView is one handling history row.
type TransitionView struct {
	private
	Revision          int64
	Action            incident.Action
	From, To          incident.State
	Note              *string
	DuplicateOf       string
	ActorMembershipID string
	RecordedAt        time.Time
}

// AttachmentView is one linked document with its own storage state (EDR-239).
type AttachmentView struct {
	private
	DocumentID string
	Storage    string
	LinkedAt   time.Time
}

// ReportView is one report with its text, history and linked documents.
type ReportView struct {
	private
	Summary     ReportSummary
	Description string
	Location    *string
	UnitID      string
	CostCentre  string
	History     []TransitionView
	Attachments []AttachmentView
}

// OwnReport is one own-list row: the summary with the reporter's own text.
type OwnReport struct {
	private
	Summary     ReportSummary
	Description string
	Location    *string
}

// ReportFilter pages reports by identifier.
type ReportFilter struct {
	private
	AfterID string
	Limit   int
}

func (f ReportFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxReportPage || f.AfterID != "" && !incident.CanonicalID(f.AfterID) {
		return incident.ErrInvalid
	}
	return nil
}

type ReportPage struct {
	private
	Items  []ReportSummary
	NextID string
}

type OwnReportPage struct {
	private
	Items  []OwnReport
	NextID string
}

// IncidentRepository is the company owner; it owns the transaction, the
// in-transaction company authority and the audit event.
type IncidentRepository interface {
	DecideReport(context.Context, kernel.OrgScope, IncidentActor, string, DecisionCommand) (DecisionResult, bool, error)
	ReadReport(context.Context, kernel.OrgScope, IncidentActor, string, string) (ReportView, error)
	ListReports(context.Context, kernel.OrgScope, IncidentActor, string, ReportFilter) (ReportPage, error)
	ListOwnReports(context.Context, kernel.OrgScope, IncidentActor, string, ReportFilter) (OwnReportPage, error)
}

type IncidentService struct {
	Enabled    bool
	Repository IncidentRepository
}

// bound binds the completed request decision to this actor, policy and
// target before and after the repository call.
func bound(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, action IncidentAction, target string) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return incident.ErrInvalid
	}
	policy := IncidentPolicy(action)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func (s IncidentService) guarded(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, action IncidentAction, target string, call func() error) error {
	if !s.Enabled {
		return ErrSurfaceDisabled
	}
	if err := bound(ctx, scope, actor, decision, action, target); err != nil {
		return err
	}
	if unavailable(s.Repository) {
		return ErrUnavailable
	}
	err := call()
	if guard := bound(ctx, scope, actor, decision, action, target); guard != nil {
		return guard
	}
	return err
}

func validSummary(v ReportSummary) bool {
	if _, err := incident.ParseReportType(string(v.Type)); err != nil {
		return false
	}
	if _, err := incident.ParseState(string(v.State)); err != nil {
		return false
	}
	if (v.Type == incident.Incident) != (v.Kind != "") || v.Kind != "" && func() bool { _, err := incident.ParseKind(string(v.Kind)); return err != nil }() {
		return false
	}
	return incident.CanonicalID(v.ReportID) && incident.CanonicalID(v.AssetID) && (v.TripID == "" || incident.CanonicalID(v.TripID)) && incident.CanonicalID(v.ReporterMembershipID) &&
		v.Revision >= 1 && v.Revision <= incident.MaxRevision && (v.State == incident.Reported) == (v.Revision == 1) && validInstant(v.OccurredAt) && validInstant(v.ReceivedAt) && validInstant(v.UpdatedAt)
}

func validPage(ids []string, next string, limit int, after string) bool {
	if len(ids) > limit || next != "" && (len(ids) != limit || ids[len(ids)-1] != next) {
		return false
	}
	prior := after
	for _, id := range ids {
		if id <= prior {
			return false
		}
		prior = id
	}
	return true
}

// Decide applies one handling step under fresh check.incident.manage.
func (s IncidentService) Decide(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, command DecisionCommand) (DecisionResult, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return DecisionResult{}, false, err
	}
	var v DecisionResult
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, IncidentDecide, in.ReportID, func() error {
		var err error
		v, replayed, err = s.Repository.DecideReport(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return DecisionResult{}, false, err
	}
	if v.ReportID != in.ReportID || v.Step.Action != in.Action || v.Step.To != in.Action.Target() || !incident.Allowed(v.Step.From, v.Step.Action) || v.Step.Revision < 2 ||
		v.Step.Revision > incident.MaxRevision || v.Step.Revision != in.ExpectedRevision+1 || !validInstant(v.RecordedAt) {
		return DecisionResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

// List pages the organization's reports (an ORG grant only, EDR-241).
func (s IncidentService) List(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, filter ReportFilter) (ReportPage, error) {
	if err := filter.Validate(); err != nil {
		return ReportPage{}, err
	}
	var v ReportPage
	err := s.guarded(ctx, scope, actor, decision, IncidentQueue, scope.OrgID().String(), func() error {
		var err error
		v, err = s.Repository.ListReports(ctx, scope, actor, decision.Subject().MembershipID(), filter)
		return err
	})
	if err != nil {
		return ReportPage{}, err
	}
	ids := make([]string, 0, len(v.Items))
	for _, item := range v.Items {
		if !validSummary(item) {
			return ReportPage{}, ErrUnavailable
		}
		ids = append(ids, item.ReportID)
	}
	if !validPage(ids, v.NextID, filter.Limit, filter.AfterID) {
		return ReportPage{}, ErrUnavailable
	}
	return ReportPage{Items: slices.Clone(v.Items), NextID: v.NextID}, nil
}

// Mine pages the member's own reports under check.incident.report.
func (s IncidentService) Mine(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, filter ReportFilter) (OwnReportPage, error) {
	if err := filter.Validate(); err != nil {
		return OwnReportPage{}, err
	}
	member := decision.Subject().MembershipID()
	var v OwnReportPage
	err := s.guarded(ctx, scope, actor, decision, IncidentOwn, member, func() error {
		var err error
		v, err = s.Repository.ListOwnReports(ctx, scope, actor, member, filter)
		return err
	})
	if err != nil {
		return OwnReportPage{}, err
	}
	ids := make([]string, 0, len(v.Items))
	out := make([]OwnReport, 0, len(v.Items))
	for _, item := range v.Items {
		if !validSummary(item.Summary) || item.Summary.ReporterMembershipID != member || !incident.ValidText(item.Description, incident.MaxDescriptionScalars) ||
			item.Location != nil && !incident.ValidText(*item.Location, incident.MaxLocationScalars) {
			return OwnReportPage{}, ErrUnavailable
		}
		ids = append(ids, item.Summary.ReportID)
		item.Location = copyText(item.Location)
		out = append(out, item)
	}
	if !validPage(ids, v.NextID, filter.Limit, filter.AfterID) {
		return OwnReportPage{}, ErrUnavailable
	}
	return OwnReportPage{Items: out, NextID: v.NextID}, nil
}

// Read reads one report, its history and its linked documents' states.
func (s IncidentService) Read(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, reportID string) (ReportView, error) {
	if !incident.CanonicalID(reportID) {
		return ReportView{}, ErrNotFound
	}
	var v ReportView
	err := s.guarded(ctx, scope, actor, decision, IncidentRead, reportID, func() error {
		var err error
		v, err = s.Repository.ReadReport(ctx, scope, actor, decision.Subject().MembershipID(), reportID)
		return err
	})
	if err != nil {
		return ReportView{}, err
	}
	if v.Summary.ReportID != reportID || !validSummary(v.Summary) || !incident.ValidText(v.Description, incident.MaxDescriptionScalars) ||
		v.Location != nil && !incident.ValidText(*v.Location, incident.MaxLocationScalars) || int64(len(v.History)) != v.Summary.Revision-1 {
		return ReportView{}, ErrUnavailable
	}
	state := incident.Reported
	for i, h := range v.History {
		if h.Revision != int64(i+2) || h.From != state || h.To != h.Action.Target() || !incident.Allowed(h.From, h.Action) || !incident.CanonicalID(h.ActorMembershipID) ||
			h.Action.NeedsNote() != (h.Note != nil) || h.DuplicateOf != "" && h.Action != incident.Dismiss || !validInstant(h.RecordedAt) {
			return ReportView{}, ErrUnavailable
		}
		state = h.To
	}
	if state != v.Summary.State {
		return ReportView{}, ErrUnavailable
	}
	for _, a := range v.Attachments {
		if !incident.CanonicalID(a.DocumentID) || a.Storage == "" || !validInstant(a.LinkedAt) {
			return ReportView{}, ErrUnavailable
		}
	}
	return v, nil
}
