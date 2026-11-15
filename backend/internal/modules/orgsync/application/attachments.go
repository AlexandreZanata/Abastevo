package application

import (
	"context"
	"time"
)

// AttachmentSchema is the signed attachment session wire version (EMP-OFFL-005).
const AttachmentSchema = 1

// AttachmentDeclaration is one device attachment: the document it declares
// and the record it links to (B-BR-EMP-417). Parsing it grants nothing.
type AttachmentDeclaration struct {
	privateValue
	DeclarationID, DocumentID, Kind, SHA256, MediaType string
	Bytes                                              int64
	LinkID, TargetKind, TargetID                       string
}

// AttachmentSession is the document's current storage state and, only while
// it is UPLOADING inside its session, a fresh upload capability.
type AttachmentSession struct {
	privateValue
	DocumentID, Storage string
	StorageRevision     int64
	Capability          string
	ExpiresAt           time.Time
}

// AttachmentSessions is implemented by the document owner over the verified
// device source; it rechecks document and target authority itself.
type AttachmentSessions interface {
	OpenAttachment(context.Context, Source, AttachmentDeclaration) (AttachmentSession, error)
}
