package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestEveryOriginalProfileAndSelectorFieldParticipatesInDigest(t *testing.T) {
	s := scope(t)
	original, e := app.NewAssetWrite(input(t))
	if e != nil {
		t.Fatal(e)
	}
	hash := original.Digest(s)
	for name, alter := range map[string]func(*app.AssetWriteInput){
		"operation": func(v *app.AssetWriteInput) { v.OperationID = id(11) }, "kind": func(v *app.AssetWriteInput) { v.Kind = app.ChangeAsset; v.AssetID = id(20); v.ExpectedRevision = 1 }, "category-label": func(v *app.AssetWriteInput) { v.Category.NewLabel = "Outra categoria" }, "category-id": func(v *app.AssetWriteInput) { v.Category = app.CategoryChoice{ExistingID: id(30)} }, "unit": func(v *app.AssetWriteInput) { v.UnitID = id(31) },
		"asset-kind": func(v *app.AssetWriteInput) { v.Profile.Kind = fleet.Machine }, "name": func(v *app.AssetWriteInput) { v.Profile.Name = "Another synthetic asset" }, "fleet-number": func(v *app.AssetWriteInput) { v.Profile.FleetNumber = "F-02" }, "plate": func(v *app.AssetWriteInput) { v.Profile.Plate = "xyz-1d23" }, "serial": func(v *app.AssetWriteInput) { v.Profile.SerialNumber = "S-002" }, "basis": func(v *app.AssetWriteInput) { v.Profile.UsageBasis = fleet.Hours },
		"tank-role": func(v *app.AssetWriteInput) {
			v.Profile.Tanks[0], _ = fleet.NewTank(fuel.Catalog{}, fleet.Auxiliary, "275000", []string{"DIESEL_S10"})
		}, "tank-capacity": func(v *app.AssetWriteInput) {
			v.Profile.Tanks[0], _ = fleet.NewTank(fuel.Catalog{}, fleet.Main, "275001", []string{"DIESEL_S10"})
		}, "tank-products": func(v *app.AssetWriteInput) {
			v.Profile.Tanks[0], _ = fleet.NewTank(fuel.Catalog{}, fleet.Main, "275000", []string{"DIESEL_S10", "DIESEL_S500"})
		}, "tank-unit": func(v *app.AssetWriteInput) {
			v.Profile.Tanks[0], _ = fleet.NewTank(fuel.Catalog{}, fleet.Main, "275000", []string{"ELECTRICITY"})
			v.Profile.Expected = nil
		},
		"expected-value": func(v *app.AssetWriteInput) {
			v.Profile.Expected[0], _ = fleet.NewExpectedConsumption(fuel.Catalog{}, "DIESEL_S10", fleet.Distance, "8501", "Synthetic manufacturer", "2026-11-14")
		}, "expected-basis": func(v *app.AssetWriteInput) {
			v.Profile.Expected[0], _ = fleet.NewExpectedConsumption(fuel.Catalog{}, "DIESEL_S10", fleet.Hours, "8500", "Synthetic manufacturer", "2026-11-14")
		}, "expected-source": func(v *app.AssetWriteInput) {
			v.Profile.Expected[0], _ = fleet.NewExpectedConsumption(fuel.Catalog{}, "DIESEL_S10", fleet.Distance, "8500", "Changed source", "2026-11-14")
		}, "expected-date": func(v *app.AssetWriteInput) {
			v.Profile.Expected[0], _ = fleet.NewExpectedConsumption(fuel.Catalog{}, "DIESEL_S10", fleet.Distance, "8500", "Synthetic manufacturer", "2026-11-15")
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := input(t)
			alter(&v)
			changed, e := app.NewAssetWrite(v)
			if e != nil || changed.Digest(s) == hash {
				t.Fatal("original intent field was redacted or omitted from digest", e)
			}
		})
	}
	changed := input(t)
	changed.Kind = app.ChangeAsset
	changed.AssetID = id(20)
	changed.ExpectedRevision = 1
	a, e := app.NewAssetWrite(changed)
	if e != nil {
		t.Fatal(e)
	}
	changed.AssetID = id(21)
	b, _ := app.NewAssetWrite(changed)
	if a.Digest(s) == b.Digest(s) {
		t.Fatal("target missing from digest")
	}
	changed.AssetID = id(20)
	changed.ExpectedRevision = 2
	b, _ = app.NewAssetWrite(changed)
	if a.Digest(s) == b.Digest(s) {
		t.Fatal("revision missing from digest")
	}
	otherID, _ := kernel.ParseOrgID(id(99))
	other, _ := kernel.NewOrgScope(otherID)
	if original.Digest(other) == hash || original.Digest(kernel.OrgScope{}) != "" {
		t.Fatal("organization missing from digest")
	}
}

