package protocol_test

import (
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	sync "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func binding(t *testing.T, alias string) sync.Binding {
	t.Helper()
	owner := "7b0e2a54-0000-4000-8000-000000000001"
	org := "7b0e2a54-0000-4000-8000-000000000065"
	origin := "https://api.example.invalid"
	switch alias {
	case "A":
	case "OWNER_B":
		owner = "7b0e2a54-0000-4000-8000-000000000002"
	case "ORG_B":
		org = "7b0e2a54-0000-4000-8000-000000000066"
	case "ORIGIN_B":
		origin = "https://other.example.invalid"
	default:
		t.Fatal("unknown binding alias")
	}
	result, err := sync.NewBinding(owner, origin, org)
	if err != nil {
		t.Fatal("invalid synthetic binding")
	}
	return result
}
func TestOperationVectors(t *testing.T) {
	vectors(t, "operations-v1.psv", 202, func(t *testing.T, operation string, args map[string]string) map[string]string {
		if operation == "ORIGIN" {
			if err := sync.ValidateOrigin(string(rawHex(t, args["origin_hex"]))); err != nil {
				return errorFields(t, err)
			}
			return map[string]string{"valid": "true"}
		}
		bound := binding(t, args["binding"])
		switch operation {
		case "SEAL", "VERIFY":
			var value sync.Operation
			var err error
			if operation == "SEAL" {
				value, err = sync.Seal(rawHex(t, args["raw_hex"]), bound)
			} else {
				value, err = sync.ParseOperation(rawHex(t, args["raw_hex"]), bound)
			}
			if err != nil {
				return errorFields(t, err)
			}
			return map[string]string{"canonical_hex": hex.EncodeToString(value.CanonicalBytes()), "payload_digest": value.PayloadDigest(), "supported": strconv.FormatBool(value.SupportsSchema())}
		case "BATCH", "SCAN":
			batch, err := sync.ParseBatch(rawHex(t, args["raw_hex"]))
			if err != nil {
				return errorFields(t, err)
			}
			candidates := batch.Candidates()
			if operation == "BATCH" {
				return map[string]string{"operations": strconv.Itoa(len(candidates))}
			}
			var outcomes []string
			for _, candidate := range candidates {
				value, err := candidate.Parse(bound)
				if err != nil {
					outcomes = append(outcomes, errorFields(t, err)["error"])
				} else if !value.SupportsSchema() {
					outcomes = append(outcomes, "UNSUPPORTED")
				} else {
					outcomes = append(outcomes, "OK")
				}
			}
			return map[string]string{"outcomes": strings.Join(outcomes, ",")}
		case "ACK":
			intent, err := sync.ParseOperation(rawHex(t, args["operation_hex"]), bound)
			if err != nil {
				t.Fatal("bad oracle intent")
			}
			ack, err := sync.ParseAcknowledgement(rawHex(t, args["raw_hex"]), intent)
			if err != nil {
				return errorFields(t, err)
			}
			code := ack.Code()
			if code == "" {
				code = "?"
			}
			return map[string]string{"final": strconv.FormatBool(ack.IsFinal()), "result": ack.Result(), "code": code}
		case "REPLAY":
			first, err := sync.ParseOperation(rawHex(t, args["first_hex"]), bound)
			if err != nil {
				t.Fatal("bad first intent")
			}
			second, err := sync.ParseOperation(rawHex(t, args["second_hex"]), bound)
			if err != nil {
				t.Fatal("bad second intent")
			}
			same, err := sync.SameIntent(first, second)
			if err != nil {
				return errorFields(t, err)
			}
			return map[string]string{"same": strconv.FormatBool(same)}
		}
		t.Fatal("unknown operation vector")
		return nil
	})
}
