package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDomainStdlibOnly(t *testing.T) {
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(path, ".") {
				t.Errorf("%s imports non-stdlib %q", name, path)
			}
		}
	}
}

func TestReviewGate(t *testing.T) {
	cases := []struct {
		name        string
		staged      int64
		quarantined int64
		prev        int64
		hasPrev     bool
		publish     bool
		reason      string
	}{
		{"clean first import", 100, 0, 0, false, true, ""},
		{"exact one percent publishes", 99, 1, 0, false, true, ""},
		{"over one percent reviews", 98, 2, 0, false, false, "quarantine-share"},
		{"empty refused", 0, 0, 0, false, false, "empty"},
		{"only quarantine refused", 0, 5, 0, false, false, "quarantine-share"},
		{"exact twenty percent drop publishes", 80, 0, 100, true, true, ""},
		{"over twenty percent reviews", 79, 0, 100, true, false, "row-drop"},
		{"growth publishes", 150, 0, 100, true, true, ""},
		{"drop without baseline publishes", 10, 0, 0, false, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := ReviewGate(c.staged, c.quarantined, c.prev, c.hasPrev)
			if v.Publish != c.publish || v.Reason != c.reason {
				t.Errorf("got %+v", v)
			}
		})
	}
}

func TestQuarantineTally(t *testing.T) {
	var q QuarantineTally
	if q.Total() != 0 {
		t.Error("fresh tally nonzero")
	}
	q.Add("unknown-fuel-label", "row-8")
	q.Add("unknown-fuel-label", "row-9")
	if q.Total() != 2 || q.Counts["unknown-fuel-label"] != 2 {
		t.Errorf("tally = %+v", q)
	}
	for i := 0; i < 50; i++ {
		q.Add("x", "y")
	}
	if len(q.Samples) != 20 {
		t.Errorf("samples unbounded: %d", len(q.Samples))
	}
}
