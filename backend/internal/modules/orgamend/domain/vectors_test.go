package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

const (
	corpusPath  = "../../../../../contracts/testdata/enterprise/amendment/amendment-v1.psv"
	corpusHash  = "da7d99d9355d92a0c0869a1d97ceb1d55ab9b633e42d044eb348b89a08e81a2e"
	corpusTotal = 193
)

var (
	corpusCounts = map[string]int{"DECIDE": 165, "RESOLVE": 28}
	periodKeys   = "record state effective latest day period reopen reopen_reason_hex"
	allowedArgs  = map[string]string{
		"DECIDE": periodKeys + " intent expected revision base actor at reason_hex edit amend void" +
			" posting_day posting_period posting_reopen posting_reopen_reason_hex",
		"RESOLVE": periodKeys + " decided result at",
	}
	errorCodes = []struct {
		code string
		err  error
	}{
		{"INVALID", domain.ErrInvalid}, {"CONFLICT", domain.ErrConflict},
		{"APPROVED_IMMUTABLE", domain.ErrApprovedImmutable}, {"NOT_EDITABLE", domain.ErrNotEditable},
		{"NOT_APPROVED", domain.ErrNotApproved}, {"AMENDMENT_PENDING", domain.ErrAmendmentPending},
		{"ALREADY_VOIDED", domain.ErrAlreadyVoided}, {"LOST_ORIGINAL", domain.ErrLostOriginal},
		{"REASON_REQUIRED", domain.ErrReasonRequired}, {"REASON_INVALID", domain.ErrReasonInvalid},
		{"UNAUTHORIZED", domain.ErrUnauthorized}, {"PERIOD_LOCKED", domain.ErrPeriodLocked},
		{"PERIOD_NOT_LOCKED", domain.ErrPeriodNotLocked}, {"REOPEN_UNAUTHORIZED", domain.ErrReopenUnauthorized},
	}
	errMalformed = errors.New("fixture: malformed")
)

type fields map[string]string

type vectorCase struct {
	id, op     string
	args, want fields
}

func uuid(n int) string { return fmt.Sprintf("9e000000-0000-4000-8000-%012d", n) }

func fixtureID(alias string) string {
	switch alias {
	case "F":
		return uuid(1)
	case "U":
		return uuid(31)
	case "?":
		return ""
	}
	return alias
}

func alias(id string) string {
	for _, a := range []string{"F", "U"} {
		if fixtureID(a) == id {
			return a
		}
	}
	return id
}

func parseFields(raw string) (fields, error) {
	out := fields{}
	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, errMalformed
		}
		if _, dup := out[key]; dup {
			return nil, errMalformed
		}
		out[key] = value
	}
	return out, nil
}

func parseNumber(raw string) (int64, error) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || strconv.FormatInt(v, 10) != raw {
		return 0, errMalformed
	}
	return v, nil
}

func parseBool(raw string) (bool, error) {
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, errMalformed
}

// parseHex decodes raw bytes; invalid UTF-8 is deliberately representable.
func parseHex(raw string) (string, error) {
	if raw == "?" {
		return "", nil
	}
	v, err := hex.DecodeString(raw)
	if err != nil || raw != strings.ToLower(raw) {
		return "", errMalformed
	}
	return string(v), nil
}

// parseAt validates the shared instant profile and keeps the written offset.
func parseAt(raw string) (time.Time, error) {
	if raw == "?" {
		return time.Time{}, nil
	}
	if _, err := orgtime.ParseInstant(raw); err != nil {
		return time.Time{}, errMalformed
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, errMalformed
	}
	return at, nil
}

func absentText(raw string) string {
	if raw == "?" {
		return ""
	}
	return raw
}

func parseCorpus(raw []byte) ([]vectorCase, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		return nil, errMalformed
	}
	ids := map[string]bool{}
	var cases []vectorCase
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || p[0] == "" || strings.TrimSpace(p[0]) != p[0] || ids[p[0]] {
			return nil, errMalformed
		}
		allowed, known := allowedArgs[p[1]]
		if !known {
			return nil, errMalformed
		}
		args, err := parseFields(p[2])
		if err != nil {
			return nil, err
		}
		want, err := parseFields(p[3])
		if err != nil {
			return nil, err
		}
		for key := range args {
			if !strings.Contains(" "+allowed+" ", " "+key+" ") {
				return nil, errMalformed
			}
		}
		ids[p[0]] = true
		cases = append(cases, vectorCase{id: p[0], op: p[1], args: args, want: want})
	}
	if scanner.Err() != nil {
		return nil, errMalformed
	}
	return cases, nil
}

