//go:build integration

package read

import (
	"context"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

func TestMEX18CancelledEmptyReadsAndWithdrawnLeaseRefuse(t *testing.T) {
	p, r, _, in, pub, station, _ := companyReadFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	emptyLease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	empty, e := r.ResolveCompanyScope(ctx, now, emptyLease, station)
	if e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if rows, _, e := r.CompanySanctionsAtScope(cancelled, emptyLease, empty, "", "", "", 10); !errors.Is(e, context.Canceled) || len(rows) != 0 {
		t.Errorf("cancelled empty page error=%v", e)
	}
	if _, e := r.CompanySanctionAtScope(cancelled, emptyLease, empty, "11111111-1111-4111-8111-111111111111"); !errors.Is(e, context.Canceled) {
		t.Errorf("cancelled detail error=%v", e)
	}
	rev, e := pub.PublishNativeFines(ctx, in, companyReadIDMaker())
	if e != nil {
		t.Fatal(e)
	}
	lease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := r.ResolveCompanyScope(ctx, now, lease, station)
	if e != nil {
		t.Fatal(e)
	}
	rows, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10)
	if e != nil || len(rows) != 2 {
		t.Fatal("fixture", e)
	}
	if _, e = p.Exec(ctx, "UPDATE quality_revisions SET status='withdrawn' WHERE id=$1", rev); e != nil {
		t.Fatal(e)
	}
	if got, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10); e == nil || len(got) != 0 {
		t.Errorf("withdrawn stale page returned %d rows error=%v", len(got), e)
	}
	if got, e := r.CompanySanctionAtScope(ctx, lease, scope, rows[0].ID); e == nil || got.ID != "" {
		t.Errorf("withdrawn stale detail returned ID error=%v", e)
	}
}

func waitMEX18Block(t *testing.T, p *pgxpool.Pool, condition string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var ok bool
		if e := p.QueryRow(ctx, condition, args...).Scan(&ok); e != nil {
			t.Fatal(e)
		}
		if ok {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("expected PostgreSQL lock not observed")
		case <-tick.C:
		}
	}
}

func TestMEX18ReaderHoldsDirectoryThroughActualPageDetailAndCancel(t *testing.T) {
	for _, mode := range []string{"page", "detail"} {
		for _, change := range []string{"retire", "status"} {
			for _, cancelRead := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/cancel=%v", mode, change, cancelRead), func(t *testing.T) {
					p, r, repo, in, pub, station, _ := companyReadFixture(t)
					ctx, cancelAll := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancelAll()
					now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
					if _, e := pub.PublishNativeFines(ctx, in, companyReadIDMaker()); e != nil {
						t.Fatal(e)
					}
					lease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
					if e != nil {
						t.Fatal(e)
					}
					scope, e := r.ResolveCompanyScope(ctx, now, lease, station)
					if e != nil {
						t.Fatal(e)
					}
					page, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10)
					if e != nil {
						t.Fatal(e)
					}
					blocker, e := p.Begin(ctx)
					if e != nil {
						t.Fatal(e)
					}
					defer blocker.Rollback(context.Background())
					if _, e = blocker.Exec(ctx, "LOCK quality_sanction_events IN ACCESS EXCLUSIVE MODE"); e != nil {
						t.Fatal(e)
					}
					pid := make(chan uint32, 1)
					observed := r.WithCompanyIdentity(r.companyProbe, func(ctx context.Context, tx pgx.Tx, b []application.CompanyBinding) error {
						if e := r.companyGuard(ctx, tx, b); e != nil {
							return e
						}
						pid <- tx.Conn().PgConn().PID()
						return nil
					})
					readCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					done := make(chan error, 1)
					go func() {
						if mode == "page" {
							rows, _, e := observed.CompanySanctionsAtScope(readCtx, lease, scope, "", "", "", 10)
							if e != nil && len(rows) != 0 {
								e = errors.New("partial page escaped failure")
							}
							done <- e
						} else {
							row, e := observed.CompanySanctionAtScope(readCtx, lease, scope, page[0].ID)
							if e != nil && row.ID != "" {
								e = errors.New("partial detail escaped failure")
							}
							done <- e
						}
					}()
					var readPID uint32
					select {
					case readPID = <-pid:
					case <-ctx.Done():
						t.Fatal("guard not reached")
					}
					// Reader is now in its real generated SQL query, blocked after Directory guards.
					waitMEX18Block(t, p, "SELECT cardinality(pg_blocking_pids($1))>0", readPID)
					writer := make(chan error, 1)
					go func() {
						if change == "retire" {
							writer <- repo.RetireIdentifier(ctx, "CNPJ", "11222333000181")
						} else {
							_, e := p.Exec(ctx, "UPDATE directory_stations SET status='suspended' WHERE id=$1", station)
							writer <- e
						}
					}()
					waitMEX18Block(t, p, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND $1=ANY(pg_blocking_pids(pid)))", readPID)
					select {
					case e := <-writer:
						t.Fatalf("Directory writer escaped read transaction: %v", e)
					default:
					}
					if cancelRead {
						cancel()
					} else {
						if e := blocker.Commit(ctx); e != nil {
							t.Fatal(e)
						}
					}
					select {
					case e := <-done:
						if (e != nil) != cancelRead {
							t.Fatalf("read cancel=%v error=%v", cancelRead, e)
						}
					case <-ctx.Done():
						t.Fatal("reader did not finish")
					}
					select {
					case e := <-writer:
						if e != nil {
							t.Fatal(e)
						}
					case <-ctx.Done():
						t.Fatal("reader leaked Directory locks")
					}
					_ = blocker.Rollback(context.Background())
					rows, _, _, e := r.SanctionsAtLease(ctx, lease, "", "", "", 10)
					if e != nil || len(rows) != 4 {
						t.Fatal("global source facts lost", len(rows), e)
					}
					fresh, e := r.ResolveCompanyScope(ctx, now, lease, station)
					if e != nil || fresh.Fingerprint() == scope.Fingerprint() {
						t.Fatal("Directory change did not change fresh fingerprint", e)
					}
					rows, _, e = r.CompanySanctionsAtScope(ctx, lease, fresh, "", "", "", 10)
					if e != nil || len(rows) != 0 {
						t.Fatal("fresh empty scope exposed stale attribution", e)
					}
				})
			}
		}
	}
}

