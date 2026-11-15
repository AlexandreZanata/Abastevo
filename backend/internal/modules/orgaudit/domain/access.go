package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Sensitive-access vocabulary (EMP-PRIV-005, BR-AUD-003/004, BR-AUTH-011).
// A protected read checks scope, membership, session and the category's own
// permission, records one access through an AccessRecorder and only then
// mints a capability or releases content.

// AccessCategory is the closed set of sensitive reads that are recorded.
type AccessCategory string

const (
	DocumentContent AccessCategory = "DOCUMENT_CONTENT"
	ExportFile      AccessCategory = "EXPORT_FILE"
	LocationLive    AccessCategory = "LOCATION_LIVE"
	LocationTrack   AccessCategory = "LOCATION_TRACK"
)

// AccessStage says whether a short-lived capability was granted or content
// left the API.
type AccessStage string

const (
	AccessIssued AccessStage = "ISSUED"
	AccessServed AccessStage = "SERVED"
)

// AccessReason is the only registered reason: a member authorized in scope.
// No operator or support reason exists until support sessions do.
const AccessReason = "member.authorized"

// ErrReplayed reports a request ID that already holds this exact record. The
// caller must not serve again under it: every actual access needs its own.
var ErrReplayed = errors.New("orgaudit: access already recorded")

type accessSpec struct{ permission, targetType, issued, served string }

// A live position is served directly; the other categories are delivered
// through a short-lived capability (BR-PRIV-004, UC-PRIV-05). The document
// actions are the EMP-PLAT-006 names, kept unchanged.
var accessSpecs = map[AccessCategory]accessSpec{
	DocumentContent: {"doc.document.view_content", "document", "document.access.issued", "document.content.accessed"},
	ExportFile:      {"rpt.export.run", "export", "export.access.issued", "export.content.accessed"},
	LocationLive:    {"gps.location.read_live", "trip", "", "location.live.accessed"},
	LocationTrack:   {"gps.track.read", "trip", "location.track.issued", "location.track.accessed"},
}

// AccessCategories returns the closed category set in a stable order.
func AccessCategories() []AccessCategory {
	categories := make([]AccessCategory, 0, len(accessSpecs))
	for category := range accessSpecs {
		categories = append(categories, category)
	}
	slices.Sort(categories)
	return categories
}

// Permission is the catalog permission the owning read must check in scope.
// It is never implied by reading the trip, export or document record.
func (category AccessCategory) Permission() string { return accessSpecs[category].permission }

// TargetType is the audited target reference type.
func (category AccessCategory) TargetType() string { return accessSpecs[category].targetType }

// Action is the registered audit action of a category at one stage.
func (category AccessCategory) Action(stage AccessStage) (string, error) {
	spec := accessSpecs[category]
	action := ""
	switch stage {
	case AccessIssued:
		action = spec.issued
	case AccessServed:
		action = spec.served
	}
	if action == "" {
		return "", ErrInvalid
	}
	return action, nil
}

// SensitiveAccess holds only scoped references and a bounded binding digest.
// AccountID is the verified actor and MembershipID its current membership in
// the scope. RequestID is server-generated for each actual access. AccessID
// names the capability: a capability read references its issuance, while an
// issuance or a live read may use its own request ID. Binding is the owning
// module's 64-hex digest of the served item's identity, never content.
type SensitiveAccess struct {
	Category     AccessCategory
	Stage        AccessStage
	AccountID    string
	MembershipID string
	TargetID     string
	AccessID     string
	RequestID    string
	Binding      string
}

// Validate refuses anything but UUIDs, closed codes and a 64-hex digest, so
// content, names, keys, URLs, tokens or coordinates cannot be recorded.
func (access SensitiveAccess) Validate() error {
	if _, err := access.Category.Action(access.Stage); err != nil {
		return ErrInvalid
	}
	for _, id := range []string{access.AccountID, access.MembershipID, access.TargetID, access.AccessID, access.RequestID} {
		if !ValidID(id) {
			return ErrInvalid
		}
	}
	if !hashPattern.MatchString(access.Binding) {
		return ErrInvalid
	}
	switch {
	case access.Category == LocationLive && access.AccessID != access.RequestID:
		return ErrInvalid
	case access.Category != LocationLive && access.Stage == AccessServed && access.AccessID == access.RequestID:
		return ErrInvalid
	}
	return nil
}

// Event derives the generic audit event. Its digest covers every fact,
// including the membership, grant and binding that are not event columns.
func (access SensitiveAccess) Event(scope kernel.OrgScope) (Event, error) {
	if scope.Validate() != nil || access.Validate() != nil {
		return Event{}, ErrInvalid
	}
	action, _ := access.Category.Action(access.Stage)
	targetType := access.Category.TargetType()
	raw, _ := json.Marshal([]string{"orgaudit.access.v1", scope.OrgID().String(), access.AccountID, access.MembershipID,
		string(access.Category), string(access.Stage), targetType, access.TargetID, access.AccessID, access.RequestID, access.Binding})
	sum := sha256.Sum256(raw)
	return Event{OperationID: access.RequestID, ActorID: access.AccountID, Action: action, TargetType: targetType,
		TargetID: access.TargetID, ReasonCode: AccessReason, RequestHash: hex.EncodeToString(sum[:])}, nil
}

// AccessDefinitions returns the registered vocabulary of the named categories
// (all when none is named). An unknown or repeated category returns nil, which
// NewCatalog refuses, so composition fails closed.
func AccessDefinitions(categories ...AccessCategory) []Definition {
	if len(categories) == 0 {
		categories = AccessCategories()
	}
	var definitions []Definition
	for index, category := range categories {
		if _, known := accessSpecs[category]; !known || slices.Contains(categories[:index], category) {
			return nil
		}
		for _, stage := range []AccessStage{AccessIssued, AccessServed} {
			if action, err := category.Action(stage); err == nil {
				definitions = append(definitions, Definition{Action: action, TargetType: category.TargetType(),
					Reasons: []string{AccessReason}, RequireReason: true})
			}
		}
	}
	return definitions
}
