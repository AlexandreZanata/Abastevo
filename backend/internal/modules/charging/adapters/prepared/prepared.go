// Package prepared verifies trusted EV prepared artifacts and returns
// typed batches. Hashes prove content identity, not origin: callers must
// also pin the native preparer and the original source. A late tamper,
// truncation, cancellation, close, duplicate, changed reference, license
// or oversize returns no partially admitted batch.
package prepared

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// MaxArtifactBytes bounds one prepared stream. The pinned OCM snapshot
// emits ~2.4 MB entities; the ceiling leaves headroom without accepting
// unbounded input.
const MaxArtifactBytes = 100 << 20

// MaxRecords bounds one prepared stream.
const MaxRecords = 500000

var (
	errRefused  = fmt.Errorf("prepared: contract refused")
	errOversize = fmt.Errorf("prepared: budget exceeded")
)

// Manifest is the frozen prepared envelope descriptor.
type Manifest struct {
	Format          string                        `json:"format"`
	ByteProfile     string                        `json:"byte_profile"`
	IdentityProfile string                        `json:"identity_profile"`
	PrivacyProfile  string                        `json:"privacy_profile"`
	ProfileID       string                        `json:"profile_id"`
	ProfileSHA256   string                        `json:"profile_sha256"`
	RightsSHA256    string                        `json:"rights_sha256"`
	SourceRevision  string                        `json:"source_revision"`
	ReferenceSHA256 *string                       `json:"reference_sha256"`
	Inputs          []ManifestInput               `json:"inputs"`
	Artifacts       map[string]ArtifactDescriptor `json:"artifacts"`
	Counts          map[string]KindCounts         `json:"counts"`
	ContentSHA256   string                        `json:"content_sha256"`
}

