package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// MaxLinkPage bounds one page of link metadata.
const MaxLinkPage = 100

// Binding names the reference column that carries a resolved target
// (EDR-162). Each binding belongs to exactly one target kind.
type Binding string

const (
	BindAsset            Binding = "ASSET"
	BindEmployee         Binding = "EMPLOYEE"
	BindMeterReading     Binding = "METER_READING"
	BindHeldMeterReading Binding = "HELD_METER_READING"
	// BindIncident carries an EMP-CHECK-003 report (EDR-239).
	BindIncident Binding = "INCIDENT"
	// BindFueling carries an EMP-FUEL-002 fueling (EMP-FUEL-011, EDR-297).
	BindFueling Binding = "FUELING"
)

// Kind reports the target kind a binding may carry.
func (b Binding) Kind() (domain.TargetKind, bool) {
	switch b {
	case BindAsset:
		return domain.Asset, true
	case BindEmployee:
		return domain.Employee, true
	case BindMeterReading, BindHeldMeterReading:
		return domain.MeterReading, true
	case BindIncident:
		return domain.Incident, true
	case BindFueling:
		return domain.Fueling, true
	}
	return "", false
}

// ResolvedTarget is the owning module's current fact about one target record.
// AuthoredBy is the authoring membership used for SELF coverage, if any.
type ResolvedTarget struct {
	Binding    Binding
	AuthoredBy string
}

// TargetResolver is implemented by the module that owns a target kind. The
// permission is the owner's own fixed permission for associating a document
// with (or listing the associations of) one of its records; the resolver
// reads the record inside the caller's scoped transaction. A missing or
// foreign record is ErrNotFound. Neither method grants document access.
type TargetResolver interface {
	DocumentTargetPermission(domain.TargetKind) (permissions.Permission, error)
	ResolveDocumentTargetTx(context.Context, pgx.Tx, kernel.OrgScope, domain.TargetKind, string) (ResolvedTarget, error)
}

// Targets is the closed registry of linkable kinds: a kind without a
// registered owner resolver is refused, never stored as a dangling reference.
type Targets map[domain.TargetKind]TargetResolver

// Resolver returns the registered owner resolver of a kind.
func (t Targets) Resolver(kind domain.TargetKind) (TargetResolver, error) {
	if _, err := domain.ParseTargetKind(string(kind)); err != nil {
		return nil, ErrInvalid
	}
	resolver, ok := t[kind]
	if !ok || resolver == nil {
		return nil, ErrInvalid
	}
	return resolver, nil
}

// Policy builds the owner's target-side policy for the given effect: never a
// document permission, never fresh, Read or OperationalWrite only (EDR-163).
func (t Targets) Policy(kind domain.TargetKind, effect access.Effect) (access.Policy, TargetResolver, error) {
	resolver, err := t.Resolver(kind)
	if err != nil {
		return access.Policy{}, nil, err
	}
	if effect != access.Read && effect != access.OperationalWrite {
		return access.Policy{}, nil, ErrUnavailable
	}
	permission, err := resolver.DocumentTargetPermission(kind)
	if err != nil {
		return access.Policy{}, nil, ErrUnavailable
	}
	policy := access.Policy{Permission: permission, Capability: "organizations", Effect: effect}
	if strings.HasPrefix(string(permission), "doc.") || policy.Validate() != nil {
		return access.Policy{}, nil, ErrUnavailable
	}
	return policy, resolver, nil
}

// TargetRef selects one target record by kind and UUID.
type TargetRef struct {
	Kind domain.TargetKind
	ID   string
}

// Validate applies the EMP-DOC-001 closed kind and canonical identifier; the
// registry, not the syntax, decides whether the kind is linkable.
func (r TargetRef) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrNotFound
	}
	if _, err := domain.NewTarget(scope, r.Kind, r.ID); err != nil {
		return ErrInvalid
	}
	return nil
}

// Link associates one document with one target record (EMP-DOC-003).
type Link struct {
	private
	OperationID, DocumentID string
	Target                  TargetRef
}

func (l Link) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrNotFound
	}
	if !validID(l.OperationID) {
		return ErrInvalid
	}
	if !validID(l.DocumentID) {
		return ErrNotFound
	}
	return l.Target.Validate(scope)
}

