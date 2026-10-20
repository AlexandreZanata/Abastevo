//go:build integration

package adapters

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scopedFineFixture(t *testing.T, period string) (*pgxpool.Pool, *directory.Repository, *Publisher, *application.CompanyAssociationInput, func(*Publisher, *application.CompanyAssociationInput, string, func() (string, error)) (string, error)) {
	t.Helper()
	p := freshPool(t)
	ctx := context.Background()
	repo := directory.NewRepository(p)
	station, e := repo.ResolveCNPJ(ctx, "11222333000181", "Synthetic company publication", nil)
	if e != nil {
		t.Fatal(e)
	}
	reader := directory.NewSourceIdentityReader(p)
	pub := NewPublisher(p).WithCompanyGuard(func(ctx context.Context, tx pgx.Tx, bindings []application.CompanyBinding) error {
		expected := make([]directoryapp.SourceBinding, 0, len(bindings))
		for _, b := range bindings {
			expected = append(expected, directoryapp.SourceBinding{CNPJ: b.CNPJ, StationID: b.StationID})
		}
		return reader.Guard(ctx, tx, expected)
	})
	pub = pub.WithGuard(func(context.Context, pgx.Tx) error { return nil })
	records := []domain.CompanyObservationRecord{}
	if period == "2025" {
		for _, r := range nativeFineInput("a").Observations {
			records = append(records, domain.CompanyObservationRecord{RecordKey: r.ObservationKey, CNPJ: r.SourceCNPJ})
		}
	} else {
		for _, r := range historicalFineInput(period, "a").Observations {
			records = append(records, domain.CompanyObservationRecord{RecordKey: r.ObservationKey, CNPJ: r.SourceCNPJ})
		}
	}
	decisions, e := application.PlanCompanyAssociations(ctx, records, "2026-10-20", func(ctx context.Context, ids []string) (application.CompanyIdentitySnapshot, error) {
		rows, e := reader.Evidence(ctx, ids)
		out := application.CompanyIdentitySnapshot{}
		for _, r := range rows {
			out.Evidence = append(out.Evidence, domain.IdentityEvidence{CNPJ: r.CNPJ, StationID: r.StationID, Current: r.Current, StationActive: r.StationActive})
		}
		return out, e
	})
	if e != nil || len(decisions) != 2 || decisions[0].StationID != station.ID {
		t.Fatal("actual Directory policy preparation", e)
	}
	plan := &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20", Decisions: decisions}
	publish := func(pub *Publisher, plan *application.CompanyAssociationInput, hash string, next func() (string, error)) (string, error) {
		if period == "2025" {
			in := nativeFineInput(hash)
			in.CompanyAssociations = plan
			return pub.PublishNativeFines(ctx, in, next)
		}
		in := historicalFineInput(period, hash)
		in.CompanyAssociations = plan
		return pub.PublishHistoricalFines(ctx, in, next)
	}
	return p, repo, pub, plan, publish
}
func cloneCompanyPlan(in *application.CompanyAssociationInput) *application.CompanyAssociationInput {
	out := *in
	out.Decisions = append([]domain.CompanyObservationDecision(nil), in.Decisions...)
	return &out
}

