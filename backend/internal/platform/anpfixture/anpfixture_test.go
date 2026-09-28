// Package anpfixture verifies the shared ANP normalization fixtures in
// contracts/testdata/anp (P02-T01). It checks manifest integrity (version,
// sample hashes), provenance, compatibility classification and the expected
// legacy-versus-target outputs, including the A03/A04/A05/A06/A07 bridges.
// No parser lives here; P02-T04 consumes these cases.
package anpfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type manifestCase struct {
	ID            string `json:"id"`
	File          string `json:"file"`
	SHA256        string `json:"sha256"`
	Compatibility string `json:"compatibility"`
}

type manifest struct {
	FormatVersion int            `json:"format_version"`
	Provenance    string         `json:"provenance"`
	Cases         []manifestCase `json:"cases"`
}

type fixture struct {
	ID            string         `json:"id"`
	Provenance    string         `json:"provenance"`
	Compatibility string         `json:"compatibility"`
	Reason        string         `json:"reason"`
	Input         map[string]any `json:"input"`
	Backend       map[string]any `json:"expected_backend"`
	Legacy        map[string]any `json:"expected_legacy"`
}

// Documented synthetic test identifiers with valid check digits. Corrupted
// variants flip the final digit. No other CNPJ-looking value may appear in
// the fixtures, so production identifiers cannot leak in unnoticed.
var testCNPJs = []string{
	"04218406000104",
	"11222333000181",
	"12ABC34501DE35",
	"04218406000100",
	"12ABC34501DE30",
}

var compatClasses = []string{"identical", "legacy-divergent", "backend-quarantine"}

func contractsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "contracts")
}

func anpDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(contractsDir(t), "testdata", "anp")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("missing fixture dir %s", dir)
	}
	return dir
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(anpDir(t), "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

func loadCase(t *testing.T, dir, file string) fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var c fixture
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return c
}

func TestManifestIntegrity(t *testing.T) {
	m := loadManifest(t)
	if m.FormatVersion != 1 {
		t.Errorf("format_version = %d, want 1", m.FormatVersion)
	}
	if m.Provenance != "synthetic" {
		t.Errorf("manifest provenance = %q, want synthetic", m.Provenance)
	}
	if len(m.Cases) == 0 {
		t.Fatal("manifest lists zero cases")
	}
	seen := map[string]bool{}
	dir := anpDir(t)
	for _, c := range m.Cases {
		if c.ID == "" || c.File == "" || c.SHA256 == "" {
			t.Errorf("manifest entry needs id/file/sha256: %+v", c)
		}
		if seen[c.ID] {
			t.Errorf("duplicate manifest id %s", c.ID)
		}
		seen[c.ID] = true
		if !slices.Contains(compatClasses, c.Compatibility) {
			t.Errorf("%s: unknown compatibility %q", c.ID, c.Compatibility)
		}
		raw, err := os.ReadFile(filepath.Join(dir, c.File))
		if err != nil {
			t.Errorf("missing case file %s: %v", c.File, err)
			continue
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != c.SHA256 {
			t.Errorf("%s: hash mismatch (fixture changed without manifest update)", c.File)
		}
		fx := loadCase(t, dir, c.File)
		if fx.ID != c.ID {
			t.Errorf("%s: file id %q != manifest id %q", c.File, fx.ID, c.ID)
		}
		if fx.Compatibility != c.Compatibility {
			t.Errorf("%s: file compatibility %q != manifest %q", c.File, fx.Compatibility, c.Compatibility)
		}
		if fx.Provenance != "synthetic" {
			t.Errorf("%s: provenance = %q, want synthetic", c.File, fx.Provenance)
		}
	}
}

func TestLegacyVersusTarget(t *testing.T) {
	m := loadManifest(t)
	dir := anpDir(t)
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		t.Run(fx.ID, func(t *testing.T) {
			switch fx.Compatibility {
			case "identical":
				if fx.Reason != "" {
					t.Errorf("identical case carries a reason: %q", fx.Reason)
				}
				if fx.Legacy == nil {
					t.Error("identical case needs expected_legacy")
				} else if !reflect.DeepEqual(fx.Backend, fx.Legacy) {
					t.Error("identical case: backend and legacy outputs differ")
				}
			case "legacy-divergent":
				if fx.Reason == "" {
					t.Error("divergent case needs a documented reason")
				}
				if fx.Legacy == nil {
					t.Error("divergent case needs expected_legacy")
				} else if reflect.DeepEqual(fx.Backend, fx.Legacy) {
					t.Error("divergent case: outputs unexpectedly equal")
				}
			case "backend-quarantine":
				if fx.Reason == "" {
					t.Error("quarantine case needs a documented reason")
				}
				if fx.Legacy != nil {
					t.Error("quarantine case must leave legacy absent until P10")
				}
				if fx.Backend["outcome"] == "ok" {
					t.Errorf("quarantine case claims ok: %v", fx.Backend)
				}
			default:
				t.Errorf("unknown compatibility %q", fx.Compatibility)
			}
		})
	}
}

func TestSevenProductsWithExactUnits(t *testing.T) {
	wantUnits := map[string]string{
		"ETHANOL": "L", "GASOLINE_REGULAR": "L", "GASOLINE_ADDITIVED": "L",
		"DIESEL_S500": "L", "DIESEL_S10": "L", "CNG": "M3", "LPG_P13": "KG_13",
	}
	seen := map[string]string{}
	m := loadManifest(t)
	dir := anpDir(t)
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		if fx.Compatibility == "backend-quarantine" {
			continue
		}
		p, _ := fx.Backend["product"].(string)
		u, _ := fx.Backend["unit"].(string)
		if p == "" {
			continue
		}
		if wantUnits[p] == "" {
			t.Errorf("%s: product %q outside the 7-product wire vocabulary", fx.ID, p)
			continue
		}
		if u != wantUnits[p] {
			t.Errorf("%s: product %s has unit %q, want %q", fx.ID, p, u, wantUnits[p])
		}
		seen[p] = fx.ID
	}
	for p := range wantUnits {
		if seen[p] == "" {
			t.Errorf("wire product %s has no passing case", p)
		}
	}
}

