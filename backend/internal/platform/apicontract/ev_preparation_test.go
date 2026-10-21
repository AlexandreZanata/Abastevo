package apicontract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// Independent schema/byte oracle only. EE-06 still owns the real bounded Go
// bridge, source census parity, cancellation/close and atomic admission.
func TestEVPreparationContract(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(filepath.Join(contractsDir(t), "openapi", "ev-preparation-planned.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatal(err)
	}
	if doc.Paths.Len() != 0 || doc.Extensions["x-implementation-status"] != "CONTRACT_FROZEN_RUNTIME_PENDING" {
		t.Fatal("prepared contracts cannot expose live routes")
	}
	base := filepath.Join(contractsDir(t), "testdata", "ev", "prepared-v1")
	for _, dir := range []string{"nested", "empty"} {
		for file, schema := range map[string]string{
			"manifest.json": "PreparedManifest", "entities.ndjson": "PreparedEntity",
			"decisions.ndjson": "PreparedDecision", "aliases.ndjson": "PreparedAlias",
		} {
			t.Run(dir+"/"+file, func(t *testing.T) {
				raw, err := os.ReadFile(filepath.Join(base, dir, file))
				if err != nil {
					t.Fatal(err)
				}
				for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
					if line == "" && file != "manifest.json" {
						continue
					}
					var value map[string]any
					if err := json.Unmarshal([]byte(line), &value); err != nil {
						t.Fatal(err)
					}
					s := doc.Components.Schemas[schema].Value
					if err := s.VisitJSON(value); err != nil {
						t.Fatal(err)
					}
					// Every envelope refuses extra private fields/missing fields.
					value["private_contact"] = "NEVER_COPY"
					if s.VisitJSON(value) == nil {
						t.Fatal("extra private field admitted")
					}
					delete(value, "private_contact")
					for field := range value {
						v := value[field]
						delete(value, field)
						if s.VisitJSON(value) == nil {
							t.Fatal("missing required field", field)
						}
						value[field] = v
					}
					if schema == "PreparedEntity" {
						payload := value["payload"].(map[string]any)
						kind := value["kind"].(string)
						if err := doc.Components.Schemas["Payload"+kind].Value.VisitJSON(payload); err != nil {
							t.Fatal(err)
						}
						payload["gps"] = "NEVER_COPY"
						if s.VisitJSON(value) == nil {
							t.Fatal("private payload field admitted")
						}
					}
				}
			})
		}
	}
}

func TestEVPreparedIdentityGoldens(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(contractsDir(t), "testdata", "ev", "prepared-v1", "byte-goldens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Canonical []struct {
			Value    json.RawMessage `json:"value"`
			Expected string          `json:"expected_utf8"`
		} `json:"canonical"`
		Identities []struct {
			Namespace  string `json:"namespace"`
			Kind       string `json:"kind"`
			ExternalID string `json:"external_id"`
			SHA256     string `json:"sha256"`
		} `json:"identities"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Canonical) != 3 || len(fixture.Identities) != 4 {
		t.Fatal("byte/identity vectors missing")
	}
	for _, c := range fixture.Canonical {
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(c.Value)))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		var buf strings.Builder
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(value); err != nil {
			t.Fatal(err)
		}
		// Go escapes U+2028/2029 even with EscapeHTML(false); the byte profile
		// keeps their original UTF-8. This test-only oracle handles the reviewed
		// goldens; it is not a general JSON canonicalizer or runtime bridge.
		got := strings.TrimSuffix(buf.String(), "\n")
		got = strings.ReplaceAll(strings.ReplaceAll(got, `\u2028`, "\u2028"), `\u2029`, "\u2029")
		if got != c.Expected {
			t.Fatalf("canonical bytes differ: %q vs %q", got, c.Expected)
		}
	}
	for _, c := range fixture.Identities {
		h := sha256.New()
		for _, field := range []string{"ev-source-key-v1", c.Namespace, c.Kind, c.ExternalID} {
			var length [4]byte
			binary.BigEndian.PutUint32(length[:], uint32(len(field)))
			h.Write(length[:])
			h.Write([]byte(field))
		}
		if hex.EncodeToString(h.Sum(nil)) != c.SHA256 {
			t.Fatal("source key framing/hash differs")
		}
	}
}