func TestCompanyPublicationAtomicReplayCorrectionAndActualDirectoryGuard(t *testing.T) {
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024", "2025"} {
		t.Run(period, func(t *testing.T) {
			p, repo, pub, plan, publish := scopedFineFixture(t, period)
			ctx := context.Background()
			m := &idMaker{}
			q := quality.New(p)
			// The generic guard is insufficient; missing typed guard refuses before IDs/I/O.
			allocated := 0
			if _, e := publish(NewPublisher(nil).WithGuard(func(context.Context, pgx.Tx) error { return nil }), plan, "a", func() (string, error) { allocated++; return m.next() }); e == nil || allocated != 0 {
				t.Fatal("unrelated guard authorized company binding", e)
			}
			partial := cloneCompanyPlan(plan)
			partial.Decisions = partial.Decisions[:1]
			guardCalls := 0
			validating := NewPublisher(nil).WithCompanyGuard(func(context.Context, pgx.Tx, []application.CompanyBinding) error { guardCalls++; return nil })
			if _, e := publish(validating, partial, "a", func() (string, error) { allocated++; return m.next() }); e == nil || allocated != 0 || guardCalls != 0 {
				t.Fatal("partial typed source reached guard/database", e)
			}
			previous, e := publish(pub, nil, "a", m.next)
			if e != nil {
				t.Fatal(e)
			}
			first, e := publish(pub, plan, "a", m.next)
			if e != nil || first == previous {
				t.Fatal("declared policy did not make correction", e)
			}
			later := cloneCompanyPlan(plan)
			later.DecisionDay = "2026-10-21"
			for i := range later.Decisions {
				later.Decisions[i].DecidedAt = later.DecisionDay
			}
			replay, e := publish(pub, later, "a", m.next)
			if e != nil || replay != first {
				t.Fatal("later-day replay forked", e)
			}
			rid, _ := mustUUID(first)
			contextRow, e := q.CompanyAssociationContext(ctx, rid)
			if e != nil || contextRow.DecisionDay.Time.Format("2006-01-02") != "2026-10-20" || contextRow.ObservationCount != 2 {
				t.Fatal("frozen decision context changed", e)
			}
			old, _ := mustUUID(previous)
			if _, e := q.CompanyAssociationContext(ctx, old); !errors.Is(e, pgx.ErrNoRows) {
				t.Fatal("old unlinked source was rewritten", e)
			}
			// Guard derives expected station from the actual submitted decisions.
			another, e := repo.ResolveCNPJ(ctx, "04218406000104", "Synthetic different company", nil)
			if e != nil {
				t.Fatal(e)
			}
			wrong := cloneCompanyPlan(plan)
			for i := range wrong.Decisions {
				wrong.Decisions[i].StationID = another.ID
			}
			if _, e := publish(pub, wrong, "b", m.next); !errors.Is(e, directory.ErrSourceIdentityChanged) {
				t.Fatal("declared mismatched station bypassed actual guard", e)
			}
			if e := repo.RetireIdentifier(ctx, "CNPJ", "11222333000181"); e != nil {
				t.Fatal(e)
			}
			if _, e := publish(pub, plan, "a", m.next); !errors.Is(e, directory.ErrSourceIdentityChanged) {
				t.Fatal("retired replay skipped Directory guard", e)
			}
			correction := cloneCompanyPlan(later)
			for i := range correction.Decisions {
				d := &correction.Decisions[i]
				d.Kind = domain.ResolutionSuccession
				d.Reason = domain.ReasonSuccession
				d.StationID = ""
				d.Scope = ""
			}
			second, e := publish(pub, correction, "a", m.next)
			if e != nil || second == first {
				t.Fatal("association correction failed", e)
			}
			if _, e := publish(pub, plan, "a", m.next); !errors.Is(e, directory.ErrSourceIdentityChanged) {
				t.Fatal("old retired binding resurrected", e)
			}
			frozen, e := q.CompanyAssociationDecisions(ctx, rid)
			if e != nil || len(frozen) != 2 || !frozen[0].StationID.Valid {
				t.Fatal("correction rewrote original association", e)
			}
			current, e := q.GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: "sanction-v1"})
			if e != nil || uuidString(current) != second {
				t.Fatal("current company revision drift", e)
			}
			var revisions, contexts, decisions int
			if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_company_association_revisions),(SELECT count(*) FROM quality_company_associations)`).Scan(&revisions, &contexts, &decisions); e != nil || revisions != 3 || contexts != 2 || decisions != 4 {
				t.Fatal("failure/replay partially published company state", revisions, contexts, decisions, e)
			}
		})
	}
}

func TestCompanyPublicationConcurrentConvergenceAndLaterDecisionRollback(t *testing.T) {
	for _, period := range []string{"2025", "2021", "2016-2019"} {
		t.Run(period, func(t *testing.T) {
			p, _, pub, plan, publish := scopedFineFixture(t, period)
			m := &idMaker{}
			ctx := context.Background()
			results := make(chan string, 8)
			errs := make(chan error, 8)
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); id, e := publish(pub, plan, "a", m.next); results <- id; errs <- e }()
			}
			wg.Wait()
			close(results)
			close(errs)
			for e := range errs {
				if e != nil {
					t.Fatal("company convergence", e)
				}
			}
			first := ""
			for id := range results {
				if first == "" {
					first = id
				}
				if id != first {
					t.Fatal("company concurrent fork")
				}
			}
			// A real later-stage storage failure rolls back source/context/decision and pointer.
			if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_company_failure() RETURNS trigger LANGUAGE plpgsql AS $fail$ BEGIN IF (SELECT count(*) FROM quality_company_associations WHERE revision_id=NEW.revision_id)>0 THEN RAISE EXCEPTION 'synthetic failure'; END IF; RETURN NEW; END $fail$; CREATE TRIGGER synthetic_company_failure BEFORE INSERT ON quality_company_associations FOR EACH ROW EXECUTE FUNCTION synthetic_company_failure()`); e != nil {
				t.Fatal(e)
			}
			if _, e := publish(pub, plan, "b", m.next); e == nil {
				t.Fatal("later company failure acknowledged")
			}
			var revisions, contexts, decisions int
			if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_company_association_revisions),(SELECT count(*) FROM quality_company_associations)`).Scan(&revisions, &contexts, &decisions); e != nil || revisions != 1 || contexts != 1 || decisions != 2 {
				t.Fatal("company failure partially published", e)
			}
			current, e := quality.New(p).GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: "sanction-v1"})
			if e != nil || uuidString(current) != first {
				t.Fatal("failed correction moved pointer", e)
			}
			if _, e := p.Exec(ctx, `DROP TRIGGER synthetic_company_failure ON quality_company_associations; DROP FUNCTION synthetic_company_failure()`); e != nil {
				t.Fatal(e)
			}
			// Returning rows is insufficient: a corrupt later stage must fail final census.
			if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_company_census_failure() RETURNS trigger LANGUAGE plpgsql AS $fail$ BEGIN DELETE FROM quality_company_associations WHERE revision_id=NEW.revision_id AND record_key<>NEW.record_key; RETURN NEW; END $fail$; CREATE TRIGGER synthetic_company_census_failure AFTER INSERT ON quality_company_associations FOR EACH ROW EXECUTE FUNCTION synthetic_company_census_failure()`); e != nil {
				t.Fatal(e)
			}
			if _, e := publish(pub, plan, "c", m.next); e == nil || !strings.Contains(e.Error(), "census") {
				t.Fatal("divergent staged census accepted", e)
			}
			if e := p.QueryRow(ctx, `SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_company_association_revisions),(SELECT count(*) FROM quality_company_associations)`).Scan(&revisions, &contexts, &decisions); e != nil || revisions != 1 || contexts != 1 || decisions != 2 {
				t.Fatal("census failure partially published", e)
			}
			if _, e := p.Exec(ctx, `DROP TRIGGER synthetic_company_census_failure ON quality_company_associations; DROP FUNCTION synthetic_company_census_failure()`); e != nil {
				t.Fatal(e)
			}
			second, e := publish(pub, plan, "b", m.next)
			if e != nil || second == first {
				t.Fatal("company recovery correction failed", e)
			}
			if _, e := publish(pub, plan, "a", m.next); !errors.Is(e, application.ErrSupersededPublication) {
				t.Fatal("superseded company publication resurrected", e)
			}
			if !strings.Contains(plan.Decisions[0].Reason, "unknown") {
				t.Fatal("historical claim introduced")
			}
		})
	}
}

