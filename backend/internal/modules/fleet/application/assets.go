// Package application owns private fleet command boundaries; repositories prove current authority.
package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/jackc/pgx/v5"
)

var (
	ErrConflict        = errors.New("fleet: conflicting current revision or state")
	ErrOperationReused = errors.New("fleet: operation identifier reused")
	ErrIdentifierTaken = errors.New("fleet: asset identifier already used")
	ErrNotFound        = errors.New("fleet: private asset not found")
	ErrUnavailable     = errors.New("fleet: source unavailable")
	ErrUnauthenticated = errors.New("fleet: current company proof required")
	ErrSurfaceDisabled = errors.New("fleet: surface disabled")
)

// AssetEntityType is the reserved lower_snake intake entity token. An applied
// effect counts as use only for this exact type and the scoped asset identity.
const AssetEntityType = "fleet_asset"

type private struct{}

func (private) String() string                 { return "EnterpriseFleetCommand(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }

type FleetActor struct {
	private
	AccountID, SessionID string
}

func canonicalID(raw string) bool {
	id, e := kernel.ParseOrgID(raw)
	return e == nil && id.String() == raw
}
func (a FleetActor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return fleet.ErrInvalid
	}
	return nil
}

type FleetAction uint8

const (
	FleetRead FleetAction = iota + 1
	FleetManage
)

// FleetAuthority retains the established account/proof -> capability -> root ->
// current membership/role order in the caller's transaction. It accepts only
// these source-owned actions, never a caller-selected permission or fake SELF.
type FleetAuthority interface {
	CheckFleetTx(context.Context, pgx.Tx, kernel.OrgScope, FleetActor, string, string, FleetAction) error
}

