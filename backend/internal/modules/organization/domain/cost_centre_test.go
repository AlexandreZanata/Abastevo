package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestCostCentreLifecyclePreservesIdentityAndCodeCorrection(t *testing.T) {
	scope := legalScope(t, 301)
	centre, err := d.NewCostCentre(scope, legalID(901), d.CostCentreProfile{Name: " Synthetic field ", Code: " FARM-01 "})
	if err != nil {
		t.Fatal(err)
	}
	first, err := centre.Snapshot(scope)
	if err != nil || first.Name != "Synthetic field" || first.Code != "FARM-01" || first.Revision != 1 || first.State != d.CostCentreActive {
		t.Fatal("cost-centre canonical identity", err)
	}
	changed, err := centre.Change(scope, 1, d.CostCentreProfile{Name: "Synthetic renamed", Code: "FARM-02"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := changed.Snapshot(scope)
	if err != nil || next.ID != first.ID || next.Code != "FARM-02" || first.Code != "FARM-01" || next.Revision != 2 {
		t.Fatal("correction rewrote previous snapshot", err)
	}
	for _, usage := range []d.StructureUsage{{Assets: 1}, {Employees: 1}, {Budgets: 1}, {Operations: 1}} {
		if _, err := changed.Archive(scope, 2, usage); !errors.Is(err, d.ErrStructureInUse) {
			t.Fatal("in-use centre archived", err)
		}
	}
	if _, err := changed.Archive(scope, 2, d.StructureUsage{Assets: -1}); !errors.Is(err, d.ErrConditions) {
		t.Fatal("invalid source facts accepted", err)
	}
	archived, err := changed.Archive(scope, 2, d.StructureUsage{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archived.Archive(scope, 3, d.StructureUsage{}); !errors.Is(err, d.ErrTransition) {
		t.Fatal("repeated archive admitted", err)
	}
	restored, err := archived.Restore(scope, 3)
	if err != nil {
		t.Fatal(err)
	}
	final, err := restored.Snapshot(scope)
	if err != nil || final.ID != first.ID || final.State != d.CostCentreActive || final.Revision != 4 {
		t.Fatal("restore lost stable centre", err)
	}
	if _, err := restored.Restore(scope, 4); !errors.Is(err, d.ErrTransition) {
		t.Fatal("active restore admitted", err)
	}
	if _, err := restored.Snapshot(legalScope(t, 302)); !errors.Is(err, d.ErrScope) {
		t.Fatal("foreign centre snapshot", err)
	}
	if _, err := restored.Change(scope, 3, d.CostCentreProfile{Name: "new", Code: "C"}); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale centre changed", err)
	}
	final.Revision = math.MaxInt64
	maximum, err := d.RehydrateCostCentre(scope, final)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := maximum.Change(scope, math.MaxInt64, d.CostCentreProfile{Name: "new", Code: "C"}); !errors.Is(err, d.ErrRevision) {
		t.Fatal("centre revision overflow", err)
	}
}
func TestCostCentreRejectsCorruptStorageAndRedactsNamesAndCodes(t *testing.T) {
	scope := legalScope(t, 301)
	for _, profile := range []d.CostCentreProfile{{Name: "", Code: "C"}, {Name: "name", Code: " "}, {Name: "name\nprivate", Code: "C"}, {Name: "name", Code: "\u202eprivate"}, {Name: strings.Repeat("n", 121), Code: "C"}} {
		if _, err := d.NewCostCentre(scope, legalID(901), profile); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid cost-centre profile accepted", err)
		}
	}
	profile := d.CostCentreProfile{Name: "Synthetic private centre", Code: "PRIVATE-CODE"}
	centre, err := d.NewCostCentre(scope, legalID(901), profile)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := centre.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{profile, centre, snap} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			value := fmt.Sprintf(verb, v)
			if strings.Contains(value, "Synthetic") || strings.Contains(value, "PRIVATE-CODE") {
				t.Fatal("private cost-centre formatting leaked")
			}
		}
		raw, err := json.Marshal(v)
		if err != nil || strings.Contains(string(raw), "Synthetic") || strings.Contains(string(raw), "PRIVATE-CODE") {
			t.Fatal("private cost-centre JSON leaked", err)
		}
	}
	for _, change := range []func(*d.CostCentreSnapshot){func(s *d.CostCentreSnapshot) { s.Name = " untrimmed " }, func(s *d.CostCentreSnapshot) { s.Code = " untrimmed " }, func(s *d.CostCentreSnapshot) { s.State = "DELETED" }, func(s *d.CostCentreSnapshot) { s.Revision = 0 }, func(s *d.CostCentreSnapshot) { s.ID = "" }} {
		bad := snap
		change(&bad)
		if _, err := d.RehydrateCostCentre(scope, bad); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("corrupt cost-centre storage normalized", err)
		}
	}
}
