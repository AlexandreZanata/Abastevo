package domain_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// B-BR-EMP-228/229: the lifecycle is closed; a lease release is the only way
// back to QUEUED and nothing leaves a terminal state.
func TestLifecycleIsClosedAndTerminalStatesStay(t *testing.T) {
	all := []domain.State{domain.Queued, domain.Running, domain.Ready, domain.Expired, domain.Failed}
	allowed := map[[2]domain.State]bool{
		{domain.Queued, domain.Running}: true, {domain.Running, domain.Queued}: true,
		{domain.Running, domain.Ready}: true, {domain.Running, domain.Failed}: true,
		{domain.Queued, domain.Failed}: true, {domain.Ready, domain.Expired}: true,
	}
	for _, from := range all {
		if !from.Valid() {
			t.Fatal("documented state refused", from)
		}
		for _, to := range all {
			if domain.CanTransition(from, to) != allowed[[2]domain.State{from, to}] {
				t.Fatalf("transition %s→%s drifted", from, to)
			}
		}
	}
	for _, state := range []domain.State{"", "queued", "CANCELLED", "DONE"} {
		if state.Valid() || domain.CanTransition(state, domain.Running) || domain.CanTransition(domain.Queued, state) {
			t.Fatal("unknown state admitted", state)
		}
	}
	for state, want := range map[domain.State][2]bool{domain.Queued: {false, true}, domain.Running: {false, true}, domain.Ready: {false, false}, domain.Expired: {true, false}, domain.Failed: {true, false}} {
		if state.Terminal() != want[0] || state.Open() != want[1] {
			t.Fatal("terminal/open classification drifted", state)
		}
	}
	for _, code := range []domain.FailureCode{domain.TooLarge, domain.RightsLost, domain.FormatUnavailable, domain.AttemptsExhausted} {
		if !code.Valid() {
			t.Fatal("documented failure code refused", code)
		}
	}
	for _, code := range []domain.FailureCode{"", "EXPORT_CANCELLED", "export_too_large", "FAILED"} {
		if code.Valid() {
			t.Fatal("undocumented failure code admitted", code)
		}
	}
}

func TestCeilingsAndDefaultsAreTheRecordedInfrastructureValues(t *testing.T) {
	if domain.MaxRows != 500_000 || domain.MaxBytes != 16<<20 || domain.MaxDuration != 120*time.Second || domain.MaxAttempts != 3 || domain.OpenJobLimit != 3 ||
		domain.DefaultRetention != 7*24*time.Hour || domain.CapabilityTTL != 5*time.Minute || domain.MaxParameterBytes != 4096 || domain.Permission != "rpt.export.run" {
		t.Fatal("infrastructure ceilings drifted from EDR-156 / BR-RPT-009 / BR-PRIV-004")
	}
	if d := domain.DefaultLimits(); d != (domain.Limits{Rows: domain.MaxRows, Bytes: domain.MaxBytes, Duration: domain.MaxDuration}) || d.Validate() != nil {
		t.Fatal("default limits are not the infrastructure ceilings")
	}
	for _, bad := range []domain.Limits{{}, {Rows: 0, Bytes: 1, Duration: time.Second}, {Rows: 1, Bytes: 0, Duration: time.Second}, {Rows: 1, Bytes: 1, Duration: 0},
		{Rows: domain.MaxRows + 1, Bytes: 1, Duration: time.Second}, {Rows: 1, Bytes: domain.MaxBytes + 1, Duration: time.Second},
		{Rows: 1, Bytes: 1, Duration: domain.MaxDuration + time.Millisecond}, {Rows: 1, Bytes: 1, Duration: 999 * time.Millisecond},
		{Rows: 1, Bytes: 1, Duration: time.Second + time.Microsecond}, {Rows: -1, Bytes: 1, Duration: time.Second}} {
		if !errors.Is(bad.Validate(), domain.ErrInvalid) {
			t.Fatal("limit outside the ceilings admitted", bad)
		}
	}
	if (domain.Limits{Rows: 1, Bytes: 1, Duration: time.Second}).Validate() != nil {
		t.Fatal("smallest limits refused")
	}
	for d, want := range map[time.Duration]bool{24 * time.Hour: true, 7 * 24 * time.Hour: true, 30 * 24 * time.Hour: true, 24*time.Hour - time.Second: false, 30*24*time.Hour + time.Second: false, 24*time.Hour + time.Millisecond: false, 0: false} {
		if domain.ValidRetention(d) != want {
			t.Fatal("retention range drifted", d)
		}
	}
}

