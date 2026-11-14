package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
)

const (
	member1 = "7b0e2a54-0000-4000-8000-000000002101"
	member2 = "7b0e2a54-0000-4000-8000-000000002102"
	report  = "7b0e2a54-0000-4000-8000-000000002103"
)

func reportKind() domain.Kind {
	return domain.Kind{
		Name: "test.report.ready", Permission: "rpt.export.run", Templates: []string{"test.report.ready.v1", "test.report.ready.v2"},
		Params: map[string]domain.Param{
			"report_id": {Type: domain.ParamID},
			"format":    {Type: domain.ParamCode, Codes: []string{"CSV", "PDF"}},
			"rows":      {Type: domain.ParamCount},
			"day":       {Type: domain.ParamDate},
		},
		Link: "/reports/{report_id}",
	}
}

func securityKind() domain.Kind {
	return domain.Kind{Name: "test.access.changed", Mandatory: true, Templates: []string{"test.access.changed.v1"}}
}

func catalog(t *testing.T) domain.Catalog {
	t.Helper()
	c, err := domain.NewCatalog([]domain.Kind{reportKind(), securityKind()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func validRequest() domain.Request {
	return domain.Request{Key: "report:" + report + ":ready", Kind: "test.report.ready", Template: "test.report.ready.v1",
		Params:     map[string]string{"report_id": report, "format": "CSV", "rows": "42", "day": "2026-11-14"},
		Recipients: []string{member2, member1}}
}

func TestCatalogAcceptsClosedKindsAndAllowsEmpty(t *testing.T) {
	c := catalog(t)
	kinds := c.Kinds()
	if len(kinds) != 2 || kinds[0].Name != "test.access.changed" || kinds[1].Name != "test.report.ready" {
		t.Fatal("kinds not sorted/complete", kinds)
	}
	k, ok := c.Lookup("test.report.ready")
	if !ok || k.Permission != "rpt.export.run" || k.Link != "/reports/{report_id}" {
		t.Fatal("lookup failed")
	}
	// Returned kinds are copies: callers cannot mutate the catalog.
	k.Templates[0] = "mutated.template"
	k.Params["format"] = domain.Param{Type: domain.ParamCount}
	again, _ := c.Lookup("test.report.ready")
	if again.Templates[0] != "test.report.ready.v1" || again.Params["format"].Type != domain.ParamCode {
		t.Fatal("catalog mutated through a returned kind")
	}
	if _, ok := c.Lookup("unknown.kind"); ok {
		t.Fatal("unknown kind found")
	}
	empty, err := domain.NewCatalog(nil)
	if err != nil || len(empty.Kinds()) != 0 {
		t.Fatal("empty composition catalog refused", err)
	}
	if _, err := empty.Prepare(validRequest()); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty catalog prepared a notice", err)
	}
}

func TestCatalogRejectsInvalidDefinitions(t *testing.T) {
	mutate := func(f func(*domain.Kind)) []domain.Kind {
		k := reportKind()
		f(&k)
		return []domain.Kind{k}
	}
	for name, kinds := range map[string][]domain.Kind{
		"single segment name": mutate(func(k *domain.Kind) { k.Name = "report" }),
		"uppercase name":      mutate(func(k *domain.Kind) { k.Name = "Test.Report" }),
		"long name":           mutate(func(k *domain.Kind) { k.Name = "a." + strings.Repeat("b", 64) }),
		"unknown permission":  mutate(func(k *domain.Kind) { k.Permission = "rpt.report.invent" }),
		"no templates":        mutate(func(k *domain.Kind) { k.Templates = nil }),
		"bad template":        mutate(func(k *domain.Kind) { k.Templates = []string{"Bad Template"} }),
		"duplicate template":  mutate(func(k *domain.Kind) { k.Templates = []string{"a.b", "a.b"} }),
		"too many templates":  mutate(func(k *domain.Kind) { k.Templates = strings.Split("a.a,a.b,a.c,a.d,a.e,a.f,a.g,a.h,a.i", ",") }),
		"bad param name":      mutate(func(k *domain.Kind) { k.Params["Bad"] = domain.Param{Type: domain.ParamCount} }),
		"unknown param type":  mutate(func(k *domain.Kind) { k.Params["x"] = domain.Param{Type: 9} }),
		"code without values": mutate(func(k *domain.Kind) { k.Params["format"] = domain.Param{Type: domain.ParamCode} }),
		"codes on count":      mutate(func(k *domain.Kind) { k.Params["rows"] = domain.Param{Type: domain.ParamCount, Codes: []string{"A"}} }),
		"lowercase code": mutate(func(k *domain.Kind) {
			k.Params["format"] = domain.Param{Type: domain.ParamCode, Codes: []string{"csv"}}
		}),
		"duplicate code": mutate(func(k *domain.Kind) {
			k.Params["format"] = domain.Param{Type: domain.ParamCode, Codes: []string{"CSV", "CSV"}}
		}),
		"free text code": mutate(func(k *domain.Kind) {
			k.Params["format"] = domain.Param{Type: domain.ParamCode, Codes: []string{"Synthetic Name"}}
		}),
		"link without slash":      mutate(func(k *domain.Kind) { k.Link = "reports/{report_id}" }),
		"link with query":         mutate(func(k *domain.Kind) { k.Link = "/reports/{report_id}?token=x" }),
		"link with fragment":      mutate(func(k *domain.Kind) { k.Link = "/reports#x" }),
		"absolute link":           mutate(func(k *domain.Kind) { k.Link = "https://example.invalid/reports" }),
		"link traversal":          mutate(func(k *domain.Kind) { k.Link = "/reports/../admin" }),
		"empty link segment":      mutate(func(k *domain.Kind) { k.Link = "/reports//x" }),
		"link non-id placeholder": mutate(func(k *domain.Kind) { k.Link = "/reports/{format}" }),
		"link unknown param":      mutate(func(k *domain.Kind) { k.Link = "/reports/{other_id}" }),
		"duplicate kinds":         {reportKind(), reportKind()},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewCatalog(kinds); !errors.Is(err, domain.ErrInvalid) {
				t.Fatal("invalid kind accepted", err)
			}
		})
	}
	tooMany := reportKind()
	for i := range domain.MaxParams {
		tooMany.Params[fmt.Sprintf("extra_%d", i)] = domain.Param{Type: domain.ParamCount}
	}
	if _, err := domain.NewCatalog([]domain.Kind{tooMany}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("too many params accepted", err)
	}
	codes := reportKind()
	values := make([]string, domain.MaxCodes+1)
	for i := range values {
		values[i] = fmt.Sprintf("C%d", i)
	}
	codes.Params["format"] = domain.Param{Type: domain.ParamCode, Codes: values}
	if _, err := domain.NewCatalog([]domain.Kind{codes}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("too many codes accepted", err)
	}
}

