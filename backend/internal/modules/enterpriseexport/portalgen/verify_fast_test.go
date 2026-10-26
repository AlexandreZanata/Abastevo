package portalgen

import (
	"bytes"
	"compress/gzip"
	"math/rand"
	"testing"
)

// The fast scanners must agree exactly with the regular expressions they
// replace on the hot verification path; the expressions stay as oracles.
func TestContainsCPFShapedMatchesRegexOracle(t *testing.T) {
	cases := []string{
		"", "12345678909", "a12345678909", "12345678909b", " 12345678909 ", "x 123.456.789-09.",
		"1.123.456.789-09", "123.456.789-091", "1234567890", "123456789012", "é12345678909é",
		"R$ 12345678909,00", "\xff12345678909\xff", "123.456.789-0", "00000000000", "abc 123.456.789-09",
		"12345678909" + "a", "-12345678909-", "04218406000104", "1234567890 12345678909",
	}
	rng := rand.New(rand.NewSource(7))
	alphabet := []byte("0123456789.-aZ é\xff")
	for i := 0; i < 20000; i++ {
		n := rng.Intn(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = alphabet[rng.Intn(len(alphabet))]
		}
		cases = append(cases, string(b))
	}
	for _, c := range cases {
		if got, want := containsCPFShaped([]byte(c)), cpfTextPattern.Match([]byte(c)); got != want {
			t.Fatalf("containsCPFShaped(%q) = %v, regex = %v", c, got, want)
		}
	}
}

func TestCheckHTMLPrivacyPrefilterKeepsVerdicts(t *testing.T) {
	cases := []string{
		`<p>ok</p>`,
		`<script type="module" src="/assets/portal.0123abcd.js" defer>`,
		`<script src="/x.js">`,
		`<SCRIPT src="/x.js">`,
		`<img src=x OnError =alert(1)>`,
		`<img src=x onerror=alert(1)>`,
		`<p>onerror is a word</p>`,
		`<a href=" JavaScript:alert(1)">`,
		`<a href="javascript:alert(1)">`,
		`<p>javascript: text</p>`,
	}
	for _, c := range cases {
		got := checkHTMLPrivacy([]byte(c)) == nil
		want := checkHTMLPrivacyRegex([]byte(c)) == nil
		if got != want {
			t.Fatalf("checkHTMLPrivacy(%q) ok=%v, regex ok=%v", c, got, want)
		}
	}
}

func checkHTMLPrivacyRegex(body []byte) error {
	for _, tag := range rawScriptTag.FindAll(body, -1) {
		if !allowedScript.Match(tag) {
			return errUnsafeForTest
		}
	}
	if unsafeAttribute.Match(body) || unsafeURL.Match(body) {
		return errUnsafeForTest
	}
	return nil
}

var errUnsafeForTest = bytes.ErrTooLarge

func TestGzipSizerMatchesFreshWriter(t *testing.T) {
	sizer := newGzipSizer()
	for _, body := range [][]byte{nil, []byte("a"), bytes.Repeat([]byte("<p>posto</p>"), 5000), []byte("\x00\xff\x10")} {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(body)
		_ = w.Close()
		got, err := sizer.size(body)
		if err != nil || got != int64(buf.Len()) {
			t.Fatalf("gzip size = %d, %v; want %d", got, err, buf.Len())
		}
	}
}