func TestMEX18MissingPortsContextCensusAndSQLFailure(t *testing.T) {
	p, r, _, in, pub, station, _ := companyReadFixture(t)
	ctx := context.Background()
	now := time.Now()
	rev, e := pub.PublishNativeFines(ctx, in, companyReadIDMaker())
	if e != nil {
		t.Fatal(e)
	}
	lease, e := r.LeaseBlock(ctx, now, "sanctions", "sanction-v1")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := r.ResolveCompanyScope(ctx, now, lease, station)
	if e != nil {
		t.Fatal(e)
	}
	for _, bad := range []*Reader{r.WithCompanyIdentity(nil, r.companyGuard), r.WithCompanyIdentity(r.companyProbe, nil), r.WithCompanyIdentity(func(context.Context, []string) ([]domain.IdentityEvidence, error) {
		return nil, errors.New("synthetic outage")
	}, r.companyGuard)} {
		if _, e = bad.ResolveCompanyScope(ctx, now, lease, station); e == nil {
			t.Fatal("missing/failing identity port admitted")
		}
	}
	bad := r.WithCompanyIdentity(r.companyProbe, func(context.Context, pgx.Tx, []application.CompanyBinding) error {
		return errors.New("synthetic guard outage")
	})
	if rows, _, e := bad.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10); e == nil || len(rows) != 0 {
		t.Fatal("guard failure leaked page")
	}
	if _, e = p.Exec(ctx, "UPDATE quality_company_association_revisions SET observation_count=5 WHERE revision_id=$1", rev); e != nil {
		t.Fatal(e)
	}
	if _, e = r.ResolveCompanyScope(ctx, now, lease, station); e == nil {
		t.Fatal("divergent census accepted")
	}
	if _, e = p.Exec(ctx, "ALTER TABLE quality_company_associations RENAME TO mex18_private_fault"); e != nil {
		t.Fatal(e)
	}
	if rows, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10); e == nil || len(rows) != 0 {
		t.Fatal("real SQL fault leaked partial rows")
	}
	if _, e = p.Exec(ctx, "ALTER TABLE mex18_private_fault RENAME TO quality_company_associations"); e != nil {
		t.Fatal(e)
	}
	if _, e = p.Exec(ctx, "DELETE FROM quality_company_association_revisions WHERE revision_id=$1", rev); e != nil {
		t.Fatal(e)
	}
	missing, e := r.ResolveCompanyScope(ctx, now, lease, station)
	if e != nil || missing.View().Available || missing.View().Reason != "ASSOCIATION_EVIDENCE_UNAVAILABLE" {
		t.Fatal("legacy evidence invented", e)
	}
	rows, _, _, e := r.SanctionsAtLease(ctx, lease, "", "", "", 10)
	if e != nil || len(rows) != 4 {
		t.Fatal("global facts lost on missing association context", e)
	}
}

