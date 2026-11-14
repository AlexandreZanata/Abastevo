package fiscal_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	doc "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain/fiscal"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	issuerOne = "SYNTHE00000140"
	keyOne    = "432611SYNTHE00000140650010000001231100000010"
	qrOne     = "https://www.sefaz.rs.gov.br/NFCE/NFCE-COM.aspx?p=" + keyOne + "|3|1"
	nfeNS     = "http://www.portalfiscal.inf.br/nfe"
)

// synthKey appends an independently computed modulo-11 digit (values ASCII−48).
func synthKey(body string) string {
	sum, weight := 0, 2
	for i := len(body) - 1; i >= 0; i-- {
		sum += int(body[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	digit := 0
	if r := sum % 11; r > 1 {
		digit = 11 - r
	}
	return body + string(rune('0'+digit))
}

func minimalXML(key, cStat string) string {
	return `<nfeProc xmlns="` + nfeNS + `" versao="4.00"><NFe><infNFe Id="NFe` + key + `" versao="4.00"><ide><cUF>43</cUF>` +
		`<cNF>10000001</cNF><mod>65</mod><serie>1</serie><nNF>123</nNF><dhEmi>2026-11-14T10:00:00-03:00</dhEmi>` +
		`<tpEmis>1</tpEmis><cDV>0</cDV></ide><emit><CNPJ>` + issuerOne + `</CNPJ></emit><det nItem="1"/>` +
		`<total><ICMSTot><vNF>57.30</vNF></ICMSTot></total></infNFe></NFe><protNFe><infProt><chNFe>` + key +
		`</chNFe><cStat>` + cStat + `</cStat></infProt></protNFe></nfeProc>`
}

func samples() map[string]fiscal.Result {
	ok, _ := fiscal.Compare(fiscal.CheckAccessKey(keyOne), fiscal.CheckQRCode(qrOne))
	bad, _ := fiscal.Compare(fiscal.CheckAccessKey(keyOne), fiscal.CheckAccessKey(synthKey("532601"+issuerOne+"65001999999999199999999")))
	return map[string]fiscal.Result{
		"key-ok": fiscal.CheckAccessKey(keyOne), "key-invalid": fiscal.CheckAccessKey(keyOne[:43] + "9"),
		"key-unsupported": fiscal.CheckAccessKey(synthKey("432611" + issuerOne + "57001000000001140000004")),
		"qr-ok":           fiscal.CheckQRCode(qrOne), "qr-invalid": fiscal.CheckQRCode(qrOne + "#x"),
		"qr-unsupported": fiscal.CheckQRCode("https://nfce.example.com/?p=" + keyOne + "|3|1"),
		"xml-ok":         fiscal.CheckXML([]byte(minimalXML(keyOne, "100"))), "xml-cancelled-status": fiscal.CheckXML([]byte(minimalXML(keyOne, "101"))),
		"xml-invalid": fiscal.CheckXML([]byte("<!DOCTYPE a><a/>")), "xml-unsupported": fiscal.CheckXML([]byte("<a/>")),
		"compare-ok": ok, "compare-invalid": bad,
	}
}

func TestSamplesHaveTheExpectedOutcomes(t *testing.T) {
	want := map[string]fiscal.Outcome{"key-ok": fiscal.StructureOK, "key-invalid": fiscal.StructureInvalid,
		"key-unsupported": fiscal.Unsupported, "qr-ok": fiscal.StructureOK, "qr-invalid": fiscal.StructureInvalid,
		"qr-unsupported": fiscal.Unsupported, "xml-ok": fiscal.StructureOK, "xml-cancelled-status": fiscal.StructureOK,
		"xml-invalid": fiscal.StructureInvalid, "xml-unsupported": fiscal.Unsupported, "compare-ok": fiscal.StructureOK,
		"compare-invalid": fiscal.StructureInvalid}
	for name, r := range samples() {
		if r.Outcome() != want[name] {
			t.Fatal("sample outcome", name, r.Outcome(), r.Reason())
		}
	}
}

var forbiddenWords = []string{"authoriz", "cancel", "consult", "fiscal", "valid", "protocol", "signature", "cstat", "status"}

func surface(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	names := []string{typ.Name()}
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	for i := 0; i < typ.NumField(); i++ {
		names = append(names, typ.Field(i).Name)
	}
	return names
}

func TestResultHasNoAuthorizationSurface(t *testing.T) {
	for _, value := range []any{fiscal.Result{}, fiscal.AccessKey{}, fiscal.QRReference{}, fiscal.XMLSummary{}, fiscal.Amount{}} {
		for _, name := range surface(t, reflect.TypeOf(value)) {
			lower := strings.ToLower(name)
			for _, word := range forbiddenWords {
				if strings.Contains(lower, word) {
					t.Fatalf("%T exposes %q", value, name)
				}
			}
		}
	}
}

func TestStructureTransitionFeedsOnlyStructure(t *testing.T) {
	for name, r := range samples() {
		command, ok := r.StructureTransition()
		if ok != (r.Outcome() != fiscal.Unsupported) {
			t.Fatal("UNSUPPORTED must command nothing and the rest must command structure", name)
		}
		if !ok {
			if command != (doc.Transition{}) {
				t.Fatal("absent command is not empty", name)
			}
			continue
		}
		wantTo := doc.StructureInvalid
		if r.Outcome() == fiscal.StructureOK {
			wantTo = doc.StructureOK
		}
		if command.Dimension != doc.Structure || command.From != doc.Unchecked || command.To != wantTo || command.ConsultedAt != nil {
			t.Fatal("result commands more than the structure dimension", name, command)
		}
	}
	if _, ok := (fiscal.Result{}).StructureTransition(); ok {
		t.Fatal("an empty result commands the structure dimension")
	}
}

func newDocument(t *testing.T) doc.Document {
	t.Helper()
	id, err := kernel.ParseOrgID("9d000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := doc.New(scope, "9d000000-0000-4000-8000-000000000201", strings.Repeat("1a", 32))
	if err != nil {
		t.Fatal(err)
	}
	d, err = d.Apply(doc.Transition{Dimension: doc.Structure, From: doc.NotApplicable, To: doc.Unchecked})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAuthorizationProtocolNeverReachesFiscalConsultation(t *testing.T) {
	for _, name := range []string{"xml-ok", "xml-cancelled-status", "qr-ok", "key-ok"} {
		r := samples()[name]
		command, ok := r.StructureTransition()
		if !ok {
			t.Fatal("expected a structure command", name)
		}
		d, err := newDocument(t).Apply(command)
		if err != nil {
			t.Fatal(err)
		}
		if s := d.Statuses(); s.Structure != doc.StructureOK || s.Fiscal != doc.NotConsulted || d.ConsultedAt() != nil {
			t.Fatal("a structure result changed fiscal consultation", name, s)
		}
		// Even re-aimed by a caller, the command carries no consultation instant,
		// so the document domain refuses AUTHORIZED_AS_OF and CANCELLED_AS_OF.
		for _, to := range []doc.State{doc.AuthorizedAsOf, doc.CancelledAsOf} {
			misused := command
			misused.Dimension, misused.From, misused.To = doc.Fiscal, doc.NotConsulted, to
			if _, err := d.Apply(misused); err == nil {
				t.Fatal("a structure result drove fiscal consultation", name, to)
			}
		}
	}
}

func TestValuesAreRedacted(t *testing.T) {
	r := fiscal.CheckQRCode("http://www.fazenda.pr.gov.br/nfce/qrcode?p=" + keyOne + "|2|1|14|57.30|AB|1|" + strings.Repeat("A", 40))
	x := fiscal.CheckXML([]byte(minimalXML(keyOne, "100")))
	key, _ := r.AccessKey()
	qr, _ := r.QRCode()
	total, _ := qr.Total()
	summary, _ := x.XML()
	for _, value := range []any{r, x, key, qr, total, summary, summary.Total()} {
		var logs bytes.Buffer
		untimed := &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		}}
		slog.New(slog.NewJSONHandler(&logs, untimed)).Info("fiscal", "value", value)
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v %#v %s %q", value, value, value, value), string(encoded), logs.String()} {
			for _, secret := range []string{keyOne, issuerOne, "57.30", "5730", "2611"} {
				if strings.Contains(out, secret) {
					t.Fatalf("%T leaks %q in %q", value, secret, out)
				}
			}
		}
	}
}

// The published digit maps remainders 0 and 1 to 0, so only a key whose digit
// is 0 can collide with a single-character substitution; every other
// substitution (and any change of the digit itself) is refused.
func TestCheckDigitRefusesSubstitutionsExceptDigitZeroCollisions(t *testing.T) {
	bodies := []string{"432611" + issuerOne + "65001000000123110000001", "412611" + issuerOne + "65002000004567920000002",
		"350001" + "00000000000000" + "55999999999999999999999"}
	collisions := 0
	for _, body := range bodies {
		valid := synthKey(body)
		if r := fiscal.CheckAccessKey(valid); r.Outcome() != fiscal.StructureOK {
			t.Fatal("synthetic key refused", r.Reason())
		}
		for i := 0; i < len(valid); i++ {
			if valid[i] < '0' || valid[i] > '9' {
				continue
			}
			for d := byte('0'); d <= '9'; d++ {
				if d == valid[i] {
					continue
				}
				mutated := valid[:i] + string(d) + valid[i+1:]
				r := fiscal.CheckAccessKey(mutated)
				if i < 43 && synthKey(mutated[:43]) == mutated {
					if valid[43] != '0' {
						t.Fatal("collision with a nonzero digit", i, string(d))
					}
					collisions++
					if r.Reason() == fiscal.ReasonKeyCheckDigit {
						t.Fatal("independent digit accepts what the checker refuses", i, string(d))
					}
					continue
				}
				if r.Reason() != fiscal.ReasonKeyCheckDigit {
					t.Fatal("single substitution not refused by the check digit", i, string(d), r.Reason())
				}
			}
		}
	}
	if collisions == 0 {
		t.Fatal("expected the documented digit-zero collisions of the published algorithm")
	}
}

func TestUnsafeXMLIsRefusedWithoutExpansion(t *testing.T) {
	lol := `<!DOCTYPE l [<!ENTITY a "aaaaaaaaaa">`
	prev := "a"
	for i := 1; i <= 12; i++ {
		name := fmt.Sprintf("a%d", i)
		lol += `<!ENTITY ` + name + ` "` + strings.Repeat("&"+prev+";", 10) + `">`
		prev = name
	}
	lol += `]><l>&` + prev + `;</l>`
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	r := fiscal.CheckXML([]byte(lol))
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if r.Reason() != fiscal.ReasonXMLDoctype {
		t.Fatal("entity declarations not refused", r.Reason())
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 8<<20 || elapsed > 5*time.Second {
		t.Fatal("refusal expanded entities", grown, elapsed)
	}
}

func TestDocumentedBounds(t *testing.T) {
	if fiscal.MaxXMLBytes != 1048576 || fiscal.MaxXMLDepth != 32 || fiscal.MaxXMLElements != 100000 ||
		fiscal.MaxItems != 990 || fiscal.MaxQRBytes != 2953 || fiscal.KeyLength != 44 {
		t.Fatal("bounds differ from B-BR-EMP-233/234 and BR-PRIV-020")
	}
	if r := fiscal.CheckXML(make([]byte, fiscal.MaxXMLBytes+1)); r.Reason() != fiscal.ReasonXMLTooLarge || len(r.Checks()) != 1 {
		t.Fatal("oversized input reached the parser", r.Checks())
	}
}

func TestResultsAreImmutableAndCompareRequiresKeys(t *testing.T) {
	r := fiscal.CheckAccessKey(keyOne)
	checks := r.Checks()
	if len(checks) != 3 {
		t.Fatal("key checks not reported", checks)
	}
	checks[0] = "CHANGED"
	if r.Checks()[0] != fiscal.CheckKeyFormat {
		t.Fatal("checks slice is shared")
	}
	for _, pair := range [][2]fiscal.Result{{{}, r}, {r, {}}, {fiscal.CheckAccessKey(""), r}, {r, fiscal.CheckQRCode("")}} {
		if _, err := fiscal.Compare(pair[0], pair[1]); err != fiscal.ErrNotComparable {
			t.Fatal("non-OK operand compared", err)
		}
	}
	same, err := fiscal.Compare(r, fiscal.CheckXML([]byte(minimalXML(keyOne, "100"))))
	if err != nil || same.Outcome() != fiscal.StructureOK || same.Kind() != fiscal.Comparison {
		t.Fatal("equal keys not matched", err, same.Outcome())
	}
	if _, ok := same.QRCode(); ok {
		t.Fatal("comparison exposes a QR reference")
	}
	again, err := fiscal.Compare(same, r)
	if err != nil || again.Outcome() != fiscal.StructureOK {
		t.Fatal("comparison result is not comparable", err)
	}
	var zeroKey fiscal.AccessKey
	if zeroKey.State() != "" || zeroKey.CheckDigit() != "" {
		t.Fatal("zero key exposes fields")
	}
	if _, ok := (fiscal.Amount{}).Centavos(); ok {
		t.Fatal("zero amount has centavos")
	}
	if _, ok := (fiscal.QRReference{}).Total(); ok {
		t.Fatal("zero QR reference has a total")
	}
}

func TestSyntheticIssuersAreKernelCNPJs(t *testing.T) {
	for _, raw := range []string{"SYNTHE00000140", "SYNTHE00000221", "00000000000000"} {
		c, err := kernel.ParseCNPJ(raw)
		if err != nil || c.Normalized() != raw {
			t.Fatal("fixture issuer is not a kernel CNPJ", raw)
		}
	}
	if synthKey(keyOne[:43]) != keyOne {
		t.Fatal("fixture key check digit differs from the independent helper")
	}
}