func TestPrepareRendersReferenceOnlyNotice(t *testing.T) {
	c := catalog(t)
	p, err := c.Prepare(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "test.report.ready" || p.Template != "test.report.ready.v1" || p.Link != "/reports/"+report || p.Key != "report:"+report+":ready" {
		t.Fatal("unexpected prepared notice", p.Kind, p.Template, p.Link)
	}
	if len(p.Recipients) != 2 || p.Recipients[0] != member1 || p.Recipients[1] != member2 {
		t.Fatal("recipients not canonical", p.Recipients)
	}
	if len(p.Hash) != 64 || strings.Trim(p.Hash, "0123456789abcdef") != "" {
		t.Fatal("invalid content hash", p.Hash)
	}
	var params map[string]string
	if err := json.Unmarshal(p.ParamsJSON(), &params); err != nil || len(params) != 4 || params["rows"] != "42" || params["format"] != "CSV" {
		t.Fatal("params JSON drift", string(p.ParamsJSON()), err)
	}
	if string(p.ParamsJSON()) != `{"day":"2026-11-14","format":"CSV","report_id":"`+report+`","rows":"42"}` {
		t.Fatal("params JSON not canonical", string(p.ParamsJSON()))
	}
	// Order of recipients and map iteration never changes the canonical digest.
	other := validRequest()
	other.Recipients = []string{member1, member2}
	q, err := c.Prepare(other)
	if err != nil || q.Hash != p.Hash {
		t.Fatal("digest depends on recipient order", err)
	}
	for _, change := range []func(*domain.Request){
		func(r *domain.Request) { r.Template = "test.report.ready.v2" },
		func(r *domain.Request) { r.Params["rows"] = "43" },
		func(r *domain.Request) { r.Recipients = []string{member1} },
		func(r *domain.Request) { r.Key = "report:other" },
	} {
		r := validRequest()
		change(&r)
		changed, err := c.Prepare(r)
		if err != nil || changed.Hash == p.Hash {
			t.Fatal("content change kept the digest", err)
		}
	}
	// The caller's map and slice stay independent of the prepared value.
	in := validRequest()
	prepared, err := c.Prepare(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Params["rows"] = "1"
	in.Recipients[0] = member1
	if prepared.Params["rows"] != "42" || prepared.Recipients[1] != member2 {
		t.Fatal("prepared notice aliases caller input")
	}
	security, err := c.Prepare(domain.Request{Key: "access:1", Kind: "test.access.changed", Template: "test.access.changed.v1", Recipients: []string{member1}})
	if err != nil || security.Link != "" || string(security.ParamsJSON()) != "{}" {
		t.Fatal("parameterless notice", err)
	}
}

func TestPrepareRefusesPrivateOrUnboundedContent(t *testing.T) {
	c := catalog(t)
	many := make([]string, domain.MaxRecipients+1)
	for i := range many {
		many[i] = fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", 3000+i)
	}
	for name, change := range map[string]func(*domain.Request){
		"unknown kind":              func(r *domain.Request) { r.Kind = "test.unknown.kind" },
		"unknown template":          func(r *domain.Request) { r.Template = "test.report.ready.v9" },
		"attachment parameter":      func(r *domain.Request) { r.Params["attachment"] = report },
		"document bytes parameter":  func(r *domain.Request) { r.Params["document"] = "JVBERi0xLjQK" },
		"coordinates parameter":     func(r *domain.Request) { r.Params["latitude"] = "-23.5" },
		"token parameter":           func(r *domain.Request) { r.Params["token"] = "secret-token" },
		"missing parameter":         func(r *domain.Request) { delete(r.Params, "rows") },
		"free text code":            func(r *domain.Request) { r.Params["format"] = "Synthetic Person Name" },
		"unlisted code":             func(r *domain.Request) { r.Params["format"] = "XLSX" },
		"uppercase uuid":            func(r *domain.Request) { r.Params["report_id"] = strings.ToUpper(report) },
		"nil uuid":                  func(r *domain.Request) { r.Params["report_id"] = "00000000-0000-0000-0000-000000000000" },
		"text in uuid":              func(r *domain.Request) { r.Params["report_id"] = "see attached file" },
		"negative count":            func(r *domain.Request) { r.Params["rows"] = "-1" },
		"leading zero count":        func(r *domain.Request) { r.Params["rows"] = "042" },
		"oversized count":           func(r *domain.Request) { r.Params["rows"] = "1000000001" },
		"fractional count":          func(r *domain.Request) { r.Params["rows"] = "4.2" },
		"invalid date":              func(r *domain.Request) { r.Params["day"] = "2026-02-30" },
		"timestamp as date":         func(r *domain.Request) { r.Params["day"] = "2026-11-14T10:00:00Z" },
		"no recipients":             func(r *domain.Request) { r.Recipients = nil },
		"duplicate recipient":       func(r *domain.Request) { r.Recipients = []string{member1, member1} },
		"invalid recipient":         func(r *domain.Request) { r.Recipients = []string{"member-one"} },
		"too many recipients":       func(r *domain.Request) { r.Recipients = many },
		"empty key":                 func(r *domain.Request) { r.Key = "" },
		"uppercase key":             func(r *domain.Request) { r.Key = "Report:1" },
		"long key":                  func(r *domain.Request) { r.Key = strings.Repeat("k", domain.MaxKeyLength+1) },
		"key with space":            func(r *domain.Request) { r.Key = "report 1" },
		"control character in code": func(r *domain.Request) { r.Params["format"] = "CSV\n" },
	} {
		t.Run(name, func(t *testing.T) {
			r := validRequest()
			change(&r)
			if _, err := c.Prepare(r); !errors.Is(err, domain.ErrInvalid) {
				t.Fatal("unsafe notice accepted", err)
			}
		})
	}
	if _, err := c.PrepareAll(nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty request batch accepted")
	}
	batch := make([]domain.Request, domain.MaxRequestsPerEvent+1)
	for i := range batch {
		batch[i] = validRequest()
		batch[i].Key = fmt.Sprintf("report:%d", i)
	}
	if _, err := c.PrepareAll(batch); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unbounded request batch accepted")
	}
	if _, err := c.PrepareAll([]domain.Request{validRequest(), validRequest()}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("duplicate idempotency key in one event accepted")
	}
	ok, err := c.PrepareAll(batch[:domain.MaxRequestsPerEvent])
	if err != nil || len(ok) != domain.MaxRequestsPerEvent || ok[3].Key != "report:3" {
		t.Fatal("bounded batch refused", err)
	}
	invalid := append([]domain.Request{validRequest()}, domain.Request{Key: "x"})
	if _, err := c.PrepareAll(invalid); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("batch with invalid request accepted")
	}
}

