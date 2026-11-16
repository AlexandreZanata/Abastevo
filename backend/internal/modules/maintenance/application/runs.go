package application

// EMP-CHECK-002 checklist runs (B-BR-EMP-439..442, EDR-284/285): the signed
// check.run.complete codec and owner ports, and the company run read and
// release boundary.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
)

const (
	// RunOperationType completes one checklist run (EDR-284).
	RunOperationType = "check.run.complete"
	RunEntityType    = "checklist_run"
	// ExecutePermission fills runs (S: own records and assigned assets).
	ExecutePermission = "check.checklist.execute"
	// MaxRunRevision: 1 at receipt, 2 after a release.
	MaxRunRevision = 2
	// MaxReleaseReasonScalars bounds a release reason (maintenance §10).
	MaxReleaseReasonScalars = 500
	// MaxRunCounter bounds a template version or policy revision (SQL INTEGER).
	MaxRunCounter = 2147483647
)

// ---- signed check.run.complete (B-BR-EMP-439..441) ----

// RunAction selects the intake write or the receipt read policy.
type RunAction uint8

const (
	RunWrite RunAction = iota + 1
	RunReceipt
)

// RunPolicy is check.checklist.execute: an operational write for a new run
// and a read for a receipt re-read (the member's own record).
func RunPolicy(action RunAction) access.Policy {
	switch action {
	case RunWrite:
		return access.Policy{Permission: ExecutePermission, Capability: "organizations", Effect: access.OperationalWrite}
	case RunReceipt:
		return access.Policy{Permission: ExecutePermission, Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// Exact schema 1 payload (B-BR-EMP-439).
var (
	runKeys    = []string{"asset_id", "context", "template_id", "template_version", "policy_revision", "started_at", "answers", "device_outcome", "acknowledged"}
	answerKeys = []string{"answer", "note", "photo"}
)

// RunInput is a parsed check.run.complete.
type RunInput struct {
	private
	OperationID, PayloadDigest, RunID, AuthorMembershipID, OwnerAccountID string
	AssetID, TemplateID                                                   string
	Context                                                               checklist.Context
	Version, PolicyRevision                                               int64
	StartedAt                                                             time.Time
	Responses                                                             []checklist.Response
	DeviceOutcome                                                         checklist.Outcome
	Acknowledged                                                          bool
	Declared                                                              orgtime.Declaration
}

type RunCommand struct {
	private
	value RunInput
	valid bool
}

// Snapshot returns an independent copy of the parsed operation.
func (c RunCommand) Snapshot() (RunInput, error) {
	if !c.valid {
		return RunInput{}, ErrUnavailable
	}
	v := c.value
	v.Responses = slices.Clone(v.Responses)
	return v, nil
}

func exactObject(raw json.RawMessage, keys []string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != len(keys) {
		return nil, false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	return fields, true
}

func counter(raw json.RawMessage) (int64, bool) {
	var v int64
	if isNull(raw) || json.Unmarshal(raw, &v) != nil || v < 1 || v > MaxRunCounter {
		return 0, false
	}
	return v, true
}

func flagField(raw json.RawMessage) (bool, bool) {
	var v bool
	if isNull(raw) || json.Unmarshal(raw, &v) != nil {
		return false, false
	}
	return v, true
}

// canonicalInstant reads a canonical UTC instant with at most microsecond
// precision, the form the run stores.
func canonicalInstant(raw string) (time.Time, bool) {
	at, err := orgtime.ParseInstant(raw)
	if err != nil || orgtime.FormatInstant(at) != raw || !at.Equal(at.Truncate(time.Microsecond)) {
		return time.Time{}, false
	}
	return at, true
}

func parseAnswers(raw json.RawMessage) ([]checklist.Response, bool, bool) {
	var items []json.RawMessage
	if isNull(raw) || json.Unmarshal(raw, &items) != nil || len(items) < 1 || len(items) > checklist.MaxItems {
		return nil, false, false
	}
	out := make([]checklist.Response, 0, len(items))
	photo := false
	for _, item := range items {
		fields, ok := exactObject(item, answerKeys)
		if !ok {
			return nil, false, false
		}
		var answer string
		if isNull(fields["answer"]) || json.Unmarshal(fields["answer"], &answer) != nil {
			return nil, false, false
		}
		switch checklist.Answer(answer) {
		case checklist.AnswerOK, checklist.AnswerNotOK, checklist.AnswerNotApplicable:
		default:
			return nil, false, false
		}
		note, ok := optionalField(fields, "note")
		if !ok || note != nil && !checklist.ValidNote(*note) {
			return nil, false, false
		}
		declared, ok := flagField(fields["photo"])
		if !ok || declared && checklist.Answer(answer) != checklist.AnswerNotOK {
			return nil, false, false
		}
		photo = photo || declared
		r := checklist.Response{Answer: checklist.Answer(answer), Photo: declared}
		if note != nil {
			r.Note = *note
		}
		out = append(out, r)
	}
	return out, photo, true
}

// ParseRun decodes one check.run.complete schema 1 operation. ErrMalformed
// is final; ErrUnsupported keeps a photo declaration, an attachment or a soft
// reference unavailable until the run-photo channel exists (EDR-284/286).
func ParseRun(op protocol.Operation) (RunCommand, error) {
	v := op.Snapshot()
	if v.Type != RunOperationType || !op.SupportsSchema() || v.Entity.Type != RunEntityType || v.Entity.ExpectedRevision != nil || !checklist.CanonicalID(v.Entity.ID) || len(v.DependsOn) != 0 {
		return RunCommand{}, ErrMalformed
	}
	fields, ok := exactObject(op.PayloadBytes(), runKeys)
	if !ok {
		return RunCommand{}, ErrMalformed
	}
	asset, okAsset := textField(fields, "asset_id")
	template, okTemplate := textField(fields, "template_id")
	rawContext, okContext := textField(fields, "context")
	rawStarted, okStarted := textField(fields, "started_at")
	rawOutcome, okOutcome := textField(fields, "device_outcome")
	version, okVersion := counter(fields["template_version"])
	policy, okPolicy := counter(fields["policy_revision"])
	acknowledged, okAck := flagField(fields["acknowledged"])
	responses, photo, okAnswers := parseAnswers(fields["answers"])
	if !okAsset || !okTemplate || !okContext || !okStarted || !okOutcome || !okVersion || !okPolicy || !okAck || !okAnswers ||
		!checklist.CanonicalID(asset) || !checklist.CanonicalID(template) {
		return RunCommand{}, ErrMalformed
	}
	context, err := checklist.ParseContext(rawContext)
	if err != nil {
		return RunCommand{}, ErrMalformed
	}
	outcome, err := checklist.ParseOutcome(rawOutcome)
	if err != nil || acknowledged && outcome != checklist.CompletedWithIssues {
		return RunCommand{}, ErrMalformed
	}
	started, ok := canonicalInstant(rawStarted)
	if !ok {
		return RunCommand{}, ErrMalformed
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return RunCommand{}, ErrMalformed
	}
	declared, err := orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds})
	if err != nil || started.After(declared.OccurredAt) {
		return RunCommand{}, ErrMalformed
	}
	if photo || len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return RunCommand{}, ErrUnsupported
	}
	return RunCommand{valid: true, value: RunInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), RunID: v.Entity.ID, AuthorMembershipID: v.MembershipID,
		OwnerAccountID: v.OwnerAccountID, AssetID: asset, TemplateID: template, Context: context, Version: version, PolicyRevision: policy, StartedAt: started,
		Responses: responses, DeviceOutcome: outcome, Acknowledged: acknowledged, Declared: declared}}, nil
}