func merged(defaults, args fields) fields {
	out := fields{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range args {
		out[k] = v
	}
	return out
}

func vectorPeriod(f fields, stateKey, reopenKey, reasonKey string) (domain.Period, error) {
	p := domain.Period{State: domain.PeriodState(f[stateKey])}
	reopen, has := f[reopenKey]
	if !has || reopen == "?" {
		if _, reason := f[reasonKey]; reason {
			return p, errMalformed
		}
		return p, nil
	}
	proven, err := parseBool(reopen)
	if err != nil {
		return p, err
	}
	reason := hex.EncodeToString([]byte("Late receipt correction"))
	if raw, ok := f[reasonKey]; ok {
		reason = raw
	}
	text, err := parseHex(reason)
	if err != nil {
		return p, err
	}
	p.Reopening = &domain.Reopening{AuthorityProven: proven, Reason: text}
	return p, nil
}

func vectorFacts(f fields) (domain.Facts, error) {
	effective, err := parseNumber(f["effective"])
	if err != nil {
		return domain.Facts{}, err
	}
	latest, err := parseNumber(f["latest"])
	if err != nil {
		return domain.Facts{}, err
	}
	period, err := vectorPeriod(f, "period", "reopen", "reopen_reason_hex")
	if err != nil {
		return domain.Facts{}, err
	}
	return domain.Facts{
		RecordID: fixtureID(f["record"]), State: domain.ReviewState(f["state"]),
		Effective: effective, Latest: latest, ReportingDay: absentText(f["day"]), Period: period,
	}, nil
}

func numberOr(f fields, key string, fallback int64) (int64, error) {
	if raw, ok := f[key]; ok {
		return parseNumber(raw)
	}
	return fallback, nil
}

func vectorDecide(args fields) (domain.Facts, domain.Request, error) {
	f := merged(fields{
		"record": "F", "state": "APPROVED", "effective": "1", "latest": "1", "day": "2026-10-31",
		"period": "OPEN", "intent": "AMEND", "actor": "U", "at": "2026-11-14T12:00:00Z",
		"edit": "true", "amend": "true", "void": "true",
		"reason_hex": hex.EncodeToString([]byte("Pump quantity corrected")),
	}, args)
	intent := domain.Intent(f["intent"])
	if intent == domain.IntentAdjust {
		f = merged(fields{"posting_day": "2026-11-14", "posting_period": "OPEN"}, f)
	}
	facts, err := vectorFacts(f)
	if err != nil {
		return facts, domain.Request{}, err
	}
	r := domain.Request{Intent: intent, ActorID: fixtureID(f["actor"]), PostingDay: absentText(f["posting_day"])}
	nextDefault, baseDefault := int64(0), facts.Effective
	if intent == domain.IntentEdit || intent == domain.IntentAmend {
		if facts.Latest == math.MaxInt64 {
			if _, explicit := f["revision"]; !explicit {
				return facts, r, errMalformed
			}
		} else {
			nextDefault = facts.Latest + 1
		}
	}
	if intent == domain.IntentEdit {
		baseDefault = facts.Latest
	}
	if r.Expected, err = numberOr(f, "expected", facts.Latest); err != nil {
		return facts, r, err
	}
	if r.Revision, err = numberOr(f, "revision", nextDefault); err != nil {
		return facts, r, err
	}
	if r.Base, err = numberOr(f, "base", baseDefault); err != nil {
		return facts, r, err
	}
	if r.At, err = parseAt(f["at"]); err != nil {
		return facts, r, err
	}
	if r.Reason, err = parseHex(f["reason_hex"]); err != nil {
		return facts, r, err
	}
	for key, target := range map[string]*bool{"edit": &r.Authority.Edit, "amend": &r.Authority.Amend, "void": &r.Authority.Void} {
		if *target, err = parseBool(f[key]); err != nil {
			return facts, r, err
		}
	}
	if raw, ok := f["posting_period"]; ok && raw != "?" {
		posting, err := vectorPeriod(f, "posting_period", "posting_reopen", "posting_reopen_reason_hex")
		if err != nil {
			return facts, r, err
		}
		r.PostingPeriod = &posting
	} else if _, reopen := f["posting_reopen"]; reopen {
		return facts, r, errMalformed
	}
	return facts, r, nil
}

func vectorResolve(args fields) (domain.Facts, domain.Review, error) {
	f := merged(fields{
		"record": "F", "state": "IN_REVIEW", "effective": "1", "latest": "2", "day": "2026-10-31",
		"period": "OPEN", "result": "APPROVED", "at": "2026-11-15T09:00:00Z",
	}, args)
	facts, err := vectorFacts(f)
	if err != nil {
		return facts, domain.Review{}, err
	}
	v := domain.Review{Result: domain.ReviewState(f["result"])}
	if v.Revision, err = numberOr(f, "decided", facts.Latest); err != nil {
		return facts, v, err
	}
	v.At, err = parseAt(f["at"])
	return facts, v, err
}

func orAbsent(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func addDerived(out fields, d *domain.Recalculation) fields {
	if d == nil {
		out["derived"] = "?"
		return out
	}
	out["derived"] = string(d.Cause)
	out["derived_record"] = alias(d.RecordID)
	out["derived_revision"] = strconv.FormatInt(d.Revision, 10)
	out["derived_superseded"] = strconv.FormatInt(d.Superseded, 10)
	out["derived_day"] = d.Day
	out["derived_at"] = d.At.Format(time.RFC3339Nano)
	return out
}

func decisionFields(d domain.Decision) fields {
	reason := "?"
	if d.Reason != "" {
		reason = hex.EncodeToString([]byte(d.Reason))
	}
	return addDerived(fields{
		"outcome": string(d.Outcome), "record": alias(d.RecordID),
		"revision": strconv.FormatInt(d.Revision, 10), "base": strconv.FormatInt(d.Base, 10),
		"effective": strconv.FormatInt(d.Effective, 10), "voided": strconv.FormatBool(d.Voided),
		"actor": alias(d.ActorID), "at": d.At.Format(time.RFC3339Nano), "reason_hex": reason,
		"day": d.ReportingDay, "posting_day": orAbsent(d.PostingDay),
	}, d.Derived)
}

func resolutionFields(r domain.Resolution) fields {
	return addDerived(fields{
		"result": string(r.Result), "record": alias(r.RecordID),
		"revision": strconv.FormatInt(r.Revision, 10), "effective": strconv.FormatInt(r.Effective, 10),
		"superseded": strconv.FormatInt(r.Superseded, 10), "pending": strconv.FormatBool(r.Pending),
	}, r.Derived)
}

// errorCode requires exactly one closed refusal; anything else is UNEXPECTED.
func errorCode(err error) string {
	code := "UNEXPECTED"
	matches := 0
	for _, c := range errorCodes {
		if errors.Is(err, c.err) {
			code = c.code
			matches++
		}
	}
	if matches != 1 {
		return "UNEXPECTED"
	}
	return code
}

func runVector(c vectorCase) (fields, error) {
	switch c.op {
	case "DECIDE":
		facts, request, err := vectorDecide(c.args)
		if err != nil {
			return nil, err
		}
		d, err := domain.Decide(facts, request)
		if err != nil {
			return fields{"error": errorCode(err)}, nil
		}
		return decisionFields(d), nil
	case "RESOLVE":
		facts, review, err := vectorResolve(c.args)
		if err != nil {
			return nil, err
		}
		r, err := domain.Resolve(facts, review)
		if err != nil {
			return fields{"error": errorCode(err)}, nil
		}
		return resolutionFields(r), nil
	}
	return nil, errMalformed
}

func loadCorpus(t *testing.T) []vectorCase {
	t.Helper()
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != corpusHash {
		t.Fatal("shared corpus hash changed")
	}
	cases, err := parseCorpus(raw)
	if err != nil {
		t.Fatal("malformed shared corpus")
	}
	counts := map[string]int{}
	for _, c := range cases {
		counts[c.op]++
	}
	if len(cases) != corpusTotal || !reflect.DeepEqual(counts, corpusCounts) {
		t.Fatal("incomplete corpus", len(cases), counts)
	}
	return cases
}

func TestSharedAmendmentVectors(t *testing.T) {
	for _, c := range loadCorpus(t) {
		t.Run(c.id, func(t *testing.T) {
			got, err := runVector(c)
			if err != nil {
				t.Fatal("malformed fixture arguments")
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("shared mismatch:\n got %v\nwant %v", got, c.want)
			}
		})
	}
}

// Every closed vocabulary member must be pinned by at least one expectation.
func TestSharedAmendmentCorpusCoverage(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range loadCorpus(t) {
		for _, key := range []string{"error", "outcome", "derived", "result"} {
			if v, ok := c.want[key]; ok {
				seen[key+"="+v] = true
			}
		}
		for _, key := range []string{"intent", "state", "period", "posting_period"} {
			if v, ok := c.args[key]; ok {
				seen["arg:"+strings.TrimPrefix(key, "posting_")+"="+v] = true
			}
		}
	}
	var required []string
	for _, c := range errorCodes {
		required = append(required, "error="+c.code)
	}
	for _, v := range []string{"EDIT", "AMENDMENT", "VOID", "ADJUSTMENT"} {
		required = append(required, "outcome="+v)
	}
	for _, v := range []string{"AMENDED", "VOIDED", "ADJUSTED", "?"} {
		required = append(required, "derived="+v)
	}
	for _, v := range []string{"APPROVED", "RETURNED", "REJECTED", "CANCELLED"} {
		required = append(required, "result="+v)
	}
	for _, v := range []string{"EDIT", "AMEND", "VOID", "ADJUST"} {
		required = append(required, "arg:intent="+v)
	}
	for _, v := range []string{"DRAFT", "SUBMITTED", "IN_REVIEW", "RETURNED", "APPROVED", "REJECTED", "CANCELLED", "VOIDED"} {
		required = append(required, "arg:state="+v)
	}
	for _, v := range []string{"OPEN", "LOCKED", "REOPENED"} {
		required = append(required, "arg:period="+v)
	}
	for _, key := range required {
		if !seen[key] {
			t.Errorf("corpus does not exercise %s", key)
		}
	}
}

func TestVectorGrammarRejectsMalformedFixtures(t *testing.T) {
	for _, raw := range []string{"", "a", "=1", " a=1", "a=1;a=2", "a=1;"} {
		if _, err := parseFields(raw); !errors.Is(err, errMalformed) {
			t.Errorf("field grammar accepted %q", raw)
		}
	}
	for _, raw := range []string{"", "01", "+1", "-0", " 1", "1.0", "9223372036854775808", "?"} {
		if _, err := parseNumber(raw); !errors.Is(err, errMalformed) {
			t.Errorf("integer grammar accepted %q", raw)
		}
	}
	for _, raw := range []string{"", "TRUE", "1", "?"} {
		if _, err := parseBool(raw); !errors.Is(err, errMalformed) {
			t.Errorf("boolean grammar accepted %q", raw)
		}
	}
	for _, raw := range []string{"4A", "4", "zz"} {
		if _, err := parseHex(raw); !errors.Is(err, errMalformed) {
			t.Errorf("hex grammar accepted %q", raw)
		}
	}
	for _, raw := range []string{"", "2026-11-14", "2026-11-14T12:00:00", "0000-01-01T00:00:00Z"} {
		if _, err := parseAt(raw); !errors.Is(err, errMalformed) {
			t.Errorf("instant grammar accepted %q", raw)
		}
	}
	header := "id|operation|args|expected\n"
	for name, body := range map[string]string{
		"header":        "id|op|args|expected\n",
		"columns":       header + "a|DECIDE|intent=EDIT\n",
		"duplicate id":  header + "a|DECIDE|intent=EDIT|error=INVALID\na|DECIDE|intent=EDIT|error=INVALID\n",
		"unknown op":    header + "a|PLAN|intent=EDIT|error=INVALID\n",
		"unknown arg":   header + "a|DECIDE|replace=true|error=INVALID\n",
		"resolve arg":   header + "a|RESOLVE|intent=EDIT|error=INVALID\n",
		"blank id":      header + "|DECIDE|intent=EDIT|error=INVALID\n",
		"padded id":     header + " a|DECIDE|intent=EDIT|error=INVALID\n",
		"empty expect":  header + "a|DECIDE|intent=EDIT|\n",
		"missing value": header + "a|DECIDE|intent|error=INVALID\n",
	} {
		if _, err := parseCorpus([]byte(body)); !errors.Is(err, errMalformed) {
			t.Errorf("corpus grammar accepted %s", name)
		}
	}
	for name, args := range map[string]fields{
		"reopen bool":            {"period": "REOPENED", "reopen": "yes"},
		"reason without reopen":  {"reopen_reason_hex": "61"},
		"posting reopen alone":   {"posting_reopen": "true"},
		"ambiguous max revision": {"effective": "9223372036854775807", "latest": "9223372036854775807"},
		"bad authority":          {"amend": "1"},
		"bad at":                 {"at": "2026-11-14"},
	} {
		if _, _, err := vectorDecide(args); !errors.Is(err, errMalformed) {
			t.Errorf("decide fixture accepted %s", name)
		}
	}
	if _, _, err := vectorResolve(fields{"decided": "two"}); !errors.Is(err, errMalformed) {
		t.Error("resolve fixture accepted a malformed revision")
	}
	if code := errorCode(errors.New("other")); code != "UNEXPECTED" {
		t.Error("foreign error mapped to a shared code")
	}
	if code := errorCode(errors.Join(domain.ErrInvalid, domain.ErrConflict)); code != "UNEXPECTED" {
		t.Error("ambiguous error mapped to a shared code")
	}
}