func TestCommandShapeRejectsTransitionSmugglingAndInvalidSelectors(t *testing.T) {
	for _, kind := range []app.AssetWriteKind{app.ChangeAsset, app.DiscardAsset, app.ActivateAsset} {
		v := app.AssetWriteInput{Kind: kind, OperationID: id(10), AssetID: id(20), ExpectedRevision: 1}
		if kind == app.ChangeAsset {
			v = input(t)
			v.Kind = kind
			v.AssetID = id(20)
			v.ExpectedRevision = 1
		}
		if _, e := app.NewAssetWrite(v); e != nil {
			t.Fatal("valid operation shape refused", e)
		}
		for _, revision := range []int64{0, -1, math.MaxInt64} {
			v.ExpectedRevision = revision
			if _, e := app.NewAssetWrite(v); !errors.Is(e, fleet.ErrInvalid) {
				t.Fatal("non-advancing revision accepted")
			}
		}
	}
	for name, alter := range map[string]func(*app.AssetWriteInput){"unknown-kind": func(v *app.AssetWriteInput) { v.Kind = "ARCHIVE" }, "bad-operation": func(v *app.AssetWriteInput) { v.OperationID = "not-an-id" }, "create-target": func(v *app.AssetWriteInput) { v.AssetID = id(20) }, "create-revision": func(v *app.AssetWriteInput) { v.ExpectedRevision = 1 }, "double-category": func(v *app.AssetWriteInput) { v.Category.ExistingID = id(30) }, "profile-category": func(v *app.AssetWriteInput) { v.Profile.Category = id(30) }, "invalid-unit": func(v *app.AssetWriteInput) { v.UnitID = "bad-unit" }, "unknown-kind-value": func(v *app.AssetWriteInput) { v.Profile.Kind = "BOAT" }, "unknown-basis": func(v *app.AssetWriteInput) { v.Profile.UsageBasis = "NONE" }} {
		t.Run(name, func(t *testing.T) {
			v := input(t)
			alter(&v)
			if _, e := app.NewAssetWrite(v); !errors.Is(e, fleet.ErrInvalid) {
				t.Fatal("invalid command accepted", e)
			}
		})
	}
	for _, kind := range []app.AssetWriteKind{app.DiscardAsset, app.ActivateAsset} {
		base := app.AssetWriteInput{Kind: kind, OperationID: id(10), AssetID: id(20), ExpectedRevision: 1}
		for _, alter := range []func(*app.AssetWriteInput){func(v *app.AssetWriteInput) { v.Profile.Name = "smuggled" }, func(v *app.AssetWriteInput) { v.UnitID = id(30) }, func(v *app.AssetWriteInput) { v.Category.NewLabel = "smuggled" }} {
			v := base
			alter(&v)
			if _, e := app.NewAssetWrite(v); !errors.Is(e, fleet.ErrInvalid) {
				t.Fatal("transition replaced profile/selector")
			}
		}
	}
	if _, e := (app.AssetWrite{}).Snapshot(); !errors.Is(e, fleet.ErrInvalid) || (app.AssetWrite{}).Digest(scope(t)) != "" {
		t.Fatal("zero command valid")
	}
	for _, f := range []app.AssetFilter{{}, {Limit: 101}, {Limit: -1}, {Limit: 10, State: "DELETED"}, {Limit: 10, Kind: "BOAT"}, {Limit: 10, AfterID: "bad"}} {
		if !errors.Is(f.Validate(), fleet.ErrInvalid) {
			t.Fatal("invalid filter accepted")
		}
	}
	for _, a := range []app.FleetActor{{}, {AccountID: id(2), SessionID: "bad"}, {AccountID: strings.ToUpper(id(2)), SessionID: id(3)}} {
		if !errors.Is(a.Validate(), fleet.ErrInvalid) {
			t.Fatal("invalid actor accepted")
		}
	}
	long := strings.Repeat("Categoria 😀 ", 100)
	if e := (app.CategoryChoice{NewLabel: long}).Validate(); e != nil {
		t.Fatal("invented category label cap", e)
	}
	if e := (app.CategoryChoice{NewLabel: string([]byte{0xff})}).Validate(); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("invalid Unicode category accepted")
	}
}