// RunTarget names server-resolved record facts; an empty RunID is the root
// selector, a RunID without an asset a run not stored here.
type RunTarget struct {
	private
	RunID, AssetID, UnitID, CostCentreID, AuthorMembershipID string
}

// RunAuthority is the signed-intake authority for check.checklist.execute
// (organization adapters; the EDR-177 history rule).
type RunAuthority interface {
	CheckRunTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, RunTarget, RunAction) error
	ClassifyRunTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, time.Time) (ReportDisposition, error)
}

// RunFacts are the server facts a new run stores besides the recheck.
type RunFacts struct {
	private
	EmployeeID, UnitID, CostCentreID string
}

// RunTemplate is the stored template facts the recheck reads, with the
// asset's category.
type RunTemplate struct {
	private
	Facts      checklist.RunFacts
	CategoryID string
}

// RunRecord is one stored run.
type RunRecord struct {
	private
	ID, OperationID, PayloadDigest, AssetID, UnitID, CostCentreID, TemplateID string
	AuthorAccountID, AuthorMembershipID, AuthorEmployeeID, EnrollmentID       string
	Context                                                                   checklist.Context
	Version, PolicyRevision, ServerPolicyRevision, Revision                   int64
	StartedAt, ReceivedAt                                                     time.Time
	Declared                                                                  orgtime.Declaration
	Responses                                                                 []checklist.Response
	DeviceOutcome, RecheckOutcome, Outcome                                    checklist.Outcome
	Acknowledged, Superseded, Differs                                         bool
	Issues, Critical                                                          []int
	Release                                                                   *ReleaseView
}

