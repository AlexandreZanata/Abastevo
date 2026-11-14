package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	stdsync "sync"
	"testing"

	sync "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

const syntheticID = "7b0e2a54-0000-4000-8000-000000000191"

func syntheticRaw() []byte {
	return []byte(`{"schema_version":1,"operation_id":"` + syntheticID + `","owner_account_id":"7b0e2a54-0000-4000-8000-000000000001","api_origin":"https://api.example.invalid","org_id":"7b0e2a54-0000-4000-8000-000000000065","membership_id":"7b0e2a54-0000-4000-8000-000000000003","enrollment_id":"7b0e2a54-0000-4000-8000-000000000004","type":"trip.start","operation_schema":1,"entity":{"type":"trip","id":"7b0e2a54-0000-4000-8000-000000000005","expected_revision":9},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"America/Cuiaba","device_offset_seconds":-14400,"depends_on":["7b0e2a54-0000-4000-8000-000000000006"],"soft_references":[{"type":"asset","id":"7b0e2a54-0000-4000-8000-000000000007"}],"attachments":[{"id":"7b0e2a54-0000-4000-8000-000000000008","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","bytes":5,"mime":"image/jpeg"}],"payload":{"note":"synthetic-sensitive-payload"}}`)
}

func syntheticOperation(t *testing.T) sync.Operation {
	t.Helper()
	operation, err := sync.Seal(syntheticRaw(), binding(t, "A"))
	if err != nil {
		t.Fatal("synthetic operation could not be sealed")
	}
	return operation
}

func acknowledgementRaw(operation sync.Operation, result, code, received, review, revision string) []byte {
	return []byte(fmt.Sprintf(`{"schema_version":1,"operation_id":%q,"payload_digest":%q,"result":%q,"code":%s,"entity_revision":%s,"received_at":%s,"review_item_id":%s,"warnings":["CLOCK_SKEW"]}`, operation.ID(), operation.PayloadDigest(), result, code, revision, received, review))
}

func TestAcknowledgementRequiresNullInsteadOfEmpty(t *testing.T) {
	operation := syntheticOperation(t)
	for name, raw := range map[string][]byte{
		"accepted-code":   acknowledgementRaw(operation, sync.Accepted, `""`, `"2026-11-13T12:00:01Z"`, "null", "10"),
		"accepted-review": acknowledgementRaw(operation, sync.Accepted, "null", `"2026-11-13T12:00:01Z"`, `""`, "10"),
		"rejected-review": acknowledgementRaw(operation, sync.Rejected, `"FORBIDDEN"`, `"2026-11-13T12:00:01Z"`, `""`, "null"),
		"retry-receipt":   acknowledgementRaw(operation, sync.Retry, `"UNAVAILABLE"`, `""`, "null", "null"),
		"retry-review":    acknowledgementRaw(operation, sync.Retry, `"DEPENDENCY_PENDING"`, "null", `""`, "null"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := sync.ParseAcknowledgement(raw, operation); !errors.Is(err, sync.ErrInvalid) {
				t.Fatal("a required null field accepted an empty string")
			}
		})
	}
}

func TestTrustedLimitsCannotExceedBatchSize(t *testing.T) {
	if _, err := sync.NewLimits(1, 100, 101, 4); !errors.Is(err, sync.ErrInvalid) {
		t.Fatal("operation limit can exceed transport limit")
	}
}

func TestOriginRejectsAmbiguousNumericHost(t *testing.T) {
	for _, origin := range []string{"https://123", "https://123."} {
		if err := sync.ValidateOrigin(origin); !errors.Is(err, sync.ErrInvalid) {
			t.Fatal("ambiguous numeric host accepted")
		}
	}
}

func batchRaw(rawOperation []byte, token string) []byte {
	return []byte(`{"schema_version":1,"family_id":"7b0e2a54-0000-4000-8000-000000000009","access_token":"` + token + `","operations":[ ` + strings.TrimSpace(string(rawOperation)) + ` ]}`)
}

func TestBatchCapturesTrustedOperationLimit(t *testing.T) {
	operation := syntheticOperation(t)
	limits, err := sync.NewLimits(2, sync.MaxBatchBytes, len(operation.CanonicalBytes())-1, sync.MaxTransportDepth)
	if err != nil {
		t.Fatal("could not configure lower trusted operation limit")
	}
	batch, err := sync.ParseBatch(batchRaw(operation.CanonicalBytes(), "synthetic_token"), limits)
	if err != nil {
		t.Fatal("transport cannot retain an individually over-limit candidate")
	}
	candidates := batch.Candidates()
	if len(candidates) != 1 {
		t.Fatal("candidate count changed")
	}
	if _, err := candidates[0].Parse(binding(t, "A")); !errors.Is(err, sync.ErrLimit) {
		t.Fatal("candidate lost its captured trusted operation limit")
	}
}