func TestKindAndMediaTypeVocabulary(t *testing.T) {
	for _, kind := range []string{"test.synthetic_rows", "fuel.fueling_list", "a.b"} {
		if !domain.ValidKind(kind) {
			t.Fatal("valid kind refused", kind)
		}
	}
	for _, kind := range []string{"", "synthetic", "Fuel.list", "fuel..list", "fuel.list.", ".fuel", "fuel list", "fuel/list", "fuel.lísta", strings.Repeat("a", 33) + "." + strings.Repeat("b", 31)} {
		if domain.ValidKind(kind) {
			t.Fatal("invalid kind admitted", kind)
		}
	}
	for _, media := range []string{"text/csv", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "application/zip", "text/plain"} {
		if !domain.ValidMediaType(media) {
			t.Fatal("valid media type refused", media)
		}
	}
	for _, media := range []string{"", "text", "text/csv; charset=utf-8", "Text/CSV", "text/", "/csv", "text/csv\n", strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64)} {
		if domain.ValidMediaType(media) {
			t.Fatal("invalid media type admitted", media)
		}
	}
}

// B-BR-EMP-227: parameters are a canonical bounded object; the digest binds
// organization and kind and never echoes the values.
func TestCanonicalParametersAndDigest(t *testing.T) {
	canonical, err := domain.CanonicalParameters([]byte(` { "unit_id" : "x", "after_day":"2026-10-01", "nested": {"b": [1, 2.50, true], "a": "y"} } `))
	if err != nil || string(canonical) != `{"after_day":"2026-10-01","nested":{"a":"y","b":[1,2.50,true]},"unit_id":"x"}` {
		t.Fatalf("canonical form drifted: %s %v", canonical, err)
	}
	if empty, err := domain.CanonicalParameters([]byte(`{}`)); err != nil || string(empty) != `{}` {
		t.Fatal("empty object refused", err)
	}
	for _, bad := range []string{``, `null`, `[]`, `"x"`, `1`, `{"a":null}`, `{"a":1,"a":2}`, `{"A":1}`, `{"a-b":1}`, `{"1a":1}`, `{"a":1} {}`, `{"a":1}x`, `{"a":{"b":null}}`, `{"a":[null]}`, `{"a":"\ud800"}`, "{\"a\":\"\xff\"}", `{"a":1`,
		`{"a":"` + strings.Repeat("x", domain.MaxParameterBytes) + `"}`} {
		if _, err := domain.CanonicalParameters([]byte(bad)); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid parameters admitted: %q", bad)
		}
	}
	a, b := scope(t, 1), scope(t, 2)
	digest, err := domain.ParametersDigest(a, "test.synthetic_rows", canonical)
	if err != nil || len(digest) != 64 || strings.Contains(digest, "unit") {
		t.Fatal("digest", err)
	}
	again, _ := domain.ParametersDigest(a, "test.synthetic_rows", canonical)
	other, _ := domain.ParametersDigest(b, "test.synthetic_rows", canonical)
	kind, _ := domain.ParametersDigest(a, "test.other_rows", canonical)
	changed, _ := domain.ParametersDigest(a, "test.synthetic_rows", []byte(`{"unit_id":"y"}`))
	if again != digest || other == digest || kind == digest || changed == digest {
		t.Fatal("digest is not a deterministic binding of organization, kind and parameters")
	}
	for _, call := range []func() (string, error){
		func() (string, error) {
			return domain.ParametersDigest(kernel.OrgScope{}, "test.synthetic_rows", canonical)
		},
		func() (string, error) { return domain.ParametersDigest(a, "invalid", canonical) },
		func() (string, error) {
			return domain.ParametersDigest(a, "test.synthetic_rows", []byte(`{"b":1,"a":2}`))
		},
	} {
		if value, err := call(); !errors.Is(err, domain.ErrInvalid) || value != "" {
			t.Fatal("digest of a non-canonical input")
		}
	}
}

