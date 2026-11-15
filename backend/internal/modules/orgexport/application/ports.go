// Package application owns the organization export boundary (EMP-RPT-001):
// request, requester reads, audited delivery and the worker that produces and
// publishes a bounded private file. Repositories prove current authority
// inside their own transaction; formats are a closed registry.
package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound          = errors.New("orgexport: export not found")
	ErrUnavailable       = errors.New("orgexport: unavailable")
	ErrUnauthenticated   = errors.New("orgexport: current company proof required")
	ErrOperationReused   = errors.New("orgexport: operation identifier reused")
	ErrSurfaceDisabled   = errors.New("orgexport: surface disabled")
	ErrFormatUnavailable = errors.New("orgexport: export format unavailable")
	ErrQueueFull         = errors.New("orgexport: open export limit reached")
	ErrNotReady          = errors.New("orgexport: export not ready")
	ErrExpired           = errors.New("orgexport: export expired")
	// ErrRightsLost is the worker's verdict that the requester no longer
	// holds export authority (BR-RPT-002).
	ErrRightsLost = errors.New("orgexport: requester authority lost")
	// ErrBusy means another holder owns the job now; the queue retries later.
	ErrBusy = errors.New("orgexport: export held by another worker")
	// ErrSettled means the job is READY or terminal: nothing to do.
	ErrSettled = errors.New("orgexport: export already settled")
)

type private struct{}

