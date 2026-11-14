package http

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestMeterProjectionPreservesExactObservationAndRealWebProvenance(t *testing.T) {
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	id := func(n string) string { return "7b0e2a54-0000-4000-8000-00000000" + n }
	assetID, _ := fleet.ParseAssetID(id("8201"))
	asset, _ := fleet.NewAssetRef(scope, assetID)
	modulus := int64(1000000000001)
	m, _ := meter.NewMeterRef(scope, asset, id("8401"), meter.Odometer, &modulus)
	seg, _ := meter.NewSegmentRef(scope, m, id("8402"))
	ref, _ := meter.NewReadingRef(scope, seg, id("8403"))
	author, _ := meter.NewMembershipRef(scope, id("8101"))
	operation, _ := meter.NewOperationRef(scope, id("8102"))
	capture, _ := meter.NewCaptureSource(scope, meter.CaptureWebSession, id("8103"))
	value, _ := meter.NewValue(meter.Odometer, 1000000000000, &modulus)
	at := time.Date(2026, 11, 14, 0, 0, 0, 123456789, time.UTC)
	declaration := orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	reading, e := meter.NewReading(scope, meter.ReadingInput{Reference: ref, Value: value, Declared: &declaration, ReceivedAt: at.Add(time.Second), Author: author, Capture: &capture, Operation: operation, Origin: meter.Installation, Method: meter.Typed, State: meter.Valid})
	if e != nil {
		t.Fatal(e)
	}
	v := app.MeterView{Reference: m, Label: "Private observed display", InstalledAt: at, Segment: seg, Latest: &reading}
	out, e := projectMeter(scope, v)
	if e != nil {
		t.Fatal("real web observation refused", e)
	}
	raw, e := json.Marshal(out)
	if e != nil {
		t.Fatal(e)
	}
	var wire map[string]any
	if json.Unmarshal(raw, &wire) != nil {
		t.Fatal("private projection")
	}
	latest := wire["latest"].(map[string]any)
	if wire["modulus"] != "1000000000001" || latest["value"] != "1000000000000" || latest["occurred_at"] != "2026-11-14T00:00:00.123456789Z" || latest["capture"].(map[string]any)["kind"] != "WEB_SESSION" {
		t.Fatal("observed evidence rounded/retagged")
	}
	foreign := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008002")
	if _, e := projectMeter(foreign, v); !errors.Is(e, orgapi.Unavailable) {
		t.Fatal("foreign evidence published")
	}
	for name, edit := range map[string]func(*app.MeterView){"empty-label": func(v *app.MeterView) { v.Label = "" }, "foreign-segment": func(v *app.MeterView) { v.Segment = meter.SegmentRef{} }, "backwards-service": func(v *app.MeterView) { v.RemovedAt = &at }, "other-reading": func(v *app.MeterView) { n := meter.Reading{}; v.Latest = &n }} {
		t.Run(name, func(t *testing.T) {
			copy := v.Clone()
			edit(&copy)
			if _, e := projectMeter(scope, copy); !errors.Is(e, orgapi.Unavailable) {
				t.Fatal("unverified private evidence projected")
			}
		})
	}
}

func TestMeterConflictsUseBoundedWireCodes(t *testing.T) {
	for _, tc := range []struct {
		source error
		code   orgapi.Code
		status int
	}{{meter.ErrAlreadyInService, orgapi.MeterAlreadyInService, 409}, {meter.ErrOperationReused, orgapi.OperationReused, 409}, {meter.ErrInvalid, orgapi.ValidationFailed, 422}, {meter.ErrScope, orgapi.NotFound, 404}} {
		code, ok := orgapi.Registered(meterMapped(tc.source))
		w := httptest.NewRecorder()
		orgapi.WriteError(w, httptest.NewRequest("POST", "/", nil), code)
		if !ok || code != tc.code || w.Code != tc.status {
			t.Fatal("unsafe meter error mapping", code)
		}
	}
}