// B-BR-EMP-228: a ceiling is a loud failure and the sink never yields a
// truncated file afterwards.
func TestSinkRefusesBeyondCeilingsWithoutTruncation(t *testing.T) {
	if _, err := domain.NewSink(domain.Limits{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("sink with invalid limits")
	}
	sink, err := domain.NewSink(domain.Limits{Rows: 2, Bytes: 64, Duration: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Result(); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty file accepted")
	}
	if n, err := sink.Write([]byte("id,value\n")); err != nil || n != 9 {
		t.Fatal("header", err)
	}
	for _, row := range []string{"1,a\n", "2,b\n"} {
		if err := sink.Row([]byte(row)); err != nil {
			t.Fatal("row within ceiling", err)
		}
	}
	file, err := sink.Result()
	sum := sha256.Sum256([]byte("id,value\n1,a\n2,b\n"))
	if err != nil || string(file.Bytes) != "id,value\n1,a\n2,b\n" || file.Rows != 2 || file.SHA256 != hex.EncodeToString(sum[:]) || sink.Rows() != 2 || sink.Bytes() != 17 {
		t.Fatal("complete file lost a fact", err)
	}
	file.Bytes[0] = 'X'
	if again, _ := sink.Result(); again.Bytes[0] != 'i' {
		t.Fatal("result shares the sink buffer")
	}
	if err := sink.Row([]byte("3,c\n")); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("row ceiling not enforced", err)
	}
	if err := sink.Row(nil); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("poisoned sink accepted a later row")
	}
	if _, err := sink.Write([]byte("x")); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("poisoned sink accepted later bytes")
	}
	if file, err := sink.Result(); !errors.Is(err, domain.ErrTooLarge) || file.Bytes != nil || file.Rows != 0 || file.SHA256 != "" {
		t.Fatal("a truncated file escaped the row ceiling")
	}
	bytesSink, _ := domain.NewSink(domain.Limits{Rows: 10, Bytes: 8, Duration: time.Second})
	if err := bytesSink.Row([]byte("12345678")); err != nil {
		t.Fatal("exact byte ceiling refused", err)
	}
	if _, err := bytesSink.Write([]byte("9")); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("byte ceiling not enforced")
	}
	if _, err := bytesSink.Result(); !errors.Is(err, domain.ErrTooLarge) {
		t.Fatal("a truncated file escaped the byte ceiling")
	}
	if bytesSink.Bytes() != 8 {
		t.Fatal("refused bytes were counted")
	}
	if fmt.Sprint(file) != "orgexport: file redacted" || fmt.Sprintf("%#v", file) != "orgexport: file redacted" || !strings.Contains(file.LogValue().String(), "redacted") {
		t.Fatal("file content can be formatted")
	}
}