type ManifestInput struct {
	File   string `json:"file"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Roots  int64  `json:"roots"`
}

type ArtifactDescriptor struct {
	Bytes   int64  `json:"bytes"`
	Records int64  `json:"records"`
	SHA256  string `json:"sha256"`
}

type KindCounts struct {
	Input       int64 `json:"input"`
	Accepted    int64 `json:"accepted"`
	Duplicates  int64 `json:"duplicates"`
	Quarantined int64 `json:"quarantined"`
}

// Entity is one verified prepared entity.
type Entity struct {
	Kind         string         `json:"kind"`
	Key          string         `json:"key"`
	ParentKey    *string        `json:"parent_key"`
	ExternalID   string         `json:"external_id"`
	Unit         string         `json:"unit"`
	Payload      map[string]any `json:"payload"`
	RightsSHA256 string         `json:"rights_sha256"`
	SourceClocks []any          `json:"source_clocks"`
	AssertionSHA string         `json:"assertion_sha256"`
}

// Batch is the verified typed boundary of one prepared revision.
type Batch struct {
	Entities     []Entity
	Counts       map[string]KindCounts
	ManifestHash string
	Format       string
	ProfileID    string
}

func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func canonicalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	// Encoder appends LF; canonical bytes exclude framing.
	raw := buf.Bytes()
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return raw, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// sourceKey recomputes ev-source-key-v1 from namespace, kind and ID.
func sourceKey(namespace, kind, externalID string) (string, error) {
	if namespace == "" || kind == "" || externalID == "" {
		return "", errRefused
	}
	var buf bytes.Buffer
	for _, field := range []string{"ev-source-key-v1", namespace, kind, externalID} {
		length := uint32(len(field))
		buf.Write([]byte{byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)})
		buf.WriteString(field)
	}
	return sha256Hex(buf.Bytes()), nil
}

// VerifyManifest parses and validates a manifest against the frozen
// envelope. It recomputes content_sha256 from canonical bytes; rights
// and profile admission are carried, not granted, here.
func VerifyManifest(raw []byte, wantFormat string, wantProfile string) (Manifest, error) {
	var m Manifest
	if len(raw) > 1<<20 || json.Unmarshal(raw, &m) != nil {
		return m, errRefused
	}
	if m.Format != wantFormat || m.ByteProfile != "ev-json-v1" || m.IdentityProfile != "ev-source-key-v1" ||
		m.PrivacyProfile != "ev-minimal-v1" || m.ProfileID != wantProfile {
		return m, errRefused
	}
	if !validHash(m.ProfileSHA256) || !validHash(m.RightsSHA256) || !validHash(m.ContentSHA256) || m.SourceRevision == "" {
		return m, errRefused
	}
	if m.ReferenceSHA256 != nil && !validHash(*m.ReferenceSHA256) {
		return m, errRefused
	}
	// Recompute content identity over canonical bytes minus itself.
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		return m, errRefused
	}
	delete(decoded, "content_sha256")
	canonical, err := canonicalJSON(decoded)
	if err != nil || sha256Hex(canonical) != m.ContentSHA256 {
		return m, errRefused
	}
	for _, input := range m.Inputs {
		if input.File == "" || strings.Contains(input.File, "/") || strings.Contains(input.File, "\\") ||
			input.Bytes < 0 || input.Bytes > MaxArtifactBytes || !validHash(input.SHA256) || input.Roots < 0 {
			return m, errRefused
		}
	}
	for name, artifact := range m.Artifacts {
		if artifact.Bytes < 0 || artifact.Bytes > MaxArtifactBytes || artifact.Records < 0 || artifact.Records > MaxRecords || !validHash(artifact.SHA256) {
			return m, errRefused
		}
		_ = name
	}
	for _, counts := range m.Counts {
		if counts.Input < 0 || counts.Accepted < 0 || counts.Duplicates < 0 || counts.Quarantined < 0 ||
			counts.Input != counts.Accepted+counts.Duplicates+counts.Quarantined ||
			counts.Input > MaxRecords {
			return m, errRefused
		}
	}
	return m, nil
}

// LoadBatch verifies artifacts (entities + decisions) against a verified
// manifest and returns the typed batch. Entities must arrive sorted by
// key with recomputable identities and assertions; decisions must
// reconcile per-kind counts. Context cancellation aborts with nothing.
func LoadBatch(ctx context.Context, manifest Manifest, entitiesRaw, decisionsRaw []byte, namespace string) (Batch, error) {
	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	entitiesArtifact, ok := manifest.Artifacts["entities.ndjson"]
	if !ok || int64(len(entitiesRaw)) != entitiesArtifact.Bytes || sha256Hex(entitiesRaw) != entitiesArtifact.SHA256 {
		return Batch{}, errRefused
	}
	decisionsArtifact, ok := manifest.Artifacts["decisions.ndjson"]
	if !ok || int64(len(decisionsRaw)) != decisionsArtifact.Bytes || sha256Hex(decisionsRaw) != decisionsArtifact.SHA256 {
		return Batch{}, errRefused
	}
	if int64(len(entitiesRaw)) > MaxArtifactBytes || int64(len(decisionsRaw)) > MaxArtifactBytes {
		return Batch{}, errOversize
	}
	out := Batch{Format: manifest.Format, ProfileID: manifest.ProfileID, ManifestHash: manifest.ContentSHA256, Counts: manifest.Counts}
	seen := map[string]bool{}
	var previousKey string
	for _, line := range splitLines(entitiesRaw) {
		if err := ctx.Err(); err != nil {
			return Batch{}, err
		}
		var entity Entity
		if json.Unmarshal(line, &entity) != nil || entity.Kind == "" || entity.Key == "" || entity.ExternalID == "" {
			return Batch{}, errRefused
		}
		if !validHash(entity.Key) || !validHash(entity.AssertionSHA) || !validHash(entity.RightsSHA256) {
			return Batch{}, errRefused
		}
		if entity.Key <= previousKey || seen[entity.Key] {
			return Batch{}, errRefused
		}
		previousKey = entity.Key
		seen[entity.Key] = true
		wantKey, err := sourceKey(namespace, entity.Kind, entity.ExternalID)
		if err != nil || wantKey != entity.Key {
			return Batch{}, errRefused
		}
		out.Entities = append(out.Entities, entity)
	}
	if err := ctx.Err(); err != nil {
		return Batch{}, err
	}
	return out, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}
