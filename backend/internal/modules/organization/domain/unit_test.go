package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	times "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func unitProfile() d.UnitProfile {
	return d.UnitProfile{Name: " Synthetic North ", Code: " N01 ", MunicipalityCode: "5103403"}
}
func TestOperatingUnitLifecycleRetainsIdentityAndProtectsArchive(t *testing.T) {
	scope := legalScope(t, 301)
	unit, err := d.NewUnit(scope, legalID(801), unitProfile(), "America/Cuiaba")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := unit.Snapshot(scope)
	if err != nil || snap.State != d.UnitActive || snap.Revision != 1 || snap.UnitProfile.Name != "Synthetic North" || snap.UnitProfile.Code != "N01" || snap.UnitProfile.TimeZone != "America/Cuiaba" {
		t.Fatal("unit defaults/canonical profile", err)
	}
	for _, test := range []struct {
		facts d.UnitArchiveFacts
		want  error
	}{
		{d.UnitArchiveFacts{ActiveUnits: 0}, d.ErrConditions},
		{d.UnitArchiveFacts{ActiveUnits: 1}, d.ErrLastUnit},
		{d.UnitArchiveFacts{ActiveUnits: 2, Usage: d.UnitUsage{Assets: 1}}, d.ErrStructureInUse},
		{d.UnitArchiveFacts{ActiveUnits: 2, Usage: d.UnitUsage{Employees: 2}}, d.ErrStructureInUse},
		{d.UnitArchiveFacts{ActiveUnits: 2, Usage: d.UnitUsage{Budgets: 1}}, d.ErrStructureInUse},
		{d.UnitArchiveFacts{ActiveUnits: 2, Usage: d.UnitUsage{Operations: 3}}, d.ErrStructureInUse},
		{d.UnitArchiveFacts{ActiveUnits: 2, Usage: d.UnitUsage{Assets: -1}}, d.ErrConditions},
	} {
		if _, err := unit.Archive(scope, 1, test.facts); !errors.Is(err, test.want) {
			t.Fatal("unsafe archive admitted", err)
		}
	}
	archived, err := unit.Archive(scope, 1, d.UnitArchiveFacts{ActiveUnits: 2})
	if err != nil {
		t.Fatal(err)
	}
	archivedSnap, err := archived.Snapshot(scope)
	if err != nil || archivedSnap.ID != snap.ID || archivedSnap.UnitProfile != snap.UnitProfile || archivedSnap.State != d.UnitArchived || archivedSnap.Revision != 2 {
		t.Fatal("archival changed identity/history", err)
	}
	if _, err := archived.Archive(scope, 2, d.UnitArchiveFacts{ActiveUnits: 2}); !errors.Is(err, d.ErrTransition) {
		t.Fatal("terminal archive repeated", err)
	}
	renamed := archivedSnap.UnitProfile
	renamed.Name = "Synthetic restored name"
	changed, err := archived.Change(scope, 2, renamed, "UTC")
	if err != nil {
		t.Fatal("archived metadata cannot resolve restoration collision", err)
	}
	restored, err := changed.Restore(scope, 3)
	if err != nil {
		t.Fatal(err)
	}
	final, err := restored.Snapshot(scope)
	if err != nil || final.State != d.UnitActive || final.Revision != 4 || final.ID != snap.ID || final.UnitProfile.Name != renamed.Name {
		t.Fatal("restore lost stable unit", err)
	}
	if _, err := restored.Restore(scope, 4); !errors.Is(err, d.ErrTransition) {
		t.Fatal("active restore admitted", err)
	}
	if _, err := unit.Snapshot(legalScope(t, 302)); !errors.Is(err, d.ErrScope) {
		t.Fatal("foreign snapshot", err)
	}
	for _, expected := range []int64{0, 2, -1} {
		if _, err := unit.Change(scope, expected, renamed, "UTC"); !errors.Is(err, d.ErrConflict) {
			t.Fatal("stale unit revision", err)
		}
	}
	exhausted := snap
	exhausted.Revision = math.MaxInt64
	maximum, err := d.RehydrateUnit(scope, exhausted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maximum.Change(scope, math.MaxInt64, renamed, "UTC"); !errors.Is(err, d.ErrRevision) {
		t.Fatal("unit revision overflow", err)
	}
}
func TestOperatingUnitValidationAndPrivateRepresentations(t *testing.T) {
	scope := legalScope(t, 301)
	for _, field := range []string{"name", "code", "zone", "municipality", "establishment"} {
		p := unitProfile()
		switch field {
		case "name":
			p.Name = "\u202esynthetic"
		case "code":
			p.Code = "bad\ncode"
		case "zone":
			p.TimeZone = "GMT"
		case "municipality":
			p.MunicipalityCode = "0000000"
		case "establishment":
			p.EstablishmentID = "not-an-id"
		}
		if _, err := d.NewUnit(scope, legalID(801), p, "UTC"); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid unit profile accepted", field, err)
		}
	}
	p := unitProfile()
	p.EstablishmentID = legalID(901)
	unit, err := d.NewUnit(scope, legalID(801), p, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := unit.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []any{p, unit, snap} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, item), "Synthetic") {
				t.Fatal("private unit formatted")
			}
		}
		raw, err := json.Marshal(item)
		if err != nil || strings.Contains(string(raw), "Synthetic") {
			t.Fatal("private unit encoded", err)
		}
	}
	for _, mutate := range []func(*d.UnitSnapshot){func(s *d.UnitSnapshot) { s.UnitProfile.Name = " noncanonical " }, func(s *d.UnitSnapshot) { s.UnitProfile.TimeZone = "" }, func(s *d.UnitSnapshot) { s.State = "DELETED" }, func(s *d.UnitSnapshot) { s.Revision = 0 }, func(s *d.UnitSnapshot) { s.ID = "" }} {
		invalid := snap
		mutate(&invalid)
		if _, err := d.RehydrateUnit(scope, invalid); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("corrupt unit storage normalized", err)
		}
	}
}
func TestOperatingUnitZoneRevisionDoesNotRebucketPriorRecord(t *testing.T) {
	scope := legalScope(t, 301)
	unit, err := d.NewUnit(scope, legalID(801), unitProfile(), "America/Cuiaba")
	if err != nil {
		t.Fatal(err)
	}
	before, err := unit.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 12, 2, 30, 0, 0, time.UTC)
	declaration := times.Declaration{OccurredAt: at, EntryZone: "UTC"}
	old, err := times.Capture(declaration, at.Add(time.Minute), times.ReportingZones{Organization: "UTC", Unit: &before.UnitProfile.TimeZone})
	if err != nil {
		t.Fatal(err)
	}
	next := before.UnitProfile
	next.TimeZone = "UTC"
	revised, err := unit.Change(scope, 1, next, "America/Cuiaba")
	if err != nil {
		t.Fatal(err)
	}
	after, err := revised.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := times.Capture(declaration, at.Add(2*time.Minute), times.ReportingZones{Organization: "UTC", Unit: &after.UnitProfile.TimeZone})
	if err != nil {
		t.Fatal(err)
	}
	if old.ReportingDay != "2026-11-11" || old.ReportingZone != "America/Cuiaba" || fresh.ReportingDay != "2026-11-12" || fresh.ReportingZone != "UTC" || before.Revision != 1 || after.Revision != 2 {
		t.Fatal("zone revision rewrote historical reporting day")
	}
}