// B-BR-EMP-229: the key is derived from validated server facts under the
// organization export prefix and retrieved bytes must match exactly.
func TestObjectKeyAndExactCheck(t *testing.T) {
	a := scope(t, 1)
	raw := []byte("id\n1\n")
	sink, _ := domain.NewSink(domain.DefaultLimits())
	_, _ = sink.Write([]byte("id\n"))
	_ = sink.Row([]byte("1\n"))
	file, _ := sink.Result()
	object, err := domain.ObjectFor(a, id(50), "text/csv", file)
	if err != nil || object.OrgID != id(1) || object.ExportID != id(50) || object.Bytes != int64(len(raw)) || object.Rows != 1 || object.MediaType != "text/csv" || object.Validate(a) != nil {
		t.Fatal("object identity", err)
	}
	key, err := object.Key(a)
	prefix, perr := object.Prefix(a)
	if err != nil || perr != nil || key != "org/"+id(1)+"/export/"+id(50)+"/"+file.SHA256 || prefix != "org/"+id(1)+"/export/"+id(50)+"/" {
		t.Fatal("object key layout drifted", key, err)
	}
	if object.Check(raw) != nil {
		t.Fatal("exact bytes refused")
	}
	for _, bad := range [][]byte{nil, raw[:3], append(bytes.Clone(raw), 'x'), []byte("id\n2\n")} {
		if !errors.Is(object.Check(bad), domain.ErrIntegrity) {
			t.Fatal("partial, longer or substituted bytes accepted", string(bad))
		}
	}
	b := scope(t, 2)
	if _, err := object.Key(b); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("key derived in a foreign organization")
	}
	for _, change := range []func(*domain.Object){
		func(o *domain.Object) { o.ExportID = "../x" }, func(o *domain.Object) { o.SHA256 = "A" + o.SHA256[1:] },
		func(o *domain.Object) { o.MediaType = "text/csv; x=y" }, func(o *domain.Object) { o.Bytes = 0 },
		func(o *domain.Object) { o.Bytes = domain.MaxBytes + 1 }, func(o *domain.Object) { o.Rows = -1 },
		func(o *domain.Object) { o.Rows = domain.MaxRows + 1 }, func(o *domain.Object) { o.OrgID = id(2) },
	} {
		changed := object
		change(&changed)
		if !errors.Is(changed.Validate(a), domain.ErrInvalid) {
			t.Fatal("invalid object admitted", changed)
		}
		if _, err := changed.Key(a); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("key of invalid object")
		}
		if _, err := changed.Prefix(a); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("prefix of invalid object")
		}
	}
	if _, err := domain.ObjectFor(a, "bad", "text/csv", file); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("object for invalid export")
	}
	if _, err := domain.ObjectFor(a, id(50), "text/csv", domain.File{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("object for empty file")
	}
	tampered := file
	tampered.SHA256 = strings.Repeat("0", 64)
	if _, err := domain.ObjectFor(a, id(50), "text/csv", tampered); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("object whose digest does not match its bytes")
	}
	if !domain.ValidID(id(1)) || domain.ValidID("00000000-0000-0000-0000-000000000000") || domain.ValidID(strings.ToUpper(id(1))) {
		t.Fatal("identifier syntax drifted")
	}
}

// Escaped text survives canonicalization exactly; lossy or malformed escapes
// are refused rather than replaced.
func TestCanonicalParametersKeepEscapesLossless(t *testing.T) {
	for raw, want := range map[string]string{
		`{"a":"😀"}`:                 "{\"a\":\"\U0001F600\"}",
		`{"a":"line\nnext é"}`:      `{"a":"line\nnext é"}`,
		`{"a":"quote \" slash \\"}`: `{"a":"quote \" slash \\"}`,
		`{"b":{"c":[1,{"d":"x"}]}}`: `{"b":{"c":[1,{"d":"x"}]}}`,
	} {
		got, err := domain.CanonicalParameters([]byte(raw))
		if err != nil || string(got) != want {
			t.Fatalf("lossless escape %s: %s %v", raw, got, err)
		}
	}
	for _, raw := range []string{`{"a":"\ud83d"}`, `{"a":"\ud83dA"}`, `{"a":"\udc00"}`, `{"a":"\uZZZZ"}`, `{"a":"\u12"}`, `{"a":"x\`, `{"a":"unterminated}`,
		`{"a":` + strings.Repeat(`[`, 20) + `1` + strings.Repeat(`]`, 20) + `}`, `{"` + strings.Repeat("k", 65) + `":1}`} {
		if _, err := domain.CanonicalParameters([]byte(raw)); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("malformed parameters admitted: %s", raw)
		}
	}
}
