package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func footprintService() (*Service, *MemStore) {
	store := NewMemStore()
	var n int
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		Comments:      store,
		Votes:         store,
		CheckAccount:  allowAll,
		StationExists: knownStations,
		IDGen: func() (string, error) {
			n++
			return "fp-" + string(rune('a'+n)), nil
		},
	}, store
}

func mustRate(t *testing.T, svc *Service, acc, station, product string, stars int) {
	t.Helper()
	if _, err := svc.Rate(context.Background(), acc, station, product, stars); err != nil {
		t.Fatalf("Rate %s: %v", acc, err)
	}
}

func TestExportAccountRedactsToOwner(t *testing.T) {
	svc, _ := footprintService()
	ctx := context.Background()

	mustRate(t, svc, "acc-1", "station-1", "GASOLINE_REGULAR", 5)
	mustRate(t, svc, "acc-2", "station-1", "GASOLINE_REGULAR", 3)

	c1, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "acc-1 thread")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := svc.SubmitComment(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", "acc-2 thread")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Vote(ctx, "acc-2", c1.ID, domain.VoteValid); err != nil {
		t.Fatalf("vote c1: %v", err)
	}
	if _, err := svc.Vote(ctx, "acc-1", c2.ID, domain.VoteValid); err != nil {
		t.Fatalf("vote c2: %v", err)
	}

	env, err := svc.ExportAccount(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ExportAccount: %v", err)
	}
	if env.Format != FeedbackExportFormat {
		t.Errorf("format must be %q, got %q", FeedbackExportFormat, env.Format)
	}
	if env.AccountID != "acc-1" || env.Footprint.AccountID != "acc-1" {
		t.Errorf("envelope must name acc-1, got %+v", env)
	}
	if len(env.Footprint.Ratings) != 1 || env.Footprint.Ratings[0].Stars != 5 {
		t.Errorf("acc-1 export must hold one 5-star rating, got %+v", env.Footprint.Ratings)
	}
	if len(env.Footprint.Comments) != 1 || env.Footprint.Comments[0].ID != c1.ID {
		t.Errorf("acc-1 export must hold only c1, got %+v", env.Footprint.Comments)
	}
	if len(env.Footprint.Votes) != 1 || env.Footprint.Votes[0].CommentID != c2.ID {
		t.Errorf("acc-1 export must hold only the c2 vote, got %+v", env.Footprint.Votes)
	}
	for _, r := range env.Footprint.Ratings {
		if strings.Contains(r.ID, "acc-2") {
			t.Errorf("export leaked foreign rating: %+v", r)
		}
	}
	raw, err := svc.ExportAccountBytes(ctx, "acc-1")
	if err != nil {
		t.Fatalf("ExportAccountBytes: %v", err)
	}
	var decoded ExportEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("export bytes must be JSON: %v", err)
	}
	if decoded.Footprint.AccountID != "acc-1" || len(decoded.Footprint.Comments) != 1 {
		t.Errorf("decoded envelope wrong: %+v", decoded)
	}
	again, err := svc.ExportAccountBytes(ctx, "acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(again) {
		t.Errorf("export must be deterministic")
	}
	if _, err := svc.ExportAccount(ctx, "   "); !errors.Is(err, domain.ErrTargetInvalid) {
		t.Errorf("blank account must refuse, got %v", err)
	}
	empty, err := svc.ExportAccount(ctx, "ghost")
	if err != nil {
		t.Fatalf("unknown owner exports empty: %v", err)
	}
	if len(empty.Footprint.Ratings) != 0 || len(empty.Footprint.Comments) != 0 || len(empty.Footprint.Votes) != 0 {
		t.Errorf("unknown owner must export empty sections, got %+v", empty.Footprint)
	}
}

