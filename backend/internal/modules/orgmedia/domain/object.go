// Package domain defines private original identities. It performs no I/O.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid     = errors.New("orgmedia: invalid input")
	ErrNotFound    = errors.New("orgmedia: not found")
	ErrUnavailable = errors.New("orgmedia: unavailable")
	ErrDisabled    = errors.New("orgmedia: disabled")
	ErrIntegrity   = errors.New("orgmedia: integrity mismatch")
	ErrTooLarge    = errors.New("orgmedia: size exceeded")
	ErrUnsupported = errors.New("orgmedia: unsupported content")
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const AccessTTL = 5 * 60 // seconds; the transport token's separate deadline is mandatory.
type Intent uint8

const (
	Upload Intent = iota
	Issue
	Read
)

type Member struct{ AccountID, MembershipID string }

func ValidID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}
func (member Member) Validate() error {
	if !ValidID(member.AccountID) || !ValidID(member.MembershipID) {
		return ErrInvalid
	}
	return nil
}

// Object comes from current server-owned document metadata, never client JSON.
// Ready is set only by the owning document verification flow.
type Object struct {
	OrgID, DocumentID, SHA256, MIME string
	Bytes, Revision                 int64
	Synthetic, Ready                bool
}

func ByteLimit(mime string) (int64, error) {
	switch mime {
	case "image/jpeg", "image/png":
		return 5 << 20, nil
	case "application/pdf":
		return 10 << 20, nil
	case "application/xml":
		return 1 << 20, nil
	default:
		return 0, ErrUnsupported
	}
}
func (object Object) Validate(scope kernel.OrgScope, id string) error {
	limit, err := ByteLimit(object.MIME)
	if scope.Validate() != nil || !ValidID(id) || object.OrgID != scope.OrgID().String() ||
		object.DocumentID != id || !hashPattern.MatchString(object.SHA256) || err != nil ||
		object.Bytes < 1 || object.Bytes > limit || object.Revision < 1 || !object.Synthetic {
		return ErrInvalid
	}
	return nil
}
func (object Object) Key(scope kernel.OrgScope) (string, error) {
	if err := object.Validate(scope, object.DocumentID); err != nil {
		return "", err
	}
	return "org/" + object.OrgID + "/doc/" + object.DocumentID + "/" + object.SHA256, nil
}
func (object Object) CheckSnapshot(raw []byte) error {
	if int64(len(raw)) > object.Bytes {
		return ErrTooLarge
	}
	if int64(len(raw)) != object.Bytes {
		return ErrIntegrity
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != object.SHA256 {
		return ErrIntegrity
	}
	return nil
}
func (object Object) SameIdentity(other Object) bool {
	object.Ready = false
	other.Ready = false
	return object == other
}