func TestCompanyPublicationEmptyExplicitPolicyPreservesPriorUnscopedRevision(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(p)
	m := &idMaker{}
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024", "2025"} {
		publish := func(plan *application.CompanyAssociationInput) (string, error) {
			if period == "2025" {
				in := nativeFineInput("e")
				in.Observations = nil
				in.Counts = domain.Counts{}
				in.CompanyAssociations = plan
				return pub.PublishNativeFines(ctx, in, m.next)
			}
			in := historicalFineInput(period, "e")
			in.Observations = nil
			in.Notes = nil
			in.Counts = domain.Counts{}
			in.Context.SourceRecordCount = 0
			in.Context.MetadataRecords = 0
			in.CompanyAssociations = plan
			return pub.PublishHistoricalFines(ctx, in, m.next)
		}
		old, e := publish(nil)
		if e != nil {
			t.Fatal(e)
		}
		plan := &application.CompanyAssociationInput{PolicyVersion: domain.CompanyObservationPolicyVersion, DecisionDay: "2026-10-20"}
		first, e := publish(plan)
		if e != nil || first == old {
			t.Fatal("empty declared company policy lost", e)
		}
		plan.DecisionDay = "2026-10-21"
		replay, e := publish(plan)
		if e != nil || replay != first {
			t.Fatal("empty replay changed identity", e)
		}
		rid, _ := mustUUID(first)
		frozen, e := quality.New(p).CompanyAssociationContext(ctx, rid)
		if e != nil || frozen.ObservationCount != 0 || frozen.HistoricalBusinessWindowKnown || frozen.DecisionDay.Time.Format("2006-01-02") != "2026-10-20" {
			t.Fatal("empty frozen policy changed", e)
		}
	}
}