func TestEraseAccountTombstonesAndRebuilds(t *testing.T) {
	svc, _ := footprintService()
	ctx := context.Background()

	mustRate(t, svc, "acc-1", "station-1", "GASOLINE_REGULAR", 5)
	mustRate(t, svc, "acc-2", "station-1", "GASOLINE_REGULAR", 3)

	c1, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "acc-1 thread")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := svc.SubmitComment(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", "acc-2 thread")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := svc.Reply(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", c1.ID, "acc-2 reply")
	if err != nil {
		t.Fatalf("reply: %v", err)
	}
	if _, err := svc.Vote(ctx, "acc-2", c1.ID, domain.VoteValid); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Vote(ctx, "acc-1", c2.ID, domain.VoteValid); err != nil {
		t.Fatal(err)
	}

	report, err := svc.EraseAccount(ctx, "acc-1")
	if err != nil {
		t.Fatalf("EraseAccount: %v", err)
	}
	if report.AccountID != "acc-1" || report.Ratings != 1 || report.Comments != 1 || report.Votes != 1 {
		t.Errorf("erase must tombstone 1/1/1, got %+v", report)
	}
	if report.RatingKeys != 1 || report.Tallies != 1 {
		t.Errorf("erase must rebuild 1 key + 1 tally, got %+v", report)
	}

	stats, found, err := svc.Store.Stats(ctx, "station-1", "GASOLINE_REGULAR")
	if err != nil || !found {
		t.Fatalf("stats must persist: %+v %v %v", stats, found, err)
	}
	if stats.Count != 1 || stats.Sum != 3 {
		t.Errorf("stats must be 1/3 after erase, got %+v", stats)
	}
	rebuilt, err := svc.RebuildStats(ctx, "station-1", "GASOLINE_REGULAR")
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt != stats {
		t.Errorf("maintained stats must equal rebuild: %+v vs %+v", stats, rebuilt)
	}

	if _, err := svc.ViewComment(ctx, c1.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("erased comment must vanish from views, got %v", err)
	}
	page, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == c1.ID {
			t.Errorf("erased comment leaked into list: %+v", item)
		}
	}
	if _, err := svc.ViewComment(ctx, c2.ID); err != nil {
		t.Errorf("survivor comment must stay readable, got %v", err)
	}
	if _, err := svc.ViewComment(ctx, reply.ID); err != nil {
		t.Errorf("others' reply on erased parent stays readable, got %v", err)
	}
	tally, err := svc.Tally(ctx, c2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tally.Valid != 0 || tally.Invalid != 0 {
		t.Errorf("c2 tally must be 0/0 after vote erase, got %+v", tally)
	}
	agreement, err := tally.Agreement()
	if err != nil {
		t.Fatal(err)
	}
	if agreement.HasVotes {
		t.Errorf("zero votes must be null agreement, got %+v", agreement)
	}

	second, err := svc.EraseAccount(ctx, "acc-1")
	if err != nil {
		t.Fatalf("replay erase: %v", err)
	}
	if second.Ratings != 0 || second.Comments != 0 || second.Votes != 0 {
		t.Errorf("replay must converge zero, got %+v", second)
	}

	after, err := svc.ExportAccount(ctx, "acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Footprint.Ratings) != 1 || after.Footprint.Ratings[0].DeletedAt == 0 {
		t.Errorf("export after erase keeps tombstoned history, got %+v", after.Footprint.Ratings)
	}
	if len(after.Footprint.Comments) != 1 || after.Footprint.Comments[0].DeletedAt == 0 {
		t.Errorf("export after erase keeps comment history, got %+v", after.Footprint.Comments)
	}

	if _, err := svc.EraseAccount(ctx, "  "); !errors.Is(err, domain.ErrTargetInvalid) {
		t.Errorf("blank erase must refuse, got %v", err)
	}
	bare := &Service{}
	if _, err := bare.EraseAccount(ctx, "acc-1"); !errors.Is(err, domain.ErrGateRequired) {
		t.Errorf("missing ports must fail closed, got %v", err)
	}
}
