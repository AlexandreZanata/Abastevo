//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	official "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	datapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters"
	dataweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/adapters/preparation"
	data "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type regionalExportFixture struct {
	stations []directory.ExportStation
	revision official.ExportRevision
	prices   []official.ExportPrice
}

func (f regionalExportFixture) ExportPage(context.Context, string, int) ([]directory.ExportStation, string, error) {
	return f.stations, "", nil
}

func (f regionalExportFixture) ExportRevisions(context.Context, string, string, string, int) ([]official.ExportRevision, string, error) {
	return []official.ExportRevision{f.revision}, "", nil
}

func (f regionalExportFixture) ExportPrices(context.Context, string, string, int) ([]official.ExportPrice, string, error) {
	return f.prices, "", nil
}

const marketProbe = "synthetic-station-marker-for-exclusion"

func buildRegionalExport(t *testing.T, month string, size int) (string, enterpriseexport.Manifest) {
	t.Helper()
	day := "2026-" + month + "-01"
	checksum := "sha256:" + strings.Repeat(month, 32)
	source := regionalExportFixture{revision: official.ExportRevision{ID: onboardingID(t), SurveyStart: day, SurveyEnd: day,
		PublishedAt: day + "T01:00:00Z", SourceURL: "https://source.example.invalid/prices", ParserVersion: "v1", SourceChecksum: checksum}}
	for i := range size + 1 {
		station := directory.ExportStation{ID: onboardingID(t), DisplayName: marketProbe, State: "MT",
			Status: "ACTIVE", MunicipalityCode: "5103403"}
		if i == size {
			station.State = "SP"
		}
		source.stations = append(source.stations, station)
		for _, fuel := range []string{"GASOLINE_REGULAR", "ETHANOL"} {
			if fuel == "ETHANOL" && i >= 2 {
				continue
			}
			source.prices = append(source.prices, official.ExportPrice{ID: onboardingID(t), RevisionID: source.revision.ID,
				StationID: station.ID, FuelProduct: fuel, Unit: "L", AmountMilliBRL: int64(5800 + i*100), CollectedOn: day})
		}
	}
	path := filepath.Join(t.TempDir(), "core")
	manifest, err := (enterpriseexport.Builder{Stations: source, Prices: source}).Build(t.Context(), enterpriseexport.Options{
		FromSurvey: day, ToSurvey: day, OutputDir: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	return path, manifest
}

func TestRegionalDataAPIRechecksRightsAndKeepsOnlyAdmittedAggregates(t *testing.T) {
	env := dataUsageFixture(t, 20)
	fx, ctx := env.fx, t.Context()
	org := fx.scope.OrgID().String()
	var runtimeRole string
	if err := env.pool.QueryRow(ctx, "SELECT current_user").Scan(&runtimeRole); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{runtimeRole}.Sanitize()
	for _, grant := range []string{
		"GRANT SELECT ON enterprise_data_source_reviews,enterprise_regional_datasets,enterprise_data_plan_datasets TO ",
		"GRANT SELECT,INSERT ON enterprise_regional_series,enterprise_regional_series_samples TO ",
	} {
		if _, err := fx.f.Admin.Exec(ctx, grant+role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_rate_policies
(product_code,plan_code,plan_revision,window_seconds,window_calls,approval_ref)
VALUES('data','synthetic',1,60,100,'synthetic-reviewed-rate')`); err != nil {
		t.Fatal(err)
	}
	paths, manifests := make([]string, 2), make([]enterpriseexport.Manifest, 2)
	paths[0], manifests[0] = buildRegionalExport(t, "01", 5)
	paths[1], manifests[1] = buildRegionalExport(t, "02", 4)
	const dataset = "regional-mt"
	for i, manifest := range manifests {
		registry, price := manifest.Files[0].SHA256, manifest.Sources.Official[0].SourceChecksum
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref,license_ref,attribution)
VALUES('S-ESP-01',$1,1,'APPROVED','synthetic-review','Synthetic registry license','Synthetic registry'),
      ('S-ESP-02',$2,1,'APPROVED','synthetic-review','Synthetic survey license','Synthetic survey')`,
			registry, price); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_regional_datasets
(dataset_key,revision,display_name,state_code,minimum_stations,method_version,snapshot_digest,official_revision,
 registry_digest,registry_review,price_digest,price_review)
VALUES($1,$2,'Synthetic state observations','MT',5,'abastevo-metrics-v1',$3,$4,$5,1,$6,1)`,
			dataset, i+1, manifest.ContentSHA256, manifest.Sources.Official[0].RevisionID, registry, price); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(paths[0], "prices.jsonl")
	good, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	altered := bytes.Replace(good, []byte("GASOLINE_REGULAR"), []byte("GASOLINE_INVALID"), 1)
	if err := os.WriteFile(file, altered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := preparation.Prepare(ctx, env.pool, dataset, 1, paths[0]); !errors.Is(err, enterpriseexport.ErrUnverifiedSnapshot) {
		t.Fatal("an altered official artifact was accepted", err)
	}
	if err := os.WriteFile(file, good, 0o600); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		hash, err := preparation.Prepare(ctx, env.pool, dataset, int64(i+1), path)
		if err != nil || len(hash) != 64 {
			t.Fatalf("prepared series was not installed: %v", err)
		}
		if other, err := preparation.Prepare(ctx, env.pool, dataset, int64(i+1), path); err != nil || hash != other {
			t.Fatalf("matching preparation was not idempotent: %v", err)
		}
	}
	if _, err := fx.f.Admin.Exec(ctx, "REVOKE INSERT ON enterprise_regional_series,enterprise_regional_series_samples FROM "+role); err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	dataweb.RegisterRegionalSeries(router, datapg.RegionalSeries{Meter: env.meter, CursorSecret: []byte(strings.Repeat("v", 32))})
	send := func(org string, bearer domain.Bearer, operation, cursor string, revisions ...int) *httptest.ResponseRecorder {
		t.Helper()
		path := "/v1/data/orgs/" + org + "/datasets/" + dataset + "/regional-series"
		body, err := json.Marshal(map[string]any{"schema_version": 1, "request_id": operation, "cursor": cursor, "revisions": revisions, "limit": 1})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
		request.Header.Set("Content-Type", "application/json")
		if !bearer.Empty() {
			request.Header.Set("Authorization", "Bearer "+bearer.Reveal())
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatal("private series was cacheable")
		}
		return response
	}
	key := env.original.Bearer
	for _, probe := range []struct {
		o string
		b domain.Bearer
		c int
	}{{org, domain.Bearer{}, 401}, {fx.f.Organizations[1].Scope(t).OrgID().String(), key, 404}, {org, key, 404}} {
		if response := send(probe.o, probe.b, onboardingID(t), "", 1, 2); response.Code != probe.c {
			t.Fatalf("unauthorized Data request returned %d, want %d", response.Code, probe.c)
		}
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_plan_datasets
(product_code,plan_code,plan_revision,dataset_key,dataset_revision,approval_ref)
VALUES('data','synthetic',1,$1,1,'synthetic-scope'),('data','synthetic',1,$1,2,'synthetic-scope')`, dataset); err != nil {
		t.Fatal(err)
	}
	requestID := onboardingID(t)
	first := send(org, key, requestID, "", 1, 2)
	var page datapg.SeriesPage
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil || first.Code != 200 || page.Region != "MT" ||
		len(page.Periods) != 1 || page.NextCursor == "" || len(page.Periods[0].Samples) != 1 {
		t.Fatalf("regional aggregates did not resolve correctly: status=%d body=%s error=%v", first.Code, first.Body, err)
	}
	sample := page.Periods[0].Samples[0]
	if sample.Fuel != "GASOLINE_REGULAR" || sample.Stations != 5 || sample.SumMilliBRL != 30000 ||
		bytes.Contains(first.Body.Bytes(), []byte(marketProbe)) || bytes.Contains(first.Body.Bytes(), []byte("station_sample_sha256")) ||
		bytes.Contains(first.Body.Bytes(), []byte("ETHANOL")) {
		t.Fatal("regional exposure ignored cutoff, fuel, source or geography")
	}
	if replay := send(org, key, requestID, "", 2, 1); replay.Code != 200 {
		t.Fatalf("canonical query retry did not converge: %d", replay.Code)
	}
	if changed := send(org, key, requestID, "", 1); changed.Code != 409 {
		t.Fatalf("request identity was rebound: %d", changed.Code)
	}
	second := send(org, key, onboardingID(t), page.NextCursor, 1, 2)
	var tail datapg.SeriesPage
	if err := json.Unmarshal(second.Body.Bytes(), &tail); err != nil || second.Code != 200 ||
		len(tail.Periods) != 1 || tail.Periods[0].Readiness != "INSUFFICIENT_DATA" || len(tail.Periods[0].Samples) != 0 {
		t.Fatalf("suppressed sample was published or misrepresented: status=%d body=%s error=%v", second.Code, second.Body, err)
	}
	independent := env.issue
	independent.OperationID = onboardingID(t)
	other, err := env.keys.WriteKey(ctx, fx.scope, fx.actor, fx.member, independent)
	if err != nil {
		t.Fatal(err)
	}
	if foreign := send(org, other.Bearer, onboardingID(t), page.NextCursor, 1, 2); foreign.Code != 400 {
		t.Fatalf("cursor leaked across credential families: %d", foreign.Code)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_data_source_reviews
(source_code,source_digest,revision,status,decision_ref)
VALUES('S-ESP-02',$1,2,'WITHDRAWN','synthetic-withdrawal')`, manifests[0].Sources.Official[0].SourceChecksum); err != nil {
		t.Fatal(err)
	}
	if response := send(org, key, requestID, "", 1, 2); response.Code != 404 {
		t.Fatalf("retry served a withdrawn source: %d", response.Code)
	}
	var used int
	if err := fx.f.Admin.QueryRow(ctx, "SELECT calls FROM org_data_usage_terms WHERE org_id=$1", org).Scan(&used); err != nil || used != 2 {
		t.Fatalf("refused Data requests moved usage: calls=%d err=%v", used, err)
	}
	usage := datapg.UsageStore{Pool: env.pool, Plans: env.meter.Plans, Flags: env.flags, Authority: env.auth, Enabled: true}
	view, err := usage.ReadUsage(ctx, fx.scope, fx.actor, fx.member, data.DatasetAfter{}, 20)
	if err != nil || !view.Enabled || view.Term == nil || view.Term.Used != 2 || view.Term.Ceiling != 20 ||
		view.Rate == nil || view.Rate.Calls != 100 || len(view.Datasets) != 1 || view.Datasets[0].Revision != 2 {
		t.Fatalf("usage view did not keep the current scope and drop withdrawn rights: %+v error=%v", view, err)
	}
	if _, err := usage.ReadUsage(ctx, fx.f.Organizations[1].Scope(t), fx.actor, fx.member, data.DatasetAfter{}, 20); err == nil {
		t.Fatal("foreign company exposed its usage")
	}
}
