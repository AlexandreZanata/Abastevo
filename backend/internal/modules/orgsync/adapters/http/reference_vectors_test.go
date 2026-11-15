package http

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// Shared wire oracle of the reference feed client (EMP-FLEET-013, EDR-227).
// The Kotlin client consumes the same file; this consumer pins the server side.
const (
	referenceVectorPath   = "../../../../../../contracts/testdata/enterprise/sync/reference-feed-v1.psv"
	referenceVectorHeader = "id|operation|input_hex|context_hex|outcome|expected_hex"
	referenceVectorSHA    = "ec39616cd9d58d0b54bb7ed9129e023fe91f8cb868c8d4a8546cf73e0ffd2098"
)

type referenceVector struct {
	id, operation, outcome   string
	input, context, expected []byte
}

func referenceHex(raw string) ([]byte, error) {
	value, err := hex.DecodeString(raw)
	if err != nil || len(value) == 0 || hex.EncodeToString(value) != raw {
		return nil, errors.New("invalid fixture hex")
	}
	return value, nil
}

func loadReferenceVectors(t *testing.T) []referenceVector {
	t.Helper()
	raw, err := os.ReadFile(referenceVectorPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != referenceVectorSHA {
		t.Fatal("reference vector file changed without updating its pinned SHA-256")
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 131 || lines[0] != referenceVectorHeader || lines[len(lines)-1] != "" {
		t.Fatal("reference vector framing or count")
	}
	outcomes := map[string]map[string]bool{
		"REQUEST":  {"OK": true, "INVALID": true, "LIMIT": true},
		"RESPONSE": {"OK": true, "INVALID": true, "LIMIT": true, "BINDING": true},
		"APPLY":    {"OK": true, "INVALID": true, "BINDING": true},
		"CACHE":    {"OK": true, "INVALID": true},
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	covered := map[string]bool{}
	out := make([]referenceVector, 0, 129)
	for _, line := range lines[1 : len(lines)-1] {
		fields := strings.Split(line, "|")
		if len(fields) != 6 || fields[0] == "" || seen[fields[0]] {
			t.Fatal("reference vector identity or columns", line[:min(len(line), 60)])
		}
		seen[fields[0]] = true
		allowed, known := outcomes[fields[1]]
		if !known || !allowed[fields[4]] {
			t.Fatal("unknown reference operation or outcome", fields[0])
		}
		v := referenceVector{id: fields[0], operation: fields[1], outcome: fields[4]}
		if v.input, err = referenceHex(fields[2]); err != nil {
			t.Fatal(v.id, err)
		}
		hasContext := v.operation == "RESPONSE" || v.operation == "APPLY"
		if hasContext {
			if v.context, err = referenceHex(fields[3]); err != nil {
				t.Fatal(v.id, err)
			}
		} else if fields[3] != "-" {
			t.Fatal("unexpected context", v.id)
		}
		if v.outcome == "OK" {
			if v.expected, err = referenceHex(fields[5]); err != nil {
				t.Fatal(v.id, err)
			}
		} else if fields[5] != "-" {
			t.Fatal("unexpected expected bytes", v.id)
		}
		counts[v.operation]++
		covered[v.operation+"/"+v.outcome] = true
		out = append(out, v)
	}
	if !reflect.DeepEqual(counts, map[string]int{"REQUEST": 23, "RESPONSE": 79, "APPLY": 16, "CACHE": 11}) {
		t.Fatal("reference vector group counts", counts)
	}
	for operation, allowed := range outcomes {
		for outcome := range allowed {
			if !covered[operation+"/"+outcome] {
				t.Fatal("no vector for", operation, outcome)
			}
		}
	}
	return out
}

func referenceInstant(raw string) bool {
	at, err := time.Parse(time.RFC3339Nano, raw)
	return err == nil && !at.IsZero() && at.Year() >= 1 && at.Year() <= 9999 && orgtime.FormatInstant(at) == raw
}

func referenceIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func referenceAssetValid(asset *app.ReferenceAsset) bool {
	if asset == nil || !referenceUUID.MatchString(asset.ID) || asset.Revision < 1 || !referenceIn(asset.Kind, "VEHICLE", "MACHINE", "IMPLEMENT") ||
		!referenceIn(asset.UsageBasis, "DISTANCE", "HOURS") || !referenceIn(asset.State, "DRAFT", "ACTIVE", "INACTIVE", "DISPOSED", "ARCHIVED") ||
		utf8.RuneCountInString(asset.Name) > 80 || utf8.RuneCountInString(asset.FleetNumber) > 40 || utf8.RuneCountInString(asset.Plate) > 40 ||
		asset.Catalog != app.ReferenceCatalog || asset.Products == nil || len(asset.Products) > 64 || asset.Meters == nil || len(asset.Meters) > 16 ||
		!referenceIn(asset.Assignment.Kind, "PRIMARY", "TEMPORARY", "ADDITIONAL") || asset.Assignment.ValidTo != nil && !referenceInstant(*asset.Assignment.ValidTo) {
		return false
	}
	for _, product := range asset.Products {
		if utf8.RuneCountInString(product) > 64 {
			return false
		}
	}
	for _, meter := range asset.Meters {
		if !referenceUUID.MatchString(meter.ID) || !referenceUUID.MatchString(meter.SegmentID) || !referenceIn(meter.Kind, "ODOMETER", "HOUR_METER") ||
			meter.Modulus != nil && *meter.Modulus < 1 || !referenceInstant(meter.InstalledAt) {
			return false
		}
	}
	return true
}

// referenceServerProducible reports whether raw is exactly what the reference
// handler can write: the wire struct re-encodes it byte for byte and it keeps
// the frozen OpenAPI bounds and the B-BR-EMP-339/340 delta invariants.
func referenceServerProducible(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var wire referenceResponseWire
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing data")
	}
	encoded, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(encoded, raw) {
		return errors.New("not the server encoding")
	}
	if wire.SchemaVersion != app.ReferenceSchema || !referenceUUID.MatchString(wire.RequestID) || !referenceUUID.MatchString(wire.MembershipID) || !referenceUUID.MatchString(wire.EnrollmentID) ||
		wire.Revision < 0 || !referenceInstant(wire.ConfirmedAt) || len(wire.Cursor) < 1 || len(wire.Cursor) > 1024 ||
		!reflect.DeepEqual(wire.EntityTypes, []string{app.ReferenceEntityAsset}) || wire.Changes == nil || len(wire.Changes) > app.ReferenceMaxEntities {
		return errors.New("response bounds")
	}
	prior := int64(0)
	switch wire.Mode {
	case app.ReferenceDelta:
		if wire.Reason != nil || wire.FromRevision == nil || *wire.FromRevision < 0 || *wire.FromRevision > wire.Revision {
			return errors.New("delta shape")
		}
		prior = *wire.FromRevision
	case app.ReferenceReset:
		if wire.Reason == nil || !referenceIn(*wire.Reason, app.ReferenceNewCursor, app.ReferenceExpired, app.ReferenceAhead, app.ReferenceForbidden) || wire.FromRevision != nil {
			return errors.New("reset shape")
		}
		if *wire.Reason == app.ReferenceForbidden && len(wire.Changes) != 0 {
			return errors.New("forbidden with entities")
		}
	default:
		return errors.New("mode")
	}
	entities := map[string]bool{}
	for _, change := range wire.Changes {
		if change.Revision <= prior || change.Revision > wire.Revision || change.EntityType != app.ReferenceEntityAsset || !referenceUUID.MatchString(change.EntityID) || entities[change.EntityID] {
			return errors.New("change order or identity")
		}
		entities[change.EntityID] = true
		prior = change.Revision
		switch change.Op {
		case app.ReferenceUpsert:
			if !referenceAssetValid(change.Asset) || change.Asset.ID != change.EntityID {
				return errors.New("upsert content")
			}
		case app.ReferenceDelete:
			if change.Asset != nil || wire.Mode == app.ReferenceReset {
				return errors.New("tombstone content")
			}
		default:
			return errors.New("op")
		}
	}
	return nil
}

func TestReferenceFeedSharedVectors(t *testing.T) {
	for _, v := range loadReferenceVectors(t) {
		t.Run(v.id, func(t *testing.T) {
			switch v.operation {
			case "REQUEST":
				parsed, err := parseReferenceRequest(v.input)
				switch v.outcome {
				case "OK":
					var want referenceRequest
					if err != nil || json.Unmarshal(v.expected, &want) != nil || !reflect.DeepEqual(parsed, want) {
						t.Fatal("the server parser disagrees with the client request", err)
					}
				case "LIMIT":
					if len(v.input) <= referenceRequestBytes {
						t.Fatal("a LIMIT request fits the server body bound")
					}
				default:
					if err == nil {
						t.Fatal("the server parser accepted a request the client refuses")
					}
				}
			case "RESPONSE", "APPLY":
				err := referenceServerProducible(v.input)
				if v.outcome == "OK" || v.outcome == "BINDING" {
					if err != nil {
						t.Fatal("a response the client accepts or binds is not server-producible:", err)
					}
					// The checker discriminates: the client's canonical projection
					// (sorted keys, no HTML escapes) is not the server encoding.
					if v.operation == "RESPONSE" && v.outcome == "OK" && referenceServerProducible(v.expected) == nil {
						t.Fatal("the canonical projection passed as server output")
					}
				} else if err == nil {
					t.Fatal("a response the client refuses as malformed is server-producible")
				}
			case "CACHE":
				// Client-only local format (EDR-224); grammar is checked by the loader.
			}
		})
	}
}