func TestProtocolValuesDefensivelyCopyPrivateState(t *testing.T) {
	raw := syntheticRaw()
	operation, err := sync.Seal(raw, binding(t, "A"))
	if err != nil {
		t.Fatal("synthetic seal failed")
	}
	want := operation.CanonicalBytes()
	wantPayload := operation.PayloadBytes()
	for i := range raw {
		raw[i] = 'x'
	}
	canonical := operation.CanonicalBytes()
	canonical[0] = 'x'
	payload := operation.PayloadBytes()
	payload[0] = 'x'
	snapshot := operation.Snapshot()
	*snapshot.Entity.ExpectedRevision = 0
	snapshot.DependsOn[0] = "changed"
	snapshot.SoftReferences[0].ID = "changed"
	snapshot.Attachments[0].MIME = "changed"
	if !bytes.Equal(operation.CanonicalBytes(), want) || !bytes.Equal(operation.PayloadBytes(), wantPayload) {
		t.Fatal("input/output bytes can mutate the immutable intent")
	}
	fresh := operation.Snapshot()
	if *fresh.Entity.ExpectedRevision != 9 || fresh.DependsOn[0] == "changed" || fresh.SoftReferences[0].ID == "changed" || fresh.Attachments[0].MIME == "changed" {
		t.Fatal("snapshot can mutate the immutable intent")
	}
	batchBytes := batchRaw(want, "synthetic-sensitive-token")
	batch, err := sync.ParseBatch(batchBytes)
	if err != nil {
		t.Fatal("synthetic batch failed")
	}
	for i := range batchBytes {
		batchBytes[i] = 'x'
	}
	exported := batch.RawCandidates()
	exported[0][0] = 'x'
	exported[0] = nil
	proof := batch.Proof()
	proof.AccessToken = "changed"
	candidates := batch.Candidates()
	candidateRaw := candidates[0].RawBytes()
	candidateRaw[0] = 'x'
	candidates[0] = sync.Candidate{}
	if !bytes.Equal(batch.Candidates()[0].RawBytes(), want) || batch.Proof().AccessToken != "synthetic-sensitive-token" {
		t.Fatal("transport exports can mutate private captured state")
	}
	ack, err := sync.ParseAcknowledgement(acknowledgementRaw(operation, sync.Accepted, "null", `"2026-11-13T12:00:01Z"`, "null", "10"), operation)
	if err != nil {
		t.Fatal("synthetic acknowledgement failed")
	}
	warnings := ack.Warnings()
	warnings[0] = "changed"
	if ack.Warnings()[0] != "CLOCK_SKEW" {
		t.Fatal("warnings export can mutate a source acknowledgement")
	}
}

func TestDiagnosticsRedactProtocolValues(t *testing.T) {
	operation := syntheticOperation(t)
	batch, err := sync.ParseBatch(batchRaw(operation.CanonicalBytes(), "synthetic-sensitive-token"))
	if err != nil {
		t.Fatal("synthetic batch failed")
	}
	ack, err := sync.ParseAcknowledgement(acknowledgementRaw(operation, sync.Accepted, "null", `"2026-11-13T12:00:01Z"`, "null", "10"), operation)
	if err != nil {
		t.Fatal("synthetic acknowledgement failed")
	}
	snapshot := operation.Snapshot()
	values := []any{binding(t, "A"), sync.DefaultLimits(), operation, &operation, snapshot, snapshot.Entity, snapshot.SoftReferences[0], snapshot.Attachments[0], batch, batch.Proof(), batch.Candidates()[0], ack}
	assertRedacted := func(raw string) {
		t.Helper()
		if !strings.Contains(raw, "redacted") {
			t.Fatal("diagnostic lacks redaction marker")
		}
		for _, secret := range []string{"7b0e2a54", "api.example.invalid", "synthetic-sensitive", operation.PayloadDigest(), strings.Repeat("a", 64)} {
			if strings.Contains(raw, secret) {
				t.Fatal("diagnostic exposed a private protocol value")
			}
		}
	}
	for _, value := range values {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			assertRedacted(fmt.Sprintf(format, value))
		}
		encoded, err := json.Marshal(map[string]any{"nested": []any{value}})
		if err != nil {
			t.Fatal("diagnostic JSON failed")
		}
		assertRedacted(string(encoded))
		var output bytes.Buffer
		slog.New(slog.NewJSONHandler(&output, nil)).Info("synthetic", "value", value)
		assertRedacted(output.String())
	}
}

