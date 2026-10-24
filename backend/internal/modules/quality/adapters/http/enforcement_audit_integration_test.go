//go:build integration

package http

import (
	"context"
	"errors"
	"fmt"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestEnforcementAuditLiveIdentityAndReplay(t *testing.T) {
	f, in := newEnforcementAuditFixture(t)
	ctx := context.Background()
	t.Run("bound publication requires live guard", func(t *testing.T) {
		if _, err := parent.NewPublisher(f.p).PublishEnforcementSource(ctx, in, f.next); err == nil {
			t.Fatal("unguarded current-company attribution published")
		}
	})
	_, err := f.pub.PublishEnforcementSource(ctx, in, f.next)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("replay also rechecks live guard", func(t *testing.T) {
		called := false
		pub := parent.NewPublisher(f.p).WithCompanyGuard(func(context.Context, pgx.Tx, []application.CompanyBinding) error {
			called = true
			return errors.New("identity unavailable")
		})
		if _, err := pub.PublishEnforcementSource(ctx, in, f.next); err == nil || !called {
			t.Fatal("replay bypassed identity proof")
		}
	})
	t.Run("association plan belongs to publication identity", func(t *testing.T) {
		other, original := newEnforcementAuditFixture(t)
		old, err := other.pub.PublishEnforcementSource(ctx, original, other.next)
		if err != nil {
			t.Fatal(err)
		}
		changed := original
		plan := *original.CompanyAssociations
		plan.Decisions = append([]domain.CompanyObservationDecision(nil), plan.Decisions...)
		plan.Decisions[0] = domain.ResolveCompanyObservation(domain.CompanyObservationRecord{RecordKey: original.Records[0].RecordKey, CNPJ: original.Records[0].SourceCNPJ}, plan.DecisionDay, nil, false)
		changed.CompanyAssociations = &plan
		got, err := other.pub.PublishEnforcementSource(ctx, changed, other.next)
		if err != nil {
			t.Fatal(err)
		}
		if got == old {
			t.Fatal("different association plan replayed old revision")
		}
	})
	t.Run("current identifier retirement removes live attribution", func(t *testing.T) {
		first := companyRequest(f.r, "/v1/quality/enforcement/stations?limit=1", "")
		if first.Code != 200 {
			t.Fatal(first.Code, first.Body.String())
		}
		if !strings.Contains(first.Header().Get("Cache-Control"), "no-store") {
			t.Error("mutable company proof cached publicly")
		}
		coverage := companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations/coverage", ""))
		if coverage["stations"] != float64(2) {
			t.Fatal(coverage)
		}
		if _, err := f.p.Exec(ctx, `UPDATE directory_identifiers SET valid_to=now() WHERE normalized_value='11222333000181'`); err != nil {
			t.Fatal(err)
		}
		page := companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations?limit=1", first.Header().Get("ETag")))
		rows := page["items"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["id"] != f.b || page["next_cursor"] != nil {
			t.Fatal("stale binding or incomplete refill", page)
		}
		coverage = companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations/coverage", ""))
		if coverage["stations"] != float64(1) || coverage["records"] != float64(1) {
			t.Fatal("coverage retained retired company", coverage)
		}
	})
}

func newEnforcementAuditFixture(t *testing.T) (*companyHTTPFixture, application.EnforcementSourceInput) {
	f := newCompanyHTTPFixture(t)
	ctx := context.Background()
	records := []application.EnforcementSourceRecord{
		facetRecord("11222333000181", "company-shaped", "DF", "BRASILIA", "Revenda", "2026-10-04", "DOC-A", "Synthetic", 2),
		facetRecord("04218406000104", "company-shaped", "DF", "BRASILIA", "Revenda", "2026-10-03", "DOC-B", "Synthetic", 3),
	}
	decisions, err := parent.PlanEnforcementCompanyAssociations(ctx, records, "2026-10-22", func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
		return application.CompanyIdentitySnapshot{Evidence: []domain.IdentityEvidence{
			{CNPJ: "11222333000181", StationID: f.a, Current: true, StationActive: true},
			{CNPJ: "04218406000104", StationID: f.b, Current: true, StationActive: true},
		}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := application.EnforcementSourceInput{
		Context:   application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: "synthetic.xlsx", DateSystem: "EXCEL_1900", FirstDocumentDay: "2026-10-03", LastDocumentDay: "2026-10-04"},
		Resources: []application.Resource{{URL: "https://www.gov.br/synthetic.xlsx", SHA256: strings.Repeat("a", 64), Bytes: 10}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat("b", 64), Records: records,
		CompanyAssociations: &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-22", Decisions: decisions},
	}

	return f, in
}

func TestEnforcementAuditConcurrentRevocation(t *testing.T) {
	for _, which := range []string{"identifier", "revision"} {
		t.Run(which, func(t *testing.T) {
			f, in := newEnforcementAuditFixture(t)
			ctx := context.Background()
			rev, err := f.pub.PublishEnforcementSource(ctx, in, f.next)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := f.reader.LeaseBlock(ctx, time.Now(), "enforcement", domain.StreamEnforcement)
			if err != nil {
				t.Fatal(err)
			}
			locked, release := make(chan struct{}), make(chan struct{})
			released := false
			defer func() {
				if !released {
					close(release)
				}
			}()
			reader := f.reader.WithCompanyIdentity(f.probe, func(ctx context.Context, tx pgx.Tx, b []application.CompanyBinding) error {
				if err := f.guard(ctx, tx, b); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			})
			done := make(chan error, 1)
			go func() {
				_, _, _, err := reader.EnforcementStationsAtLease(ctx, lease, qualityread.EnforcementStationFilter{}, "", 100)
				done <- err
			}()
			select {
			case <-locked:
			case err := <-done:
				t.Fatal(err)
			case <-time.After(5 * time.Second):
				t.Fatal("read did not reach live guard")
			}
			tx, err := f.p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, `SET LOCAL lock_timeout='50ms'`)
			if err != nil {
				t.Fatal(err)
			}
			if which == "identifier" {
				_, err = tx.Exec(ctx, `UPDATE directory_identifiers SET valid_to=now() WHERE normalized_value='11222333000181'`)
			} else {
				_, err = tx.Exec(ctx, `UPDATE quality_revisions SET status='withdrawn' WHERE id=$1`, rev)
			}
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "55P03" {
				t.Errorf("concurrent revocation escaped read fence: %v", err)
			}
			_ = tx.Rollback(ctx)
			close(release)
			released = true
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("read failed to complete")
			}
			if which == "identifier" {
				_, err = f.p.Exec(ctx, `UPDATE directory_identifiers SET valid_to=now() WHERE normalized_value='11222333000181'`)
			} else {
				_, err = f.p.Exec(ctx, `UPDATE quality_revisions SET status='withdrawn' WHERE id=$1`, rev)
			}
			if err != nil {
				t.Fatal(err)
			}
			if which == "revision" {
				if _, _, err := f.reader.EnforcementStationCoverageAtLease(ctx, lease); err == nil {
					t.Fatal("withdrawn revision served")
				}
			}
		})
	}
}

func TestEnforcementAuditSingleConnectionDoesNotStarve(t *testing.T) {
	f, in := newEnforcementAuditFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := f.pub.PublishEnforcementSource(ctx, in, f.next); err != nil {
		t.Fatal(err)
	}
	lease, err := f.reader.LeaseBlock(ctx, time.Now(), "enforcement", domain.StreamEnforcement)
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.p.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	single, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	identity := directory.NewSourceIdentityReader(single)
	probe := func(ctx context.Context, ids []string) ([]domain.IdentityEvidence, error) {
		rows, err := identity.Evidence(ctx, ids)
		out := []domain.IdentityEvidence{}
		for _, v := range rows {
			out = append(out, domain.IdentityEvidence{CNPJ: v.CNPJ, StationID: v.StationID, Current: v.Current, StationActive: v.StationActive})
		}
		return out, err
	}
	reader := qualityread.NewReader(single).WithCompanyIdentity(probe, f.guard).WithCompanyIdentityTx(f.txProbe)
	rows, _, _, err := reader.EnforcementStationsAtLease(ctx, lease, qualityread.EnforcementStationFilter{}, "", 100)
	if err != nil || len(rows) != 2 {
		t.Fatalf("read consumed a second pool connection: count=%d error=%v", len(rows), err)
	}
}

// 511 one-company stations and a final two-company station force one complete
// group across the private 512-row batch boundary. Public pages stay 100 rows.
func TestEnforcementAuditCompleteBoundedTraversal(t *testing.T) {
	f, in := newEnforcementAuditFixture(t)
	ctx := context.Background()
	ids, cnpjs := []string{}, []string{}
	in.Records = nil
	decisions := []domain.CompanyObservationDecision{}
	for i := 1; i <= 513; i++ {
		station := i
		if station == 513 {
			station = 512
		}
		id := fmt.Sprintf("%08d-1111-4111-8111-%012d", station, station)
		base := fmt.Sprintf("99887766%04d", i)
		digits := base
		for _, weights := range [][]int{{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}, {6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}} {
			total := 0
			for j, w := range weights {
				total += int(digits[j]-'0') * w
			}
			d := 11 - total%11
			if d >= 10 {
				d = 0
			}
			digits += fmt.Sprint(d)
		}
		ids = append(ids, id)
		cnpjs = append(cnpjs, digits)
		row := facetRecord(digits, "company-shaped", "DF", "BRASILIA", "Revenda", "2026-10-04", fmt.Sprint("DOC-", i), "Synthetic batch", i+1)
		in.Records = append(in.Records, row)
		decisions = append(decisions, domain.ResolveCompanyObservation(domain.CompanyObservationRecord{RecordKey: row.RecordKey, CNPJ: digits}, "2026-10-22", []domain.IdentityEvidence{{CNPJ: digits, StationID: id, Current: true, StationActive: true}}, false))
	}
	if _, err := f.p.Exec(ctx, `INSERT INTO directory_stations(id,display_name,state,status) SELECT DISTINCT id::uuid,'Synthetic audit station','DF','active' FROM unnest($1::text[]) id`, ids); err != nil {
		t.Fatal(err)
	}
	if _, err := f.p.Exec(ctx, `INSERT INTO directory_identifiers(id,station_id,kind,normalized_value) SELECT md5(cnpj)::uuid,id::uuid,'CNPJ',cnpj FROM unnest($1::text[],$2::text[]) p(id,cnpj)`, ids, cnpjs); err != nil {
		t.Fatal(err)
	}
	in.Context.FirstDocumentDay = "2026-10-04"
	in.Context.LastDocumentDay = "2026-10-04"
	in.Counts = domain.Counts{Input: 513, Accepted: 513}
	in.CompanyAssociations.Decisions = decisions
	if _, err := f.pub.PublishEnforcementSource(ctx, in, f.next); err != nil {
		t.Fatal(err)
	}
	coverage := companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations/coverage", ""))
	if coverage["stations"] != float64(512) || coverage["records"] != float64(513) {
		t.Fatal("split group changed census", coverage)
	}
	seen := map[string]bool{}
	total := float64(0)
	cursor := ""
	for pages := 0; ; pages++ {
		if pages >= 6 {
			t.Fatal("unbounded continuation")
		}
		page := companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations?limit=100&cursor="+url.QueryEscape(cursor), ""))
		rows := page["items"].([]any)
		if len(rows) > 100 {
			t.Fatal("page exceeds budget")
		}
		for _, raw := range rows {
			row := raw.(map[string]any)
			id := row["id"].(string)
			if seen[id] {
				t.Fatal("repeated station")
			}
			seen[id] = true
			total += row["record_count"].(float64)
		}
		next, _ := page["next_cursor"].(string)
		if next == "" {
			break
		}
		if next == cursor {
			t.Fatal("cursor did not advance")
		}
		cursor = next
	}
	if len(seen) != 512 || total != 513 {
		t.Fatalf("incomplete traversal: %d stations %.0f records", len(seen), total)
	}
}

func TestEnforcementAuditMissingGeographyStaysNull(t *testing.T) {
	f, in := newEnforcementAuditFixture(t)
	if _, err := f.pub.PublishEnforcementSource(context.Background(), in, f.next); err != nil {
		t.Fatal(err)
	}
	page := companyJSON(t, companyRequest(f.r, "/v1/quality/enforcement/stations", ""))
	rows := page["items"].([]any)
	if len(rows) != 2 {
		t.Fatal("missing-geography stations lost")
	}
	for _, raw := range rows {
		row := raw.(map[string]any)
		if row["state"] != nil || row["municipality"] != nil {
			t.Fatal("unknown geography fabricated as empty text")
		}
	}
}