func (private) String() string                 { return "OrgExport(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return []byte(`"OrgExport(redacted)"`), nil }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }

// Actor is the current company account and its web proof.
type Actor struct {
	private
	AccountID, SessionID string
}

func canonicalID(raw string) bool { return domain.ValidID(raw) }

func (a Actor) Validate() error {
	if !canonicalID(a.AccountID) || !canonicalID(a.SessionID) {
		return ErrUnauthenticated
	}
	return nil
}

// Requester is the account and membership recorded on a job.
type Requester struct {
	private
	AccountID, MembershipID string
}

func (r Requester) Validate() error {
	if !canonicalID(r.AccountID) || !canonicalID(r.MembershipID) {
		return domain.ErrInvalid
	}
	return nil
}

// Action is the closed set of export policies (EDR-157).
type Action uint8

const (
	ActionRequest Action = iota + 1
	ActionRead
)

// Policy returns the fixed policy of an action. The catalog requires fresh
// proof for every rpt.export.run policy.
func Policy(action Action) access.Policy {
	switch action {
	case ActionRequest:
		return access.Policy{Permission: domain.Permission, Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true}
	case ActionRead:
		return access.Policy{Permission: domain.Permission, Capability: "organizations", Effect: access.Read, Fresh: true}
	}
	return access.Policy{}
}

// Authority is the export permission port. CheckExportTx retains the company
// account/proof → capability → root → membership/role order for the
// organization target in the caller's transaction. CheckDataTx runs in the
// same transaction after CheckExportTx and proves that the same ACTIVE
// membership holds a current grant covering the kind's declared read
// permission for the whole organization (EMP-AUTH-011); a missing grant is the
// bounded permission denial of that key, never implied by rpt.export.run.
// CheckRequesterTx re-evaluates a recorded requester without a session for
// the worker: active account, the same ACTIVE membership, a root that accepts
// work and current roles covering rpt.export.run and the kind's read
// permission; lost authority is ErrRightsLost.
type Authority interface {
	CheckExportTx(context.Context, pgx.Tx, kernel.OrgScope, Actor, string, Action) error
	CheckDataTx(context.Context, pgx.Tx, kernel.OrgScope, Actor, string, permissions.Permission) error
	CheckRequesterTx(context.Context, pgx.Tx, kernel.OrgScope, Requester, string, permissions.Permission) error
}

// ExportableRead reports a permission an export kind may declare as the read
// permission of its data (EMP-AUTH-011): a registered
// <module>.<resource>.read key outside location (gps.*). Document content,
// live position and tracks are separate permissions with their own audit
// categories (BR-AUTH-011, BR-RPT-003); rpt.export.run is never its own read.
func ExportableRead(permission permissions.Permission) bool {
	if _, err := permissions.SystemCatalog().Lookup(permissions.Owner, permission); err != nil {
		return false
	}
	parts := strings.Split(string(permission), ".")
	return len(parts) == 3 && parts[0] != "gps" && parts[2] == "read"
}

// ProduceInput is what a format reads: the scope, the recorded requester
// (whose scoped rights the format applies), canonical parameters and the
// "as of" instant. Content must be deterministic for the same input.
type ProduceInput struct {
	private
	Scope      kernel.OrgScope
	ExportID   string
	Requester  Requester
	Parameters []byte
	AsOf       time.Time
}

// Format is an admitted export format (EMP-RPT-002 owns real ones).
type Format interface {
	Kind() string
	// ReadPermission is the read permission of the exported data. The
	// request, every worker attempt and every delivery check it in addition
	// to rpt.export.run (EMP-AUTH-011, UC-RPT-01).
	ReadPermission() permissions.Permission
	MediaType() string
	Limits() domain.Limits
	// Canonical validates and canonicalizes infrastructure-canonical
	// parameters; an error is a validation failure.
	Canonical([]byte) ([]byte, error)
	// Produce writes only through the bounded sink and observes ctx.
	Produce(context.Context, ProduceInput, *domain.Sink) error
}

// Registry is the closed set of admitted formats.
type Registry struct{ formats map[string]Format }

// NewRegistry admits each format once, with a valid kind, an exportable read
// permission, a media type and ceilings within the infrastructure ceilings.
func NewRegistry(formats ...Format) (Registry, error) {
	r := Registry{formats: make(map[string]Format, len(formats))}
	for _, f := range formats {
		if unavailable(f) || !domain.ValidKind(f.Kind()) || !ExportableRead(f.ReadPermission()) || !domain.ValidMediaType(f.MediaType()) || f.Limits().Validate() != nil {
			return Registry{}, domain.ErrInvalid
		}
		if _, duplicate := r.formats[f.Kind()]; duplicate {
			return Registry{}, domain.ErrInvalid
		}
		r.formats[f.Kind()] = f
	}
	return r, nil
}

func (r Registry) Lookup(kind string) (Format, bool) {
	f, ok := r.formats[kind]
	return f, ok
}

// ReadPermission returns the read permission an admitted kind declares.
func (r Registry) ReadPermission(kind string) (permissions.Permission, bool) {
	f, ok := r.formats[kind]
	if !ok {
		return "", false
	}
	return f.ReadPermission(), true
}

// Kinds lists the admitted kinds in a stable order.
func (r Registry) Kinds() []string {
	kinds := make([]string, 0, len(r.formats))
	for kind := range r.formats {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
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

// Objects stores private export files (BR-PRIV-004). Put is create-only and
// verifies the stored copy; Get is bounded and verified; Delete confirms
// absence.
type Objects interface {
	Put(context.Context, kernel.OrgScope, domain.Object, []byte) error
	Get(context.Context, kernel.OrgScope, domain.Object) ([]byte, error)
	Delete(context.Context, kernel.OrgScope, domain.Object) error
}

// Capability is the short-lived requester-bound download grant.
type Capability struct {
	Token     string
	ExpiresAt time.Time
}

func (Capability) String() string       { return "orgexport: capability redacted" }
func (Capability) GoString() string     { return "orgexport: capability redacted" }
func (Capability) LogValue() slog.Value { return slog.StringValue("orgexport: capability redacted") }

// Tokens mints and verifies capabilities bound to the requester and file.
type Tokens interface {
	Issue(kernel.OrgScope, Requester, domain.Object, string, time.Time) (Capability, error)
	Verify(kernel.OrgScope, Requester, domain.Object, string, time.Time) (string, error)
}

// Content is the audited delivery result.
type Content struct {
	ExportID, MediaType, SHA256 string
	Rows                        int64
	Bytes                       []byte
}

func (Content) String() string       { return "orgexport: content redacted" }
func (Content) GoString() string     { return "orgexport: content redacted" }
func (Content) LogValue() slog.Value { return slog.StringValue("orgexport: content redacted") }
