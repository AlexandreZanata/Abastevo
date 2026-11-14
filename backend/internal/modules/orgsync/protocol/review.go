package protocol

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ReviewAccepted                = "ACCEPTED"
	ReviewRefused                 = "REFUSED"
	MaxReviewResolutionBytes      = 4096
	MaxReviewResultsRequestBytes  = 65536
	MaxReviewResultsResponseBytes = 131072
	MaxReviewResultSelectors      = 50
)

// ReviewResolutionSnapshot is immutable outcome data, never reviewer proof.
// Schema version1 is owned by the constructor and canonical codec.
type ReviewResolutionSnapshot struct {
	redacted
	OperationID, PayloadDigest, ReviewItemID, ResolutionID string
	ReviewRevision                                         int64
	Outcome, Reason, ResolvedAt                            string
	EntityRevision                                         *int64
}

// ReviewResolution is separate from the original final REVIEW acknowledgement.
type ReviewResolution struct {
	redacted
	value     ReviewResolutionSnapshot
	canonical []byte
}

func NewReviewResolution(value ReviewResolutionSnapshot) (ReviewResolution, error) {
	if !uuid(value.OperationID) || !digestText(value.PayloadDigest) || !uuid(value.ReviewItemID) || !uuid(value.ResolutionID) || value.ReviewRevision != 2 || !canonicalInstant(value.ResolvedAt, true) || !reviewReason(value.Reason, value.Outcome == ReviewRefused) {
		return ReviewResolution{}, ErrInvalid
	}
	switch value.Outcome {
	case ReviewAccepted:
		if value.EntityRevision == nil || *value.EntityRevision < 1 {
			return ReviewResolution{}, ErrInvalid
		}
	case ReviewRefused:
		if value.EntityRevision != nil {
			return ReviewResolution{}, ErrInvalid
		}
	default:
		return ReviewResolution{}, ErrInvalid
	}
	copy := ReviewResolution{value: value}.Snapshot()
	root := &node{kind: 'o', fields: map[string]*node{
		"schema_version": reviewInteger(1), "operation_id": reviewText(copy.OperationID), "payload_digest": reviewText(copy.PayloadDigest),
		"review_item_id": reviewText(copy.ReviewItemID), "resolution_id": reviewText(copy.ResolutionID), "review_revision": reviewInteger(2),
		"outcome": reviewText(copy.Outcome), "reason": reviewText(copy.Reason), "resolved_at": reviewText(copy.ResolvedAt), "entity_revision": {kind: 'n', value: "null"},
	}}
	if copy.EntityRevision != nil {
		root.fields["entity_revision"] = reviewInteger(*copy.EntityRevision)
	}
	raw := canonical(root)
	if len(raw) > MaxReviewResolutionBytes {
		return ReviewResolution{}, ErrLimit
	}
	return ReviewResolution{value: copy, canonical: raw}, nil
}

func ParseReviewResolution(raw []byte) (ReviewResolution, error) {
	if len(raw) > MaxReviewResolutionBytes {
		return ReviewResolution{}, ErrLimit
	}
	root, err := parseJSON(raw, MaxTransportDepth)
	if err != nil {
		return ReviewResolution{}, err
	}
	return reviewResolutionNode(root)
}

func reviewResolutionNode(root *node) (ReviewResolution, error) {
	if !shape(root, "schema_version", "operation_id", "payload_digest", "review_item_id", "resolution_id", "review_revision", "outcome", "reason", "resolved_at", "entity_revision") {
		return ReviewResolution{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return ReviewResolution{}, ErrInvalid
	}
	value := ReviewResolutionSnapshot{}
	for key, destination := range map[string]*string{"operation_id": &value.OperationID, "payload_digest": &value.PayloadDigest, "review_item_id": &value.ReviewItemID, "resolution_id": &value.ResolutionID, "outcome": &value.Outcome, "reason": &value.Reason, "resolved_at": &value.ResolvedAt} {
		*destination, ok = stringValue(root.fields[key])
		if !ok {
			return ReviewResolution{}, ErrInvalid
		}
	}
	value.ReviewRevision, ok = integer(root.fields["review_revision"])
	if !ok {
		return ReviewResolution{}, ErrInvalid
	}
	value.EntityRevision, ok = optionalRevision(root.fields["entity_revision"])
	if !ok {
		return ReviewResolution{}, ErrInvalid
	}
	return NewReviewResolution(value)
}

func reviewText(value string) *node   { return &node{kind: 's', value: value} }
func reviewInteger(value int64) *node { return &node{kind: 'i', value: strconv.FormatInt(value, 10)} }
func reviewReason(value string, required bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 280 || (value != "" || required) && strings.TrimSpace(value) == "" {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (value ReviewResolution) Snapshot() ReviewResolutionSnapshot {
	copy := value.value
	if copy.EntityRevision != nil {
		n := *copy.EntityRevision
		copy.EntityRevision = &n
	}
	return copy
}

func (value ReviewResolution) CanonicalBytes() []byte {
	return append([]byte(nil), value.canonical...)
}
