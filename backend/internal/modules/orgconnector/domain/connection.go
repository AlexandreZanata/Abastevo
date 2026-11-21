package domain

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid   = errors.New("orgconnector: invalid connection")
	ErrDenied    = errors.New("orgconnector: connection denied")
	ErrConflict  = errors.New("orgconnector: connection state conflict")
	ErrNotReady  = errors.New("orgconnector: readiness approval required")
	providerCode = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

type Kind string

const (
	Telematics         Kind = "TELEMATICS"
	FuelCardStatement  Kind = "FUEL_CARD_STATEMENT"
	FiscalLookup       Kind = "FISCAL_LOOKUP"
	DocumentExtraction Kind = "DOCUMENT_EXTRACTION"
	OutboundWebhook    Kind = "OUTBOUND_WEBHOOK"
	AccountingExport   Kind = "ACCOUNTING_EXPORT"
)

func (k Kind) Valid() bool {
	switch k {
	case Telematics, FuelCardStatement, FiscalLookup, DocumentExtraction, OutboundWebhook, AccountingExport:
		return true
	}
	return false
}

type State string

const (
	Configured State = "CONFIGURED"
	Active     State = "ACTIVE"
	Degraded   State = "DEGRADED"
	Disabled   State = "DISABLED"
)

func (s State) Allows(next State) bool {
	switch next {
	case Active:
		return s == Configured || s == Degraded || s == Disabled
	case Disabled:
		return s == Configured || s == Active || s == Degraded
	case Degraded:
		return s == Active
	}
	return false
}

type Connection struct {
	ID, Label, Provider string
	Kind                Kind
	State               State
	Revision            int64
	ChangedAt           time.Time
	ApprovalCurrent     bool
}

func ValidLabel(value string) bool {
	return len(value) > 0 && len(value) <= 100 && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}

type CredentialRef struct{ id string }

func NewCredentialRef(raw string) (CredentialRef, error) {
	if raw != "" && !validID(raw) {
		return CredentialRef{}, ErrInvalid
	}
	return CredentialRef{id: raw}, nil
}

func (CredentialRef) String() string       { return "[private connector reference]" }
func (CredentialRef) GoString() string     { return "[private connector reference]" }
func (CredentialRef) LogValue() slog.Value { return slog.StringValue("[private connector reference]") }
func (r CredentialRef) Reference() string  { return r.id }

type Approval struct {
	ID, Provider, Resource string
	Revision               int64
	Kind                   Kind
	Decision               string
	Credential             CredentialRef
	MaxBytes               int64
	MaxItems               int32
	ReadyAt                time.Time
}

func (a Approval) Check(kind Kind, resource string, now time.Time, requireReady bool) error {
	if !validID(a.ID) || a.Revision < 1 || !kind.Valid() || !providerCode.MatchString(a.Provider) ||
		len(a.Provider) > 64 || a.MaxBytes < 1 || a.MaxItems < 1 || now.IsZero() ||
		(a.Resource != "" && !validID(a.Resource)) || a.Kind != kind || a.Resource != resource || a.Decision != "APPROVED" {
		return ErrDenied
	}
	if requireReady && (a.ReadyAt.IsZero() || a.ReadyAt.After(now) || kind != FuelCardStatement && a.Credential.Reference() == "") {
		return ErrNotReady
	}
	return nil
}

func validID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}