func TestTrustedLimitsAndZeroValuesRefuseInvalidConfiguration(t *testing.T) {
	for _, values := range [][4]int{
		{0, 65536, 8192, 64}, {51, 65536, 8192, 64}, {1, 0, 1, 4}, {1, 65537, 1, 4},
		{1, 65536, 0, 4}, {1, 65536, 8193, 4}, {1, 65536, 8192, 3}, {1, 65536, 8192, 65},
	} {
		if _, err := sync.NewLimits(values[0], values[1], values[2], values[3]); !errors.Is(err, sync.ErrInvalid) {
			t.Fatal("trusted configuration widened or invalidated a hard bound")
		}
	}
	bound := binding(t, "A")
	operation := syntheticOperation(t)
	if _, err := sync.Seal(syntheticRaw(), sync.Binding{}); !errors.Is(err, sync.ErrBinding) {
		t.Fatal("zero owner binding accepted")
	}
	if _, err := sync.NewBinding("bad", "https://api.example.invalid", "bad"); !errors.Is(err, sync.ErrBinding) {
		t.Fatal("invalid source binding accepted")
	}
	if _, err := sync.Seal(syntheticRaw(), bound, sync.Limits{}); !errors.Is(err, sync.ErrInvalid) {
		t.Fatal("zero trusted limits accepted")
	}
	if _, err := sync.ParseBatch(batchRaw(operation.CanonicalBytes(), "synthetic"), sync.DefaultLimits(), sync.DefaultLimits()); !errors.Is(err, sync.ErrInvalid) {
		t.Fatal("ambiguous trusted configurations accepted")
	}
	if _, err := sync.ParseAcknowledgement([]byte(`{}`), sync.Operation{}); !errors.Is(err, sync.ErrBinding) {
		t.Fatal("acknowledgement accepted without original intent")
	}
	if _, err := sync.SameIntent(sync.Operation{}, operation); !errors.Is(err, sync.ErrInvalid) {
		t.Fatal("zero operation accepted as replay identity")
	}
	if _, err := (sync.Candidate{}).Parse(bound); !errors.Is(err, sync.ErrInvalid) {
		t.Fatal("zero transport candidate accepted")
	}
	if (sync.Acknowledgement{}).IsFinal() || (sync.Operation{}).SupportsSchema() {
		t.Fatal("zero value claims a certified result")
	}
	limits, err := sync.NewLimits(1, 100, 100, 4)
	if err != nil {
		t.Fatal("minimum trusted profile rejected")
	}
	if _, err := sync.ParseBatch([]byte(`{"schema_version":1,"family_id":"7b0e2a54-0000-4000-8000-000000000009","access_token":"x","operations":[]}`), limits); !errors.Is(err, sync.ErrLimit) {
		t.Fatal("smaller trusted raw transport bound ignored")
	}
}

func TestDeliveryCredentialsRenewWithoutChangingIntent(t *testing.T) {
	operation := syntheticOperation(t)
	raw := strings.ReplaceAll(string(operation.CanonicalBytes()), `"payload":`, `"payload" : `)
	first, err := sync.ParseBatch(batchRaw([]byte(raw), "synthetic-first-token"))
	if err != nil {
		t.Fatal("first synthetic batch failed")
	}
	secondBytes := batchRaw([]byte(raw), "synthetic-renewed-token")
	secondBytes = bytes.ReplaceAll(secondBytes, []byte("000000000009"), []byte("00000000000a"))
	second, err := sync.ParseBatch(secondBytes)
	if err != nil {
		t.Fatal("renewed synthetic delivery failed")
	}
	if string(first.Candidates()[0].RawBytes()) != raw {
		t.Fatal("raw transport candidate was reserialized")
	}
	a, err := first.Candidates()[0].Parse(binding(t, "A"))
	if err != nil {
		t.Fatal("first candidate rejected")
	}
	b, err := second.Candidates()[0].Parse(binding(t, "A"))
	if err != nil {
		t.Fatal("renewed candidate rejected")
	}
	same, err := sync.SameIntent(a, b)
	if err != nil || !same || first.Proof().AccessToken == second.Proof().AccessToken || first.Proof().FamilyID == second.Proof().FamilyID {
		t.Fatal("delivery credentials changed immutable operation identity")
	}
}

func TestConcurrentProtocolReadsKeepImmutableIdentity(t *testing.T) {
	operation := syntheticOperation(t)
	bound := binding(t, "A")
	batch, err := sync.ParseBatch(batchRaw(operation.CanonicalBytes(), "synthetic_token"))
	if err != nil {
		t.Fatal("synthetic batch failed")
	}
	failures := make(chan bool, 32)
	var group stdsync.WaitGroup
	for range 32 {
		group.Go(func() {
			candidate := batch.Candidates()[0]
			copyRaw := candidate.RawBytes()
			copyRaw[0] = 'x'
			copyValue := operation.Snapshot()
			*copyValue.Entity.ExpectedRevision = 0
			parsed, err := candidate.Parse(bound)
			failures <- err != nil || parsed.PayloadDigest() != operation.PayloadDigest() || *operation.Snapshot().Entity.ExpectedRevision != 9
		})
	}
	group.Wait()
	close(failures)
	for failed := range failures {
		if failed {
			t.Fatal("concurrent copied reads changed immutable protocol state")
		}
	}
}
