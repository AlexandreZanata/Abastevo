// Package application owns the persisted private document, its upload session
// and audited private access (EMP-DOC-002). Bytes only pass through orgmedia;
// this package never holds a storage key, URL or original outside a transfer.
package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

var (
	ErrDisabled        = errors.New("document: surface disabled")
	ErrUnavailable     = errors.New("document: unavailable")
	ErrNotFound        = errors.New("document: not found")
	ErrInvalid         = errors.New("document: invalid declaration")
	ErrConflict        = errors.New("document: current state differs")
	ErrOperationReused = errors.New("document: operation reused")
)

// C1 accepts synthetic receipts whose type has an active content inspector
// (EDR-153). A byte budget alone never enables a format (EMP-PLAT-006).
const (
	KindReceipt   = "RECEIPT"
	DataMode      = "SYNTHETIC"
	MediaTypeJPEG = "image/jpeg"
	UploadTTL     = 5 * time.Minute
)

type private struct{}

func (private) String() string                 { return "EnterpriseDocument(redacted)" }
func (p private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, p.String()) }
func (p private) GoString() string             { return p.String() }
func (p private) LogValue() slog.Value         { return slog.StringValue(p.String()) }
func (p private) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }

func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }

// Actor is the verified proof subject: the company session plane ("web") or a
// future signed device plane ("android"). It never comes from a request body.
type Actor struct {
	private
	AccountID, MembershipID             string
	Channel, ContributorID, DeviceKeyID string
}

func (a Actor) Validate() error {
	if !validID(a.AccountID) || !validID(a.MembershipID) {
		return ErrNotFound
	}
	switch a.Channel {
	case "web":
		if a.ContributorID != "" || a.DeviceKeyID != "" {
			return ErrNotFound
		}
	case "android":
		if !validID(a.ContributorID) || !validID(a.DeviceKeyID) {
			return ErrNotFound
		}
	default:
		return ErrNotFound
	}
	return nil
}
func (a Actor) Member() media.Member {
	return media.Member{AccountID: a.AccountID, MembershipID: a.MembershipID}
}

// Declaration is the client-declared original. Only a verified upload proves it.
type Declaration struct {
	private
	Kind, SHA256, MediaType string
	Bytes                   int64
}

// Create opens an upload session for one declared document.
type Create struct {
	private
	OperationID, DocumentID string
	Original                Declaration
}

func (c Create) Validate(scope kernel.OrgScope) error {
	if scope.Validate() != nil {
		return ErrNotFound
	}
	if !validID(c.OperationID) {
		return ErrInvalid
	}
	// The EMP-DOC-001 container decides identity and digest syntax.
	if _, err := domain.New(scope, c.DocumentID, c.Original.SHA256); err != nil {
		return ErrInvalid
	}
	limit, err := media.ByteLimit(c.Original.MediaType)
	if err != nil || c.Original.Kind != KindReceipt || c.Original.MediaType != MediaTypeJPEG || c.Original.Bytes < 1 || c.Original.Bytes > limit {
		return ErrInvalid
	}
	return nil
}

// Digest pins the whole command and its verified actor for replay comparison.
func (c Create) Digest(scope kernel.OrgScope, actor Actor) string {
	raw, _ := json.Marshal([]any{"document.upload.created.v1", scope.OrgID().String(), actor.AccountID, actor.MembershipID,
		c.OperationID, c.DocumentID, c.Original.Kind, c.Original.SHA256, c.Original.Bytes, c.Original.MediaType})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Revisions are independent per commanded dimension; reconciliation is derived.
type Revisions struct{ Storage, Extraction, Structure, Conference, Fiscal int64 }

type View struct {
	private
	ID, Kind, DataMode, OperationID, CreatedBy string
	Original                                   Declaration
	Statuses                                   domain.Statuses
	Revisions                                  Revisions
	FiscalConsultedAt                          *time.Time
	CreatedAt, UpdatedAt                       time.Time
}

// UploadAccess is the short member- and document-bound upload capability.
type UploadAccess struct {
	private
	Token     string
	ExpiresAt time.Time
}

type Created struct {
	private
	View     View
	Replayed bool
	Upload   *UploadAccess
}
type Stored struct {
	private
	View     View
	Replayed bool
}

// Facts are current server facts for a coverage decision: the document (or the
// organization root for a new document) and its authoring membership.
type Facts struct{ TargetID, AuthoredBy string }

// AttachPolicy governs declaring and uploading an own document; ContentPolicy
// governs content access. Reading a record never implies content (BR-AUTH-011).
func AttachPolicy() access.Policy {
	return access.Policy{Permission: "doc.document.attach", Capability: "organizations", Effect: access.OperationalWrite}
}
func ContentPolicy() access.Policy {
	return access.Policy{Permission: "doc.document.view_content", Capability: "organizations", Effect: access.Read}
}
