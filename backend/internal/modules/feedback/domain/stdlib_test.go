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
	// TARGET_ARCHITECTURE: feedback domain depends on the standard
	// library only, mirroring the account, kernel and identity packages.
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

func TestVerdictCodesAreStable(t *testing.T) {
	cases := map[error]string{
		ErrTextEmpty:           "text-empty",
		ErrTextTooLong:         "text-too-long",
		ErrTextInvalidEncoding: "text-invalid-encoding",
		ErrRatingOutOfRange:    "rating-out-of-range",
		ErrAgreementNegative:   "agreement-negative",
		ErrTargetInvalid:       "target-invalid",
		ErrGateRequired:        "gate-required",
		ErrStatsMissing:        "stats-missing",
		ErrRatingNotFound:      "rating-not-found",
	}
	for err, want := range cases {
		if got := VerdictCode(err); got != want {
			t.Errorf("VerdictCode(%v) = %q, want %q", err, got, want)
		}
	}
	if got := VerdictCode(nil); got != VerdictOK {
		t.Errorf("VerdictCode(nil) = %q, want ok", got)
	}
}