// RunStore persists runs in the intake transaction (EDR-285).
type RunStore interface {
	RunOperationTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	AuthorEmployeeTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	AssetStateTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (string, error)
	// RunTemplateTx reads the named template version of this organization,
	// every version stamp, the archive instant, the policy history and the
	// asset's category; ErrNotFound when the version is not stored here.
	RunTemplateTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, int64, string) (RunTemplate, error)
	InsertRunTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, RunCommand, checklist.Recheck, RunFacts) error
	ReadRunTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (RunRecord, error)
}

// RecheckClaim builds the pure claim of a parsed run for its asset category.
func RecheckClaim(in RunInput, categoryID string) checklist.RunClaim {
	return checklist.RunClaim{Context: in.Context, AssetID: in.AssetID, CategoryID: categoryID, Version: in.Version, PolicyRevision: in.PolicyRevision,
		StartedAt: in.StartedAt, CompletedAt: in.Declared.OccurredAt, Responses: slices.Clone(in.Responses), DeviceOutcome: in.DeviceOutcome, Acknowledged: in.Acknowledged}
}

// ---- company run read and release (B-BR-EMP-441/442) ----

// RunCompanyAction is the closed set of company run policies (EDR-285).
type RunCompanyAction uint8

const (
	// RunRead reads one run of an asset under fleet.asset.read.
	RunRead RunCompanyAction = iota + 1
	// RunRelease releases a blocking run (fresh check.incident.manage).
	RunRelease
	// RunQueue lists the organization's runs (EMP-CHECK-004, EDR-328)
	// exactly under the incident queue's policy.
	RunQueue
)

// RunCompanyPolicy returns the fixed policy of a company run action.
func RunCompanyPolicy(action RunCompanyAction) access.Policy {
	switch action {
	case RunRead:
		// Exactly the applicable read: fleet.asset.read on the asset.
		return Policy(ApplicableRead)
	case RunRelease:
		// Exactly the incident handling step: fresh check.incident.manage.
		return IncidentPolicy(IncidentDecide)
	case RunQueue:
		// Exactly the incident queue: check.incident.manage on the organization.
		return IncidentPolicy(IncidentQueue)
	}
	return access.Policy{}
}

// RunCompanyAuthority proves a company run policy in the caller's
// transaction: the read on the asset, the release on the stored run.
type RunCompanyAuthority interface {
	CheckRunCompanyTx(context.Context, pgx.Tx, kernel.OrgScope, IncidentActor, string, string, RunCompanyAction) (Handler, error)
}

