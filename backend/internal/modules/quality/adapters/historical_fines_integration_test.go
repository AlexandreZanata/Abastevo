//go:build integration

package adapters

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5"
)

func historicalFineInput(period, hash string) application.HistoricalFineInput {
	p, _ := application.HistoricalFineProfile(period)
	clock := application.FineHistoryClock{Precision: p.ClockPrecision, Value: "2024-06"}
	if p.ClockPrecision == "DAY" {
		clock.Value = "2020-02-06"
	}
	paid := int64(0)
	a := application.HistoricalFineObservation{Schema: "anp-fine-history-observation-v1", Source: "sanctions", SourceCNPJ: "11222333000181", ProceedingRef: "000000000000001", ReferenceLabel: p.ReferenceLabel, ReferenceRef: "SYNTHETIC/1", Status: "Synthetic source status", DueOn: "2019-02-28", AmountText: " R$ 1.234,56 ", AmountMinor: 123456, PaidText: " R$ 0,00 ", PaidMinor: &paid, SourcePeriod: period, Clock: clock, InputContract: p.Contract, EvidenceRef: p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+1)}
	if period == "2023" {
		a.PaidMinor = nil
		a.PaidText = " R$ -   "
	}
	if p.Transit {
		date := "2019-02-20"
		a.SourceTransitOn = &date
	}
	if p.Unlabelled {
		a.UnlabelledField = "SIM"
	}
	a.ObservationKey = a.ObservationIdentity()
	b := a
	b.ReferenceRef = "SYNTHETIC/2"
	b.EvidenceRef = p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+2)
	b.ObservationKey = b.ObservationIdentity()
	in := application.HistoricalFineInput{Context: application.FineHistoryContext{InputContract: p.Contract, SourcePeriod: period, SourceFile: p.SourceFile, HeaderRecord: p.HeaderRecord, Clock: clock, SourceRecordCount: 2}, Resources: []application.Resource{{URL: "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/" + p.SourceFile, SHA256: strings.Repeat("a", 64), Bytes: 100}}, Counts: domain.Counts{Input: 2, Accepted: 2}, ManifestHash: strings.Repeat(hash, 64), Observations: []application.HistoricalFineObservation{a, b}}
	if period == "2020" || period == "2021" {
		in.Context.SourceRecordCount++
		in.Context.MetadataRecords = 1
		in.Counts.Input++
		in.Counts.Quarantined = 1
		in.Notes = []application.FineHistoryNote{{Schema: "anp-fine-source-note-v1", Kind: "PUBLIC_PAYMENT_NOTE", Text: "(*) Para verificação do valor total pago dos processos com status parcelamento em andamento é preciso acessar o link: https://app.anp.gov.br/anp-gpc-consulta-web/public/emissao-gru/list.xhtml", EvidenceRef: p.SourceFile + ":" + strconv.Itoa(p.HeaderRecord+3)}}
	}
	return in
}
func TestHistoricalFineAtomicReplayCorrectionRollbackAndFrozenRead(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	pub := NewPublisher(p)
	m := &idMaker{}
	in := historicalFineInput("2023", "b")
	first, e := pub.PublishHistoricalFines(ctx, in, m.next)
	if e != nil {
		t.Fatal(e)
	}
	if again, e := pub.PublishHistoricalFines(ctx, in, m.next); e != nil || again != first {
		t.Fatal("historical replay forked", e)
	}
	if _, e = pub.WithGuard(func(context.Context, pgx.Tx) error { return errors.New("synthetic live guard refusal") }).PublishHistoricalFines(ctx, in, m.next); e == nil {
		t.Fatal("replay bypassed live guard")
	}
	r := qualityread.NewReader(p)
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	rows, next, lease, e := r.Sanctions(ctx, now, "sanctions", "sanction-v1", "", "", "", 1)
	if e != nil || len(rows) != 1 || next == "" || !lease.Stale || lease.SourceClockValue != "2024-06" || lease.SourcePeriod != "2023" || lease.SourceRecordCount == nil || *lease.SourceRecordCount != 2 {
		t.Fatal("historical source lease/page", e)
	}
	row := rows[0]
	if row.PaidMinor != nil || row.PaidText != " R$ -   " || row.DecidedOn != "" || row.AsOf != "" || row.ReferenceLabel != "Número do DUF" || row.DUFRef != "" || row.InputContract != in.Context.InputContract {
		t.Fatal("historical facts relabelled")
	}
	corrected := historicalFineInput("2023", "e")
	corrected.Context.Clock.Value = "2024-07"
	for i := range corrected.Observations {
		corrected.Observations[i].Clock = corrected.Context.Clock
		corrected.Observations[i].Status = "Source correction"
		corrected.Observations[i].ObservationKey = corrected.Observations[i].ObservationIdentity()
	}
	second, e := pub.PublishHistoricalFines(ctx, corrected, m.next)
	if e != nil || second == first {
		t.Fatal("historical correction", e)
	}
	old, _, frozen, e := r.AtRevision(first).Sanctions(ctx, now, "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(old) != 2 || frozen.Status != "superseded" || frozen.SourceClockValue != "2024-06" || old[0].Status != "Synthetic source status" {
		t.Fatal("historical correction overwrote frozen facts", e)
	}
	if _, e = pub.PublishHistoricalFines(ctx, in, m.next); !errors.Is(e, application.ErrSupersededPublication) {
		t.Fatal("superseded source replay resurrected facts", e)
	}
	calls := 0
	bad := historicalFineInput("2023", "f")
	if _, e = pub.PublishHistoricalFines(ctx, bad, func() (string, error) {
		calls++
		if calls == 4 {
			return "", errors.New("synthetic second row failure")
		}
		return m.next()
	}); e == nil {
		t.Fatal("partial historical source committed")
	}
	current, e := quality.New(p).GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: "sanction-v1"})
	if e != nil || uuidString(current) != second {
		t.Fatal("failed historical correction switched pointer", e)
	}
	var revisions, contexts int
	if e = p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_historical_fine_revisions)").Scan(&revisions, &contexts); e != nil || revisions != 2 || contexts != 2 {
		t.Fatal("partial context/revision survived", revisions, contexts, e)
	}
}
func TestHistoricalFineAllProfilesNotesAndConcurrentConvergence(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	reader := qualityread.NewReader(p)
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024"} {
		t.Run(period, func(t *testing.T) {
			in := historicalFineInput(period, "b")
			var wg sync.WaitGroup
			ids := make(chan string, 8)
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					id, e := pub.PublishHistoricalFines(ctx, in, m.next)
					if e != nil {
						t.Error("historical concurrent publication", e)
						return
					}
					ids <- id
				}()
			}
			wg.Wait()
			close(ids)
			first := ""
			n := 0
			for id := range ids {
				n++
				if first == "" {
					first = id
				} else if first != id {
					t.Fatal("historical publication forked")
				}
			}
			if n != 8 {
				t.Fatal("concurrent publication lost", n)
			}
			rows, _, lease, e := reader.AtRevision(first).Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
			if e != nil || len(rows) != 2 || lease.SourcePeriod != period || lease.SourceClockPrecision != in.Context.Clock.Precision || lease.SourceClockValue != in.Context.Clock.Value || lease.SourceMetadataRecords == nil || *lease.SourceMetadataRecords != in.Context.MetadataRecords || len(lease.SourceNotes) != len(in.Notes) {
				t.Fatal("historical profile context/notes lost", e)
			}
			for _, row := range rows {
				if row.ReferenceLabel != in.Observations[0].ReferenceLabel || row.Department != "" || row.SourceTransitOn != func() string {
					if in.Observations[0].SourceTransitOn == nil {
						return ""
					}
					return *in.Observations[0].SourceTransitOn
				}() || row.UnlabelledField != in.Observations[0].UnlabelledField {
					t.Fatal("source-specific fact lost")
				}
			}
		})
	}
	var revisions, contexts int
	if e := p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_revisions),(SELECT count(*) FROM quality_historical_fine_revisions)").Scan(&revisions, &contexts); e != nil || revisions != 6 || contexts != 6 {
		t.Fatal("history revision count", revisions, contexts, e)
	}
}
func TestHistoricalFineEmptySourceClockAndLateHistoryCannotMoveCurrent(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	current, e := pub.PublishNativeFines(ctx, nativeFineInput("a"), m.next)
	if e != nil {
		t.Fatal(e)
	}
	empty := historicalFineInput("2016-2019", "b")
	empty.Counts = domain.Counts{}
	empty.Context.SourceRecordCount = 0
	empty.Observations = nil
	id, e := pub.PublishHistoricalFines(ctx, empty, m.next)
	if e != nil {
		t.Fatal(e)
	}
	rows, _, lease, e := qualityread.NewReader(p).AtRevision(id).Sanctions(ctx, time.Now(), "sanctions", "sanction-v1", "", "", "", 10)
	if e != nil || len(rows) != 0 || !lease.HasPublished || !lease.Stale || lease.SourceClockPrecision != "DAY" || lease.SourceClockValue != "2020-02-06" || lease.SourceRecordCount == nil || *lease.SourceRecordCount != 0 || lease.IsCurrentStream {
		t.Fatal("empty historical context lost or current moved", e)
	}
	actual, e := quality.New(p).GetCurrentRevision(ctx, quality.GetCurrentRevisionParams{Source: "sanctions", Stream: "sanction-v1"})
	if e != nil || uuidString(actual) != current {
		t.Fatal("late source replaced newer current", e)
	}
}
func TestHistoricalFineNoteFailureRollsBackEntirePublication(t *testing.T) {
	p := freshPool(t)
	ctx := context.Background()
	m := &idMaker{}
	pub := NewPublisher(p)
	in := historicalFineInput("2020", "b")
	if _, e := p.Exec(ctx, `CREATE FUNCTION synthetic_history_note_failure() RETURNS trigger LANGUAGE plpgsql AS $test$ BEGIN RAISE EXCEPTION 'synthetic note failure'; END $test$; CREATE TRIGGER synthetic_history_note_failure BEFORE INSERT ON quality_historical_fine_notes FOR EACH ROW EXECUTE FUNCTION synthetic_history_note_failure()`); e != nil {
		t.Fatal(e)
	}
	if _, e := pub.PublishHistoricalFines(ctx, in, m.next); e == nil {
		t.Fatal("failed source note publication accepted")
	}
	var partial int
	if e := p.QueryRow(ctx, "SELECT (SELECT count(*) FROM quality_historical_fine_revisions)+(SELECT count(*) FROM quality_sanction_events)+(SELECT count(*) FROM quality_historical_fine_notes)+(SELECT count(*) FROM quality_current_revisions)+(SELECT count(*) FROM quality_revisions)").Scan(&partial); e != nil || partial != 0 {
		t.Fatal("partial note publication survived", partial, e)
	}
	if _, e := p.Exec(ctx, "DROP TRIGGER synthetic_history_note_failure ON quality_historical_fine_notes; DROP FUNCTION synthetic_history_note_failure()"); e != nil {
		t.Fatal(e)
	}
	if _, e := pub.PublishHistoricalFines(ctx, in, m.next); e != nil {
		t.Fatal("retry after note failure refused", e)
	}
}
