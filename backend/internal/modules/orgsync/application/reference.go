package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Reference data feed for the selected organization (EMP-OFFL-006,
// EDR-216..218): a purpose-bound read copy with monotonic revisions,
// durable tombstones and a server confirmation instant. Never authority.

const (
	ReferenceSchema      = 1
	ReferenceEntityAsset = "asset"
	ReferenceUpsert      = "UPSERT"
	ReferenceDelete      = "DELETE"
	ReferenceDelta       = "DELTA"
	ReferenceReset       = "RESET"
	ReferenceNewCursor   = "NEW_CURSOR"
	ReferenceExpired     = "CURSOR_EXPIRED"
	ReferenceAhead       = "CURSOR_AHEAD"
	ReferenceForbidden   = "FORBIDDEN"
	ReferenceCatalog     = "fuel-products-v1"
	// ReferenceMaxEntities bounds one member's feed; more is unavailable
	// until a paging owner exists (retained).
	ReferenceMaxEntities = 200
)

var ErrReferenceCursor = errors.New("orgsync: reference cursor not found")

type ReferenceMeter struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Modulus     *int64 `json:"modulus"`
	SegmentID   string `json:"segment_id"`
	InstalledAt string `json:"installed_at"`
}

type ReferenceAssignment struct {
	Kind    string  `json:"kind"`
	ValidTo *string `json:"valid_to"`
}

// ReferenceAsset is the minimised asset entity (B-BR-EMP-336). Slices are
// sorted by the source so the digest is deterministic.
type ReferenceAsset struct {
	ID          string              `json:"id"`
	Revision    int64               `json:"revision"`
	Kind        string              `json:"kind"`
	UsageBasis  string              `json:"usage_basis"`
	State       string              `json:"state"`
	Name        string              `json:"name"`
	FleetNumber string              `json:"fleet_number"`
	Plate       string              `json:"plate"`
	Catalog     string              `json:"catalog"`
	Products    []string            `json:"products"`
	Meters      []ReferenceMeter    `json:"meters"`
	Assignment  ReferenceAssignment `json:"assignment"`
}

// ReferenceAssetSource adds the server-only coverage fact (current unit).
type ReferenceAssetSource struct {
	Asset  ReferenceAsset
	UnitID string
}

// ReferenceAssets reads the member's currently assigned assets at the
// database instant inside the caller's transaction (fleet owner).
type ReferenceAssets interface {
	ReferenceAssetsDBTX(context.Context, DBTX, kernel.OrgScope, string, time.Time) ([]ReferenceAssetSource, error)
}

// ReferenceEntry is one logged change; Digest is empty for a tombstone.
type ReferenceEntry struct {
	Revision                         int64
	EntityType, EntityID, Op, Digest string
}

type ReferenceChange struct {
	Revision             int64
	EntityType, EntityID string
	Op                   string
	Asset                *ReferenceAsset
}

type ReferenceCursor struct {
	OrgID, MembershipID, EnrollmentID, Epoch string
	Revision                                 int64
}

type ReferenceFeed struct {
	Mode, Reason string
	FromRevision *int64
	Revision     int64
	ConfirmedAt  time.Time
	Cursor       string
	Changes      []ReferenceChange
}

type ReferenceRepository interface {
	PullReference(context.Context, Source, *ReferenceCursor) (ReferenceFeed, error)
}

// Digest is the SHA-256 of the canonical JSON projection.
func (asset ReferenceAsset) Digest() (string, error) {
	raw, err := json.Marshal(asset)
	if err != nil {
		return "", ErrUnavailable
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// DiffReference returns the changes to log, ordered by entity id, between the
// last logged state per entity and the current authorized digests: an upsert
// when an entity is new, returns or changed; one tombstone when it left.
func DiffReference(last map[string]ReferenceEntry, current map[string]string) []ReferenceEntry {
	ids := make([]string, 0, len(last)+len(current))
	for id := range current {
		ids = append(ids, id)
	}
	for id := range last {
		if _, present := current[id]; !present {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var out []ReferenceEntry
	for _, id := range ids {
		prior, logged := last[id]
		digest, present := current[id]
		switch {
		case present && (!logged || prior.Op != ReferenceUpsert || prior.Digest != digest):
			out = append(out, ReferenceEntry{EntityType: ReferenceEntityAsset, EntityID: id, Op: ReferenceUpsert, Digest: digest})
		case !present && logged && prior.Op == ReferenceUpsert:
			out = append(out, ReferenceEntry{EntityType: ReferenceEntityAsset, EntityID: id, Op: ReferenceDelete})
		}
	}
	return out
}

var referenceID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type referenceCursorWire struct {
	Version      int    `json:"v"`
	OrgID        string `json:"o"`
	MembershipID string `json:"m"`
	EnrollmentID string `json:"e"`
	Epoch        string `json:"p"`
	Revision     int64  `json:"r"`
}

func (cursor ReferenceCursor) valid() bool {
	return referenceID.MatchString(cursor.OrgID) && referenceID.MatchString(cursor.MembershipID) && referenceID.MatchString(cursor.EnrollmentID) && referenceID.MatchString(cursor.Epoch) && cursor.Revision >= 0
}

func referenceMAC(secret, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("reference-cursor-v1."))
	mac.Write(payload)
	return mac.Sum(nil)
}

func SignReferenceCursor(secret []byte, cursor ReferenceCursor) (string, error) {
	if len(secret) < 32 || !cursor.valid() {
		return "", ErrUnavailable
	}
	payload, err := json.Marshal(referenceCursorWire{Version: 1, OrgID: cursor.OrgID, MembershipID: cursor.MembershipID, EnrollmentID: cursor.EnrollmentID, Epoch: cursor.Epoch, Revision: cursor.Revision})
	if err != nil {
		return "", ErrUnavailable
	}
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(referenceMAC(secret, payload)), nil
}

// OpenReferenceCursor verifies the signature and returns the bound cursor.
func OpenReferenceCursor(secret []byte, raw string) (ReferenceCursor, error) {
	encoded, signature, ok := strings.Cut(raw, ".")
	if len(secret) < 32 || !ok || len(raw) > 1024 {
		return ReferenceCursor{}, ErrReferenceCursor
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	mac, err2 := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || err2 != nil || !hmac.Equal(mac, referenceMAC(secret, payload)) {
		return ReferenceCursor{}, ErrReferenceCursor
	}
	var v referenceCursorWire
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&v) != nil || v.Version != 1 {
		return ReferenceCursor{}, ErrReferenceCursor
	}
	cursor := ReferenceCursor{OrgID: v.OrgID, MembershipID: v.MembershipID, EnrollmentID: v.EnrollmentID, Epoch: v.Epoch, Revision: v.Revision}
	if !cursor.valid() {
		return ReferenceCursor{}, ErrReferenceCursor
	}
	return cursor, nil
}
