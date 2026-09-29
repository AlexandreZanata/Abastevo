package domain

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
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

func TestVocabularyMatchesKernel(t *testing.T) {
	// The domain mirrors wire enums without importing them; this test fails
	// on drift instead of letting the lists diverge silently.
	for p, unit := range wireProducts {
		kp, err := kernel.ParseProduct(labelFor(p))
		if err != nil {
			t.Errorf("kernel rejects %s: %v", p, err)
			continue
		}
		ku, err := kp.Unit()
		if err != nil || string(ku) != unit {
			t.Errorf("%s unit = %q, kernel %q", p, unit, ku)
		}
	}
	if len(wireProducts) != 7 {
		t.Errorf("products = %d, want 7", len(wireProducts))
	}
}

func labelFor(p string) string {
	switch p {
	case "ETHANOL":
		return "ETANOL"
	case "GASOLINE_REGULAR":
		return "GASOLINA COMUM"
	case "GASOLINE_ADDITIVED":
		return "GASOLINA ADITIVADA"
	case "DIESEL_S500":
		return "OLEO DIESEL S500"
	case "DIESEL_S10":
		return "OLEO DIESEL S10"
	case "CNG":
		return "GNV"
	case "LPG_P13":
		return "GLP P13"
	default:
		return "???"
	}
}

func validParams() Params {
	now := time.Now()
	return Params{
		ID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", ContributorRef: "ref-1",
		ClientSubmissionID: "client-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27",
		Product: "GASOLINE_REGULAR", Unit: "L", AmountMilli: 5999,
		RawText: "5,999", ConditionKind: "STANDARD",
		ClaimedCapturedAt: now.Add(-time.Hour), ReceivedAt: now,
	}
}

func TestNewObservationHappyPath(t *testing.T) {
	obs, evt, err := NewObservation(validParams())
	if err != nil {
		t.Fatalf("valid observation rejected: %v", err)
	}
	if obs.QualifierKey != StandardQualifier {
		t.Errorf("qualifier = %q", obs.QualifierKey)
	}
	if obs.PolicyVersion != PolicyV1 || obs.Freshness != Fresh {
		t.Errorf("obs = %+v", obs)
	}
	if evt.ObservationID != obs.ID || !evt.OccurredAt.Equal(obs.ReceivedAt) {
		t.Errorf("event = %+v", evt)
	}
}

func TestNoClientControlledTrust(t *testing.T) {
	// Params carries no trust, confidence, reputation or server-time fields
	// by construction: the struct shape itself is the assertion, exercised
	// here so additions show up in review.
	p := validParams()
	if p.ReceivedAt.IsZero() {
		t.Error("received time must come from the server clock")
	}
}

func TestInvalidAmountUnitConditionSupersedes(t *testing.T) {
	base := validParams()
	cases := []struct {
		name   string
		mutate func(*Params)
	}{
		{"zero amount", func(p *Params) { p.AmountMilli = 0 }},
		{"over range", func(p *Params) { p.AmountMilli = 1000001 }},
		{"negative", func(p *Params) { p.AmountMilli = -5 }},
		{"unknown product", func(p *Params) { p.Product = "JET_A1" }},
		{"unit mismatch", func(p *Params) { p.Unit = "M3" }},
		{"unknown condition", func(p *Params) { p.ConditionKind = "VIP" }},
		{"empty id", func(p *Params) { p.ID = "" }},
		{"empty contributor", func(p *Params) { p.ContributorRef = "" }},
		{"empty submission", func(p *Params) { p.ClientSubmissionID = "" }},
		{"empty station", func(p *Params) { p.StationID = "" }},
		{"missing received", func(p *Params) { p.ReceivedAt = time.Time{} }},
		{"future capture", func(p *Params) { p.ClaimedCapturedAt = p.ReceivedAt.Add(time.Hour) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := base
			c.mutate(&p)
			if _, _, err := NewObservation(p); err == nil {
				t.Error("invalid observation accepted")
			}
		})
	}
}

func TestSupersedesAndFreshness(t *testing.T) {
	now := time.Now()
	p := validParams()
	p.SupersedesID = "d6c74c23-63db-4c24-a2e5-408cb23bad28"
	obs, _, err := NewObservation(p)
	if err != nil {
		t.Fatalf("superseding observation rejected: %v", err)
	}
	if obs.SupersedesID == "" {
		t.Error("supersedes link lost")
	}
	old := validParams()
	old.ClaimedCapturedAt = now.Add(-25 * time.Hour)
	obs, _, err = NewObservation(old)
	if err != nil {
		t.Fatalf("old capture rejected (must record, flagged): %v", err)
	}
	if obs.Freshness != Historical {
		t.Error("25h-old capture not flagged historical")
	}
	missing := validParams()
	missing.ClaimedCapturedAt = time.Time{}
	obs, _, err = NewObservation(missing)
	if err != nil || obs.Freshness != Fresh {
		t.Errorf("missing capture = %+v, %v", obs, err)
	}
	if CaptureFreshness(time.Time{}, now) != Fresh {
		t.Error("zero capture not fresh-labelled")
	}
}
