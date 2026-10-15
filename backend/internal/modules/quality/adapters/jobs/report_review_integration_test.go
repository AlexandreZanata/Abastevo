//go:build integration

package jobs

import (
	"context"
	"encoding/json"
	"errors"
	moderation "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestReportReviewSQLReplayStaleAndWithdrawal(t *testing.T) {
	p := freshJobsDB(t)
	ctx := context.Background()
	owner := "11111111-1111-4111-8111-111111111111"
	station := "22222222-2222-4222-8222-222222222222"
	id := "33333333-3333-4333-8333-333333333333"
	for _, q := range []string{"INSERT INTO accounts(id,alias,status) VALUES('" + owner + "','synthetic-review','active')", "INSERT INTO directory_stations(id,display_name) VALUES('" + station + "','Synthetic review station')"} {
		if _, e := p.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	store := &adapters.ReportStore{Pool: p, Now: time.Now, Station: func(context.Context, string) (bool, error) { return true, nil }}
	access := adapters.ReportAccess{Owner: owner, Guard: func(context.Context, pgx.Tx) error { return nil }}
	command := domain.ReportCommand{CommandID: "44444444-4444-4444-8444-444444444444", ReportID: id, Action: "SUBMIT", Kind: "EXPERIENCE", StationID: station, Fuel: "ETHANOL", OccurredOn: "2026-10-13", Symptom: "OTHER", Text: "Synthetic private experience"}
	if _, e := store.Execute(ctx, access, command); e != nil {
		t.Fatal(e)
	}
	payload, _ := json.Marshal(adapters.ReportReviewPayload{Version: 1, ReportID: id, Revision: 1})
	job := platformjobs.Job{Payload: payload}
	h := ReportReview{Pool: p, Open: func(context.Context, pgx.Tx, string, string) error { return errors.New("synthetic moderation outage") }}
	if e := h.Handle(ctx, job); e == nil {
		t.Fatal("outage hidden")
	}
	m := moderation.NewStore(p)
	h.Open = func(ctx context.Context, tx pgx.Tx, id, kind string) error {
		caseID, e := platformjobs.NewUUIDv4()
		if e != nil {
			return e
		}
		c, _, e := moderationdomain.NewCase(moderationdomain.CaseParams{ID: caseID, TargetType: moderationdomain.TargetQualityReport, TargetID: id, Priority: "P2", Reason: kind, OpenedAt: time.Now()})
		if e != nil {
			return e
		}
		_, _, e = m.OpenCaseTx(ctx, tx, c)
		return e
	}
	for i := 0; i < 2; i++ {
		if e := h.Handle(ctx, job); e != nil {
			t.Fatal(e)
		}
	}
	var n int
	_ = p.QueryRow(ctx, "SELECT count(*) FROM moderation_cases WHERE target_type='QUALITY_REPORT'").Scan(&n)
	if n != 1 {
		t.Fatal("review fork", n)
	}
	if _, e := store.Execute(ctx, access, domain.ReportCommand{CommandID: "55555555-5555-4555-8555-555555555555", ReportID: id, Action: "WITHDRAW", ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	h.Open = func(context.Context, pgx.Tx, string, string) error { t.Fatal("withdrawn report reopened"); return nil }
	if e := h.Handle(ctx, job); e != nil {
		t.Fatal(e)
	}
	if _, e := store.EraseAccountBatch(ctx, owner); e != nil {
		t.Fatal(e)
	}
	if e := h.Handle(ctx, job); e != nil {
		t.Fatal("erased replay", e)
	}
	job.Payload = []byte(`{"version":2}`)
	if e := h.Handle(ctx, job); e == nil {
		t.Fatal("unknown version")
	}
}
