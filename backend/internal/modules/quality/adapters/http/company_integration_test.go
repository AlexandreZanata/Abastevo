//go:build integration

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type companyHTTPFixture struct {
	p              *pgxpool.Pool
	h              Handler
	r              chi.Router
	reader         *qualityread.Reader
	repo           *directory.Repository
	pub            *parent.Publisher
	in             application.NativeFineInput
	a, b, revision string
	next           func() (string, error)
	enabled        *bool
}

func newCompanyHTTPFixture(t *testing.T) *companyHTTPFixture {
	t.Helper()
	_, p := freshRouter(t, true)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	a, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic scoped HTTP A", nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := repo.ResolveCNPJ(ctx, "04218406000104", "Synthetic scoped HTTP B", nil)
	if e != nil {
		t.Fatal(e)
	}
	identity := directory.NewSourceIdentityReader(p)
	probe := func(ctx context.Context, ids []string) ([]domain.IdentityEvidence, error) {
		rows, e := identity.Evidence(ctx, ids)
		out := make([]domain.IdentityEvidence, 0, len(rows))
		for _, r := range rows {
			out = append(out, domain.IdentityEvidence{CNPJ: r.CNPJ, StationID: r.StationID, Current: r.Current, StationActive: r.StationActive})
		}
		return out, e
	}
	guard := func(ctx context.Context, tx pgx.Tx, b []application.CompanyBinding) error {
		out := make([]directoryapp.SourceBinding, 0, len(b))
		for _, v := range b {
			out = append(out, directoryapp.SourceBinding{CNPJ: v.CNPJ, StationID: v.StationID})
		}
		return identity.Guard(ctx, tx, out)
	}
	reader := qualityread.NewReader(p).WithCompanyIdentity(probe, guard)
	pub := parent.NewPublisher(p).WithCompanyGuard(guard)
	in := application.NativeFineInput{Edition: "2025", Clock: application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Counts: domain.Counts{Input: 3, Accepted: 3}, ManifestHash: strings.Repeat("a", 64), Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv", SHA256: strings.Repeat("b", 64), Bytes: 100}}}
	records := []domain.CompanyObservationRecord{}
	for i, cnpj := range []string{"11222333000181", "11222333000181", "04218406000104"} {
		row := application.FineObservation{SourceCNPJ: cnpj, ProceedingRef: fmt.Sprint("SYNTHETIC-CASE-", i), DUFRef: fmt.Sprint("SYNTHETIC/", i), Department: "SYNTHETIC", Status: "Synthetic status", DueOn: "2025-02-28", AmountText: "1,00", AmountMinor: 100, PaidText: "0,00", ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: fmt.Sprint("synthetic:", i+8)}
		row.ObservationKey = row.ObservationIdentity()
		in.Observations = append(in.Observations, row)
		records = append(records, domain.CompanyObservationRecord{RecordKey: row.ObservationKey, CNPJ: cnpj})
	}
	decisions, e := application.PlanCompanyAssociations(ctx, records, "2026-10-22", func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
		rows, e := probe(ctx, ids)
		return application.CompanyIdentitySnapshot{Evidence: rows}, e
	})
	if e != nil {
		t.Fatal(e)
	}
	in.CompanyAssociations = &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-22", Decisions: decisions}
	counter := 0
	next := func() (string, error) { counter++; return fmt.Sprintf("aaaaaaaa-1919-4919-8919-%012d", counter), nil }
	rev, e := pub.PublishNativeFines(ctx, in, next)
	if e != nil {
		t.Fatal(e)
	}
	enabled := true
	stations := directoryread.NewReader(p)
	h := Handler{Read: reader, Secrets: testSecrets, CompanyEnabled: func() bool { return enabled }, Now: func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }, StationsExist: func(ctx context.Context, id string) (bool, error) {
		_, e := stations.Detail(ctx, id)
		if errors.Is(e, directoryapp.ErrUnknownStation) {
			return false, nil
		}
		return e == nil, e
	}}
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	return &companyHTTPFixture{p: p, h: h, r: router, reader: reader, repo: repo, pub: pub, in: in, a: a.ID, b: b.ID, revision: rev, next: next, enabled: &enabled}
}
func companyRequest(router chi.Router, path, tag string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", path, nil)
	if tag != "" {
		r.Header.Set("If-None-Match", tag)
	}
	router.ServeHTTP(w, r)
	return w
}
func companyJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(w.Code, e)
	}
	return body
}
func companyItems(t *testing.T, w *httptest.ResponseRecorder) []any {
	t.Helper()
	return companyJSON(t, w)["items"].([]any)
}

