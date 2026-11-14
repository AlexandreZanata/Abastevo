package meter_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestCaptureSourceHasExactKindScopedLocatorAndPrivateDiagnostics(t *testing.T) {
	s := scope(t, 1)
	for _, kind := range []meter.CaptureKind{meter.CaptureEnrolledDevice, meter.CaptureWebSession} {
		source, err := meter.NewCaptureSource(s, kind, uuid(51))
		if err != nil || source.Kind() != kind || source.ID() != uuid(51) || source.OrganizationID() != s.OrgID() {
			t.Fatal("explicit historic capture source lost", err)
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, source), source.ID()) {
				t.Fatal("capture diagnostic leaked locator")
			}
		}
		raw, err := json.Marshal(source)
		if err != nil || strings.Contains(string(raw), source.ID()) {
			t.Fatal("capture JSON leaked locator")
		}
		if strings.Contains(slog.AnyValue(source).Resolve().String(), source.ID()) {
			t.Fatal("capture log leaked locator")
		}
	}
	for _, kind := range []meter.CaptureKind{"", "DEVICE", "WEB_SESSION ", "web_session"} {
		if _, err := meter.NewCaptureSource(s, kind, uuid(51)); !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("unknown capture tag accepted")
		}
	}
	for _, id := range []string{"", "00000000-0000-0000-0000-000000000000", strings.ToUpper(uuid(51)), "session-token-private"} {
		// This generated UUID has alphabetic hex digits so uppercase is distinct.
		if id == uuid(51) {
			continue
		}
		if _, err := meter.NewCaptureSource(s, meter.CaptureWebSession, id); !errors.Is(err, meter.ErrInvalid) {
			t.Fatal("malformed historic capture locator accepted")
		}
	}
	if _, err := meter.NewCaptureSource(kernel.OrgScope{}, meter.CaptureWebSession, uuid(51)); !errors.Is(err, meter.ErrScope) {
		t.Fatal("invalid capture scope accepted")
	}
}

func TestLegacyReadingCaptureRemainsCanonicalAndRoundTrips(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	r := reading(t, s, seg, 20, 30, 10, "2026-11-02T00:00:00Z", meter.Valid)
	if source := r.CaptureSource(); source.Kind() != meter.CaptureEnrolledDevice || source.ID() != r.Snapshot().Device.ID() || source.OrganizationID() != s.OrgID() {
		t.Fatal("legacy enrolled shorthand has no canonical provenance")
	}
	copy, err := meter.NewReading(s, r.Snapshot())
	if err != nil || copy.CaptureSource() != r.CaptureSource() || copy.Snapshot().Capture != nil {
		t.Fatal("legacy snapshot compatibility lost", err)
	}
}

func TestExplicitCaptureRequiresExactlyOneSourceAndRequiredEvidence(t *testing.T) {
	s := scope(t, 1)
	other := scope(t, 2)
	seg := segment(t, s, meter.Odometer)
	base := reading(t, s, seg, 20, 30, 10, "2026-11-02T00:00:00Z", meter.Valid).Snapshot()
	for _, kind := range []meter.CaptureKind{meter.CaptureEnrolledDevice, meter.CaptureWebSession} {
		source, err := meter.NewCaptureSource(s, kind, uuid(51))
		if err != nil {
			t.Fatal("valid capture fixture failed", err)
		}
		input := base
		input.Device = meter.DeviceRef{}
		input.Capture = &source
		out, err := meter.NewReading(s, input)
		if err != nil || out.CaptureSource() != source || out.Snapshot().Device.ID() != "" || out.Snapshot().Capture == nil {
			t.Fatal("explicit capture reading refused", err)
		}
		if copy, err := meter.NewReading(s, out.Snapshot()); err != nil || copy.CaptureSource() != source {
			t.Fatal("explicit capture snapshot cannot roundtrip", err)
		}
		for name, change := range map[string]func(*meter.ReadingInput){
			"both matching locators":  func(v *meter.ReadingInput) { v.Device = base.Device },
			"both differing locators": func(v *meter.ReadingInput) { v.Device, _ = meter.NewDeviceRef(s, uuid(52)) },
			"neither":                 func(v *meter.ReadingInput) { v.Capture = nil },
			"zero capture":            func(v *meter.ReadingInput) { v.Capture = &meter.CaptureSource{} },
			"missing author":          func(v *meter.ReadingInput) { v.Author = meter.MembershipRef{} },
			"missing operation":       func(v *meter.ReadingInput) { v.Operation = meter.OperationRef{} },
		} {
			t.Run(string(kind)+"/"+name, func(t *testing.T) {
				value := input
				change(&value)
				if _, err := meter.NewReading(s, value); !errors.Is(err, meter.ErrInvalid) {
					t.Fatal("ambiguous or missing evidence accepted", err)
				}
			})
		}
		foreign, _ := meter.NewCaptureSource(other, kind, uuid(51))
		input.Capture = &foreign
		if _, err := meter.NewReading(s, input); !errors.Is(err, meter.ErrScope) {
			t.Fatal("foreign capture source accepted", err)
		}
		// Neither caller nor snapshot can retag an already accepted observation.
		input = out.Snapshot()
		replacement, _ := meter.NewCaptureSource(other, kind, uuid(52))
		*input.Capture = replacement
		if out.CaptureSource() != source || out.Snapshot().Capture.OrganizationID() != s.OrgID() {
			t.Fatal("snapshot capture aliases retained evidence")
		}
	}
}
