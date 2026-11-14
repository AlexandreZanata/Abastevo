package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
)

// ErrOpenOperations is the BR-FLEET-010 refusal; OpenOperationsError names kinds.
var ErrOpenOperations = errors.New("fleet: asset has open operations")

// OpenWorkKind is a source-owned kind of open work, never a record identity.
type OpenWorkKind string

const (
	OpenTrip      OpenWorkKind = "TRIP"
	OpenWorkOrder OpenWorkKind = "WORK_ORDER"
)

// AssetOpenWork answers, inside the caller's command transaction, which kinds
// of work are open on one asset (B-BR-EMP-205). The trip and maintenance epics
// replace the explicit "none" adapter with their actual sources.
type AssetOpenWork interface {
	OpenAssetWorkTx(context.Context, pgx.Tx, kernel.OrgScope, string) ([]OpenWorkKind, error)
}

type OpenOperationsError struct{ kinds []OpenWorkKind }

func (*OpenOperationsError) Error() string { return ErrOpenOperations.Error() }
func (*OpenOperationsError) Unwrap() error { return ErrOpenOperations }
func (e *OpenOperationsError) Kinds() []OpenWorkKind {
	return append([]OpenWorkKind(nil), e.kinds...)
}

// NewOpenOperations accepts only known, distinct kinds.
func NewOpenOperations(kinds []OpenWorkKind) error {
	if len(kinds) == 0 || len(kinds) > 2 {
		return ErrUnavailable
	}
	seen := map[OpenWorkKind]bool{}
	for _, kind := range kinds {
		if kind != OpenTrip && kind != OpenWorkOrder || seen[kind] {
			return ErrUnavailable
		}
		seen[kind] = true
	}
	return &OpenOperationsError{kinds: append([]OpenWorkKind(nil), kinds...)}
}

// AssetDisposal is the current disposal on an asset or one historical disposal.
type AssetDisposal struct {
	private
	Kind   fleet.DisposalKind
	At     time.Time
	Reason string
}

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}
func (d AssetDisposal) valid() bool {
	_, err := fleet.ParseDisposalKind(string(d.Kind))
	return err == nil && validInstant(d.At) && fleet.ValidReason(d.Reason)
}

type AssetLifecycleInput struct {
	private
	Command              fleet.LifecycleCommand
	OperationID, AssetID string
	ExpectedRevision     int64
	Reason               string
	DisposalKind         fleet.DisposalKind
	At                   time.Time
}
type AssetLifecycle struct {
	private
	value AssetLifecycleInput
}

// NewAssetLifecycle validates the exact command shape of B-BR-EMP-202/203:
// a reason exactly where §6.1 requires one, a disposal kind and instant only
// for dispose, an instant only for dispose and acquisition, nothing else.
func NewAssetLifecycle(v AssetLifecycleInput) (AssetLifecycle, error) {
	known := false
	for _, command := range fleet.LifecycleCommands() {
		known = known || command == v.Command
	}
	if !known || !canonicalID(v.OperationID) || !canonicalID(v.AssetID) || v.ExpectedRevision < 1 || v.ExpectedRevision == math.MaxInt64 || !utf8.ValidString(v.Reason) {
		return AssetLifecycle{}, fleet.ErrInvalid
	}
	if fleet.RequiresReason(v.Command) != (v.Reason != "") || v.Reason != "" && !fleet.ValidReason(v.Reason) {
		return AssetLifecycle{}, fleet.ErrInvalid
	}
	instant := v.Command == fleet.Dispose || v.Command == fleet.RecordAcquisition
	if instant != !v.At.IsZero() || instant && !validInstant(v.At) {
		return AssetLifecycle{}, fleet.ErrInvalid
	}
	if v.Command == fleet.Dispose {
		if _, err := fleet.ParseDisposalKind(string(v.DisposalKind)); err != nil {
			return AssetLifecycle{}, fleet.ErrInvalid
		}
	} else if v.DisposalKind != "" {
		return AssetLifecycle{}, fleet.ErrInvalid
	}
	if instant {
		v.At = v.At.UTC()
	}
	return AssetLifecycle{value: v}, nil
}
func (c AssetLifecycle) Snapshot() (AssetLifecycleInput, error) {
	copy, err := NewAssetLifecycle(c.value)
	if err != nil {
		return AssetLifecycleInput{}, err
	}
	return copy.value, nil
}