// ReleaseInput is one release command.
type ReleaseInput struct {
	private
	OperationID, RunID string
	ExpectedRevision   int64
	Reason             string
}

type ReleaseCommand struct {
	private
	value ReleaseInput
}

// NewReleaseCommand validates the release shape (B-BR-EMP-442); state,
// revision and separation are decided against the stored run.
func NewReleaseCommand(in ReleaseInput) (ReleaseCommand, error) {
	if !checklist.CanonicalID(in.OperationID) || !checklist.CanonicalID(in.RunID) || in.ExpectedRevision < 1 || in.ExpectedRevision > MaxRunRevision ||
		!incident.ValidText(in.Reason, MaxReleaseReasonScalars) {
		return ReleaseCommand{}, checklist.ErrInvalid
	}
	return ReleaseCommand{value: in}, nil
}

func (c ReleaseCommand) Snapshot() (ReleaseInput, error) {
	checked, err := NewReleaseCommand(c.value)
	return checked.value, err
}

// Digest binds every field; the reason travels only inside it.
func (c ReleaseCommand) Digest(scope kernel.OrgScope) string {
	v, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	raw, err := json.Marshal([]any{1, "check.run.release", scope.OrgID().String(), v.OperationID, v.RunID, v.ExpectedRevision, v.Reason})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ReleaseView is the stored release of a run (never the reason's author
// identity beyond the membership).
type ReleaseView struct {
	private
	ActorMembershipID string
	RecordedAt        time.Time
}

// RunView is one run as the company read returns it: both outcomes, the
// flags and the positions of the issues; never a note, a photo or the
// author's identity.
type RunView struct {
	private
	RunID, AssetID, TemplateID                    string
	Context                                       checklist.Context
	Version, PolicyRevision, ServerPolicyRevision int64
	Revision                                      int64
	StartedAt, CompletedAt, ReceivedAt            time.Time
	DeviceOutcome, RecheckOutcome, Outcome        checklist.Outcome
	Acknowledged, Superseded, Differs             bool
	Issues, Critical                              []int
	ReleasedAt                                    *time.Time
}

// RunRepository is the company owner of runs.
type RunRepository interface {
	ReadRun(context.Context, kernel.OrgScope, IncidentActor, string, string, string) (RunView, error)
	ReleaseRun(context.Context, kernel.OrgScope, IncidentActor, string, ReleaseCommand) (RunView, bool, error)
	ListRuns(context.Context, kernel.OrgScope, IncidentActor, string, RunQueueFilter) (RunQueuePage, error)
}

// ---- organization run queue (EMP-CHECK-004, B-BR-EMP-506) ----

// Run queue groups: the current outcome BLOCKING first, then a disagreement
// held for review, then every other run.
const (
	RunGroupBlocking = 0
	RunGroupReview   = 1
	RunGroupOther    = 2
)

// RunGroup is the attention group of a run view.
func RunGroup(v RunView) int {
	switch {
	case v.Outcome == checklist.Blocking:
		return RunGroupBlocking
	case v.Differs:
		return RunGroupReview
	}
	return RunGroupOther
}

// RunQueueKey is the queue order of one run: group, then the newest declared
// completion, then the run identifier.
type RunQueueKey struct {
	private
	Group       int
	CompletedAt time.Time
	RunID       string
}

// KeyOf returns the queue key of a run view.
func KeyOf(v RunView) RunQueueKey {
	return RunQueueKey{Group: RunGroup(v), CompletedAt: v.CompletedAt.UTC().Truncate(time.Microsecond), RunID: v.RunID}
}

func (k RunQueueKey) valid() bool {
	return k.Group >= RunGroupBlocking && k.Group <= RunGroupOther && validInstant(k.CompletedAt) && k.CompletedAt.Equal(k.CompletedAt.Truncate(time.Microsecond)) &&
		checklist.CanonicalID(k.RunID)
}

// Same reports whether both keys name the same queue position.
func (k RunQueueKey) Same(other RunQueueKey) bool {
	return k.Group == other.Group && k.CompletedAt.Equal(other.CompletedAt) && k.RunID == other.RunID
}

// Before reports whether k precedes other in the queue order.
func (k RunQueueKey) Before(other RunQueueKey) bool {
	if k.Group != other.Group {
		return k.Group < other.Group
	}
	if !k.CompletedAt.Equal(other.CompletedAt) {
		return k.CompletedAt.After(other.CompletedAt)
	}
	return k.RunID < other.RunID
}

// Cursor is the cursor form of a key (sealed by the transport).
func (k RunQueueKey) Cursor() string {
	if !k.valid() {
		return ""
	}
	return strconv.Itoa(k.Group) + "|" + strconv.FormatInt(k.CompletedAt.UnixMicro(), 10) + "|" + k.RunID
}

// ParseRunQueueKey reads the cursor form; the empty string is no key.
func ParseRunQueueKey(raw string) (*RunQueueKey, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, "|")
	if len(parts) != 3 || len(parts[0]) != 1 {
		return nil, checklist.ErrInvalid
	}
	group, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, checklist.ErrInvalid
	}
	micros, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, checklist.ErrInvalid
	}
	key := RunQueueKey{Group: group, CompletedAt: time.UnixMicro(micros).UTC(), RunID: parts[2]}
	if !key.valid() || key.Cursor() != raw {
		return nil, checklist.ErrInvalid
	}
	return &key, nil
}

