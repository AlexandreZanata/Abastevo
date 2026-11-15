// Package application owns the checklist template command and read boundary
// of EMP-CHECK-001 (B-BR-EMP-330..335). Repositories prove current authority
// inside their own transaction; the service binds the completed request
// decision before and after every repository call.
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
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound        = errors.New("checklist: private record not found")
	ErrUnavailable     = errors.New("checklist: source unavailable")
	ErrUnauthenticated = errors.New("checklist: current company proof required")
	ErrOperationReused = errors.New("checklist: operation identifier reused")
	ErrSurfaceDisabled = errors.New("checklist: surface disabled")
	// ErrConflict is a stale template revision, an archived template or a
	// target already named by another ACTIVE template of the context.
	ErrConflict = errors.New("checklist: conflict")
)

type private struct{}

func (private) String() string                 { return "EnterpriseChecklist(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return []byte(`"EnterpriseChecklist(redacted)"`), nil }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }

// ChecklistActor is the current company account and its web proof.
type ChecklistActor struct {
	private
	AccountID, SessionID string
}

func (a ChecklistActor) Validate() error {
	if !checklist.CanonicalID(a.AccountID) || !checklist.CanonicalID(a.SessionID) {
		return ErrUnauthenticated
	}
	return nil
}

// ChecklistAction is the closed set of source-owned policies.
type ChecklistAction uint8

const (
	// TemplateRead lists and reads templates (check.template.manage, ORG).
	TemplateRead ChecklistAction = iota + 1
	// TemplateManage creates, publishes, sets the policy and archives
	// (fresh check.template.manage, ORG).
	TemplateManage
	// ApplicableRead reads the applicable template of an asset
	// (fleet.asset.read on the asset).
	ApplicableRead
)

// TemplatePermission is the only key of the template routes.
const TemplatePermission = "check.template.manage"

// Policy returns the fixed policy of an action (EDR-215). The applicable read
// is exactly the Fleet asset read policy.
func Policy(action ChecklistAction) access.Policy {
	switch action {
	case TemplateRead:
		return access.Policy{Permission: TemplatePermission, Capability: "organizations", Effect: access.Read}
	case TemplateManage:
		return access.Policy{Permission: TemplatePermission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case ApplicableRead:
		return access.Policy{Permission: "fleet.asset.read", Capability: "organizations", Effect: access.Read}
	}
	return access.Policy{}
}

// ChecklistAuthority retains the company account/proof → capability → root →
// membership/role order in the caller's transaction. Template actions target
// the organization itself; the applicable read targets the asset.
type ChecklistAuthority interface {
	CheckChecklistTx(context.Context, pgx.Tx, kernel.OrgScope, ChecklistActor, string, string, ChecklistAction) error
}

// CommandKind names one template command.
type CommandKind string

const (
	CommandCreate  CommandKind = "CREATE"
	CommandPublish CommandKind = "PUBLISH"
	CommandPolicy  CommandKind = "POLICY"
	CommandArchive CommandKind = "ARCHIVE"
)

// TemplateState is the configuration lifecycle of a template.
type TemplateState string

const (
	TemplateActive   TemplateState = "ACTIVE"
	TemplateArchived TemplateState = "ARCHIVED"
)

// TemplateCommandInput is one template command. Create carries the context,
// the content and optionally the policy (zero = BLOCK, 12 hours); publish the
// content; policy the policy; archive nothing else. Every command but create
// names the expected template revision.
type TemplateCommandInput struct {
	private
	Kind             CommandKind
	OperationID      string
	TemplateID       string
	ExpectedRevision int64
	Context          checklist.Context
	Name             string
	Target           checklist.Target
	Items            []checklist.Item
	Policy           checklist.Policy
}

// TemplateCommand is a validated, immutable command.
type TemplateCommand struct {
	private
	value TemplateCommandInput
}

func content(in TemplateCommandInput) (checklist.Definition, error) {
	return checklist.NewDefinition(checklist.Definition{Name: in.Name, Context: checklist.Departure, Target: in.Target, Items: in.Items})
}

func noContent(in TemplateCommandInput) bool {
	return in.Name == "" && in.Target.Kind == "" && in.Target.IDs == nil && in.Items == nil
}

func NewTemplateCommand(in TemplateCommandInput) (TemplateCommand, error) {
	if !checklist.CanonicalID(in.OperationID) {
		return TemplateCommand{}, checklist.ErrInvalid
	}
	out := TemplateCommandInput{Kind: in.Kind, OperationID: in.OperationID, TemplateID: in.TemplateID, ExpectedRevision: in.ExpectedRevision, Context: in.Context}
	if in.Kind == CommandCreate {
		if in.TemplateID != "" || in.ExpectedRevision != 0 {
			return TemplateCommand{}, checklist.ErrInvalid
		}
		if _, err := checklist.ParseContext(string(in.Context)); err != nil {
			return TemplateCommand{}, err
		}
	} else if !checklist.CanonicalID(in.TemplateID) || in.ExpectedRevision < 1 || in.Context != "" {
		return TemplateCommand{}, checklist.ErrInvalid
	}
	switch in.Kind {
	case CommandCreate, CommandPublish:
		d, err := content(in)
		if err != nil {
			return TemplateCommand{}, err
		}
		out.Name, out.Target, out.Items = d.Name, d.Target, d.Items
		if in.Kind == CommandPublish && in.Policy != (checklist.Policy{}) {
			return TemplateCommand{}, checklist.ErrInvalid
		}
		if in.Kind == CommandCreate {
			out.Policy = checklist.DefaultPolicy()
			if in.Policy != (checklist.Policy{}) {
				if err := in.Policy.Validate(); err != nil {
					return TemplateCommand{}, err
				}
				out.Policy = in.Policy
			}
		}
	case CommandPolicy:
		if !noContent(in) {
			return TemplateCommand{}, checklist.ErrInvalid
		}
		if err := in.Policy.Validate(); err != nil {
			return TemplateCommand{}, err
		}
		out.Policy = in.Policy
	case CommandArchive:
		if !noContent(in) || in.Policy != (checklist.Policy{}) {
			return TemplateCommand{}, checklist.ErrInvalid
		}
	default:
		return TemplateCommand{}, checklist.ErrInvalid
	}
	return TemplateCommand{value: out}, nil
}

// Snapshot returns an independent copy of the validated command.
func (c TemplateCommand) Snapshot() (TemplateCommandInput, error) {
	again, err := NewTemplateCommand(c.value)
	if err != nil {
		return TemplateCommandInput{}, err
	}
	return again.value, nil
}

// Definition is the published content under the template's fixed context.
func (c TemplateCommand) Definition(moment checklist.Context) (checklist.Definition, error) {
	in, err := c.Snapshot()
	if err != nil || in.Kind != CommandCreate && in.Kind != CommandPublish {
		return checklist.Definition{}, checklist.ErrInvalid
	}
	return checklist.NewDefinition(checklist.Definition{Name: in.Name, Context: moment, Target: in.Target, Items: in.Items})
}

// Digest is the canonical request hash bound to the organization.
func (c TemplateCommand) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	type item struct {
		Label                            string
		Critical, Optional, PhotoOnNotOK bool
	}
	items := make([]item, len(in.Items))
	for i, v := range in.Items {
		items[i] = item{v.Label, v.Critical, v.Optional, v.PhotoOnNotOK}
	}
	raw, err := json.Marshal([]any{"check.template.command.v1", scope.OrgID().String(), in.Kind, in.OperationID, in.TemplateID, in.ExpectedRevision, in.Context, in.Name, in.Target.Kind, in.Target.IDs, items, in.Policy.Mode, in.Policy.ValiditySeconds})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// TemplateAck is the recorded result of one accepted command.
type TemplateAck struct {
	private
	TemplateID     string
	Revision       int64
	Version        int64
	PolicyRevision int64
	State          TemplateState
}

// PolicyView is one stored policy revision.
type PolicyView struct {
	private
	Revision    int64
	Policy      checklist.Policy
	EffectiveAt time.Time
}

// VersionView is one stored version with its ordered items.
type VersionView struct {
	private
	Version     int64
	Name        string
	Target      checklist.Target
	Items       []checklist.Item
	PublishedAt time.Time
}

// TemplateView is the head, one selected version and the current policy.
type TemplateView struct {
	private
	TemplateID     string
	Context        checklist.Context
	State          TemplateState
	Revision       int64
	Version        int64
	PolicyRevision int64
	CreatedAt      time.Time
	ArchivedAt     *time.Time
	Selected       VersionView
	Policy         PolicyView
}

// TemplateSummary is one list row (the current version's name and target kind).
type TemplateSummary struct {
	private
	TemplateID     string
	Context        checklist.Context
	State          TemplateState
	Revision       int64
	Version        int64
	PolicyRevision int64
	Name           string
	TargetKind     checklist.TargetKind
	CreatedAt      time.Time
	ArchivedAt     *time.Time
}

// MaxTemplatePage mirrors the shared private page bound (httpapi.MaxLimit).
const MaxTemplatePage = 100

// TemplateFilter is the identifier keyset page of templates.
type TemplateFilter struct {
	private
	AfterID string
	Limit   int
}

func (f TemplateFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxTemplatePage || f.AfterID != "" && !checklist.CanonicalID(f.AfterID) {
		return checklist.ErrInvalid
	}
	return nil
}

type TemplatePage struct {
	private
	Templates []TemplateSummary
	NextID    string
}

// ApplicableTemplate is what a run pins: never the template's other targets.
type ApplicableTemplate struct {
	private
	TemplateID  string
	Name        string
	Version     int64
	PublishedAt time.Time
	Items       []checklist.Item
}

// ApplicableView is the BR-CHECK-002 answer at the server instant At.
type ApplicableView struct {
	private
	AssetID  string
	Context  checklist.Context
	Match    checklist.Match
	Template *ApplicableTemplate
	Policy   *PolicyView
	At       time.Time
}

type ChecklistRepository interface {
	CommandTemplate(context.Context, kernel.OrgScope, ChecklistActor, string, string, TemplateCommand) (TemplateAck, bool, error)
	ListTemplates(context.Context, kernel.OrgScope, ChecklistActor, string, TemplateFilter) (TemplatePage, error)
	// ReadTemplate reads the template with the given version (0 = current).
	ReadTemplate(context.Context, kernel.OrgScope, ChecklistActor, string, string, int64) (TemplateView, error)
	ReadApplicable(context.Context, kernel.OrgScope, ChecklistActor, string, string, checklist.Context) (ApplicableView, error)
}

type ChecklistService struct {
	Enabled    bool
	Repository ChecklistRepository
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

// current binds the completed request decision to this actor, policy and
// target before and after the repository call.
func current(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, action ChecklistAction, target string) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return checklist.ErrInvalid
	}
	policy := Policy(action)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func (s ChecklistService) guarded(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, action ChecklistAction, target string, call func() error) error {
	if !s.Enabled {
		return ErrSurfaceDisabled
	}
	if err := current(ctx, scope, actor, decision, action, target); err != nil {
		return err
	}
	if unavailable(s.Repository) {
		return ErrUnavailable
	}
	err := call()
	if guard := current(ctx, scope, actor, decision, action, target); guard != nil {
		return guard
	}
	return err
}

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

func validAck(v TemplateAck, in TemplateCommandInput) bool {
	if !checklist.CanonicalID(v.TemplateID) || in.Kind != CommandCreate && v.TemplateID != in.TemplateID || v.Version < 1 || v.PolicyRevision < 1 {
		return false
	}
	switch in.Kind {
	case CommandCreate:
		return v.Revision == 1 && v.Version == 1 && v.PolicyRevision == 1 && v.State == TemplateActive
	case CommandPublish:
		return v.Revision == in.ExpectedRevision+1 && v.Version > 1 && v.State == TemplateActive
	case CommandPolicy:
		return v.Revision == in.ExpectedRevision+1 && v.PolicyRevision > 1 && v.State == TemplateActive
	case CommandArchive:
		return v.Revision == in.ExpectedRevision+1 && v.State == TemplateArchived
	}
	return false
}

// Command runs one template command under fresh check.template.manage.
func (s ChecklistService) Command(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, command TemplateCommand) (TemplateAck, bool, error) {
	in, err := command.Snapshot()
	if err != nil {
		return TemplateAck{}, false, err
	}
	var v TemplateAck
	replayed := false
	err = s.guarded(ctx, scope, actor, decision, TemplateManage, scope.OrgID().String(), func() error {
		newID := ""
		if in.Kind == CommandCreate {
			if s.NewID == nil {
				return ErrUnavailable
			}
			id, err := s.NewID()
			if err != nil || !checklist.CanonicalID(id) {
				return ErrUnavailable
			}
			newID = id
		}
		var err error
		v, replayed, err = s.Repository.CommandTemplate(ctx, scope, actor, decision.Subject().MembershipID(), newID, command)
		return err
	})
	if err != nil {
		return TemplateAck{}, false, err
	}
	if !validAck(v, in) {
		return TemplateAck{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func validState(state TemplateState, archivedAt *time.Time) bool {
	switch state {
	case TemplateActive:
		return archivedAt == nil
	case TemplateArchived:
		return archivedAt != nil && validInstant(*archivedAt)
	}
	return false
}

func copyInstant(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	v := *at
	return &v
}

func validPolicyView(p PolicyView, revision int64) bool {
	return p.Revision == revision && p.Policy.Validate() == nil && validInstant(p.EffectiveAt)
}

func validVersionView(v VersionView, moment checklist.Context, current int64) (VersionView, bool) {
	if v.Version < 1 || v.Version > current || !validInstant(v.PublishedAt) {
		return VersionView{}, false
	}
	d, err := checklist.NewDefinition(checklist.Definition{Name: v.Name, Context: moment, Target: v.Target, Items: v.Items})
	if err != nil || len(d.Target.IDs) != len(v.Target.IDs) {
		return VersionView{}, false
	}
	for i, id := range d.Target.IDs {
		if v.Target.IDs[i] != id {
			return VersionView{}, false
		}
	}
	v.Target, v.Items = d.Target, d.Items
	return v, true
}

// List pages the organization's templates in identifier order.
func (s ChecklistService) List(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, filter TemplateFilter) (TemplatePage, error) {
	if err := filter.Validate(); err != nil {
		return TemplatePage{}, err
	}
	var v TemplatePage
	err := s.guarded(ctx, scope, actor, decision, TemplateRead, scope.OrgID().String(), func() error {
		var err error
		v, err = s.Repository.ListTemplates(ctx, scope, actor, decision.Subject().MembershipID(), filter)
		return err
	})
	if err != nil {
		return TemplatePage{}, err
	}
	if len(v.Templates) > filter.Limit || v.NextID != "" && (len(v.Templates) != filter.Limit || v.Templates[len(v.Templates)-1].TemplateID != v.NextID) {
		return TemplatePage{}, ErrUnavailable
	}
	out := TemplatePage{Templates: make([]TemplateSummary, 0, len(v.Templates)), NextID: v.NextID}
	prior := filter.AfterID
	for _, t := range v.Templates {
		if !checklist.CanonicalID(t.TemplateID) || t.TemplateID <= prior || t.Revision < 1 || t.Version < 1 || t.PolicyRevision < 1 || !validInstant(t.CreatedAt) || !validState(t.State, t.ArchivedAt) {
			return TemplatePage{}, ErrUnavailable
		}
		if _, err := checklist.ParseContext(string(t.Context)); err != nil {
			return TemplatePage{}, ErrUnavailable
		}
		if _, err := checklist.ParseTargetKind(string(t.TargetKind)); err != nil {
			return TemplatePage{}, ErrUnavailable
		}
		if _, err := checklist.NewDefinition(checklist.Definition{Name: t.Name, Context: t.Context, Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: []checklist.Item{{Label: "x"}}}); err != nil {
			return TemplatePage{}, ErrUnavailable
		}
		t.ArchivedAt = copyInstant(t.ArchivedAt)
		out.Templates = append(out.Templates, t)
		prior = t.TemplateID
	}
	return out, nil
}

// Read reads one template with the selected version (0 = current).
func (s ChecklistService) Read(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, templateID string, version int64) (TemplateView, error) {
	if !checklist.CanonicalID(templateID) {
		return TemplateView{}, ErrNotFound
	}
	if version < 0 {
		return TemplateView{}, checklist.ErrInvalid
	}
	var v TemplateView
	err := s.guarded(ctx, scope, actor, decision, TemplateRead, scope.OrgID().String(), func() error {
		var err error
		v, err = s.Repository.ReadTemplate(ctx, scope, actor, decision.Subject().MembershipID(), templateID, version)
		return err
	})
	if err != nil {
		return TemplateView{}, err
	}
	if _, err := checklist.ParseContext(string(v.Context)); err != nil || v.TemplateID != templateID || v.Revision < 1 || v.Version < 1 || !validInstant(v.CreatedAt) || !validState(v.State, v.ArchivedAt) || !validPolicyView(v.Policy, v.PolicyRevision) {
		return TemplateView{}, ErrUnavailable
	}
	selected, ok := validVersionView(v.Selected, v.Context, v.Version)
	if !ok || version != 0 && selected.Version != version || version == 0 && selected.Version != v.Version {
		return TemplateView{}, ErrUnavailable
	}
	v.Selected, v.ArchivedAt = selected, copyInstant(v.ArchivedAt)
	return v, nil
}

// Applicable reads the applicable template of an asset for a context under
// fleet.asset.read on the asset (B-BR-EMP-332).
func (s ChecklistService) Applicable(ctx context.Context, scope kernel.OrgScope, actor ChecklistActor, decision access.Decision, assetID string, moment checklist.Context) (ApplicableView, error) {
	if !checklist.CanonicalID(assetID) {
		return ApplicableView{}, ErrNotFound
	}
	if _, err := checklist.ParseContext(string(moment)); err != nil {
		return ApplicableView{}, err
	}
	var v ApplicableView
	err := s.guarded(ctx, scope, actor, decision, ApplicableRead, assetID, func() error {
		var err error
		v, err = s.Repository.ReadApplicable(ctx, scope, actor, decision.Subject().MembershipID(), assetID, moment)
		return err
	})
	if err != nil {
		return ApplicableView{}, err
	}
	if v.AssetID != assetID || v.Context != moment || !validInstant(v.At) {
		return ApplicableView{}, ErrUnavailable
	}
	if v.Match == checklist.MatchNone {
		if v.Template != nil || v.Policy != nil {
			return ApplicableView{}, ErrUnavailable
		}
		return v, nil
	}
	if v.Match != checklist.MatchAsset && v.Match != checklist.MatchCategory && v.Match != checklist.MatchOrganization || v.Template == nil || v.Policy == nil {
		return ApplicableView{}, ErrUnavailable
	}
	t, p := *v.Template, *v.Policy
	if !checklist.CanonicalID(t.TemplateID) || t.Version < 1 || !validInstant(t.PublishedAt) || p.Revision < 1 || !validPolicyView(p, p.Revision) || p.EffectiveAt.After(v.At) || t.PublishedAt.After(v.At) {
		return ApplicableView{}, ErrUnavailable
	}
	d, err := checklist.NewDefinition(checklist.Definition{Name: t.Name, Context: moment, Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: t.Items})
	if err != nil {
		return ApplicableView{}, ErrUnavailable
	}
	t.Items = d.Items
	v.Template, v.Policy = &t, &p
	return v, nil
}
