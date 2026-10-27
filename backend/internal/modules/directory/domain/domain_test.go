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
	for _, q := range []string{QualityUnknown, QualityCityCentroid, QualityReviewed,
		QualityOSMMapped, QualityANPReported, QualityAddressPoint} {
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
	for _, q := range []string{QualityReviewed, QualityOSMMapped, QualityANPReported, QualityAddressPoint} {
		pin := LocationRevision{Quality: q, PointWKT: "POINT(-46.633 -23.550)"}
		if err := pin.Projected(); err != nil {
			t.Errorf("%s point refused: %v", q, err)
		}
	}
	cases := []struct {
		name string
		rev  LocationRevision
	}{
		{"city centroid never projects", LocationRevision{Quality: QualityCityCentroid, PointWKT: "POINT(-46.6 -23.5)"}},
		{"unknown never projects", LocationRevision{Quality: QualityUnknown, PointWKT: "POINT(-46.6 -23.5)"}},
		{"reviewed without point", LocationRevision{Quality: QualityReviewed}},
		{"reviewed blank point", LocationRevision{Quality: QualityReviewed, PointWKT: "  "}},
		{"anp-reported without point", LocationRevision{Quality: QualityANPReported}},
		{"address-point without point", LocationRevision{Quality: QualityAddressPoint, PointWKT: " "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.rev.Projected(); err == nil {
				t.Error("non-projectable revision passed the projection rule")
			}
		})
	}
}

// B-BR-MAP-02: the location ladder is reviewed > osm-mapped > anp-reported >
// address-point; city-centroid and unknown are never pins. A lower tier never
// replaces a higher one; an equal tier may refresh its point.
func TestPinLadder(t *testing.T) {
	order := []string{QualityReviewed, QualityOSMMapped, QualityANPReported, QualityAddressPoint}
	for i := 1; i < len(order); i++ {
		if PinRank(order[i-1]) <= PinRank(order[i]) {
			t.Errorf("%s must outrank %s", order[i-1], order[i])
		}
	}
	for _, q := range []string{QualityUnknown, QualityCityCentroid, "", "precise"} {
		if IsPin(q) || PinRank(q) != 0 {
			t.Errorf("%q must not be a pin", q)
		}
	}
	for _, q := range order {
		if !IsPin(q) {
			t.Errorf("%s must be a pin", q)
		}
	}
	cases := []struct {
		current, next string
		want          bool
	}{
		{"", QualityAddressPoint, true},
		{QualityUnknown, QualityANPReported, true},
		{QualityCityCentroid, QualityAddressPoint, true},
		{QualityANPReported, QualityReviewed, true},
		{QualityANPReported, QualityANPReported, true},
		{QualityReviewed, QualityOSMMapped, false},
		{QualityReviewed, QualityANPReported, false},
		{QualityOSMMapped, QualityAddressPoint, false},
		{QualityAddressPoint, QualityUnknown, false},
		{QualityUnknown, QualityCityCentroid, false},
	}
	for _, c := range cases {
		if got := MayReplace(c.current, c.next); got != c.want {
			t.Errorf("MayReplace(%q, %q) = %v, want %v", c.current, c.next, got, c.want)
		}
	}
}