// RunQueueFilter is one page request of the queue.
type RunQueueFilter struct {
	private
	After *RunQueueKey
	Limit int
}

func (f RunQueueFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxReportPage || f.After != nil && !f.After.valid() {
		return checklist.ErrInvalid
	}
	return nil
}

// RunQueuePage is one page of the queue; Next is the last item's key when
// more runs may follow.
type RunQueuePage struct {
	private
	Items []RunView
	Next  *RunQueueKey
}

type RunService struct {
	Enabled    bool
	Repository RunRepository
}

func (s RunService) guarded(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, action RunCompanyAction, target string, call func() error) error {
	if !s.Enabled {
		return ErrSurfaceDisabled
	}
	bind := func() error {
		if ctx == nil {
			return ErrUnauthenticated
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if scope.Validate() != nil {
			return checklist.ErrInvalid
		}
		policy := RunCompanyPolicy(action)
		if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
			return ErrUnauthenticated
		}
		return nil
	}
	if err := bind(); err != nil {
		return err
	}
	if unavailable(s.Repository) {
		return ErrUnavailable
	}
	err := call()
	if guard := bind(); guard != nil {
		return guard
	}
	return err
}

func positions(values []int, limit int) bool {
	prior := 0
	for _, v := range values {
		if v <= prior || v > limit {
			return false
		}
		prior = v
	}
	return true
}

// validRun checks a run view from the repository (B-BR-EMP-441).
func validRun(v RunView) bool {
	if !checklist.CanonicalID(v.RunID) || !checklist.CanonicalID(v.AssetID) || !checklist.CanonicalID(v.TemplateID) || v.Version < 1 || v.PolicyRevision < 1 ||
		v.ServerPolicyRevision < 1 || !validInstant(v.StartedAt) || !validInstant(v.CompletedAt) || !validInstant(v.ReceivedAt) || v.StartedAt.After(v.CompletedAt) {
		return false
	}
	for _, o := range []checklist.Outcome{v.DeviceOutcome, v.RecheckOutcome, v.Outcome} {
		if _, err := checklist.ParseOutcome(string(o)); err != nil {
			return false
		}
	}
	if _, err := checklist.ParseContext(string(v.Context)); err != nil {
		return false
	}
	released := v.Revision == 2 && v.RecheckOutcome == checklist.Blocking && v.Outcome == checklist.CompletedWithIssues && v.ReleasedAt != nil && validInstant(*v.ReleasedAt)
	current := v.Revision == 1 && v.Outcome == v.RecheckOutcome && v.ReleasedAt == nil
	return (released || current) && v.Differs == (v.DeviceOutcome != v.RecheckOutcome) && (!v.Acknowledged || v.DeviceOutcome == checklist.CompletedWithIssues) &&
		positions(v.Issues, checklist.MaxItems) && positions(v.Critical, checklist.MaxItems) && len(v.Critical) <= len(v.Issues) &&
		(len(v.Issues) == 0) == (v.RecheckOutcome == checklist.CompletedOK) && (v.RecheckOutcome != checklist.Blocking || len(v.Critical) > 0)
}