func TestMEX18AllHistoricalProfilesPreserveScopedFacts(t *testing.T) {
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024"} {
		t.Run(period, func(t *testing.T) {
			_, r, _, _, pub, station, _ := companyReadFixture(t)
			ctx := context.Background()
			profile, _ := application.HistoricalFineProfile(period)
			clock := application.FineHistoryClock{Precision: profile.ClockPrecision, Value: "2026-01"}
			if profile.ClockPrecision == "DAY" {
				clock.Value = "2020-02-06"
			}
			paid := int64(0)
			row := application.HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "SYNTHETIC-CASE", ReferenceLabel: profile.ReferenceLabel, ReferenceRef: "SYNTHETIC/1", Status: "Synthetic status", DueOn: "2019-02-28", AmountText: " R$ 1.234,56 ", AmountMinor: 123456, PaidText: " R$ 0,00 ", PaidMinor: &paid, SourcePeriod: period, Clock: clock, InputContract: profile.Contract, EvidenceRef: fmt.Sprint(profile.SourceFile, ":", profile.HeaderRecord+1)}
			if profile.UnknownPaid {
				row.PaidMinor = nil
				row.PaidText = " R$ -   "
			}
			if profile.Transit {
				day := "2019-02-20"
				row.SourceTransitOn = &day
			}
			if profile.Unlabelled {
				row.UnlabelledField = "SIM"
			}
			row.ObservationKey = row.ObservationIdentity()
			decisions, e := application.PlanCompanyAssociations(ctx, []domain.CompanyObservationRecord{{RecordKey: row.ObservationKey, CNPJ: row.SourceCNPJ}}, "2026-10-22", func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
				evidence, e := r.companyProbe(ctx, ids)
				return application.CompanyIdentitySnapshot{Evidence: evidence}, e
			})
			if e != nil {
				t.Fatal(e)
			}
			in := application.HistoricalFineInput{Context: application.FineHistoryContext{InputContract: profile.Contract, SourcePeriod: period, SourceFile: profile.SourceFile, HeaderRecord: profile.HeaderRecord, Clock: clock, SourceRecordCount: 1}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/" + profile.SourceFile, SHA256: strings.Repeat("b", 64), Bytes: 100}}, Counts: domain.Counts{Input: 1, Accepted: 1}, ManifestHash: strings.Repeat("c", 64), Observations: []application.HistoricalFineObservation{row}, CompanyAssociations: &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-22", Decisions: decisions}}
			rev, e := pub.PublishHistoricalFines(ctx, in, companyReadIDMaker())
			if e != nil {
				t.Fatal(e)
			}
			lease, e := r.RevisionLease(ctx, time.Now(), "sanctions", "sanction-v1", rev)
			if e != nil {
				t.Fatal(e)
			}
			scope, e := r.ResolveCompanyScope(ctx, time.Now(), lease, station)
			if e != nil {
				t.Fatal(e)
			}
			page, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10)
			if e != nil || len(page) != 1 {
				t.Fatal("historical page", e)
			}
			detail, e := r.CompanySanctionAtScope(ctx, lease, scope, page[0].ID)
			if e != nil {
				t.Fatal(e)
			}
			for _, got := range []SanctionEvent{page[0], detail} {
				if got.SourcePeriod != period || got.InputContract != profile.Contract || got.ReferenceLabel != row.ReferenceLabel || got.ReferenceRef != row.ReferenceRef || got.DueOn != row.DueOn || got.ClockPrecision != clock.Precision || got.SourceClockValue != clock.Value || got.DecidedOn != "" || got.UnlabelledField != row.UnlabelledField || (got.PaidMinor == nil) != (row.PaidMinor == nil) {
					t.Fatalf("historical facts relabeled %+v", got)
				}
				if row.SourceTransitOn != nil && got.SourceTransitOn != *row.SourceTransitOn {
					t.Fatal("transit precision lost")
				}
			}
		})
	}
}

func TestMEX18VerifiedEmptySourceIsAvailableWithoutInventedMatches(t *testing.T) {
	_, r, _, in, pub, station, _ := companyReadFixture(t)
	in.Observations = nil
	in.Counts = domain.Counts{}
	in.CompanyAssociations.Decisions = nil
	ctx := context.Background()
	if _, e := pub.PublishNativeFines(ctx, in, companyReadIDMaker()); e != nil {
		t.Fatal(e)
	}
	lease, e := r.LeaseBlock(ctx, time.Now(), "sanctions", "sanction-v1")
	if e != nil || !lease.HasPublished {
		t.Fatal("valid empty source unavailable", e)
	}
	scope, e := r.ResolveCompanyScope(ctx, time.Now(), lease, station)
	if e != nil || !scope.View().Available || scope.View().Reason != "NO_CURRENT_COMPANY_MATCH" {
		t.Fatal("verified empty scope lost", e)
	}
	rows, _, e := r.CompanySanctionsAtScope(ctx, lease, scope, "", "", "", 10)
	if e != nil || len(rows) != 0 {
		t.Fatal("empty census invented observations", e)
	}
}
