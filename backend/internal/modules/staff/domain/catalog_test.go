package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// TestCatalog_STF01_MatchesGolden proves the code catalog equals the golden
// JSON and cannot be mutated by callers.
func TestCatalog_STF01_MatchesGolden(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "admin", "staff-roles-v2.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Version string     `json:"version"`
		Roles   []RoleInfo `json:"roles"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if want.Version != CatalogVersion || !reflect.DeepEqual(want.Roles, Catalog()) {
		t.Fatalf("golden=%+v catalog=%+v", want, Catalog())
	}
	copied := Catalog()
	copied[0].Label = "tampered"
	if Catalog()[0].Label == "tampered" {
		t.Fatal("catalog slice shared with caller")
	}
	for _, info := range want.Roles {
		if !ValidRole(info.Code) || Label(info.Code) != info.Label {
			t.Fatalf("role %s label=%q", info.Code, Label(info.Code))
		}
	}
	if ValidRole("administrator") || Label("administrator") != "" {
		t.Fatal("unknown role accepted")
	}
}

// TestEndAt_STF02 covers the same bounds as ENT-04.
func TestEndAt_STF02(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if !ValidEndAt(now, nil) {
		t.Fatal("open-ended assignment refused")
	}
	minimum, maximum := now.Add(MinAssignmentDuration), now.Add(MaxAssignmentDuration)
	if !ValidEndAt(now, &minimum) || !ValidEndAt(now, &maximum) {
		t.Fatal("boundary end_at refused")
	}
	below, above := minimum.Add(-time.Nanosecond), maximum.Add(time.Nanosecond)
	if ValidEndAt(now, &below) || ValidEndAt(now, &above) {
		t.Fatal("out-of-bounds end_at accepted")
	}
}