// Digest binds the scoped canonical intent. Instants use the exact UTC
// nanosecond text, so the same instant from another zone is the same intent.
func (c AssetLifecycle) Digest(scope kernel.OrgScope) string {
	in, err := c.Snapshot()
	if err != nil || scope.Validate() != nil {
		return ""
	}
	at := ""
	if !in.At.IsZero() {
		at = orgtime.FormatInstant(in.At)
	}
	raw, _ := json.Marshal([]any{1, "fleet.asset.lifecycle", scope.OrgID().String(), in.Command, in.OperationID, in.AssetID, in.ExpectedRevision, in.Reason, in.DisposalKind, at})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// LifecycleEntry is one immutable history row of a lifecycle transition.
type LifecycleEntry struct {
	private
	Revision     int64
	Command      fleet.LifecycleCommand
	From, To     fleet.State
	Reason       string
	DisposalKind fleet.DisposalKind
	DisposedAt   *time.Time
	RecordedAt   time.Time
}

func (e LifecycleEntry) clone() LifecycleEntry {
	if e.DisposedAt != nil {
		at := *e.DisposedAt
		e.DisposedAt = &at
	}
	return e
}
func (e LifecycleEntry) valid() bool {
	if e.Command == fleet.RecordAcquisition || e.Revision < 2 || !validInstant(e.RecordedAt) {
		return false
	}
	if to, err := fleet.Transition(e.From, e.Command); err != nil || to != e.To {
		return false
	}
	if fleet.RequiresReason(e.Command) != (e.Reason != "") || e.Reason != "" && !fleet.ValidReason(e.Reason) {
		return false
	}
	if e.Command == fleet.Dispose {
		_, err := fleet.ParseDisposalKind(string(e.DisposalKind))
		return err == nil && e.DisposedAt != nil && validInstant(*e.DisposedAt)
	}
	return e.DisposalKind == "" && e.DisposedAt == nil
}

type LifecycleFilter struct {
	private
	AfterRevision int64
	Limit         int
}

func (f LifecycleFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterRevision < 0 {
		return fleet.ErrInvalid
	}
	return nil
}

type LifecyclePage struct {
	private
	Entries      []LifecycleEntry
	NextRevision int64
}

type AssetLifecycleRepository interface {
	TransitionAsset(context.Context, kernel.OrgScope, FleetActor, string, AssetLifecycle) (AssetResult, bool, error)
	ListAssetLifecycle(context.Context, kernel.OrgScope, FleetActor, string, string, LifecycleFilter) (LifecyclePage, error)
}
type AssetLifecycleService struct {
	Enabled    bool
	Repository AssetLifecycleRepository
}

func validLifecycleResult(v AssetResult, in AssetLifecycleInput) bool {
	if v.AssetID != in.AssetID || v.Discarded || v.Revision != in.ExpectedRevision+1 {
		return false
	}
	if in.Command == fleet.RecordAcquisition {
		return v.State == fleet.Draft || v.State == fleet.Active || v.State == fleet.Inactive
	}
	for _, from := range []fleet.State{fleet.Active, fleet.Inactive, fleet.Disposed} {
		if to, err := fleet.Transition(from, in.Command); err == nil && to == v.State {
			return true
		}
	}
	return false
}

func (s AssetLifecycleService) Transition(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command AssetLifecycle) (AssetResult, bool, error) {
	if !s.Enabled {
		return AssetResult{}, false, ErrSurfaceDisabled
	}
	in, err := command.Snapshot()
	if err != nil {
		return AssetResult{}, false, err
	}
	if err := current(ctx, scope, actor, decision, in.AssetID, true); err != nil {
		return AssetResult{}, false, err
	}
	if unavailable(s.Repository) {
		return AssetResult{}, false, ErrUnavailable
	}
	v, replayed, err := s.Repository.TransitionAsset(ctx, scope, actor, decision.Subject().MembershipID(), command)
	if guard := current(ctx, scope, actor, decision, in.AssetID, true); guard != nil {
		return AssetResult{}, false, guard
	}
	if err != nil {
		return AssetResult{}, false, err
	}
	if !validLifecycleResult(v, in) {
		return AssetResult{}, false, ErrUnavailable
	}
	return v, replayed, nil
}

func (s AssetLifecycleService) History(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID string, filter LifecycleFilter) (LifecyclePage, error) {
	if !s.Enabled {
		return LifecyclePage{}, ErrSurfaceDisabled
	}
	if !canonicalID(assetID) {
		return LifecyclePage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return LifecyclePage{}, err
	}
	if err := current(ctx, scope, actor, decision, assetID, false); err != nil {
		return LifecyclePage{}, err
	}
	if unavailable(s.Repository) {
		return LifecyclePage{}, ErrUnavailable
	}
	v, err := s.Repository.ListAssetLifecycle(ctx, scope, actor, decision.Subject().MembershipID(), assetID, filter)
	if guard := current(ctx, scope, actor, decision, assetID, false); guard != nil {
		return LifecyclePage{}, guard
	}
	if err != nil {
		return LifecyclePage{}, err
	}
	if len(v.Entries) > filter.Limit {
		return LifecyclePage{}, ErrUnavailable
	}
	out := LifecyclePage{Entries: make([]LifecycleEntry, 0, len(v.Entries)), NextRevision: v.NextRevision}
	prior := filter.AfterRevision
	for _, entry := range v.Entries {
		if !entry.valid() || entry.Revision <= prior {
			return LifecyclePage{}, ErrUnavailable
		}
		out.Entries = append(out.Entries, entry.clone())
		prior = entry.Revision
	}
	if out.NextRevision != 0 && (len(out.Entries) != filter.Limit || out.NextRevision != prior) {
		return LifecyclePage{}, ErrUnavailable
	}
	return out, nil
}