func TestScreenPreferenceAndMandatoryOrder(t *testing.T) {
	for _, tc := range []struct {
		kind            domain.Kind
		eligible, muted bool
		want            domain.Reason
	}{
		{reportKind(), false, false, domain.Ineligible},
		{reportKind(), false, true, domain.Ineligible},
		{securityKind(), false, true, domain.Ineligible},
		{reportKind(), true, true, domain.Muted},
		{securityKind(), true, true, ""},
		{reportKind(), true, false, ""},
	} {
		if got := domain.Screen(tc.kind, tc.eligible, tc.muted); got != tc.want {
			t.Fatal("screen", tc.kind.Name, tc.eligible, tc.muted, got)
		}
	}
	c := catalog(t)
	if err := c.Mutable("test.report.ready"); err != nil {
		t.Fatal("optional kind not mutable", err)
	}
	for _, kind := range []string{"test.access.changed", "test.unknown.kind", ""} {
		if err := c.Mutable(kind); !errors.Is(err, domain.ErrValidation) {
			t.Fatal("mandatory or unknown kind mutable", kind, err)
		}
	}
}

func TestOnlyInAppChannelIsActive(t *testing.T) {
	channels := domain.Channels()
	if len(channels) != 4 {
		t.Fatal("channel inventory drift", channels)
	}
	active := 0
	for _, c := range channels {
		if c.Enabled() {
			active++
			if c != domain.ChannelInApp {
				t.Fatal("external channel enabled before its delivery decision", c)
			}
		}
	}
	if active != 1 || domain.ChannelEmail.Enabled() || domain.ChannelPush.Enabled() || domain.ChannelAndroidLocal.Enabled() || domain.Channel("SMS").Enabled() {
		t.Fatal("channel activation drift")
	}
}

