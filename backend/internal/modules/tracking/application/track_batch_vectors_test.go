package application_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
)

// EMP-GPS-003 (B-BR-EMP-504, EDR-326): the Android producer's exact
// gps.track.batch bytes. OK rows parse through the real protocol and
// ParseBatch with exactly the row's absolute fixes; MALFORMED rows are
// well-formed envelopes ParseBatch refuses; PROTOCOL rows are refused by the
// shared envelope profile (the 8 KiB bound).
const (
	batchCorpusPath = "../../../../../contracts/testdata/enterprise/tracking/track-batch-v1.psv"
	batchCorpusHash = "9cbdd508ff2843dda9df0b3eb64a8f0aa4c30f82b4e30d281922a0aa38ccf4f6"
)

type rowFix struct {
	offset         int64
	lat, lon       int32
	accuracy       int32
	speed, bearing *int64
	flags          int16
}

func parseRowFixes(t *testing.T, raw string) []rowFix {
	t.Helper()
	var out []rowFix
	for _, item := range strings.Split(raw, ",") {
		p := strings.Split(item, ":")
		if len(p) != 7 {
			t.Fatal("fix arity", item)
		}
		n := func(s string) int64 {
			v, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				t.Fatal("fix value", s)
			}
			return v
		}
		opt := func(s string) *int64 {
			if s == "n" {
				return nil
			}
			v := n(s)
			return &v
		}
		out = append(out, rowFix{offset: n(p[0]), lat: int32(n(p[1])), lon: int32(n(p[2])), accuracy: int32(n(p[3])), speed: opt(p[4]), bearing: opt(p[5]), flags: int16(n(p[6]))})
	}
	return out
}

func TestSharedTrackBatchVectors(t *testing.T) {
	raw, err := os.ReadFile(batchCorpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != batchCorpusHash {
		t.Fatal("track batch corpus hash changed")
	}
	binding, err := protocol.NewBinding(vectorAccount, vectorOrigin, vectorOrg)
	if err != nil {
		t.Fatal(err)
	}
	vectors := loadSessionVectorsFrom(t, raw)
	counts := map[string]int{}
	for _, v := range vectors {
		counts[v.operation+"/"+v.outcome]++
	}
	if len(vectors) != 24 || fmt.Sprint(counts) != fmt.Sprint(map[string]int{"BATCH/OK": 11, "BATCH/MALFORMED": 12, "BATCH/PROTOCOL": 1}) {
		t.Fatalf("track batch corpus counts %v", counts)
	}
	for _, v := range vectors {
		t.Run(v.id, func(t *testing.T) {
			var op protocol.Operation
			switch v.outcome {
			case "OK":
				op, err = protocol.ParseOperation(v.data, binding)
				if err != nil || !bytes.Equal(op.CanonicalBytes(), v.data) {
					t.Fatal("produced bytes are not canonical server operations", err)
				}
			case "PROTOCOL":
				if _, err := protocol.Seal(v.data, binding); !errors.Is(err, protocol.ErrLimit) {
					t.Fatal("the envelope profile must refuse the oversized batch with its limit", err)
				}
				return
			case "MALFORMED":
				op, err = protocol.Seal(v.data, binding)
				if err != nil {
					t.Fatal("refusal rows must be well-formed envelopes", err)
				}
			default:
				t.Fatal("unknown outcome", v.outcome)
			}
			snapshot := op.Snapshot()
			deps := []string{}
			if dep, ok := v.args["dep"]; ok {
				deps = append(deps, dep)
			}
			if op.ID() != v.args["op"] || snapshot.Type != app.BatchOperationType || snapshot.Entity.Type != app.EntityType ||
				snapshot.Entity.ID != v.args["session"] || fmt.Sprint(snapshot.DependsOn) != fmt.Sprint(deps) {
				t.Fatal("envelope identity")
			}
			command, err := app.ParseBatch(op)
			if v.outcome == "MALFORMED" {
				if !errors.Is(err, app.ErrMalformed) {
					t.Fatal("expected MALFORMED", err)
				}
				return
			}
			if err != nil {
				t.Fatal("produced batch refused by the intake parser", err)
			}
			b, err := command.Snapshot()
			if err != nil || b.OperationID != v.args["op"] || b.SessionID != v.args["session"] || b.TripID != v.args["trip"] ||
				fmt.Sprint(b.Seq) != v.args["seq"] || b.Source != app.SourcePhone {
				t.Fatal("parsed batch identity differs from the row")
			}
			if boot, ok := v.args["boot"]; ok {
				if b.BootID == nil || *b.BootID != boot || b.BaseElapsed == nil || fmt.Sprint(*b.BaseElapsed) != v.args["elapsed"] {
					t.Fatal("boot identity")
				}
			} else if b.BootID != nil || b.BaseElapsed != nil {
				t.Fatal("unexpected boot identity")
			}
			dropped := "0:0:0:0"
			if d, ok := v.args["dropped"]; ok {
				dropped = d
			}
			if fmt.Sprintf("%d:%d:%d:%d", b.Dropped.Accuracy, b.Dropped.Stale, b.Dropped.Jump, b.Dropped.Mock) != dropped {
				t.Fatal("dropped counters")
			}
			at := "2026-11-15T12:00:00.123Z"
			if raw, ok := v.args["at"]; ok {
				at = raw
			}
			base, _ := orgtime.ParseInstant(at)
			want := parseRowFixes(t, v.args["fixes"])
			if len(b.Fixes) != len(want) {
				t.Fatal("fix count")
			}
			for i, f := range b.Fixes {
				w := want[i]
				same := func(got *int32, want *int64) bool {
					return (got == nil) == (want == nil) && (got == nil || int64(*got) == *want)
				}
				sameBearing := func(got *int16, want *int64) bool {
					return (got == nil) == (want == nil) && (got == nil || int64(*got) == *want)
				}
				if !f.At.Equal(base.Add(time.Duration(w.offset)*time.Millisecond)) || f.Lat != w.lat || f.Lon != w.lon || f.Accuracy != w.accuracy ||
					!same(f.Speed, w.speed) || !sameBearing(f.Bearing, w.bearing) || f.Flags != w.flags {
					t.Fatalf("fix %d differs from the row", i)
				}
			}
		})
	}
}