func TestMEX19ScopedHTTPIsolationCursorsCachePrivacyAndOpenAPI(t *testing.T) {
	f := newCompanyHTTPFixture(t)
	path := "/v1/quality/sanctions?station_id=" + f.a + "&limit=1"
	first := companyRequest(f.r, path, "")
	if first.Code != 200 || len(companyItems(t, first)) != 1 {
		t.Fatal(first.Code)
	}
	body := companyJSON(t, first)
	scope := body["company_scope"].(map[string]any)
	if scope["association_scope"] != "CURRENT_COMPANY_ONLY" || scope["historical_business_window_known"] != false || scope["station_id"] != f.a {
		t.Fatal("company-only limitation omitted")
	}
	tag := first.Header().Get("ETag")
	if tag == "" || first.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" {
		t.Fatal("scoped cache cannot skip fresh evidence")
	}
	if got := companyRequest(f.r, path, tag); got.Code != 304 {
		t.Fatal("unchanged conditional", got.Code)
	}
	cursor := body["next_cursor"].(string)
	last := companyRequest(f.r, path+"&cursor="+url.QueryEscape(cursor), "")
	if last.Code != 200 || len(companyItems(t, last)) != 1 || companyJSON(t, last)["next_cursor"] != nil {
		t.Fatal("scoped continuation failed")
	}
	id := body["items"].([]any)[0].(map[string]any)["id"].(string)
	detail := companyRequest(f.r, "/v1/quality/sanctions/"+id+"?station_id="+f.a, "")
	if detail.Code != 200 {
		t.Fatal("own detail", detail.Code)
	}
	if got := companyRequest(f.r, "/v1/quality/sanctions/"+id+"?station_id="+f.b, ""); got.Code != 404 {
		t.Fatal("foreign detail leak", got.Code)
	}
	if got := companyRequest(f.r, "/v1/quality/sanctions?station_id="+f.b, tag); got.Code != 200 || len(companyItems(t, got)) != 1 {
		t.Fatal("cross-station304/page leak")
	}
	for _, changed := range []string{"/v1/quality/sanctions?station_id=" + f.b + "&limit=1", path + "&status=changed", "/v1/quality/sanctions?station_id=" + f.a + "&limit=2", "/v1/quality/sanctions?limit=1"} {
		if got := companyRequest(f.r, changed+"&cursor="+url.QueryEscape(cursor), ""); got.Code != 400 {
			t.Errorf("mutated cursor filters accepted: %d", got.Code)
		}
	}
	_, file, _, _ := runtime.Caller(0)
	spec, e := openapi3.NewLoader().LoadFromFile(filepath.Join(filepath.Dir(file), "../../../../../../contracts/openapi/v1.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct {
		w      *httptest.ResponseRecorder
		schema string
	}{{first, "QualitySanctionPage"}, {detail, "QualitySanction"}} {
		if e = spec.Components.Schemas[c.schema].Value.VisitJSON(companyJSON(t, c.w)); e != nil {
			t.Fatal("actual scoped OpenAPI response", e)
		}
		for _, private := range []string{"11222333000181", "04218406000104", "party_cnpj", "source_key", "scope_fingerprint", "allowed"} {
			if strings.Contains(c.w.Body.String(), private) {
				t.Fatal("private scope evidence leaked")
			}
		}
	}
	// New source revision invalidates default continuation; explicit old source stays selectable.
	f.in.ManifestHash = strings.Repeat("d", 64)
	newRev, e := f.pub.PublishNativeFines(context.Background(), f.in, f.next)
	if e != nil || newRev == f.revision {
		t.Fatal(e)
	}
	if got := companyRequest(f.r, path+"&cursor="+url.QueryEscape(cursor), ""); got.Code != 400 {
		t.Fatal("cross-revision cursor accepted")
	}
	if got := companyRequest(f.r, path+"&revision_id="+f.revision+"&cursor="+url.QueryEscape(cursor), ""); got.Code != 200 {
		t.Fatal("frozen scoped history lost", got.Code)
	}
	if got := companyRequest(f.r, "/v1/quality/sanctions?station_id="+f.a+"&revision_id=11111111-1111-4111-8111-111111111111", ""); got.Code != 404 {
		t.Fatal("unknown revision accepted")
	}
	global := companyRequest(f.r, "/v1/quality/sanctions", "")
	if global.Code != 200 || len(companyItems(t, global)) != 3 || companyJSON(t, global)["company_scope"] != nil {
		t.Fatal("global compatibility broken")
	}
}

