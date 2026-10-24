package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func testManifest() (Manifest, []byte, []byte) {
	entities := []byte("{\"assertion_sha256\":\"" + strings.Repeat("a", 64) + "\",\"external_id\":\"1\",\"field_provenance\":[],\"key\":\"" + mustKey("ocm", "PLACE", "1") + "\",\"kind\":\"PLACE\",\"parent_key\":null,\"payload\":{\"country\":\"BR\"},\"rights_sha256\":\"" + strings.Repeat("b", 64) + "\",\"source_clocks\":[],\"unit\":\"PLACE:1\"}\n")
	decisions := []byte("{\"assertion_sha256\":\"" + strings.Repeat("a", 64) + "\",\"duplicate_of\":null,\"external_id\":\"1\",\"key\":\"" + mustKey("ocm", "PLACE", "1") + "\",\"kind\":\"PLACE\",\"outcome\":\"ACCEPTED\",\"parent_unit\":null,\"pointer\":\"1\",\"raw_sha256\":\"" + strings.Repeat("c", 64) + "\",\"reason\":null,\"root\":0,\"source_file\":\"a.json\",\"unit\":\"a.json#0\"}\n")
	manifest := Manifest{
		Format: "charging-prepared-v1", ByteProfile: "ev-json-v1", IdentityProfile: "ev-source-key-v1",
		PrivacyProfile: "ev-minimal-v1", ProfileID: "ocm-br-20261021-v1",
		ProfileSHA256: strings.Repeat("d", 64), RightsSHA256: strings.Repeat("b", 64),
		SourceRevision: "abc123",
		Inputs:         []ManifestInput{{File: "a.json", Bytes: 10, SHA256: strings.Repeat("e", 64), Roots: 1}},
		Artifacts: map[string]ArtifactDescriptor{
			"entities.ndjson":  {Bytes: int64(len(entities)), Records: 1, SHA256: sha256Hex(entities)},
			"decisions.ndjson": {Bytes: int64(len(decisions)), Records: 1, SHA256: sha256Hex(decisions)},
			"aliases.ndjson":   {Bytes: 0, Records: 0, SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		},
		Counts: map[string]KindCounts{"PLACE": {Input: 1, Accepted: 1}},
	}
	raw, _ := canonicalJSON(map[string]any{
		"format": manifest.Format, "byte_profile": manifest.ByteProfile, "identity_profile": manifest.IdentityProfile,
		"privacy_profile": manifest.PrivacyProfile, "profile_id": manifest.ProfileID, "profile_sha256": manifest.ProfileSHA256,
		"rights_sha256": manifest.RightsSHA256, "source_revision": manifest.SourceRevision, "reference_sha256": nil,
		"inputs": []any{map[string]any{"file": "a.json", "bytes": 10, "sha256": strings.Repeat("e", 64), "roots": 1}},
		"artifacts": map[string]any{
			"entities.ndjson":  map[string]any{"bytes": len(entities), "records": 1, "sha256": sha256Hex(entities)},
			"decisions.ndjson": map[string]any{"bytes": len(decisions), "records": 1, "sha256": sha256Hex(decisions)},
			"aliases.ndjson":   map[string]any{"bytes": 0, "records": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		},
		"counts": map[string]any{"PLACE": map[string]any{"input": 1, "accepted": 1, "duplicates": 0, "quarantined": 0}},
	})
	manifest.ContentSHA256 = sha256Hex(raw)
	return manifest, entities, decisions
}

func mustKey(namespace, kind, id string) string {
	key, err := sourceKey(namespace, kind, id)
	if err != nil {
		panic(err)
	}
	return key
}

func TestVerifyAndLoadAcceptsVerifiedBatch(t *testing.T) {
	manifest, entities, decisions := testManifest()
	raw, _ := json.Marshal(manifest)
	verified, err := VerifyManifest(raw, "charging-prepared-v1", "ocm-br-20261021-v1")
	if err != nil {
		t.Fatal("verified manifest refused", err)
	}
	batch, err := LoadBatch(context.Background(), verified, entities, decisions, "ocm")
	if err != nil {
		t.Fatal("verified batch refused", err)
	}
	if len(batch.Entities) != 1 || batch.Entities[0].ExternalID != "1" {
		t.Fatal("batch mistyped", len(batch.Entities))
	}
}

func TestBridgeReturnsNoPartialBatch(t *testing.T) {
	manifest, entities, decisions := testManifest()
	raw, _ := json.Marshal(manifest)
	verified, err := VerifyManifest(raw, "charging-prepared-v1", "ocm-br-20261021-v1")
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte{}, entities...)
	tampered[10] ^= 0xff
	if _, err := LoadBatch(context.Background(), verified, tampered, decisions, "ocm"); err == nil {
		t.Fatal("tampered stream accepted")
	}
	truncated := entities[:len(entities)/2]
	if b, err := LoadBatch(context.Background(), verified, truncated, decisions, "ocm"); err == nil || len(b.Entities) != 0 {
		t.Fatal("truncated stream yielded partial batch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b, err := LoadBatch(ctx, verified, entities, decisions, "ocm"); err == nil || len(b.Entities) != 0 {
		t.Fatal("cancelled load yielded partial batch")
	}
	wrongProfile := manifest
	wrongProfile.ProfileID = "unknown-v9"
	wrongRaw, _ := json.Marshal(wrongProfile)
	if _, err := VerifyManifest(wrongRaw, "charging-prepared-v1", "ocm-br-20261021-v1"); err == nil {
		t.Fatal("unknown profile accepted")
	}
	oversize := Manifest{Format: "x"}
	_ = oversize
	_ = sha256.Sum256
	_ = hex.EncodeToString
	_ = os.Getenv
	_ = strings.Repeat
}