// Digest pins the whole command and its verified actor for replay comparison.
func (l Link) Digest(scope kernel.OrgScope, actor Actor) string {
	raw, _ := json.Marshal([]any{"document.link.created.v1", scope.OrgID().String(), actor.AccountID, actor.MembershipID,
		l.OperationID, l.DocumentID, string(l.Target.Kind), l.Target.ID})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LinkView is link metadata only: it carries the document's storage status,
// never content, digest, storage locator or amount.
type LinkView struct {
	private
	DocumentID      string
	Target          TargetRef
	DocumentStorage domain.State
	OperationID     string
	CreatedBy       string
	CreatedAt       time.Time
}

type Linked struct {
	private
	View     LinkView
	Replayed bool
}

// LinkPage is one keyset page; More reports that the last item has successors.
type LinkPage struct {
	private
	Items []LinkView
	More  bool
}

// DocumentLinkFilter pages a document's links by (kind, target).
type DocumentLinkFilter struct {
	Limit int
	After *TargetRef
}

func (f DocumentLinkFilter) Validate(scope kernel.OrgScope) error {
	if f.Limit < 1 || f.Limit > MaxLinkPage {
		return ErrInvalid
	}
	if f.After != nil {
		return f.After.Validate(scope)
	}
	return nil
}

// TargetLinkFilter pages a target's links by document.
type TargetLinkFilter struct {
	Limit         int
	AfterDocument string
}

func (f TargetLinkFilter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxLinkPage || f.AfterDocument != "" && !validID(f.AfterDocument) {
		return ErrInvalid
	}
	return nil
}

// Linker is the document link port. An operation consumer calls it inside its
// own scoped transaction, so the record and its document link commit or roll
// back together; the port checks both endpoints itself.
type Linker interface {
	LinkTx(context.Context, pgx.Tx, kernel.OrgScope, Actor, Link) (LinkView, bool, error)
}

// LinkRepository persists links. Every method checks current authority on
// both endpoints in its own scoped transaction.
type LinkRepository interface {
	Linker
	Link(context.Context, kernel.OrgScope, Actor, Link) (LinkView, bool, error)
	DocumentLinks(context.Context, kernel.OrgScope, Actor, string, DocumentLinkFilter) (LinkPage, error)
	TargetLinks(context.Context, kernel.OrgScope, Actor, TargetRef, TargetLinkFilter) (LinkPage, error)
}

// LinkReadPolicy governs the link lists: the attach permission read-only.
// Creating a link uses AttachPolicy on the document.
func LinkReadPolicy() access.Policy {
	return access.Policy{Permission: "doc.document.attach", Capability: "organizations", Effect: access.Read}
}

func (s Service) linksReady(scope kernel.OrgScope, actor Actor) error {
	if !s.Enabled {
		return ErrDisabled
	}
	if s.Links == nil {
		return ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil {
		return ErrNotFound
	}
	return nil
}

// Link creates or returns one link. It allocates nothing and grants nothing.
func (s Service) Link(ctx context.Context, scope kernel.OrgScope, actor Actor, command Link) (Linked, error) {
	if err := s.linksReady(scope, actor); err != nil {
		return Linked{}, err
	}
	if err := command.Validate(scope); err != nil {
		return Linked{}, err
	}
	view, replayed, err := s.Links.Link(ctx, scope, actor, command)
	if err != nil {
		return Linked{}, err
	}
	return Linked{View: view, Replayed: replayed}, nil
}

// DocumentLinks pages the links of one document.
func (s Service) DocumentLinks(ctx context.Context, scope kernel.OrgScope, actor Actor, documentID string, filter DocumentLinkFilter) (LinkPage, error) {
	if err := s.linksReady(scope, actor); err != nil {
		return LinkPage{}, err
	}
	if !validID(documentID) {
		return LinkPage{}, ErrNotFound
	}
	if err := filter.Validate(scope); err != nil {
		return LinkPage{}, err
	}
	return s.Links.DocumentLinks(ctx, scope, actor, documentID, filter)
}

// TargetLinks pages the links of one target record.
func (s Service) TargetLinks(ctx context.Context, scope kernel.OrgScope, actor Actor, target TargetRef, filter TargetLinkFilter) (LinkPage, error) {
	if err := s.linksReady(scope, actor); err != nil {
		return LinkPage{}, err
	}
	if err := target.Validate(scope); err != nil {
		return LinkPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return LinkPage{}, err
	}
	return s.Links.TargetLinks(ctx, scope, actor, target, filter)
}
