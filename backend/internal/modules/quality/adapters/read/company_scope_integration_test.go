//go:build integration

package read

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func companyReadFixture(t *testing.T) (*pgxpool.Pool, *Reader, *directory.Repository, application.NativeFineInput, *parent.Publisher, string, string) {
	t.Helper()
	p := freshPool(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	a, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic read company A", nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := repo.ResolveCNPJ(ctx, "04218406000104", "Synthetic read company B", nil)
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
	guard := func(ctx context.Context, tx pgx.Tx, bindings []application.CompanyBinding) error {
		out := make([]directoryapp.SourceBinding, 0, len(bindings))
		for _, b := range bindings {
			out = append(out, directoryapp.SourceBinding{CNPJ: b.CNPJ, StationID: b.StationID})
		}
		return identity.Guard(ctx, tx, out)
	}
	in := application.NativeFineInput{Edition: "2025", Clock: application.NativeFineClock{ReportYear: 2025, ExtractionMonth: "2026-01"}, Counts: domain.Counts{Input: 4, Accepted: 4}, ManifestHash: strings.Repeat("a", 64), Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv", SHA256: strings.Repeat("b", 64), Bytes: 100}}}
	records := []domain.CompanyObservationRecord{}
	for i, cnpj := range []string{"11222333000181", "11222333000181", "04218406000104", "12ABC34501DE35"} {
		r := application.FineObservation{SourceCNPJ: cnpj, ProceedingRef: fmt.Sprint("synthetic:", i), DUFRef: fmt.Sprint("synthetic/", i), Department: "SYNTHETIC", Status: "Synthetic source status", DueOn: "2025-02-28", AmountText: "1,00", AmountMinor: 100, PaidText: "0,00", ReportYear: 2025, ExtractionMonth: "2026-01", EvidenceRef: fmt.Sprint("synthetic:", i+8)}
		r.ObservationKey = r.ObservationIdentity()
		in.Observations = append(in.Observations, r)
		records = append(records, domain.CompanyObservationRecord{RecordKey: r.ObservationKey, CNPJ: r.SourceCNPJ})
	}
	decisions, e := application.PlanCompanyAssociations(ctx, records, "2026-10-20", func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
		rows, e := probe(ctx, ids)
		return application.CompanyIdentitySnapshot{Evidence: rows}, e
	})
	if e != nil {
		t.Fatal(e)
	}
	in.CompanyAssociations = &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20", Decisions: decisions}
	return p, NewReader(p).WithCompanyIdentity(probe, guard), repo, in, parent.NewPublisher(p).WithCompanyGuard(guard), a.ID, b.ID
}
func companyReadIDMaker() func() (string, error) {
	n := 0
	return func() (string, error) { n++; return fmt.Sprintf("aaaaaaaa-7777-4777-8777-%012d", n), nil }
}
func TestCompanyScopedReadIsolatesSourceStationAndFrozenKeysets(t *testing.T) {
	_, r, _, in, pub, a, b := companyReadFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	next := companyReadIDMaker()
	first, e := pub.PublishNativeFines(ctx, in, next)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := r.ResolveCompanyScope(ctx, now, lease, a)
	if e != nil || !scope.View().Available || scope.View().HistoricalBusinessWindowKnown || scope.View().AssociationScope != domain.CurrentCompanyOnly {
		t.Fatal("company-only scope missing", e)
	}
	page, cursor, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 1)
	if e != nil || len(page) != 1 || cursor == "" {
		t.Fatal("scoped page/keyset", e)
	}
	parts := strings.SplitN(cursor, "|", 2)
	tail, more, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", parts[0], parts[1], 1)
	if e != nil || len(tail) != 1 || more != "" || tail[0].ID == page[0].ID {
		t.Fatal("scoped traversal", e)
	}
	own := map[string]bool{in.Observations[0].ObservationKey: true, in.Observations[1].ObservationKey: true}
	for _, event := range append(page, tail...) {
		if !own[event.NativeKey] || event.DecidedOn != "" || event.DueOn != "2025-02-28" {
			t.Fatal("global/relabeled event in station scope")
		}
		if _, e := r.CompanySanctionAtScope(ctx, lease, scope, event.ID); e != nil {
			t.Fatal("own scoped detail", e)
		}
	}
	other, e := r.ResolveCompanyScope(ctx, now, lease, b)
	if e != nil || other.Fingerprint() == scope.Fingerprint() {
		t.Fatal("cross-station scope fingerprint", e)
	}
	rows, _, e := r.CompanySanctionsAtScope(ctx, lease, other, "", "", "", 100)
	if e != nil || len(rows) != 1 {
		t.Fatal("other station census", e)
	}
	if _, e := r.CompanySanctionAtScope(ctx, lease, scope, rows[0].ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign station detail escaped scope", e)
	}
	// A source correction cannot substitute the old lease/scope during pagination.
	in.ManifestHash = strings.Repeat("c", 64)
	second, e := pub.PublishNativeFines(ctx, in, next)
	if e != nil || second == first {
		t.Fatal(e)
	}
	current, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e := r.CompanySanctionsAtScope(ctx, current, scope, "", "", "", 100); e == nil {
		t.Fatal("old scope accepted new lease")
	}
	old := r.AtRevision(first)
	frozen, e := old.RevisionLease(ctx, now, "sanctions", "sanction-v1", first)
	if e != nil {
		t.Fatal(e)
	}
	oldScope, e := old.ResolveCompanyScope(ctx, now, frozen, a)
	if e != nil {
		t.Fatal("AtRevision lost identity ports", e)
	}
	rows, _, e = old.CompanySanctionsAtScope(ctx, frozen, oldScope, "", "", "", 100)
	if e != nil || len(rows) != 2 {
		t.Fatal("frozen station source lost", e)
	}
}

