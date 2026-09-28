package kernel

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDomainStdlibOnly(t *testing.T) {
	// TARGET_ARCHITECTURE: domain depends on the standard library only.
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

func TestParseProduct(t *testing.T) {
	cases := []struct {
		name  string
		label string
		want  Product
	}{
		{"regular", "GASOLINA COMUM", GasolineRegular},
		{"regular padded", "  GASOLINA  COMUM ", GasolineRegular},
		{"additived", "GASOLINA ADITIVADA", GasolineAdditived},
		{"ethanol", "ETANOL", Ethanol},
		{"diesel s500 plain", "OLEO DIESEL S500", DieselS500},
		{"diesel s10 accented", "ÓLEO DIESEL S10", DieselS10},
		{"cng", "GNV", CNG},
		{"lpg", "GLP P13", LPGP13},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseProduct(c.label)
			if err != nil {
				t.Fatalf("valid label rejected: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	for _, label := range []string{"", "GASOLINA PODIUM", "DIESEL", "123", "GASOLINA\nCOMUM\x00"} {
		t.Run("invalid/"+strconv.Quote(label), func(t *testing.T) {
			if _, err := ParseProduct(label); err == nil {
				t.Errorf("label %q accepted", label)
			}
		})
	}
}

func TestProductUnitFixed(t *testing.T) {
	for p, want := range map[Product]Unit{
		Ethanol: Liter, GasolineRegular: Liter, GasolineAdditived: Liter,
		DieselS500: Liter, DieselS10: Liter, CNG: CubicMetre, LPGP13: Kg13,
	} {
		u, err := p.Unit()
		if err != nil || u != want {
			t.Errorf("%s unit = %q, %v; want %q", p, u, err, want)
		}
	}
	if _, err := Product("JET").Unit(); err == nil {
		t.Error("unknown product unit accepted")
	}
}

func TestParsePrice(t *testing.T) {
	valid := []struct {
		name    string
		product Product
		unit    Unit
		text    string
		milli   int64
	}{
		{"exact 3dp", GasolineRegular, Liter, "5,999", 5999},
		{"two dp", GasolineRegular, Liter, "5,99", 5990},
		{"integer", GasolineRegular, Liter, "6", 6000},
		{"trailing zero 4dp", GasolineRegular, Liter, "5,9990", 5999},
		{"cylinder", LPGP13, Kg13, "109,90", 109900},
		{"cubic metre", CNG, CubicMetre, "4,599", 4599},
		{"max", GasolineRegular, Liter, "1000,00", 1000000},
		{"padded", GasolineRegular, Liter, "  5,999  ", 5999},
	}
	for _, c := range valid {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParsePrice(c.product, c.unit, c.text)
			if err != nil {
				t.Fatalf("valid price rejected: %v", err)
			}
			if got.Milli != c.milli || got.Raw != c.text {
				t.Errorf("got %+v", got)
			}
		})
	}
	invalid := []struct {
		name    string
		product Product
		unit    Unit
		text    string
		code    string
	}{
		{"empty", GasolineRegular, Liter, "", "missing-price"},
		{"blank", GasolineRegular, Liter, "   ", "missing-price"},
		{"negative", GasolineRegular, Liter, "-1,00", "negative-price"},
		{"negative zero", GasolineRegular, Liter, "-0,00", "negative-price"},
		{"zero", GasolineRegular, Liter, "0,00", "zero-price"},
		{"over precision", GasolineRegular, Liter, "5,9999", "over-precision"},
		{"float dot", GasolineRegular, Liter, "5.999", "invalid-price"},
		{"thousands", GasolineRegular, Liter, "1.099,90", "invalid-price"},
		{"plus", GasolineRegular, Liter, "+5,00", "invalid-price"},
		{"letters", GasolineRegular, Liter, "cinco", "invalid-price"},
		{"dangling comma", GasolineRegular, Liter, "5,", "invalid-price"},
		{"leading comma", GasolineRegular, Liter, ",5", "invalid-price"},
		{"unit mismatch", GasolineRegular, CubicMetre, "5,999", "unit-mismatch"},
		{"unknown product", "JET", Liter, "5,999", "unknown-fuel-label"},
		{"over range", GasolineRegular, Liter, "1000,01", "over-range"},
	}
	for _, c := range invalid {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParsePrice(c.product, c.unit, c.text)
			if err == nil {
				t.Fatalf("invalid price %q accepted", c.text)
			}
			if got := QuarantineCode(err); got != c.code {
				t.Errorf("code = %q, want %q (err %v)", got, c.code, err)
			}
		})
	}
}

func TestParseCondition(t *testing.T) {
	q := "123e4567-e89b-12d3-a456-426614174000"
	if _, err := ParseCondition("STANDARD", nil); err != nil {
		t.Errorf("standard without qualifier rejected: %v", err)
	}
	if _, err := ParseCondition("standard", nil); err != nil {
		t.Errorf("lowercase kind rejected: %v", err)
	}
	if _, err := ParseCondition("APP", &q); err != nil {
		t.Errorf("app with qualifier rejected: %v", err)
	}
	for _, kind := range []string{"CASH", "DEBIT", "CREDIT", "LOYALTY", "OTHER"} {
		if _, err := ParseCondition(kind, nil); err != nil {
			t.Errorf("%s without qualifier rejected: %v", kind, err)
		}
	}
	bad := []struct {
		name string
		kind string
		q    *string
	}{
		{"standard with qualifier", "STANDARD", &q},
		{"empty qualifier", "APP", strptr("")},
		{"blank qualifier", "APP", strptr("  ")},
		{"unknown kind", "VIP", nil},
		{"unknown never standard", "PREMIUM", nil},
		{"empty kind", "", nil},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseCondition(c.kind, c.q); err == nil {
				t.Errorf("condition %q accepted", c.kind)
			}
		})
	}
}

