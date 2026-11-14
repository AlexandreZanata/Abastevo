package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func reviewID(n string) string { return "7b0e2a54-0000-4000-8000-00000000" + n }
func reviewSnapshot() protocol.ReviewResolutionSnapshot {
	n := int64(9223372036854775807)
	return protocol.ReviewResolutionSnapshot{OperationID: reviewID("0101"), PayloadDigest: "sha256:" + strings.Repeat("a", 64), ReviewItemID: reviewID("0102"), ResolutionID: reviewID("0103"), ReviewRevision: 2, Outcome: protocol.ReviewAccepted, Reason: "Private exact reason 😀", ResolvedAt: "2026-11-14T12:13:14.123456789Z", EntityRevision: &n}
}
func TestReviewResolutionIsSeparateExactImmutableEvidence(t *testing.T) {
	in := reviewSnapshot()
	value, err := protocol.NewReviewResolution(in)
	if err != nil {
		t.Fatal("valid distinct resolution refused", err)
	}
	*in.EntityRevision = 1
	snapshot := value.Snapshot()
	if *snapshot.EntityRevision != 9223372036854775807 || snapshot.ReviewRevision != 2 || snapshot.Reason != "Private exact reason 😀" {
		t.Fatal("constructor aliased or rounded evidence")
	}
	*snapshot.EntityRevision = 3
	raw := value.CanonicalBytes()
	parsed, err := protocol.ParseReviewResolution(raw)
	if err != nil || !bytes.Equal(parsed.CanonicalBytes(), raw) || *value.Snapshot().EntityRevision != 9223372036854775807 {
		t.Fatal("canonical exact round trip lost evidence", err)
	}
	raw[0] = '!'
	if value.CanonicalBytes()[0] != '{' {
		t.Fatal("canonical bytes aliased")
	}
	encoded, _ := json.Marshal(value)
	if strings.Contains(fmt.Sprintf("%#v %s %s", value, value.Snapshot(), encoded), "Private exact") {
		t.Fatal("private resolution leaked through diagnostics")
	}
}
func TestReviewResolutionRefusedHasIndependentReasonAndNoRevision(t *testing.T) {
	in := reviewSnapshot()
	in.Outcome, in.EntityRevision = protocol.ReviewRefused, nil
	value, err := protocol.NewReviewResolution(in)
	if err != nil || value.Snapshot().Outcome != "REFUSED" || value.Snapshot().EntityRevision != nil {
		t.Fatal("actual refusal reinterpreted as operation rejection", err)
	}
	if _, err := protocol.ParseReviewResolution(value.CanonicalBytes()); err != nil {
		t.Fatal("explicit null entity revision lost from refusal", err)
	}
	in.Reason = ""
	if _, err := protocol.NewReviewResolution(in); !errors.Is(err, protocol.ErrInvalid) {
		t.Fatal("empty refusal reason admitted", err)
	}
}
func TestReviewResolutionUnicodeAndReceiptBounds(t *testing.T) {
	in := reviewSnapshot()
	in.Reason = strings.Repeat("😀", 280)
	if _, err := protocol.NewReviewResolution(in); err != nil {
		t.Fatal("280 scalars refused as UTF16 units", err)
	}
	for name, edit := range map[string]func(*protocol.ReviewResolutionSnapshot){
		"reason281":     func(v *protocol.ReviewResolutionSnapshot) { v.Reason = strings.Repeat("😀", 281) },
		"control":       func(v *protocol.ReviewResolutionSnapshot) { v.Reason = "private\nreason" },
		"badutf8":       func(v *protocol.ReviewResolutionSnapshot) { v.Reason = string([]byte{0xff}) },
		"unset":         func(v *protocol.ReviewResolutionSnapshot) { v.ResolvedAt = "0001-01-01T00:00:00Z" },
		"offset":        func(v *protocol.ReviewResolutionSnapshot) { v.ResolvedAt = "2026-11-14T08:13:14-04:00" },
		"revision1":     func(v *protocol.ReviewResolutionSnapshot) { v.ReviewRevision = 1 },
		"missingentity": func(v *protocol.ReviewResolutionSnapshot) { v.EntityRevision = nil },
		"ackrejected":   func(v *protocol.ReviewResolutionSnapshot) { v.Outcome = "REJECTED" },
		"zerouuid":      func(v *protocol.ReviewResolutionSnapshot) { v.ResolutionID = "00000000-0000-0000-0000-000000000000" },
	} {
		t.Run(name, func(t *testing.T) {
			v := reviewSnapshot()
			edit(&v)
			if _, err := protocol.NewReviewResolution(v); !errors.Is(err, protocol.ErrInvalid) {
				t.Fatal("invalid resolution evidence admitted", err)
			}
		})
	}
}
