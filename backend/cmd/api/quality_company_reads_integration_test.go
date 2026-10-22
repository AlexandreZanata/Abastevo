//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/http"
	qualityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	qualitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMEX19RootCompanyReaderServesOnlyLiveDirectoryBindingOverHTTP(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	u, e := url.Parse(dsn)
	if e != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("explicit loopback disposable DB required")
	}
	admin, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("mex19_root_%d", time.Now().UnixNano())
	if _, e = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); e != nil {
			t.Error(e)
		}
	}()
	u.Path = "/" + name
	if _, e = migrate.Apply(ctx, u.String(), dbmigrations.Files); e != nil {
		t.Fatal(e)
	}
	p, e := pgxpool.New(ctx, u.String())
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	repo := directory.NewRepository(p)
	station, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic root station", nil)
	if e != nil {
		t.Fatal(e)
	}
	row := qualityapp.FineObservation{SourceCNPJ: "11222333000181", ProceedingRef: "SYNTHETIC", DUFRef: "SYNTHETIC/1", Department: "SYNTHETIC", Status: "Source status", DueOn: "2025-02-28", AmountText: "1,00", AmountMinor: 100, PaidText: "0,00", ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: "synthetic:8"}
	row.ObservationKey = row.ObservationIdentity()
	decision := qualitydomain.ResolveCompanyObservation(qualitydomain.CompanyObservationRecord{RecordKey: row.ObservationKey, CNPJ: row.SourceCNPJ}, "2026-10-22", []qualitydomain.IdentityEvidence{{CNPJ: row.SourceCNPJ, StationID: station.ID, Current: true, StationActive: true}}, false)
	in := qualityapp.NativeFineInput{Edition: "2025", Clock: qualityapp.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Resources: []qualityapp.Resource{{URL: "https://www.gov.br/anp/fines.csv", SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: qualitydomain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("b", 64), Observations: []qualityapp.FineObservation{row}, CompanyAssociations: &qualityapp.CompanyAssociationInput{PolicyVersion: qualitydomain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-22", Decisions: []qualitydomain.CompanyObservationDecision{decision}}}
	// Existing publisher independently validates this typed association shape.
	n := 0
	next := func() (string, error) { n++; return fmt.Sprintf("bbbbbbbb-1919-4919-8919-%012d", n), nil }
	identity := directory.NewSourceIdentityReader(p)
	pub := quality.NewPublisher(p).WithCompanyGuard(func(ctx context.Context, tx pgx.Tx, b []qualityapp.CompanyBinding) error {
		return identity.Guard(ctx, tx, []directoryapp.SourceBinding{{CNPJ: b[0].CNPJ, StationID: b[0].StationID}})
	})
	if _, e = pub.PublishNativeFines(ctx, in, next); e != nil {
		t.Fatal(e)
	}
	stations := directoryread.NewReader(p)
	r := chi.NewRouter()
	qualityhttp.Handler{Read: companyQualityReader(p), Secrets: []byte("synthetic-root-cursor-key-32-bytes"), CompanyEnabled: func() bool { return true }, StationsExist: func(ctx context.Context, id string) (bool, error) {
		_, e := stations.Detail(ctx, id)
		return e == nil, e
	}}.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()
	path := server.URL + "/v1/quality/sanctions?station_id=" + station.ID
	read := func(want int, count int) {
		t.Helper()
		response, e := http.Get(path)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var body struct {
			Items []json.RawMessage `json:"items"`
		}
		if e = json.NewDecoder(response.Body).Decode(&body); e != nil || response.StatusCode != want || len(body.Items) != count {
			t.Fatalf("root composed HTTP %d rows%d err%v", response.StatusCode, len(body.Items), e)
		}
	}
	read(200, 1)
	if e = repo.RetireIdentifier(ctx, "CNPJ", "11222333000181"); e != nil {
		t.Fatal(e)
	}
	read(200, 0)
}
