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
	// Same architecture rule as the kernel: domain is stdlib-only, so the
	// database can never leak into identity decisions.
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

func TestParseQuality(t *testing.T) {
	for _, q := range []string{QualityUnknown, QualityCityCentroid, QualityReviewed} {
		if got, err := ParseQuality(q); err != nil || got != q {
			t.Errorf("quality %q rejected: %v", q, err)
		}
	}
	if _, err := ParseQuality("precise"); err == nil {
		t.Error("unknown quality accepted")
	}
	if _, err := ParseQuality(""); err == nil {
		t.Error("empty quality accepted")
	}
}

func TestProjectionRule(t *testing.T) {
	reviewed := LocationRevision{Quality: QualityReviewed, PointWKT: "POINT(-46.633 -23.550)"}
	if err := reviewed.Projected(); err != nil {
		t.Errorf("reviewed point refused: %v", err)
	}
	cases := []struct {
		name string
		rev  LocationRevision
	}{
		{"city centroid never projects", LocationRevision{Quality: QualityCityCentroid, PointWKT: "POINT(-46.6 -23.5)"}},
		{"unknown never projects", LocationRevision{Quality: QualityUnknown, PointWKT: "POINT(-46.6 -23.5)"}},
		{"reviewed without point", LocationRevision{Quality: QualityReviewed}},
		{"reviewed blank point", LocationRevision{Quality: QualityReviewed, PointWKT: "  "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.rev.Projected(); err == nil {
				t.Error("non-projectable revision passed the projection rule")
			}
		})
	}
}
