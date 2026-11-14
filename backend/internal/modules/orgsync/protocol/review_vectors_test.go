package protocol_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

const reviewVectorHeader = "id|operation|input_hex|context_hex|outcome|canonical_hex"
const reviewVectorSHA = "db1e7499590d6c5e226f77be4f5962ec43ad0411356fcb4c1cdecfb8a5683dd8"

type reviewVector struct {
	id, operation, outcome    string
	input, context, canonical []byte
}

func decodeReviewHex(raw string) ([]byte, error) {
	value, err := hex.DecodeString(raw)
	if err != nil || len(value) == 0 || hex.EncodeToString(value) != raw {
		return nil, errors.New("invalid fixture hex")
	}
	return value, nil
}
func loadReviewVectors(raw []byte) ([]reviewVector, error) {
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 139 || lines[0] != reviewVectorHeader || lines[len(lines)-1] != "" {
		return nil, errors.New("fixture framing/count")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	values := make([]reviewVector, 0, 137)
	for _, line := range lines[1 : len(lines)-1] {
		fields := strings.Split(line, "|")
		if len(fields) != 6 || fields[0] == "" || seen[fields[0]] {
			return nil, errors.New("fixture identity/columns")
		}
		seen[fields[0]] = true
		if fields[1] != "RESOLUTION" && fields[1] != "REQUEST" && fields[1] != "RESPONSE" {
			return nil, errors.New("unknown fixture operation")
		}
		counts[fields[1]]++
		v := reviewVector{id: fields[0], operation: fields[1], outcome: fields[4]}
		var err error
		v.input, err = decodeReviewHex(fields[2])
		if err != nil {
			return nil, err
		}
		if v.operation == "RESPONSE" {
			v.context, err = decodeReviewHex(fields[3])
			if err != nil {
				return nil, err
			}
		} else if fields[3] != "-" {
			return nil, errors.New("unexpected context")
		}
		switch v.outcome {
		case "OK":
			v.canonical, err = decodeReviewHex(fields[5])
			if err != nil {
				return nil, err
			}
		case "INVALID", "LIMIT", "BINDING", "DIGEST":
			if fields[5] != "-" {
				return nil, errors.New("unexpected error canonical")
			}
		default:
			return nil, errors.New("unknown fixture outcome")
		}
		values = append(values, v)
	}
	if counts["RESOLUTION"] != 66 || counts["REQUEST"] != 33 || counts["RESPONSE"] != 38 {
		return nil, errors.New("fixture group counts")
	}
	return values, nil
}

type reviewFixtureContext struct {
	Request json.RawMessage `json:"request"`
	Source  struct {
		Owner      string `json:"owner_account_id"`
		Origin     string `json:"api_origin"`
		Org        string `json:"org_id"`
		Member     string `json:"membership_id"`
		Enrollment string `json:"enrollment_id"`
	} `json:"source"`
	Expected []struct {
		Operation string `json:"operation_id"`
		Digest    string `json:"payload_digest"`
		Review    string `json:"review_item_id"`
	} `json:"expected"`
}

func runReviewVector(v reviewVector) ([]byte, error) {
	switch v.operation {
	case "RESOLUTION":
		r, e := protocol.ParseReviewResolution(v.input)
		return r.CanonicalBytes(), e
	case "REQUEST":
		r, e := protocol.ParseReviewResultsRequest(v.input)
		return r.CanonicalBytes(), e
	case "RESPONSE":
		canonical, e := protocol.CanonicalJSON(v.context)
		if e != nil || !bytes.Equal(canonical, v.context) {
			return nil, errors.New("malformed fixture context")
		}
		var c reviewFixtureContext
		decoder := json.NewDecoder(bytes.NewReader(v.context))
		decoder.DisallowUnknownFields()
		if e := decoder.Decode(&c); e != nil {
			return nil, e
		}
		request, e := protocol.ParseReviewResultsRequest(c.Request)
		if e != nil {
			return nil, fmt.Errorf("fixture request: %w", e)
		}
		source, e := protocol.NewSourceBinding(c.Source.Owner, c.Source.Origin, c.Source.Org, c.Source.Member, c.Source.Enrollment)
		if e != nil {
			return nil, fmt.Errorf("fixture source: %w", e)
		}
		expected := make([]protocol.ReviewExpectation, len(c.Expected))
		for i, item := range c.Expected {
			expected[i] = protocol.ReviewExpectation{Selector: protocol.ReviewSelector{OperationID: item.Operation, PayloadDigest: item.Digest}, ReviewItemID: item.Review}
		}
		r, e := protocol.ParseReviewResultsResponse(v.input, request, source, expected)
		return r.CanonicalBytes(), e
	}
	return nil, errors.New("unknown fixture operation")
}
func TestReviewSharedVectors(t *testing.T) {
	raw, e := os.ReadFile("../../../../../contracts/testdata/enterprise/sync/review-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != reviewVectorSHA {
		t.Fatal("shared corpus changed without reviewed freeze")
	}
	values, e := loadReviewVectors(raw)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range values {
		t.Run(v.id, func(t *testing.T) {
			actual, err := runReviewVector(v)
			if v.outcome == "OK" {
				if err != nil || !bytes.Equal(actual, v.canonical) {
					t.Fatal("shared canonical outcome mismatch", err)
				}
				return
			}
			want := map[string]error{"INVALID": protocol.ErrInvalid, "LIMIT": protocol.ErrLimit, "BINDING": protocol.ErrBinding, "DIGEST": protocol.ErrDigest}[v.outcome]
			if !errors.Is(err, want) || len(actual) != 0 {
				t.Fatal("shared failure/prefix mismatch", err)
			}
		})
	}
}
func TestReviewSharedLoaderRejectsMalformedGrammar(t *testing.T) {
	raw, e := os.ReadFile("../../../../../contracts/testdata/enterprise/sync/review-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(string) string{"header": func(s string) string { return strings.Replace(s, "id|operation", "id|kind", 1) }, "namespace": func(s string) string { return strings.Replace(s, "|RESOLUTION|", "|UNKNOWN|", 1) }, "blank-row": func(s string) string { return strings.Replace(s, "\n", "\n\n", 1) }, "invalid-hex": func(s string) string { return strings.Replace(s, "|7b", "|zb", 1) }, "duplicate-id": func(s string) string {
		lines := strings.Split(s, "\n")
		a := strings.Split(lines[1], "|")
		b := strings.Split(lines[2], "|")
		b[0] = a[0]
		lines[2] = strings.Join(b, "|")
		return strings.Join(lines, "\n")
	}} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadReviewVectors([]byte(mutate(string(raw)))); err == nil {
				t.Fatal("malformed grammar consumed as domain case")
			}
		})
	}
}