func TestCompanyScopedReadFailsClosedOnDirectoryChange(t *testing.T) {
	for _, change := range []string{"retire", "successor", "status"} {
		t.Run(change, func(t *testing.T) {
			p, r, repo, in, pub, a, _ := companyReadFixture(t)
			ctx := context.Background()
			now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
			next := companyReadIDMaker()
			if _, e := pub.PublishNativeFines(ctx, in, next); e != nil {
				t.Fatal(e)
			}
			lease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
			if e != nil {
				t.Fatal(e)
			}
			scope, e := r.ResolveCompanyScope(ctx, now, lease, a)
			if e != nil || !scope.View().Available {
				t.Fatal("scope missing", e)
			}
			page, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 100)
			if e != nil || len(page) != 2 {
				t.Fatal("scoped census", e)
			}
			detailID := page[0].ID
			switch change {
			case "retire":
				if e := repo.RetireIdentifier(ctx, "CNPJ", "11222333000181"); e != nil {
					t.Fatal(e)
				}
			case "successor":
				if e := repo.RetireIdentifier(ctx, "CNPJ", "11222333000181"); e != nil {
					t.Fatal(e)
				}
				if _, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic successor", nil); e != nil {
					t.Fatal(e)
				}
			default:
				if _, e := p.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", a); e != nil {
					t.Fatal(e)
				}
			}
			if _, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 100); e == nil {
				t.Fatal("retired/ambiguous station page accepted")
			}
			if _, e := r.CompanySanctionAtScope(ctx, lease, scope, detailID); e == nil {
				t.Fatal("retired/ambiguous station detail accepted")
			}
			// The source-global history stays reachable without station attribution.
			global, _, _, e := r.SanctionsAtLease(ctx, lease, "", "", "", 100)
			if e != nil || len(global) != 4 {
				t.Fatal("global source history lost after directory change", len(global), e)
			}
			if _, _, e := r.SanctionOne(ctx, now, "sanctions", "sanction-v1", detailID); e != nil {
				t.Fatal("global source detail lost after directory change", e)
			}
		})
	}
}
