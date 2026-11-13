package protocol

import "sort"

const (
	Accepted          = "ACCEPTED"
	AcceptedForReview = "ACCEPTED_FOR_REVIEW"
	Rejected          = "REJECTED"
	Retry             = "RETRY"
)

var reviewCodes = map[string]bool{"ORPHAN_REFERENCE": true, "ACCESS_ENDED_BEFORE_RECEIPT": true, "PERMISSION_CHANGED": true, "ASSET_UNAVAILABLE": true, "ASSET_BUSY": true, "TRIP_ALREADY_CLOSED": true, "PERIOD_LOCKED": true, "STALE_REVISION": true, "ORG_SUSPENDED_GRACE": true, "PLAN_CHANGED": true}
var rejectionCodes = map[string]bool{"OPERATION_ID_REUSED": true, "MALFORMED": true, "DEVICE_REVOKED": true, "MEMBERSHIP_ENDED": true, "INTAKE_WINDOW_CLOSED": true, "FORBIDDEN": true, "ORG_SUSPENDED": true}

type Acknowledgement struct {
	redacted
	operationID, digest, result, code, receivedAt, reviewID string
	revision                                                *int64
	warnings                                                []string
}

func (ack Acknowledgement) Result() string { return ack.result }
func (ack Acknowledgement) Code() string   { return ack.code }
func (ack Acknowledgement) IsFinal() bool {
	return ack.result == Accepted || ack.result == AcceptedForReview || ack.result == Rejected
}
func (ack Acknowledgement) Warnings() []string { return append([]string(nil), ack.warnings...) }
func optionalString(value *node) (string, bool) {
	if value != nil && value.kind == 'n' {
		return "", true
	}
	raw, ok := stringValue(value)
	return raw, ok && raw != ""
}
func ParseAcknowledgement(raw []byte, operation Operation, options ...Limits) (Acknowledgement, error) {
	if len(operation.canonical) == 0 {
		return Acknowledgement{}, ErrBinding
	}
	limits, err := selectedLimits(options)
	if err != nil {
		return Acknowledgement{}, err
	}
	if len(raw) > limits.operationBytes {
		return Acknowledgement{}, ErrLimit
	}
	root, err := parseJSON(raw, limits.depth-2)
	if err != nil {
		return Acknowledgement{}, err
	}
	if !shape(root, "schema_version", "operation_id", "payload_digest", "result", "code", "entity_revision", "received_at", "review_item_id", "warnings") {
		return Acknowledgement{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return Acknowledgement{}, ErrInvalid
	}
	ack := Acknowledgement{}
	ack.operationID, ok = stringValue(root.fields["operation_id"])
	if !ok || !uuid(ack.operationID) {
		return Acknowledgement{}, ErrInvalid
	}
	ack.digest, ok = stringValue(root.fields["payload_digest"])
	if !ok || !digestText(ack.digest) {
		return Acknowledgement{}, ErrInvalid
	}
	ack.result, ok = stringValue(root.fields["result"])
	if !ok {
		return Acknowledgement{}, ErrInvalid
	}
	ack.code, ok = optionalString(root.fields["code"])
	if !ok {
		return Acknowledgement{}, ErrInvalid
	}
	ack.revision, ok = optionalRevision(root.fields["entity_revision"])
	if !ok {
		return Acknowledgement{}, ErrInvalid
	}
	ack.receivedAt, ok = optionalString(root.fields["received_at"])
	if !ok {
		return Acknowledgement{}, ErrInvalid
	}
	ack.reviewID, ok = optionalString(root.fields["review_item_id"])
	if !ok || ack.reviewID != "" && !uuid(ack.reviewID) {
		return Acknowledgement{}, ErrInvalid
	}
	warnings := root.fields["warnings"]
	if warnings.kind != 'a' {
		return Acknowledgement{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range warnings.list {
		code, ok := stringValue(item)
		if !ok || code != "CLOCK_SKEW" || seen[code] {
			return Acknowledgement{}, ErrInvalid
		}
		seen[code] = true
		ack.warnings = append(ack.warnings, code)
	}
	sort.Strings(ack.warnings)
	switch ack.result {
	case Accepted:
		if ack.code != "" || ack.reviewID != "" || ack.revision == nil || !canonicalInstant(ack.receivedAt, true) {
			return Acknowledgement{}, ErrInvalid
		}
	case AcceptedForReview:
		if !reviewCodes[ack.code] || ack.reviewID == "" || !canonicalInstant(ack.receivedAt, true) {
			return Acknowledgement{}, ErrInvalid
		}
	case Rejected:
		if !rejectionCodes[ack.code] || ack.reviewID != "" || ack.revision != nil || !canonicalInstant(ack.receivedAt, true) {
			return Acknowledgement{}, ErrInvalid
		}
	case Retry:
		if ack.code != "DEPENDENCY_PENDING" && ack.code != "UNAVAILABLE" || ack.revision != nil || ack.receivedAt != "" || ack.reviewID != "" {
			return Acknowledgement{}, ErrInvalid
		}
	default:
		return Acknowledgement{}, ErrInvalid
	}
	if ack.operationID != operation.ID() {
		return Acknowledgement{}, ErrBinding
	}
	if ack.digest != operation.PayloadDigest() {
		return Acknowledgement{}, ErrDigest
	}
	return ack, nil
}

type DeliveryProof struct {
	redacted
	FamilyID, AccessToken string
}
type Batch struct {
	redacted
	proof      DeliveryProof
	candidates [][]byte
	limits     Limits
}

func (batch Batch) Proof() DeliveryProof { return batch.proof }

// Candidate retains the trusted bounds captured by the transport decoder.
// It is an unadmitted intent: parsing does not grant authority or perform effects.
type Candidate struct {
	redacted
	raw    []byte
	limits Limits
}

func (candidate Candidate) RawBytes() []byte { return append([]byte(nil), candidate.raw...) }
func (candidate Candidate) Parse(binding Binding) (Operation, error) {
	return ParseOperation(candidate.raw, binding, candidate.limits)
}
func (batch Batch) Candidates() []Candidate {
	result := make([]Candidate, len(batch.candidates))
	for i, raw := range batch.candidates {
		result[i] = Candidate{raw: append([]byte(nil), raw...), limits: batch.limits}
	}
	return result
}

// RawCandidates exports copied bytes only. Use Candidates to retain trusted parse bounds.
func (batch Batch) RawCandidates() [][]byte {
	result := make([][]byte, len(batch.candidates))
	for i, raw := range batch.candidates {
		result[i] = append([]byte(nil), raw...)
	}
	return result
}

// ParseBatch validates transport only. A raw candidate is not a certified operation or identity.
// OFFL-002 must preflight all correlation identities/proof before any effects, then parse individually.
func ParseBatch(raw []byte, options ...Limits) (Batch, error) {
	limits, err := selectedLimits(options)
	if err != nil {
		return Batch{}, err
	}
	if len(raw) > limits.batchBytes {
		return Batch{}, ErrLimit
	}
	root, err := parseJSON(raw, limits.depth)
	if err != nil {
		return Batch{}, err
	}
	if !shape(root, "schema_version", "family_id", "access_token", "operations") {
		return Batch{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return Batch{}, ErrInvalid
	}
	family, ok := stringValue(root.fields["family_id"])
	if !ok || !uuid(family) {
		return Batch{}, ErrInvalid
	}
	token, ok := stringValue(root.fields["access_token"])
	if !ok || len(token) < 1 || len(token) > 512 {
		return Batch{}, ErrInvalid
	}
	for _, value := range []byte(token) {
		if value < 0x21 || value > 0x7e {
			return Batch{}, ErrInvalid
		}
	}
	operations := root.fields["operations"]
	if operations.kind != 'a' {
		return Batch{}, ErrInvalid
	}
	if len(operations.list) > limits.operations {
		return Batch{}, ErrLimit
	}
	batch := Batch{proof: DeliveryProof{FamilyID: family, AccessToken: token}, limits: limits}
	seen := map[string]bool{}
	for _, value := range operations.list {
		if value.kind == 'o' {
			id, ok := stringValue(value.fields["operation_id"])
			if ok && uuid(id) {
				if seen[id] {
					return Batch{}, ErrInvalid
				}
				seen[id] = true
			}
		}
		batch.candidates = append(batch.candidates, append([]byte(nil), raw[value.start:value.end]...))
	}
	return batch, nil
}
