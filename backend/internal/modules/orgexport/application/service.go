package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
)

// CommandInput is the decoded request body.
type CommandInput struct {
	private
	OperationID string
	Kind        string
	Parameters  []byte
}

// Command is a validated, immutable export request. Its limits and media type
// come from the admitted format, capped by the infrastructure ceilings.
type Command struct {
	private
	operationID, kind, digest, mediaType string
	read                                 permissions.Permission
	parameters                           []byte
	limits                               domain.Limits
	retention                            time.Duration
	admission                            FormatAdmission
}

// NewCommand admits only a registered kind (else ErrFormatUnavailable) with
// infrastructure- and format-canonical parameters (else domain.ErrInvalid).
func NewCommand(scope kernel.OrgScope, registry Registry, in CommandInput) (Command, error) {
	if scope.Validate() != nil || !canonicalID(in.OperationID) || !domain.ValidKind(in.Kind) {
		return Command{}, domain.ErrInvalid
	}
	format, ok := registry.Lookup(in.Kind)
	if !ok {
		return Command{}, ErrFormatUnavailable
	}
	canonical, err := domain.CanonicalParameters(in.Parameters)
	if err != nil {
		return Command{}, domain.ErrInvalid
	}
	admitted, err := format.Canonical(bytes.Clone(canonical))
	if err != nil {
		return Command{}, domain.ErrInvalid
	}
	// The format's own canonical form must stay infrastructure-canonical.
	if again, err := domain.CanonicalParameters(admitted); err != nil || !bytes.Equal(again, admitted) {
		return Command{}, domain.ErrInvalid
	}
	digest, err := domain.ParametersDigest(scope, in.Kind, admitted)
	if err != nil {
		return Command{}, domain.ErrInvalid
	}
	admission, _ := format.(FormatAdmission)
	return Command{admission: admission, operationID: in.OperationID, kind: in.Kind, digest: digest, mediaType: format.MediaType(), read: format.ReadPermission(), parameters: bytes.Clone(admitted),
		limits: format.Limits(), retention: domain.DefaultRetention}, nil
}

// ReadPermission is the read permission the kind declares (EMP-AUTH-011).
func (c Command) ReadPermission() permissions.Permission { return c.read }

func (c Command) CheckInputsTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	if c.admission == nil {
		return nil
	}
	return c.admission.CheckInputsTx(ctx, tx, scope, c.Parameters())
}

func (c Command) OperationID() string      { return c.operationID }
func (c Command) Kind() string             { return c.kind }
func (c Command) Parameters() []byte       { return bytes.Clone(c.parameters) }
func (c Command) ParametersDigest() string { return c.digest }
func (c Command) MediaType() string        { return c.mediaType }
func (c Command) Limits() domain.Limits    { return c.limits }
func (c Command) Retention() time.Duration { return c.retention }

// Valid reports a command built by NewCommand.
func (c Command) Valid() bool {
	return canonicalID(c.operationID) && domain.ValidKind(c.kind) && ExportableRead(c.read) && domain.ValidSHA256(c.digest) && domain.ValidMediaType(c.mediaType) &&
		len(c.parameters) > 0 && c.limits.Validate() == nil && domain.ValidRetention(c.retention)
}