func TestA05PremiumBridge(t *testing.T) {
	m := loadManifest(t)
	dir := anpDir(t)
	found := false
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		inLabel, _ := fx.Input["label"].(string)
		if inLabel != "GASOLINA ADITIVADA" {
			continue
		}
		found = true
		if fx.Backend["product"] != "GASOLINE_ADDITIVED" {
			t.Errorf("%s: backend product = %v, want GASOLINE_ADDITIVED", fx.ID, fx.Backend["product"])
		}
		if fx.Compatibility != "legacy-divergent" || fx.Legacy["product"] != "GASOLINE_PREMIUM" {
			t.Errorf("%s: legacy must stay GASOLINE_PREMIUM with divergent classification", fx.ID)
		}
		if !strings.Contains(fx.Reason, "A05") {
			t.Errorf("%s: reason must cite A05: %q", fx.ID, fx.Reason)
		}
	}
	if !found {
		t.Error("no GASOLINA ADITIVADA bridge case")
	}
}

func TestA06CNPJShapes(t *testing.T) {
	m := loadManifest(t)
	dir := anpDir(t)
	var leadingZero, alnum, invalid bool
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		inText, _ := fx.Input["cnpj_text"].(string)
		if inText == "" {
			continue
		}
		norm, _ := fx.Backend["cnpj_normalized"].(string)
		switch fx.Compatibility {
		case "identical":
			if norm == "" {
				t.Errorf("%s: accepted CNPJ needs normalized form", fx.ID)
			}
			if !strings.HasPrefix(norm, "0") && strings.Contains(inText, "04.218") {
				t.Errorf("%s: leading zero lost in %q", fx.ID, norm)
			}
			if norm == "04218406000104" {
				leadingZero = true
			}
			if norm == "12ABC34501DE35" {
				alnum = true
				if fx.Backend["alphanumeric"] != true {
					t.Errorf("%s: alphanumeric case must flag the new format", fx.ID)
				}
			}
		case "backend-quarantine":
			invalid = true
			if fx.Backend["reason_code"] != "cnpj-checksum-invalid" {
				t.Errorf("%s: reason_code = %v", fx.ID, fx.Backend["reason_code"])
			}
		}
	}
	if !leadingZero || !alnum || !invalid {
		t.Errorf("need leading-zero (%v), alphanumeric (%v) and invalid (%v) CNPJ cases",
			leadingZero, alnum, invalid)
	}
}

func TestA04PrecisionAndUnits(t *testing.T) {
	m := loadManifest(t)
	dir := anpDir(t)
	var exact, overPrecision bool
	var kg13, m3 bool
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		raw, _ := fx.Backend["raw_price_text"].(string)
		amt, _ := fx.Backend["amount_milli_brl"].(float64)
		if raw == "5,999" && amt == 5999 {
			exact = true
		}
		if fx.Backend["reason_code"] == "over-precision" {
			overPrecision = true
		}
		if u, _ := fx.Backend["unit"].(string); u == "KG_13" {
			kg13 = true
		}
		if u, _ := fx.Backend["unit"].(string); u == "M3" {
			m3 = true
		}
	}
	if !exact {
		t.Error("no exact 5,999 → 5999 milli-BRL case")
	}
	if !overPrecision {
		t.Error("no over-precision quarantine case")
	}
	if !kg13 || !m3 {
		t.Errorf("need KG_13 (%v) and M3 (%v) unit cases", kg13, m3)
	}
}

func TestNoProductionPersonalData(t *testing.T) {
	m := loadManifest(t)
	dir := anpDir(t)
	// Patterns are assembled at runtime so this very file never carries a
	// matchable literal: scanner exclusions stay limited to the scanner and
	// gate scripts themselves.
	secret := []string{"gh" + "p_", "BEGIN PRIVATE " + "KEY", "sk" + "_live_", "AK" + "IA"}
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		if fx.Provenance != "synthetic" {
			t.Errorf("%s: provenance = %q", fx.ID, fx.Provenance)
		}
		raw, _ := json.Marshal(fx)
		for _, pat := range secret {
			if strings.Contains(string(raw), pat) {
				t.Errorf("%s: secret-like pattern %q", fx.ID, pat)
			}
		}
	}
	// Every CNPJ-looking value must belong to the documented test set.
	allowed := map[string]bool{}
	for _, v := range testCNPJs {
		allowed[v] = true
	}
	digits := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r >= '0' && r <= '9' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	for _, c := range m.Cases {
		fx := loadCase(t, dir, c.File)
		raw, _ := json.Marshal(fx.Input)
		for _, tok := range strings.FieldsFunc(string(raw), func(r rune) bool {
			return !(r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r == '.' || r == '/' || r == '-')
		}) {
			d := digits(tok)
			if len(d) == 14 {
				plain := strings.Map(func(r rune) rune {
					if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
						return r
					}
					return -1
				}, tok)
				if !allowed[plain] {
					t.Errorf("%s: CNPJ-like value %q outside the documented test set", fx.ID, tok)
				}
			}
		}
	}
}