func Policy(write bool) access.Policy {
	if write {
		return access.Policy{Permission: "fleet.asset.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	}
	return access.Policy{Permission: "fleet.asset.read", Capability: "organizations", Effect: access.Read}
}

type CategoryChoice struct {
	private
	ExistingID, NewLabel string
}

func (c CategoryChoice) Validate() error {
	if c.ExistingID != "" {
		if c.NewLabel != "" || !canonicalID(c.ExistingID) {
			return fleet.ErrInvalid
		}
		return nil
	}
	if !utf8.ValidString(c.NewLabel) || strings.TrimSpace(c.NewLabel) == "" {
		return fleet.ErrInvalid
	}
	for _, r := range c.NewLabel {
		if unicode.IsControl(r) {
			return fleet.ErrInvalid
		}
	}
	return nil
}

type AssetWriteKind string

const (
	CreateAsset   AssetWriteKind = "CREATE"
	ChangeAsset   AssetWriteKind = "CHANGE"
	DiscardAsset  AssetWriteKind = "DISCARD"
	ActivateAsset AssetWriteKind = "ACTIVATE"
)

type AssetWriteInput struct {
	private
	Kind                 AssetWriteKind
	OperationID, AssetID string
	ExpectedRevision     int64
	Profile              fleet.Profile
	Category             CategoryChoice
	UnitID               string
}
type AssetWrite struct {
	private
	value AssetWriteInput
}

// NewAssetWrite captures command shape and immutable intent. The repository
// resolves a real persisted category and validates the full Fleet profile using
// that source and fuel-products-v1 inside its transaction before any effect.
func copyProfile(p fleet.Profile) fleet.Profile {
	p.Tanks = append([]fleet.Tank(nil), p.Tanks...)
	p.Expected = append([]fleet.ExpectedConsumption(nil), p.Expected...)
	return p
}
func emptyProfile(p fleet.Profile) bool {
	return p.Kind == "" && p.Category == "" && p.Name == "" && p.FleetNumber == "" && p.Plate == "" && p.SerialNumber == "" && p.UsageBasis == "" && len(p.Tanks) == 0 && len(p.Expected) == 0
}
func NewAssetWrite(v AssetWriteInput) (AssetWrite, error) {
	v.Profile = copyProfile(v.Profile)
	// encoding/json replaces malformed UTF8. Reject it before any digest so
	// only the exact original Unicode intent can reach a receipt comparison.
	for _, text := range []string{v.Profile.Name, v.Profile.FleetNumber, v.Profile.Plate, v.Profile.SerialNumber} {
		if !utf8.ValidString(text) {
			return AssetWrite{}, fleet.ErrInvalid
		}
	}
	if !canonicalID(v.OperationID) {
		return AssetWrite{}, fleet.ErrInvalid
	}
	switch v.Kind {
	case CreateAsset:
		if v.AssetID != "" || v.ExpectedRevision != 0 {
			return AssetWrite{}, fleet.ErrInvalid
		}
	case ChangeAsset, DiscardAsset, ActivateAsset:
		if !canonicalID(v.AssetID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 {
			return AssetWrite{}, fleet.ErrInvalid
		}
	default:
		return AssetWrite{}, fleet.ErrInvalid
	}
	if v.Kind == CreateAsset || v.Kind == ChangeAsset {
		if v.Profile.Category != "" || v.Category.Validate() != nil || v.UnitID != "" && !canonicalID(v.UnitID) {
			return AssetWrite{}, fleet.ErrInvalid
		}
		if v.Profile.Kind != fleet.Vehicle && v.Profile.Kind != fleet.Machine && v.Profile.Kind != fleet.Implement {
			return AssetWrite{}, fleet.ErrInvalid
		}
		if v.Profile.UsageBasis != fleet.Distance && v.Profile.UsageBasis != fleet.Hours {
			return AssetWrite{}, fleet.ErrInvalid
		}
	} else if !emptyProfile(v.Profile) || v.Category != (CategoryChoice{}) || v.UnitID != "" {
		return AssetWrite{}, fleet.ErrInvalid
	}
	return AssetWrite{value: v}, nil
}
func (v AssetWrite) Snapshot() (AssetWriteInput, error) {
	copy, e := NewAssetWrite(v.value)
	if e != nil {
		return AssetWriteInput{}, e
	}
	return copy.value, nil
}
func (v AssetWrite) Digest(scope kernel.OrgScope) string {
	in, e := v.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	p := in.Profile
	tanks := make([]any, 0, len(p.Tanks))
	for _, t := range p.Tanks {
		s := t.Snapshot()
		tanks = append(tanks, []any{s.Role, s.Capacity, s.Unit, s.Products})
	}
	expected := make([]any, 0, len(p.Expected))
	for _, r := range p.Expected {
		s := r.Snapshot()
		expected = append(expected, []any{s.Product, s.Basis, s.Value, s.Unit, s.Source, s.Date})
	}
	// Private domain values intentionally redact JSON. Every original primitive
	// field is expanded explicitly; no profile object is marshaled implicitly.
	raw, _ := json.Marshal([]any{1, "fleet.asset.command", scope.OrgID().String(), in.Kind, in.OperationID, in.AssetID, in.ExpectedRevision, p.Kind, p.Name, p.FleetNumber, p.Plate, p.SerialNumber, p.UsageBasis, in.Category.ExistingID, in.Category.NewLabel, in.UnitID, tanks, expected})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type AssetResult struct {
	private
	AssetID   string
	State     fleet.State
	Revision  int64
	Discarded bool
}
type UnitAllocation struct {
	private
	UnitID    string
	StartedAt time.Time
}
type AssetView struct {
	private
	Ref           fleet.AssetRef
	Profile       fleet.Profile
	CategoryLabel string
	State         fleet.State
	Revision      int64
	UnitID        string
	Allocation    *UnitAllocation
}

func (v AssetView) Clone() AssetView {
	v.Profile = copyProfile(v.Profile)
	if v.Allocation != nil {
		a := *v.Allocation
		v.Allocation = &a
	}
	return v
}

type AssetFilter struct {
	private
	AfterID string
	State   fleet.State
	Kind    fleet.Kind
	Limit   int
}

func (f AssetFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) {
		return fleet.ErrInvalid
	}
	if f.State != "" {
		if _, e := fleet.ParseState(string(f.State)); e != nil {
			return fleet.ErrInvalid
		}
	}
	if f.Kind != "" && f.Kind != fleet.Vehicle && f.Kind != fleet.Machine && f.Kind != fleet.Implement {
		return fleet.ErrInvalid
	}
	return nil
}

type AssetPage struct {
	private
	Assets []AssetView
	NextID string
}

// Repository results remain private and provisional until the source-owned
// transaction commits. Implementations recheck real proof before receipts and
// final publication; same-intent replay precedes any live target requirement.
type AssetRepository interface {
	WriteAsset(context.Context, kernel.OrgScope, FleetActor, string, string, AssetWrite) (AssetResult, bool, error)
	ReadAsset(context.Context, kernel.OrgScope, FleetActor, string, string) (AssetView, error)
	ListAssets(context.Context, kernel.OrgScope, FleetActor, string, AssetFilter) (AssetPage, error)
}
type AssetLookup interface {
	FindAsset(context.Context, kernel.OrgScope, string) (AssetView, error)
}
type AssetService struct {
	Enabled    bool
	Repository AssetRepository
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
func current(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, target string, write bool) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scope.Validate() != nil {
		return fleet.ErrScope
	}
	policy := Policy(write)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy || decision.Check(ctx, scope, actor.AccountID, policy.Permission, target) != nil {
		return ErrUnauthenticated
	}
	return nil
}
func validResult(v AssetResult, command AssetWriteInput) bool {
	if !canonicalID(v.AssetID) || v.Revision < 1 || command.Kind != CreateAsset && v.AssetID != command.AssetID {
		return false
	}
	if command.Kind == DiscardAsset {
		return v.Discarded && v.State == "" && v.Revision == command.ExpectedRevision+1
	}
	if v.Discarded {
		return false
	}
	if _, e := fleet.ParseState(string(v.State)); e != nil {
		return false
	}
	if command.Kind == CreateAsset {
		return v.State == fleet.Draft && v.Revision == 1
	}
	if v.Revision != command.ExpectedRevision+1 {
		return false
	}
	return command.Kind != ActivateAsset || v.State == fleet.Active
}
func validView(v AssetView, scope kernel.OrgScope, target string) bool {
	if v.Ref.Validate(scope) != nil || target != "" && v.Ref.ID().Value() != target || v.Revision < 1 || !canonicalID(v.Profile.Category) || (CategoryChoice{NewLabel: v.CategoryLabel}).Validate() != nil {
		return false
	}
	if _, e := fleet.ParseState(string(v.State)); e != nil {
		return false
	}
	if v.UnitID != "" && !canonicalID(v.UnitID) {
		return false
	}
	if v.State == fleet.Active && (v.UnitID == "" || v.Allocation == nil) {
		return false
	}
	if a := v.Allocation; a != nil {
		year := a.StartedAt.UTC().Year()
		if !canonicalID(a.UnitID) || a.UnitID != v.UnitID || a.StartedAt.IsZero() || year < 1 || year > 9999 {
			return false
		}
	}
	return true
}
func (s AssetService) Write(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AssetWrite) (AssetResult, bool, error) {
	if !s.Enabled {
		return AssetResult{}, false, ErrSurfaceDisabled
	}
	in, e := command.Snapshot()
	if e != nil {
		return AssetResult{}, false, e
	}
	target := in.AssetID
	if in.Kind == CreateAsset {
		target = scope.OrgID().String()
	}
	if e = current(ctx, scope, actor, decision, target, true); e != nil {
		return AssetResult{}, false, e
	}
	if unavailable(s.Repository) {
		return AssetResult{}, false, ErrUnavailable
	}
	newID := ""
	if in.Kind == CreateAsset {
		if s.NewID == nil {
			return AssetResult{}, false, ErrUnavailable
		}
		newID, e = s.NewID()
		if e != nil || !canonicalID(newID) {
			return AssetResult{}, false, ErrUnavailable
		}
	}
	v, replay, e := s.Repository.WriteAsset(ctx, scope, actor, decision.Subject().MembershipID(), newID, command)
	if guard := current(ctx, scope, actor, decision, target, true); guard != nil {
		return AssetResult{}, false, guard
	}
	if e != nil {
		return AssetResult{}, false, e
	}
	if !validResult(v, in) {
		return AssetResult{}, false, ErrUnavailable
	}
	return v, replay, nil
}
func (s AssetService) Read(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, id string) (AssetView, error) {
	if !s.Enabled {
		return AssetView{}, ErrSurfaceDisabled
	}
	if !canonicalID(id) {
		return AssetView{}, fleet.ErrInvalid
	}
	if e := current(ctx, scope, actor, decision, id, false); e != nil {
		return AssetView{}, e
	}
	if unavailable(s.Repository) {
		return AssetView{}, ErrUnavailable
	}
	v, e := s.Repository.ReadAsset(ctx, scope, actor, decision.Subject().MembershipID(), id)
	if guard := current(ctx, scope, actor, decision, id, false); guard != nil {
		return AssetView{}, guard
	}
	if e != nil {
		return AssetView{}, e
	}
	if !validView(v, scope, id) {
		return AssetView{}, ErrUnavailable
	}
	return v.Clone(), nil
}
func (s AssetService) List(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, filter AssetFilter) (AssetPage, error) {
	if !s.Enabled {
		return AssetPage{}, ErrSurfaceDisabled
	}
	if e := filter.Validate(); e != nil {
		return AssetPage{}, e
	}
	target := scope.OrgID().String()
	if e := current(ctx, scope, actor, decision, target, false); e != nil {
		return AssetPage{}, e
	}
	if unavailable(s.Repository) {
		return AssetPage{}, ErrUnavailable
	}
	v, e := s.Repository.ListAssets(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := current(ctx, scope, actor, decision, target, false); guard != nil {
		return AssetPage{}, guard
	}
	if e != nil {
		return AssetPage{}, e
	}
	if len(v.Assets) > filter.Limit {
		return AssetPage{}, ErrUnavailable
	}
	out := AssetPage{Assets: make([]AssetView, 0, len(v.Assets)), NextID: v.NextID}
	prior := filter.AfterID
	for _, asset := range v.Assets {
		if !validView(asset, scope, "") || asset.Ref.ID().Value() <= prior || filter.State != "" && asset.State != filter.State || filter.State == "" && asset.State == fleet.Archived || filter.Kind != "" && asset.Profile.Kind != filter.Kind {
			return AssetPage{}, ErrUnavailable
		}
		out.Assets = append(out.Assets, asset.Clone())
		prior = asset.Ref.ID().Value()
	}
	if out.NextID != "" && (!canonicalID(out.NextID) || len(out.Assets) != filter.Limit || out.NextID != prior) {
		return AssetPage{}, ErrUnavailable
	}
	return out, nil
}