func TestMEX19DirectoryChangeInvalidatesCursorETagAndDetail(t *testing.T) {
	for _, change := range []string{"retire", "status", "successor"} {
		t.Run(change, func(t *testing.T) {
			f := newCompanyHTTPFixture(t)
			path := "/v1/quality/sanctions?station_id=" + f.a + "&limit=1"
			old := companyRequest(f.r, path, "")
			body := companyJSON(t, old)
			cursor := body["next_cursor"].(string)
			id := body["items"].([]any)[0].(map[string]any)["id"].(string)
			detailPath := "/v1/quality/sanctions/" + id + "?station_id=" + f.a
			detailOld := companyRequest(f.r, detailPath, "")
			detailTag := detailOld.Header().Get("ETag")
			if detailOld.Code != 200 || detailTag == "" {
				t.Fatal("detail precondition", detailOld.Code)
			}
			ctx := context.Background()
			switch change {
			case "status":
				if _, e := f.p.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", f.a); e != nil {
					t.Fatal(e)
				}
			default:
				if e := f.repo.RetireIdentifier(ctx, "CNPJ", "11222333000181"); e != nil {
					t.Fatal(e)
				}
				if change == "successor" {
					if _, e := f.repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic successor", nil); e != nil {
						t.Fatal(e)
					}
				}
			}
			now := companyRequest(f.r, path, old.Header().Get("ETag"))
			if now.Code != 200 || len(companyItems(t, now)) != 0 || now.Header().Get("ETag") == old.Header().Get("ETag") {
				t.Fatal("stale station304 or attribution survived change", now.Code)
			}
			if got := companyRequest(f.r, path+"&cursor="+url.QueryEscape(cursor), ""); got.Code != 400 {
				t.Fatal("stale company-set cursor accepted")
			}
			if got := companyRequest(f.r, detailPath, ""); got.Code != 404 {
				t.Fatal("stale detail accepted")
			}
			// A stale detail ETag must revalidate the Directory evidence and
			// answer 404 instead of a cached 304 after retirement/suspension.
			if got := companyRequest(f.r, detailPath, detailTag); got.Code != 404 {
				t.Fatalf("stale detail ETag answered %d instead of404", got.Code)
			}
			global := companyRequest(f.r, "/v1/quality/sanctions", "")
			if global.Code != 200 || len(companyItems(t, global)) != 3 {
				t.Fatal("global facts lost")
			}
		})
	}
}

