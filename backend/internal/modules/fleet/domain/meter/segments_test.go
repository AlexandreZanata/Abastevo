package meter_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
)

// The track never retains caller slices or pointers and never prints values.
func TestTrackCopiesInputsAndRedacts(t *testing.T) {
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	closing := int64(184_532_000)
	spans := []meter.Span{
		{ID: "s1", Meter: "m1", Reason: meter.SegmentInstallation, OpenedAt: at, Open: 183_000_000, Closed: true, ClosedAt: at.Add(169 * time.Hour), Close: &closing},
		{ID: "s2", Meter: "m2", Reason: meter.SegmentInstallation, OpenedAt: at.Add(169 * time.Hour)},
	}
	anchors := []meter.Anchor{{ID: "r0", Segment: "s1", Value: 183_000_000, OccurredAt: at, ReceivedAt: at}, {ID: "r8", Segment: "s2", Value: 412_000, OccurredAt: at.Add(198 * time.Hour), ReceivedAt: at.Add(198 * time.Hour)}}
	track, err := meter.NewTrack(meter.Odometer, spans, anchors)
	if err != nil {
		t.Fatal(err)
	}
	closing, spans[1].Open, anchors[1].Value = 0, 7, 1
	f, err := track.Cumulative(at.Add(200 * time.Hour))
	if err != nil || !f.Known() || f.Value != 1_944_000 || f.Equivalent != 184_944_000 || f.To != "r8" {
		t.Fatalf("caller mutation reached the track: %+v %v", f, err)
	}
	raw, _ := json.Marshal(track)
	for _, out := range []string{fmt.Sprint(track), fmt.Sprintf("%+v", track), string(raw)} {
		if out != "EnterpriseMeterValue(redacted)" && out != `"EnterpriseMeterValue(redacted)"` {
			t.Fatalf("track printed %q", out)
		}
	}
	if _, err := (meter.Track{}).Cumulative(at); err == nil {
		t.Fatal("the zero track computed a figure")
	}
	if _, err := (meter.Track{}).Between(at, at); err == nil {
		t.Fatal("the zero track computed a usage")
	}
	b, err := meter.PlanReset(meter.Odometer, nil, []meter.Entry{{Point: meter.Point{ID: "r0", Value: 10, OccurredAt: at, ReceivedAt: at}, State: meter.Valid}}, at.Add(time.Hour), 20, 0)
	if err != nil || b.Close == nil || *b.Close != 20 || b.Implied != 10 || b.Previous != "r0" {
		t.Fatalf("reset plan %+v %v", b, err)
	}
}
