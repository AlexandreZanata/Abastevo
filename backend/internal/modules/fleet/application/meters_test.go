package application

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const meterAppID = "00000000-0000-4000-8000-000000000001"

func meterInstallInput(t *testing.T) MeterInstallInput {
	t.Helper()
	value, err := meter.NewValue(meter.Odometer, 123, nil)
	if err != nil {
		t.Fatal(err)
	}
	return MeterInstallInput{OperationID: meterAppID, AssetID: meterAppID, OpeningReadingID: meterAppID,
		ExpectedAssetRevision: 1, Kind: meter.Odometer, Label: "Observed display", Value: value,
		Declared: orgtime.Declaration{OccurredAt: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, Method: meter.Typed}
}
func TestMeterInstallPreservesExactObservedIntent(t *testing.T) {
	in := meterInstallInput(t)
	modulus := int64(1000)
	in.Modulus = &modulus
	command, err := NewMeterInstall(in)
	if err != nil {
		t.Fatal("actual opening evidence rejected", err)
	}
	modulus = 500
	got, err := command.Snapshot()
	if err != nil || got.Modulus == nil || *got.Modulus != 1000 || got.Value.Count() != 123 || got.Declared != in.Declared {
		t.Fatal("opening evidence or modulus aliased")
	}
	*got.Modulus = 600
	again, _ := command.Snapshot()
	if *again.Modulus != 1000 {
		t.Fatal("snapshot aliases stored modulus")
	}
	scope, _ := kernel.NewOrgScope(mustMeterOrg(t, meterAppID))
	digest := command.Digest(scope)
	if len(digest) != 64 {
		t.Fatal("missing exact command digest")
	}
	for _, edit := range []func(*MeterInstallInput){func(v *MeterInstallInput) { v.Label += " " }, func(v *MeterInstallInput) { v.OpeningReadingID = "00000000-0000-4000-8000-000000000002" }, func(v *MeterInstallInput) { v.ExpectedAssetRevision++ }, func(v *MeterInstallInput) { v.Declared.DeviceOffsetSeconds++ }, func(v *MeterInstallInput) { n := int64(1001); v.Modulus = &n }} {
		other := again
		edit(&other)
		candidate, e := NewMeterInstall(other)
		if e != nil || candidate.Digest(scope) == digest {
			t.Fatal("distinct observed intent collapsed", e)
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", command), in.Label) {
		t.Fatal("private command diagnostic")
	}
}
func mustMeterOrg(t *testing.T, id string) kernel.OrgID {
	t.Helper()
	v, e := kernel.ParseOrgID(id)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestMeterInstallInvalidInputs(t *testing.T) {
	for name, edit := range map[string]func(*MeterInstallInput){"operation": func(v *MeterInstallInput) { v.OperationID = "" }, "asset": func(v *MeterInstallInput) { v.AssetID = "" }, "reading": func(v *MeterInstallInput) { v.OpeningReadingID = "" }, "revision-zero": func(v *MeterInstallInput) { v.ExpectedAssetRevision = 0 }, "revision-overflow": func(v *MeterInstallInput) { v.ExpectedAssetRevision = math.MaxInt64 }, "kind": func(v *MeterInstallInput) { v.Kind = "OTHER" }, "mismatch": func(v *MeterInstallInput) { v.Kind = meter.HourMeter }, "label": func(v *MeterInstallInput) { v.Label = "\t" }, "unicode": func(v *MeterInstallInput) { v.Label = string([]byte{0xff}) }, "control": func(v *MeterInstallInput) { v.Label = "bad\nlabel" }, "time": func(v *MeterInstallInput) { v.Declared.OccurredAt = time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC) }, "zone": func(v *MeterInstallInput) { v.Declared.EntryZone = "missing/zone" }, "method": func(v *MeterInstallInput) { v.Method = "OTHER" }, "modulus": func(v *MeterInstallInput) { n := int64(123); v.Modulus = &n }} {
		t.Run(name, func(t *testing.T) {
			v := meterInstallInput(t)
			edit(&v)
			if _, e := NewMeterInstall(v); e == nil {
				t.Fatal("invalid opening accepted")
			}
		})
	}
}
func TestMeterReadingCommandOwnsExplicitObservation(t *testing.T) {
	base := meterInstallInput(t)
	v := MeterReadingInput{OperationID: base.OperationID, AssetID: base.AssetID, MeterID: meterAppID, SegmentID: meterAppID, ReadingID: base.OpeningReadingID, Value: base.Value, Declared: base.Declared, Origin: meter.Standalone, Method: meter.Typed}
	command, e := NewMeterReadingCommand(v)
	if e != nil {
		t.Fatal("valid explicit observation", e)
	}
	scope, _ := kernel.NewOrgScope(mustMeterOrg(t, meterAppID))
	if len(command.Digest(scope)) != 64 {
		t.Fatal("reading digest missing")
	}
	v.ReadingID = "00000000-0000-4000-8000-000000000002"
	other, e := NewMeterReadingCommand(v)
	if e != nil || other.Digest(scope) == command.Digest(scope) {
		t.Fatal("reading identity lost")
	}
	for _, edit := range []func(*MeterReadingInput){func(v *MeterReadingInput) { v.MeterID = "" }, func(v *MeterReadingInput) { v.SegmentID = "" }, func(v *MeterReadingInput) { v.ReadingID = "" }, func(v *MeterReadingInput) { v.Origin = "OTHER" }, func(v *MeterReadingInput) { v.Method = "" }, func(v *MeterReadingInput) { v.Declared.EntryZone = "" }} {
		bad := v
		edit(&bad)
		if _, e := NewMeterReadingCommand(bad); e == nil {
			t.Fatal("invalid observation accepted")
		}
	}
}
func TestMeterSourceAndFilterCannotGrantAuthority(t *testing.T) {
	scope, _ := kernel.NewOrgScope(mustMeterOrg(t, meterAppID))
	actor := FleetActor{AccountID: meterAppID, SessionID: meterAppID}
	source := MeterSource{Web: &actor}
	if source.Validate(scope) != nil || source.AccountID() != actor.AccountID {
		t.Fatal("valid shaped source refused")
	}
	clone := source.Clone()
	actor.AccountID = ""
	if clone.Web.AccountID != meterAppID {
		t.Fatal("source aliases caller")
	}
	if (MeterSource{}).Validate(scope) == nil {
		t.Fatal("missing source accepted")
	}
	if (MeterFilter{Limit: 100}).Validate() != nil {
		t.Fatal("valid bounded page")
	}
	for _, f := range []MeterFilter{{Limit: 0}, {Limit: 101}, {Limit: 1, AfterID: "foreign"}} {
		if f.Validate() == nil {
			t.Fatal("invalid page")
		}
	}
	for _, action := range []MeterAction{MeterInstallAction, MeterRecordAction, MeterReadAction} {
		if MeterPolicy(action).Validate() != nil {
			t.Fatal("source-owned action policy invalid")
		}
	}
	if MeterPolicy(0).Validate() == nil {
		t.Fatal("unknown action grants policy")
	}
}