func TestRepositoryCannotPublishWrongScopeOrMalformedMetadata(t *testing.T) {
	ctx, finish, s, actor, d := decision(t, id(20), false)
	defer finish()
	base := view(t, s, 20)
	for name, alter := range map[string]func(*app.AssetView){"foreign-target": func(v *app.AssetView) { v.Ref = view(t, s, 21).Ref }, "wrong-revision": func(v *app.AssetView) { v.Revision = 0 }, "invalid-category": func(v *app.AssetView) { v.Profile.Category = "CAT_A" }, "blank-label": func(v *app.AssetView) { v.CategoryLabel = " " }, "invalid-state": func(v *app.AssetView) { v.State = "DISCARDED" }, "active-without-allocation": func(v *app.AssetView) { v.State = fleet.Active }, "invalid-unit": func(v *app.AssetView) { v.UnitID = "bad" }, "allocation-unset-time": func(v *app.AssetView) { v.UnitID = id(31); v.Allocation = &app.UnitAllocation{UnitID: id(31)} }, "allocation-wrong-unit": func(v *app.AssetView) {
		v.UnitID = id(31)
		v.Allocation = &app.UnitAllocation{UnitID: id(32), StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	}} {
		t.Run(name, func(t *testing.T) {
			v := base.Clone()
			alter(&v)
			repo := &repository{view: v}
			out, e := (app.AssetService{Enabled: true, Repository: repo}).Read(ctx, s, actor, d, id(20))
			if !errors.Is(e, app.ErrUnavailable) || out.Ref.ID().Value() != "" {
				t.Fatal("malformed private source published", e)
			}
		})
	}
}

func TestPrivateApplicationValuesRedactDiagnosticsAndCloneViews(t *testing.T) {
	s := scope(t)
	v := view(t, s, 20)
	v.UnitID = id(31)
	v.Allocation = &app.UnitAllocation{UnitID: id(31), StartedAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
	copy := v.Clone()
	copy.Profile.Tanks[0] = fleet.Tank{}
	copy.Allocation.UnitID = id(32)
	if v.Profile.Tanks[0].Snapshot().Capacity != 275000 || v.Allocation.UnitID != id(31) {
		t.Fatal("view clone escaped")
	}
	command, _ := app.NewAssetWrite(input(t))
	snapshot, _ := command.Snapshot()
	for _, value := range []any{app.FleetActor{AccountID: id(2), SessionID: id(3)}, snapshot, command, snapshot.Category, v, app.AssetResult{AssetID: id(20)}, app.AssetPage{Assets: []app.AssetView{v}}, app.AssetFilter{AfterID: id(20)}, *v.Allocation} {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var log strings.Builder
		slog.New(slog.NewJSONHandler(&log, nil)).Info("value", "value", value)
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(raw), log.String()} {
			if !strings.Contains(out, "redacted") || strings.Contains(out, "Caminhão") || strings.Contains(out, id(20)) || strings.Contains(out, "abc-1d23") {
				t.Fatalf("private diagnostics leaked for %T", value)
			}
		}
	}
}

func TestConcurrentCommandSnapshotsCannotChangeRetainedIntent(t *testing.T) {
	s := scope(t)
	command, e := app.NewAssetWrite(input(t))
	if e != nil {
		t.Fatal(e)
	}
	hash := command.Digest(s)
	start := make(chan struct{})
	done := make(chan bool, 16)
	for range 16 {
		go func() {
			<-start
			v, e := command.Snapshot()
			if e != nil {
				done <- false
				return
			}
			v.Profile.Tanks[0] = fleet.Tank{}
			v.Profile.Expected[0] = fleet.ExpectedConsumption{}
			v.Category.NewLabel = "mutated caller snapshot"
			done <- command.Digest(s) == hash
		}()
	}
	close(start)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for range 16 {
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("parallel caller changed retained private intent")
			}
		case <-deadline.C:
			t.Fatal("parallel immutable command test did not finish")
		}
	}
}

func TestCommandRejectsMalformedUTF8BeforeDigestAndKeepsLiteralReplacement(t *testing.T) {
	s := scope(t)
	for _, field := range []string{"name", "serial"} {
		t.Run(field, func(t *testing.T) {
			valid := input(t)
			if field == "name" {
				valid.Profile.Name = "\ufffd"
			} else {
				valid.Profile.SerialNumber = "\ufffd"
			}
			original, e := app.NewAssetWrite(valid)
			if e != nil {
				t.Fatal("literal replacement rune is valid observed text", e)
			}
			malformed := valid
			if field == "name" {
				malformed.Profile.Name = string([]byte{0xff})
			} else {
				malformed.Profile.SerialNumber = string([]byte{0xff})
			}
			changed, e := app.NewAssetWrite(malformed)
			if !errors.Is(e, fleet.ErrInvalid) {
				t.Fatalf("malformed UTF8 reached digest (equal_to_literal=%t)", changed.Digest(s) == original.Digest(s))
			}
		})
	}
	for _, field := range []string{"fleet", "plate"} {
		bad := input(t)
		if field == "fleet" {
			bad.Profile.FleetNumber = string([]byte{0xff})
		} else {
			bad.Profile.Plate = string([]byte{0xff})
		}
		if _, e := app.NewAssetWrite(bad); !errors.Is(e, fleet.ErrInvalid) {
			t.Fatal("malformed identifier text reached digest")
		}
	}
}