func copyRun(v RunView) RunView {
	v.Issues, v.Critical = slices.Clone(v.Issues), slices.Clone(v.Critical)
	if v.ReleasedAt != nil {
		at := *v.ReleasedAt
		v.ReleasedAt = &at
	}
	return v
}

// Read reads one run of the asset under fleet.asset.read.
func (s RunService) Read(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, assetID, runID string) (RunView, error) {
	if !checklist.CanonicalID(assetID) || !checklist.CanonicalID(runID) {
		return RunView{}, ErrNotFound
	}
	var v RunView
	err := s.guarded(ctx, scope, actor, decision, RunRead, assetID, func() error {
		var err error
		v, err = s.Repository.ReadRun(ctx, scope, actor, decision.Subject().MembershipID(), assetID, runID)
		return err
	})
	if err != nil {
		return RunView{}, err
	}
	if v.RunID != runID || v.AssetID != assetID || !validRun(v) {
		return RunView{}, ErrUnavailable
	}
	return copyRun(v), nil
}

// List pages the organization's runs under the incident queue's policy
// (B-BR-EMP-506): every item valid, strictly in queue order after the
// requested key, at most the limit, and a next key only on a full page.
func (s RunService) List(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, filter RunQueueFilter) (RunQueuePage, error) {
	if err := filter.Validate(); err != nil {
		return RunQueuePage{}, err
	}
	var v RunQueuePage
	err := s.guarded(ctx, scope, actor, decision, RunQueue, scope.OrgID().String(), func() error {
		var err error
		v, err = s.Repository.ListRuns(ctx, scope, actor, decision.Subject().MembershipID(), filter)
		return err
	})
	if err != nil {
		return RunQueuePage{}, err
	}
	if len(v.Items) > filter.Limit || v.Next != nil && (len(v.Items) != filter.Limit || !v.Next.valid()) {
		return RunQueuePage{}, ErrUnavailable
	}
	prior := filter.After
	out := RunQueuePage{Items: make([]RunView, 0, len(v.Items))}
	for _, item := range v.Items {
		if !validRun(item) {
			return RunQueuePage{}, ErrUnavailable
		}
		key := KeyOf(item)
		if prior != nil && !prior.Before(key) {
			return RunQueuePage{}, ErrUnavailable
		}
		prior = &key
		out.Items = append(out.Items, copyRun(item))
	}
	if v.Next != nil {
		if !v.Next.Same(*prior) {
			return RunQueuePage{}, ErrUnavailable
		}
		next := *v.Next
		out.Next = &next
	}
	return out, nil
}

// Release releases a blocking run under fresh check.incident.manage.
func (s RunService) Release(ctx context.Context, scope kernel.OrgScope, actor IncidentActor, decision access.Decision, command ReleaseCommand) (RunView, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return RunView{}, false, err
	}
	var v RunView
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, RunRelease, in.RunID, func() error {
		var err error
		v, replayed, err = s.Repository.ReleaseRun(ctx, scope, actor, decision.Subject().MembershipID(), command)
		return err
	})
	if err != nil {
		return RunView{}, false, err
	}
	if v.RunID != in.RunID || v.Revision != in.ExpectedRevision+1 || v.Outcome != checklist.CompletedWithIssues || !validRun(v) {
		return RunView{}, false, ErrUnavailable
	}
	return copyRun(v), replayed, nil
}