func strptr(s string) *string { return &s }

func TestParseCNPJ(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		normalized string
		alnum      bool
	}{
		{"numeric", "11.222.333/0001-81", "11222333000181", false},
		{"leading zeros", "04.218.406/0001-04", "04218406000104", false},
		{"alphanumeric", "12ABC345/01DE-35", "12ABC34501DE35", true},
		{"lowercase alnum", "12abc345/01de-35", "12ABC34501DE35", true},
		{"bare digits", "11222333000181", "11222333000181", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseCNPJ(c.text)
			if err != nil {
				t.Fatalf("valid CNPJ rejected: %v", err)
			}
			if got.Normalized() != c.normalized {
				t.Errorf("normalized = %q, want %q", got.Normalized(), c.normalized)
			}
			if got.Alphanumeric() != c.alnum {
				t.Errorf("alphanumeric = %v, want %v", got.Alphanumeric(), c.alnum)
			}
		})
	}
	for _, text := range []string{
		"", "123", "04.218.406/0001-00", "12ABC345/01DE-30",
		"11222333000182", "04.218.406/0001-0", "04.218.406/0001-044",
		"12abc345/01de-3!", "AAAAAAAAAAAAAA", "abcdefghijklmn",
	} {
		t.Run("invalid/"+text, func(t *testing.T) {
			if _, err := ParseCNPJ(text); err == nil {
				t.Errorf("CNPJ %q accepted", text)
			} else if QuarantineCode(err) != "cnpj-checksum-invalid" {
				t.Errorf("code = %q", QuarantineCode(err))
			}
		})
	}
}

// TestANPSharedFixtures drives the P02-T01 normalization cases through the
// T02 values: every price/CNPJ/label input must produce the recorded backend
// outcome, and quarantine codes must match the fixture reason codes.
func TestANPSharedFixtures(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "contracts", "testdata", "anp")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Cases []struct {
			ID   string `json:"id"`
			File string `json:"file"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(m.Cases) == 0 {
		t.Fatal("zero fixture cases")
	}
	for _, mc := range m.Cases {
		raw, err := os.ReadFile(filepath.Join(dir, mc.File))
		if err != nil {
			t.Fatalf("read %s: %v", mc.File, err)
		}
		var fx struct {
			ID      string         `json:"id"`
			Input   map[string]any `json:"input"`
			Backend map[string]any `json:"expected_backend"`
		}
		if err := json.Unmarshal(raw, &fx); err != nil {
			t.Fatalf("parse %s: %v", mc.File, err)
		}
		t.Run(fx.ID, func(t *testing.T) {
			outcome, _ := fx.Backend["outcome"].(string)
			switch {
			case fx.Input["label"] != nil:
				label, _ := fx.Input["label"].(string)
				priceText, _ := fx.Input["price_text"].(string)
				unitText, _ := fx.Input["unit"].(string)
				product, perr := ParseProduct(label)
				unit, uerr := ParseUnit(unitText)
				var price Price
				var verr error
				if perr == nil && uerr == nil {
					price, verr = ParsePrice(product, unit, priceText)
				} else if perr != nil {
					verr = perr
				} else {
					verr = uerr
				}
				assertFixtureOutcome(t, fx.ID, outcome, verr, fx.Backend, func() map[string]any {
					return map[string]any{
						"product": string(price.Product), "unit": string(price.Unit),
						"amount_milli_brl": float64(price.Milli),
						"raw_price_text":   price.Raw, "outcome": outcome,
					}
				})
			case fx.Input["cnpj_text"] != nil:
				text, _ := fx.Input["cnpj_text"].(string)
				got, err := ParseCNPJ(text)
				if outcome == "ok" {
					if err != nil {
						t.Fatalf("valid CNPJ rejected: %v", err)
					}
					if want, _ := fx.Backend["cnpj_normalized"].(string); got.Normalized() != want {
						t.Errorf("normalized = %q, want %q", got.Normalized(), want)
					}
				} else if err == nil {
					t.Errorf("invalid CNPJ accepted")
				} else if want, _ := fx.Backend["reason_code"].(string); QuarantineCode(err) != want {
					t.Errorf("code = %q, want %q", QuarantineCode(err), want)
				}
			}
		})
	}
}

func assertFixtureOutcome(t *testing.T, id, outcome string, err error, backend map[string]any, actual func() map[string]any) {
	t.Helper()
	if outcome == "ok" || outcome == "ok-duplicate-suppressed" {
		if err != nil {
			t.Fatalf("valid input rejected: %v", err)
		}
		got := actual()
		for k, want := range backend {
			if k == "outcome" {
				continue
			}
			if !reflect.DeepEqual(got[k], want) {
				t.Errorf("%s = %v, want %v", k, got[k], want)
			}
		}
		return
	}
	if err == nil {
		t.Fatalf("quarantine input accepted")
	}
	want, _ := backend["reason_code"].(string)
	if QuarantineCode(err) != want {
		t.Errorf("code = %q, want %q (err %v)", QuarantineCode(err), want, err)
	}
}