// Digest is the canonical request hash bound to organization and requester.
func (c Command) Digest(scope kernel.OrgScope, requester Requester) string {
	if !c.Valid() || scope.Validate() != nil || requester.Validate() != nil {
		return ""
	}
	raw, _ := json.Marshal([]string{"rpt.export.request.v1", scope.OrgID().String(), requester.AccountID, requester.MembershipID, c.operationID, c.kind, c.digest, c.mediaType,
		strconv.FormatInt(c.limits.Rows, 10), strconv.FormatInt(c.limits.Bytes, 10), strconv.FormatInt(c.limits.Duration.Milliseconds(), 10), strconv.FormatInt(int64(c.retention/time.Second), 10)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// View is one stored job as its requester sees it. Absent facts are zero.
type View struct {
	private
	ExportID, Kind, ParametersDigest string
	Parameters                       []byte
	State                            domain.State
	Attempts, MaxAttempts            int
	FailureCode                      domain.FailureCode
	Rows, Bytes                      int64
	SHA256, MediaType                string
	RequestedAt, AsOf                time.Time
	ReadyAt, ExpiresAt               time.Time
	FailedAt, ExpiredAt              time.Time
}

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return !at.IsZero() && year >= 1 && year <= 9999
}

// Validate checks the stored facts are consistent with the state.
func (v View) Validate() error {
	if !canonicalID(v.ExportID) || !domain.ValidKind(v.Kind) || !domain.ValidSHA256(v.ParametersDigest) || !v.State.Valid() ||
		v.MaxAttempts < 1 || v.Attempts < 0 || v.Attempts > v.MaxAttempts || !validInstant(v.RequestedAt) || !v.AsOf.Equal(v.RequestedAt) {
		return ErrUnavailable
	}
	if canonical, err := domain.CanonicalParameters(v.Parameters); err != nil || !bytes.Equal(canonical, v.Parameters) {
		return ErrUnavailable
	}
	hasFile := v.SHA256 != ""
	if hasFile && (!domain.ValidSHA256(v.SHA256) || !domain.ValidMediaType(v.MediaType) || v.Bytes < 1 || v.Bytes > domain.MaxBytes || v.Rows < 0 || v.Rows > domain.MaxRows) ||
		!hasFile && (v.MediaType != "" || v.Bytes != 0 || v.Rows != 0) {
		return ErrUnavailable
	}
	published := v.State == domain.Ready || v.State == domain.Expired
	if published != validInstant(v.ReadyAt) || published != validInstant(v.ExpiresAt) || published && (!hasFile || !v.ExpiresAt.After(v.ReadyAt) || v.ReadyAt.Before(v.RequestedAt)) ||
		!published && (!v.ReadyAt.IsZero() || !v.ExpiresAt.IsZero()) {
		return ErrUnavailable
	}
	failed := v.State == domain.Failed
	if failed != v.FailureCode.Valid() || failed != validInstant(v.FailedAt) || !failed && (v.FailureCode != "" || !v.FailedAt.IsZero()) {
		return ErrUnavailable
	}
	if (v.State == domain.Expired) != validInstant(v.ExpiredAt) || v.State != domain.Expired && !v.ExpiredAt.IsZero() {
		return ErrUnavailable
	}
	if (v.State == domain.Running || published) && v.Attempts < 1 {
		return ErrUnavailable
	}
	return nil
}

func cloneView(v View) View {
	out := v
	out.Parameters = bytes.Clone(v.Parameters)
	return out
}

// Key is the newest-first keyset position of a list page.
type Key struct {
	RequestedAt time.Time
	ExportID    string
}

// String is the opaque cursor position "<instant>/<export_id>".
func (k Key) String() string {
	if k.Zero() {
		return ""
	}
	return orgtime.FormatInstant(k.RequestedAt) + "/" + k.ExportID
}

// ParseKey accepts only a canonical position.
func ParseKey(raw string) (Key, error) {
	stamp, id, found := strings.Cut(raw, "/")
	if !found || strings.Contains(id, "/") {
		return Key{}, domain.ErrInvalid
	}
	at, err := orgtime.ParseInstant(stamp)
	if err != nil || orgtime.FormatInstant(at) != stamp {
		return Key{}, domain.ErrInvalid
	}
	k := Key{RequestedAt: at, ExportID: id}
	if k.Validate() != nil {
		return Key{}, domain.ErrInvalid
	}
	return k, nil
}

func (k Key) Zero() bool { return k.ExportID == "" && k.RequestedAt.IsZero() }

// Before reports that k sorts after other in a newest-first page.
func (k Key) Before(other Key) bool {
	if !k.RequestedAt.Equal(other.RequestedAt) {
		return k.RequestedAt.Before(other.RequestedAt)
	}
	return k.ExportID < other.ExportID
}

func KeyOf(v View) Key { return Key{RequestedAt: v.RequestedAt, ExportID: v.ExportID} }

func (k Key) Validate() error {
	if !canonicalID(k.ExportID) || !validInstant(k.RequestedAt) {
		return domain.ErrInvalid
	}
	return nil
}

func (k Key) equal(other Key) bool {
	return k.ExportID == other.ExportID && k.RequestedAt.Equal(other.RequestedAt)
}

// MaxPage mirrors the shared private page bound.
const MaxPage = 100

type Filter struct {
	private
	State domain.State
	After Key
	Limit int
}

func (f Filter) Validate() error {
	if f.Limit < 1 || f.Limit > MaxPage || f.State != "" && !f.State.Valid() || !f.After.Zero() && f.After.Validate() != nil {
		return domain.ErrInvalid
	}
	return nil
}

type Page struct {
	private
	Exports []View
	Next    Key
}

// Repository persists requests and serves requester-bound reads. Every call
// proves current company authority in its own transaction and resolves only
// jobs of the calling membership (others are ErrNotFound).
type Repository interface {
	Request(context.Context, kernel.OrgScope, Actor, string, string, Command) (View, bool, error)
	Read(context.Context, kernel.OrgScope, Actor, string, string) (View, error)
	List(context.Context, kernel.OrgScope, Actor, string, Filter) (Page, error)
	// Deliverable returns the READY, unexpired file of the caller's job;
	// ErrNotReady, ErrExpired or ErrNotFound otherwise. In the same
	// transaction it re-checks the read permission that the registry's
	// format for the stored kind declares (ErrFormatUnavailable when the kind
	// is no longer admitted).
	Deliverable(context.Context, kernel.OrgScope, Actor, string, string, Registry) (domain.Object, error)
}

// Service is the HTTP-facing export boundary. Each call binds the completed
// request decision before and after the repository.
type Service struct {
	Enabled    bool
	Repository Repository
	Formats    Registry
	Objects    Objects
	Tokens     Tokens
	Recorder   auditing.AccessRecorder
	NewID      func() (string, error)
	Now        func() time.Time
}

// current binds the completed request decision to this actor, policy and the
// organization target.
func current(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, action Action) error {
	if ctx == nil {
		return ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope.Validate() != nil {
		return domain.ErrInvalid
	}
	policy := Policy(action)
	if actor.Validate() != nil || decision.Subject().Channel() != "web" || decision.Policy() != policy ||
		decision.Check(ctx, scope, actor.AccountID, policy.Permission, scope.OrgID().String()) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func (s Service) newID() (string, error) {
	if s.NewID == nil {
		return "", ErrUnavailable
	}
	id, err := s.NewID()
	if err != nil || !canonicalID(id) {
		return "", ErrUnavailable
	}
	return id, nil
}

func (s Service) Request(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, in CommandInput) (View, bool, error) {
	if !s.Enabled {
		return View{}, false, ErrSurfaceDisabled
	}
	if err := current(ctx, scope, actor, decision, ActionRequest); err != nil {
		return View{}, false, err
	}
	command, err := NewCommand(scope, s.Formats, in)
	if err != nil {
		return View{}, false, err
	}
	if unavailable(s.Repository) {
		return View{}, false, ErrUnavailable
	}
	exportID, err := s.newID()
	if err != nil {
		return View{}, false, err
	}
	v, replayed, err := s.Repository.Request(ctx, scope, actor, decision.Subject().MembershipID(), exportID, command)
	if guard := current(ctx, scope, actor, decision, ActionRequest); guard != nil {
		return View{}, false, guard
	}
	if err != nil {
		return View{}, false, err
	}
	if v.Validate() != nil || v.Kind != command.Kind() || v.ParametersDigest != command.ParametersDigest() || !bytes.Equal(v.Parameters, command.parameters) ||
		!replayed && (v.ExportID != exportID || v.State != domain.Queued) {
		return View{}, false, ErrUnavailable
	}
	return cloneView(v), replayed, nil
}

func (s Service) Read(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, exportID string) (View, error) {
	if !s.Enabled {
		return View{}, ErrSurfaceDisabled
	}
	if !canonicalID(exportID) {
		return View{}, ErrNotFound
	}
	if err := current(ctx, scope, actor, decision, ActionRead); err != nil {
		return View{}, err
	}
	if unavailable(s.Repository) {
		return View{}, ErrUnavailable
	}
	v, err := s.Repository.Read(ctx, scope, actor, decision.Subject().MembershipID(), exportID)
	if guard := current(ctx, scope, actor, decision, ActionRead); guard != nil {
		return View{}, guard
	}
	if err != nil {
		return View{}, err
	}
	if v.ExportID != exportID || v.Validate() != nil {
		return View{}, ErrUnavailable
	}
	return cloneView(v), nil
}

func (s Service) List(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, filter Filter) (Page, error) {
	if !s.Enabled {
		return Page{}, ErrSurfaceDisabled
	}
	if err := filter.Validate(); err != nil {
		return Page{}, err
	}
	if err := current(ctx, scope, actor, decision, ActionRead); err != nil {
		return Page{}, err
	}
	if unavailable(s.Repository) {
		return Page{}, ErrUnavailable
	}
	p, err := s.Repository.List(ctx, scope, actor, decision.Subject().MembershipID(), filter)
	if guard := current(ctx, scope, actor, decision, ActionRead); guard != nil {
		return Page{}, guard
	}
	if err != nil {
		return Page{}, err
	}
	if len(p.Exports) > filter.Limit {
		return Page{}, ErrUnavailable
	}
	out := Page{Exports: make([]View, 0, len(p.Exports)), Next: p.Next}
	prior := filter.After
	for _, v := range p.Exports {
		if v.Validate() != nil || filter.State != "" && v.State != filter.State || !prior.Zero() && !KeyOf(v).Before(prior) {
			return Page{}, ErrUnavailable
		}
		out.Exports = append(out.Exports, cloneView(v))
		prior = KeyOf(v)
	}
	if !out.Next.Zero() && (len(out.Exports) != filter.Limit || !out.Next.equal(prior)) {
		return Page{}, ErrUnavailable
	}
	return out, nil
}

// binding pins the exact delivered file without recording its digest, size or
// type as readable audit facts.
func binding(object domain.Object) string {
	raw, _ := json.Marshal([]any{"orgexport.binding.v1", object.ExportID, object.SHA256, object.MediaType, object.Bytes, object.Rows})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s Service) deliverable(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, exportID string) (Requester, domain.Object, error) {
	if err := current(ctx, scope, actor, decision, ActionRead); err != nil {
		return Requester{}, domain.Object{}, err
	}
	requester := Requester{AccountID: actor.AccountID, MembershipID: decision.Subject().MembershipID()}
	object, err := s.Repository.Deliverable(ctx, scope, actor, requester.MembershipID, exportID, s.Formats)
	if guard := current(ctx, scope, actor, decision, ActionRead); guard != nil {
		return Requester{}, domain.Object{}, guard
	}
	if err != nil {
		return Requester{}, domain.Object{}, err
	}
	if object.ExportID != exportID || object.Validate(scope) != nil || requester.Validate() != nil {
		return Requester{}, domain.Object{}, ErrUnavailable
	}
	return requester, object, nil
}

func (s Service) ready() bool {
	return s.Enabled && !unavailable(s.Repository) && !unavailable(s.Tokens) && !unavailable(s.Recorder) && s.NewID != nil && s.Now != nil
}

func (s Service) record(ctx context.Context, scope kernel.OrgScope, requester Requester, object domain.Object, stage audit.AccessStage, grant, request string) error {
	err := s.Recorder.RecordAccess(ctx, scope, audit.SensitiveAccess{Category: audit.ExportFile, Stage: stage, AccountID: requester.AccountID, MembershipID: requester.MembershipID,
		TargetID: object.ExportID, AccessID: grant, RequestID: request, Binding: binding(object)})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

// Access records EXPORT_FILE/ISSUED and only then mints the capability.
func (s Service) Access(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, exportID string) (Capability, error) {
	if !s.Enabled {
		return Capability{}, ErrSurfaceDisabled
	}
	if !canonicalID(exportID) {
		return Capability{}, ErrNotFound
	}
	if !s.ready() {
		return Capability{}, ErrUnavailable
	}
	requester, object, err := s.deliverable(ctx, scope, actor, decision, exportID)
	if err != nil {
		return Capability{}, err
	}
	grant, err := s.newID()
	if err != nil {
		return Capability{}, err
	}
	if err := s.record(ctx, scope, requester, object, audit.AccessIssued, grant, grant); err != nil {
		return Capability{}, err
	}
	capability, err := s.Tokens.Issue(scope, requester, object, grant, s.Now())
	if err != nil || capability.Token == "" {
		return Capability{}, ErrUnavailable
	}
	// Authority, state and expiry are checked again before the capability
	// leaves the service.
	_, again, err := s.deliverable(ctx, scope, actor, decision, exportID)
	if err != nil {
		return Capability{}, err
	}
	if again != object {
		return Capability{}, ErrNotFound
	}
	return capability, nil
}

// MaxCapability bounds a capability received in a request body.
const MaxCapability = 2048

// Open verifies the capability, re-checks authority and state, reads and
// verifies the object, records EXPORT_FILE/SERVED and only then returns bytes.
func (s Service) Open(ctx context.Context, scope kernel.OrgScope, actor Actor, decision access.Decision, exportID, token string) (Content, error) {
	if !s.Enabled {
		return Content{}, ErrSurfaceDisabled
	}
	if !canonicalID(exportID) || token == "" || len(token) > MaxCapability {
		return Content{}, ErrNotFound
	}
	if !s.ready() || unavailable(s.Objects) {
		return Content{}, ErrUnavailable
	}
	requester, object, err := s.deliverable(ctx, scope, actor, decision, exportID)
	if err != nil {
		return Content{}, err
	}
	grant, err := s.Tokens.Verify(scope, requester, object, token, s.Now())
	if err != nil || !canonicalID(grant) {
		return Content{}, ErrNotFound
	}
	raw, err := s.Objects.Get(ctx, scope, object)
	if err != nil || object.Check(raw) != nil {
		if ctx.Err() != nil {
			return Content{}, ctx.Err()
		}
		return Content{}, ErrUnavailable
	}
	raw = bytes.Clone(raw)
	request, err := s.newID()
	if err != nil {
		return Content{}, err
	}
	if err := s.record(ctx, scope, requester, object, audit.AccessServed, grant, request); err != nil {
		return Content{}, err
	}
	_, again, err := s.deliverable(ctx, scope, actor, decision, exportID)
	if err != nil {
		return Content{}, err
	}
	if again != object {
		return Content{}, ErrNotFound
	}
	if verified, err := s.Tokens.Verify(scope, requester, object, token, s.Now()); err != nil || verified != grant {
		return Content{}, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return Content{}, err
	}
	return Content{ExportID: object.ExportID, MediaType: object.MediaType, SHA256: object.SHA256, Rows: object.Rows, Bytes: raw}, nil
}

// errorIs reports whether err is one of the registered refusals; other
// errors stay private.
func errorIs(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
