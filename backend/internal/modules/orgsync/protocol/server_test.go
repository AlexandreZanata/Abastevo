package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	protocol "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func intakeSource(t *testing.T) protocol.SourceBinding {
	t.Helper()
	value, err := protocol.NewSourceBinding("7b0e2a54-0000-4000-8000-000000000001", "https://api.example.invalid",
		"7b0e2a54-0000-4000-8000-000000000065", "7b0e2a54-0000-4000-8000-000000000003", "7b0e2a54-0000-4000-8000-000000000004")
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func intakeVariant(t *testing.T, change func(map[string]any)) protocol.Operation {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(syntheticRaw(), &value); err != nil {
		t.Fatal(err)
	}
	change(value)
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := protocol.Seal(raw, binding(t, "A"))
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func intakeBatch(t *testing.T, raw string, options ...protocol.Limits) protocol.Batch {
	t.Helper()
	batch, err := protocol.ParseBatch([]byte(`{"schema_version":1,"family_id":"7b0e2a54-0000-4000-8000-000000000009","access_token":"synthetic-sensitive-token","operations":[`+raw+`]}`), options...)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestServerPreflightCertifiesEveryItemBeforeReturningAny(t *testing.T) {
	first := syntheticOperation(t)
	second := intakeVariant(t, func(value map[string]any) {
		value["operation_id"] = "7b0e2a54-0000-4000-8000-000000000192"
		value["operation_schema"] = 2
	})
	batch := intakeBatch(t, string(first.CanonicalBytes())+","+string(second.CanonicalBytes()))
	prepared, err := protocol.Preflight(batch, intakeSource(t))
	if err != nil || len(prepared.Operations()) != 2 || prepared.Operations()[1].SupportsSchema() {
		t.Fatal("fully correlated unsupported operation was not independently preflightable", err)
	}
	values := prepared.Operations()
	values[0] = protocol.Operation{}
	if prepared.Operations()[0].ID() != first.ID() || prepared.Proof().AccessToken != "synthetic-sensitive-token" {
		t.Fatal("preflight result can be mutated or lost delivery proof")
	}
	for _, suffix := range []string{`{}`, `null`, strings.Replace(string(second.CanonicalBytes()), second.PayloadDigest(), "sha256:"+strings.Repeat("0", 64), 1)} {
		invalid := intakeBatch(t, string(first.CanonicalBytes())+","+suffix)
		value, err := protocol.Preflight(invalid, intakeSource(t))
		if err == nil || len(value.Operations()) != 0 {
			t.Fatal("a broken trailing item exposed a valid prefix")
		}
	}
}

func TestServerPreflightRequiresExactSourceAndCapturedLimits(t *testing.T) {
	for _, field := range []string{"membership_id", "enrollment_id"} {
		operation := intakeVariant(t, func(value map[string]any) { value[field] = "7b0e2a54-0000-4000-8000-000000000099" })
		if _, err := protocol.Preflight(intakeBatch(t, string(operation.CanonicalBytes())), intakeSource(t)); !errors.Is(err, protocol.ErrBinding) {
			t.Fatal("a foreign source tuple was preflighted")
		}
	}
	operation := syntheticOperation(t)
	limits, err := protocol.NewLimits(1, protocol.MaxBatchBytes, len(operation.CanonicalBytes())-1, protocol.MaxTransportDepth)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Preflight(intakeBatch(t, string(operation.CanonicalBytes()), limits), intakeSource(t)); !errors.Is(err, protocol.ErrLimit) {
		t.Fatal("preflight widened a captured operation budget")
	}
	if _, err := protocol.Preflight(protocol.Batch{}, intakeSource(t)); err == nil {
		t.Fatal("zero transport accepted")
	}
	if _, err := protocol.Preflight(intakeBatch(t, ""), protocol.SourceBinding{}); !errors.Is(err, protocol.ErrBinding) {
		t.Fatal("zero source accepted")
	}
	if value, err := protocol.Preflight(intakeBatch(t, ""), intakeSource(t)); err != nil || len(value.Operations()) != 0 {
		t.Fatal("valid empty scan refused", err)
	}
}

func TestServerAcknowledgementBuilderUsesFrozenBranchesAndDefensiveCanonicalBytes(t *testing.T) {
	operation := syntheticOperation(t)
	revision := int64(10)
	cases := []protocol.AcknowledgementSnapshot{
		{Result: protocol.Accepted, EntityRevision: &revision, ReceivedAt: "2026-11-13T12:00:01Z", Warnings: []string{"CLOCK_SKEW"}},
		{Result: protocol.AcceptedForReview, Code: "ORPHAN_REFERENCE", ReviewItemID: "7b0e2a54-0000-4000-8000-000000000193", ReceivedAt: "2026-11-13T12:00:01Z"},
		{Result: protocol.Rejected, Code: "MALFORMED", ReceivedAt: "2026-11-13T12:00:01Z"},
		{Result: protocol.Retry, Code: "DEPENDENCY_PENDING"},
	}
	for _, value := range cases {
		ack, err := protocol.NewAcknowledgement(operation, value)
		if err != nil {
			t.Fatal("frozen ACK branch was not constructible", err)
		}
		want := ack.CanonicalBytes()
		if _, err := protocol.ParseAcknowledgement(want, operation); err != nil {
			t.Fatal("built ACK did not round trip", err)
		}
		got := ack.CanonicalBytes()
		got[0] = 'x'
		snapshot := ack.Snapshot()
		if snapshot.EntityRevision != nil {
			*snapshot.EntityRevision = 0
		}
		if len(snapshot.Warnings) != 0 {
			snapshot.Warnings[0] = "changed"
			if ack.Warnings()[0] != "CLOCK_SKEW" {
				t.Fatal("snapshot warnings changed the original receipt")
			}
		}
		if !bytes.Equal(want, ack.CanonicalBytes()) || ack.Snapshot().OperationID != operation.ID() || ack.Snapshot().PayloadDigest != operation.PayloadDigest() {
			t.Fatal("ACK snapshot/canonical export changed receipt identity")
		}
		for _, encoded := range []string{fmt.Sprintf("%#v", ack), fmt.Sprint(snapshot)} {
			if strings.Contains(encoded, operation.ID()) || strings.Contains(encoded, operation.PayloadDigest()) {
				t.Fatal("private ACK data entered default formatting")
			}
		}
	}
}

func TestServerBuildersRetainTrustedLimitsAndSourceValidation(t *testing.T) {
	for _, fields := range [][5]string{
		{"invalid", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000000065", "7b0e2a54-0000-4000-8000-000000000003", "7b0e2a54-0000-4000-8000-000000000004"},
		{"7b0e2a54-0000-4000-8000-000000000001", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000000065", "invalid", "7b0e2a54-0000-4000-8000-000000000004"},
	} {
		if _, err := protocol.NewSourceBinding(fields[0], fields[1], fields[2], fields[3], fields[4]); !errors.Is(err, protocol.ErrBinding) {
			t.Fatal("invalid independently resolved source accepted")
		}
	}
	operation := syntheticOperation(t)
	limits, err := protocol.NewLimits(1, protocol.MaxBatchBytes, 32, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []protocol.AcknowledgementSnapshot{{Result: protocol.Retry, Code: "UNAVAILABLE"}, {Result: protocol.Rejected, Code: strings.Repeat("x", 33)}, {Result: protocol.Retry, Code: "UNAVAILABLE", Warnings: make([]string, 33)}} {
		if _, err := protocol.NewAcknowledgement(operation, value, limits); !errors.Is(err, protocol.ErrLimit) {
			t.Fatal("builder widened trusted result limits")
		}
	}
	if _, err := protocol.NewAcknowledgement(operation, protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: "UNAVAILABLE"}, protocol.Limits{}); !errors.Is(err, protocol.ErrInvalid) {
		t.Fatal("zero trusted limits accepted")
	}
}

func TestServerAcknowledgementBuilderRejectsWrongIdentityAndFalseFinality(t *testing.T) {
	operation := syntheticOperation(t)
	for _, value := range []protocol.AcknowledgementSnapshot{
		{Result: protocol.Retry, Code: "UNAVAILABLE", ReceivedAt: "2026-11-13T12:00:01Z"},
		{Result: protocol.Accepted, ReceivedAt: "2026-11-13T12:00:01Z"},
		{Result: protocol.AcceptedForReview, Code: "ORPHAN_REFERENCE", ReceivedAt: "2026-11-13T12:00:01Z"},
		{Result: protocol.Rejected, Code: "MALFORMED", ReceivedAt: "0001-01-01T00:00:00Z"},
		{Result: protocol.Retry, Code: "UNAVAILABLE", Warnings: []string{"CLOCK_SKEW", "CLOCK_SKEW"}},
	} {
		if _, err := protocol.NewAcknowledgement(operation, value); err == nil {
			t.Fatal("invalid branch manufactured a receipt")
		}
	}
	if _, err := protocol.NewAcknowledgement(operation, protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: "UNAVAILABLE", OperationID: "7b0e2a54-0000-4000-8000-000000000099"}); !errors.Is(err, protocol.ErrBinding) {
		t.Fatal("foreign correlation accepted")
	}
	if _, err := protocol.NewAcknowledgement(operation, protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: "UNAVAILABLE", PayloadDigest: "sha256:" + strings.Repeat("0", 64)}); !errors.Is(err, protocol.ErrDigest) {
		t.Fatal("changed digest accepted")
	}
	if _, err := protocol.NewAcknowledgement(protocol.Operation{}, protocol.AcknowledgementSnapshot{Result: protocol.Retry, Code: "UNAVAILABLE"}); err == nil {
		t.Fatal("an unverified operation manufactured an ACK")
	}
}