func TestCommandDigestIsFramedAndStable(t *testing.T) {
	a := domain.CommandDigest("notice.read", member1, report)
	if len(a) != 64 || a != domain.CommandDigest("notice.read", member1, report) {
		t.Fatal("digest not stable")
	}
	for _, other := range []string{
		domain.CommandDigest("notice.read", member2, report),
		domain.CommandDigest("preference.set", member1, report),
		domain.CommandDigest("notice.read", member1+report),
		domain.CommandDigest("notice.read", member1, report, ""),
	} {
		if other == a {
			t.Fatal("digest collision across framing")
		}
	}
	if domain.CommandDigest("x", "ab", "c") == domain.CommandDigest("x", "a", "bc") {
		t.Fatal("unframed digest")
	}
}

func TestRequestsRedactPrivateReferences(t *testing.T) {
	c := catalog(t)
	r := validRequest()
	p, err := c.Prepare(r)
	if err != nil {
		t.Fatal(err)
	}
	var logs strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logger.Info("x", "request", r, "prepared", p)
	raw, _ := json.Marshal(struct{ R, P any }{r, p})
	for _, out := range []string{fmt.Sprint(r), fmt.Sprintf("%#v", r), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p), logs.String(), string(raw)} {
		if strings.Contains(out, member1) || strings.Contains(out, report) || strings.Contains(out, "CSV") {
			t.Fatal("private notice reference leaked", out)
		}
	}
}
