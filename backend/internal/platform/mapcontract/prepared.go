package mapcontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const PreparedFormat = "abastevo.map-prepared-v1"

// PreparedEnvelope is the frozen map-overlay envelope. Reference packages,
// public overlays and private state are separate product classes; this parser
// admits only a public overlay.
type PreparedEnvelope struct {
	Format         string                      `json:"format"`
	Version        int                         `json:"version"`
	ProductClass   string                      `json:"product_class"`
	Visibility     string                      `json:"visibility"`
	ProfileID      string                      `json:"profile_id"`
	ProfileSHA256  string                      `json:"profile_sha256"`
	RightsSHA256   string                      `json:"rights_sha256"`
	SourceRevision string                      `json:"source_revision"`
	Inputs         []PreparedInput             `json:"inputs"`
	Artifacts      map[string]PreparedArtifact `json:"artifacts"`
	Counts         PreparedCounts              `json:"counts"`
	ContentSHA256  string                      `json:"content_sha256"`
}

type PreparedInput struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Roots  int64  `json:"roots"`
}

type PreparedArtifact struct {
	Bytes   int64  `json:"bytes"`
	Records int64  `json:"records"`
	SHA256  string `json:"sha256"`
}

type PreparedCounts struct {
	Input       int64 `json:"input"`
	Accepted    int64 `json:"accepted"`
	Duplicates  int64 `json:"duplicates"`
	Quarantined int64 `json:"quarantined"`
}

// ParsePreparedOverlay validates one public overlay envelope against the
// exact assertion bytes it describes. It returns the parsed corpus only after
// every hash, count and format check succeeds.
func ParsePreparedOverlay(envelopeBytes, assertionsBytes []byte) (*PreparedEnvelope, *AssertionCorpus, error) {
	if len(envelopeBytes) == 0 || len(envelopeBytes) > 1<<20 {
		return nil, nil, fail("$", "envelope size out of bounds")
	}
	var envelope PreparedEnvelope
	if err := decode(envelopeBytes, &envelope); err != nil {
		return nil, nil, err
	}
	if envelope.Format != PreparedFormat || envelope.Version != Version {
		return nil, nil, fail("$.format", "unsupported prepared format")
	}
	if envelope.ProductClass != "PUBLIC_OVERLAY" || envelope.Visibility != "PUBLIC" {
		return nil, nil, fail("$.product_class", "public overlay required")
	}
	if strings.TrimSpace(envelope.ProfileID) == "" || len(envelope.ProfileID) > 120 ||
		!hex64.MatchString(envelope.ProfileSHA256) || !hex64.MatchString(envelope.RightsSHA256) {
		return nil, nil, fail("$.profile_id", "profile identity required")
	}
	if strings.TrimSpace(envelope.SourceRevision) == "" || len(envelope.SourceRevision) > 128 {
		return nil, nil, fail("$.source_revision", "source revision required")
	}
	if len(envelope.Inputs) < 1 || len(envelope.Inputs) > 64 {
		return nil, nil, fail("$.inputs", "input count out of bounds")
	}
	for i, input := range envelope.Inputs {
		path := "$.inputs[" + itoa(i) + "]"
		if strings.TrimSpace(input.File) == "" || len(input.File) > 160 ||
			strings.ContainsAny(input.File, `/\`) || input.Bytes < 1 ||
			!hex64.MatchString(input.SHA256) || input.Roots < 0 {
			return nil, nil, fail(path, "invalid input descriptor")
		}
	}
	artifact, ok := envelope.Artifacts["assertions"]
	if !ok {
		return nil, nil, fail("$.artifacts.assertions", "assertions artifact required")
	}
	if artifact.Bytes != int64(len(assertionsBytes)) || artifact.Bytes < 1 ||
		!hex64.MatchString(artifact.SHA256) || sha256Hex(assertionsBytes) != artifact.SHA256 {
		return nil, nil, fail("$.artifacts.assertions", "assertion bytes/hash mismatch")
	}
	if artifact.Records < 1 || artifact.Records > 10000 {
		return nil, nil, fail("$.artifacts.assertions.records", "record count out of bounds")
	}
	corpus, err := ParseAssertions(assertionsBytes)
	if err != nil {
		return nil, nil, err
	}
	if int64(len(corpus.Assertions)) != artifact.Records {
		return nil, nil, fail("$.artifacts.assertions.records", "record count mismatch")
	}
	if corpus.Generation != envelope.SourceRevision {
		return nil, nil, fail("$.source_revision", "must match assertion generation")
	}
	if envelope.Counts.Input < 1 || envelope.Counts.Input > 10000 ||
		envelope.Counts.Accepted < 0 || envelope.Counts.Duplicates < 0 || envelope.Counts.Quarantined < 0 ||
		envelope.Counts.Input != envelope.Counts.Accepted+envelope.Counts.Duplicates+envelope.Counts.Quarantined ||
		envelope.Counts.Accepted != artifact.Records {
		return nil, nil, fail("$.counts", "counts do not reconcile")
	}
	if !hex64.MatchString(envelope.ContentSHA256) {
		return nil, nil, fail("$.content_sha256", "sha256 required")
	}
	if got, err := contentSHA256(envelopeBytes); err != nil || got != envelope.ContentSHA256 {
		return nil, nil, fail("$.content_sha256", "content hash mismatch")
	}
	return &envelope, corpus, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	pos := len(digits)
	for n > 0 {
		pos--
		digits[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[pos:])
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// contentSHA256 hashes canonical JSON without the content_sha256 field. The
// producer must use the same canonical encoding (no HTML escaping, sorted map
// keys, no trailing framing).
func contentSHA256(raw []byte) (string, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return "", err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", errors.New("trailing bytes")
	}
	delete(doc, "content_sha256")
	canonical, err := canonicalJSON(doc)
	if err != nil {
		return "", err
	}
	return sha256Hex(canonical), nil
}

func canonicalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	raw := buf.Bytes()
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return raw, nil
}
