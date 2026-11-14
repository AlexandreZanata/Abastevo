package protocol

import "encoding/json"

// SourceBinding is independently resolved by the signed source boundary.
type SourceBinding struct {
	redacted
	binding            Binding
	member, enrollment string
}

func NewSourceBinding(owner, origin, organization, member, enrollment string) (SourceBinding, error) {
	binding, err := NewBinding(owner, origin, organization)
	if err != nil || !uuid(member) || !uuid(enrollment) {
		return SourceBinding{}, ErrBinding
	}
	return SourceBinding{binding: binding, member: member, enrollment: enrollment}, nil
}

type PreflightBatch struct {
	redacted
	operations []Operation
	limits     Limits
	proof      DeliveryProof
}

func (batch PreflightBatch) Operations() []Operation {
	return append([]Operation(nil), batch.operations...)
}
func (batch PreflightBatch) Limits() Limits       { return batch.limits }
func (batch PreflightBatch) Proof() DeliveryProof { return batch.proof }

// Preflight certifies all frozen envelopes and source tuples before intake writes.
func Preflight(batch Batch, source SourceBinding) (PreflightBatch, error) {
	if !source.binding.valid() || !uuid(source.member) || !uuid(source.enrollment) {
		return PreflightBatch{}, ErrBinding
	}
	limits, err := selectedLimits([]Limits{batch.limits})
	if err != nil || !uuid(batch.proof.FamilyID) || len(batch.proof.AccessToken) < 1 || len(batch.proof.AccessToken) > 512 {
		return PreflightBatch{}, ErrInvalid
	}
	for _, value := range []byte(batch.proof.AccessToken) {
		if value < 0x21 || value > 0x7e {
			return PreflightBatch{}, ErrInvalid
		}
	}
	if len(batch.candidates) > limits.operations {
		return PreflightBatch{}, ErrLimit
	}
	prepared := PreflightBatch{limits: limits, proof: batch.proof}
	seen := make(map[string]bool)
	for _, candidate := range batch.Candidates() {
		operation, err := candidate.Parse(source.binding)
		if err != nil {
			return PreflightBatch{}, err
		}
		value := operation.Snapshot()
		if value.MembershipID != source.member || value.EnrollmentID != source.enrollment {
			return PreflightBatch{}, ErrBinding
		}
		if seen[operation.ID()] {
			return PreflightBatch{}, ErrInvalid
		}
		seen[operation.ID()] = true
		prepared.operations = append(prepared.operations, operation)
	}
	return prepared, nil
}

func ValidOperationType(value string) bool { return commandType(value) }

type AcknowledgementSnapshot struct {
	redacted
	OperationID, PayloadDigest, Result, Code, ReceivedAt, ReviewItemID string
	EntityRevision                                                     *int64
	Warnings                                                           []string
}

func (ack Acknowledgement) Snapshot() AcknowledgementSnapshot {
	value := AcknowledgementSnapshot{OperationID: ack.operationID, PayloadDigest: ack.digest, Result: ack.result,
		Code: ack.code, ReceivedAt: ack.receivedAt, ReviewItemID: ack.reviewID, Warnings: ack.Warnings()}
	if ack.revision != nil {
		revision := *ack.revision
		value.EntityRevision = &revision
	}
	return value
}

func (ack Acknowledgement) CanonicalBytes() []byte { return append([]byte(nil), ack.canonical...) }

// NewAcknowledgement derives correlation from the original immutable operation.
// Nonempty supplied correlation fields must match; all branches reuse the decoder.
func NewAcknowledgement(operation Operation, value AcknowledgementSnapshot, options ...Limits) (Acknowledgement, error) {
	if len(operation.canonical) == 0 || value.OperationID != "" && value.OperationID != operation.ID() {
		return Acknowledgement{}, ErrBinding
	}
	if value.PayloadDigest != "" && value.PayloadDigest != operation.PayloadDigest() {
		return Acknowledgement{}, ErrDigest
	}
	limits, err := selectedLimits(options)
	if err != nil {
		return Acknowledgement{}, err
	}
	remaining := limits.operationBytes
	texts := []string{value.Result, value.Code, value.ReceivedAt, value.ReviewItemID}
	if len(value.Warnings) > remaining {
		return Acknowledgement{}, ErrLimit
	}
	texts = append(texts, value.Warnings...)
	for _, text := range texts {
		if len(text) > remaining {
			return Acknowledgement{}, ErrLimit
		}
		remaining -= len(text)
	}
	nullable := func(text string) any {
		if text == "" {
			return nil
		}
		return text
	}
	raw, err := json.Marshal(map[string]any{
		"schema_version": 1, "operation_id": operation.ID(), "payload_digest": operation.PayloadDigest(),
		"result": value.Result, "code": nullable(value.Code), "entity_revision": value.EntityRevision,
		"received_at": nullable(value.ReceivedAt), "review_item_id": nullable(value.ReviewItemID),
		"warnings": append([]string{}, value.Warnings...),
	})
	if err != nil {
		return Acknowledgement{}, ErrInvalid
	}
	return ParseAcknowledgement(raw, operation, limits)
}