func TestMEX19WithdrawalPortsOutageKillSwitchAndStrictFilters(t *testing.T) {
	f := newCompanyHTTPFixture(t)
	path := "/v1/quality/sanctions?station_id=" + f.a
	old := companyRequest(f.r, path, "")
	id := companyItems(t, old)[0].(map[string]any)["id"].(string)
	for _, bad := range []string{path + "&unknown=1", path + "&status=a&status=b", path + "&revision_id=" + f.revision + "&revision_id=" + f.revision, path + "&limit=1&limit=2", path + "&x=%ZZ", "/v1/quality/sanctions/" + id + "?station_id=" + f.a + "&station_id=" + f.b} {
		if got := companyRequest(f.r, bad, ""); got.Code != 400 {
			t.Error("ambiguous filter accepted", got.Code)
		}
	}
	*f.enabled = false
	if got := companyRequest(f.r, path, old.Header().Get("ETag")); got.Code != 404 {
		t.Fatal("scoped kill switch bypassed")
	}
	if got := companyRequest(f.r, "/v1/quality/sanctions", ""); got.Code != 200 {
		t.Fatal("scoped kill disabled global")
	}
	*f.enabled = true
	for _, kind := range []string{"read-port", "existence", "guard", "secret"} {
		h := f.h
		switch kind {
		case "read-port":
			h.Read = qualityread.NewReader(f.p)
		case "existence":
			h.StationsExist = func(context.Context, string) (bool, error) { return false, errors.New("synthetic private outage") }
		case "guard":
			h.Read = f.reader.WithCompanyIdentity(func(ctx context.Context, ids []string) ([]domain.IdentityEvidence, error) {
				identity := directory.NewSourceIdentityReader(f.p)
				rows, e := identity.Evidence(ctx, ids)
				out := []domain.IdentityEvidence{}
				for _, v := range rows {
					out = append(out, domain.IdentityEvidence{CNPJ: v.CNPJ, StationID: v.StationID, Current: v.Current, StationActive: v.StationActive})
				}
				return out, e
			}, func(context.Context, pgx.Tx, []application.CompanyBinding) error {
				return errors.New("synthetic guard outage")
			})
		case "secret":
			h.Secrets = nil
		}
		r := chi.NewRouter()
		h.RegisterRoutes(r)
		got := companyRequest(r, path, old.Header().Get("ETag"))
		if got.Code != 503 || got.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s outage became cached success: %d", kind, got.Code)
		}
	}
	if _, e := f.p.Exec(context.Background(), "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", f.revision); e != nil {
		t.Fatal(e)
	}
	got := companyRequest(f.r, path, old.Header().Get("ETag"))
	if got.Code != 200 || len(companyItems(t, got)) != 0 || companyJSON(t, got)["company_scope"].(map[string]any)["available"] != false {
		t.Fatal("withdrawn source leaked or returned304", got.Code)
	}
	if got = companyRequest(f.r, "/v1/quality/sanctions/"+id+"?station_id="+f.a, ""); got.Code != 404 {
		t.Fatal("withdrawn scoped detail exposed")
	}
}

func TestMEX19ForeignStreamRevisionAndEmptyCompanyScope(t *testing.T) {
	f := newCompanyHTTPFixture(t)
	ctx := context.Background()
	// A real revision from another stream must not be selectable as sanctions.
	foreign, e := f.pub.PublishEvents(ctx, application.EventsInput{
		Source: "enforcement", Stream: "enforcement-v1", Edition: "2026-09",
		Resources: []application.Resource{{URL: "https://www.gov.br/anp/acoes", SHA256: strings.Repeat("1", 64), Bytes: 10}},
		Counts:    domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("2", 64),
		Enforcement: []application.EnforcementInput{{NativeKey: "AUTO-1", Scope: "municipal", Status: "retificado", SubjectCNPJ: "11222333000181", OccurredOn: "2026-08-02", AsOf: "2026-09-30", EvidenceRef: "synthetic-correction"}},
	}, f.next)
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{
		"/v1/quality/sanctions?station_id=" + f.a + "&revision_id=" + foreign,
		"/v1/quality/sanctions?revision_id=" + foreign,
		"/v1/quality/sanctions/aaaaaaaa-1111-4111-8111-000000000099?station_id=" + f.a + "&revision_id=" + foreign,
	} {
		if got := companyRequest(f.r, path, ""); got.Code != 404 {
			t.Errorf("foreign stream revision served %d: %s", got.Code, path)
		}
	}

	// A canonical station with Directory evidence but no current-company
	// observation is an explicit empty scope, never a clean or global fallback.
	noMatch, e := f.repo.ResolveCNPJ(ctx, "99888777000166", "Synthetic no-company station", nil)
	if e != nil {
		t.Fatal(e)
	}
	page := companyRequest(f.r, "/v1/quality/sanctions?station_id="+noMatch.ID, "")
	body := companyJSON(t, page)
	scope := body["company_scope"].(map[string]any)
	if page.Code != 200 || len(companyItems(t, page)) != 0 ||
		scope["available"] != true || scope["reason"] != "NO_CURRENT_COMPANY_MATCH" ||
		scope["historical_business_window_known"] != false {
		t.Fatalf("empty scope not explicit: %d %v", page.Code, scope)
	}
	globalPage := companyRequest(f.r, "/v1/quality/sanctions", "")
	rows := companyItems(t, globalPage)
	if len(rows) == 0 {
		t.Fatal("global census empty")
	}
	rowID := rows[0].(map[string]any)["id"].(string)
	if got := companyRequest(f.r, "/v1/quality/sanctions/"+rowID+"?station_id="+noMatch.ID, ""); got.Code != 404 {
		t.Fatalf("empty scope fell back to global row %d", got.Code)
	}
}
