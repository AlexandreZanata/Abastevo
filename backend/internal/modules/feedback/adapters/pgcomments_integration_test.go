//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func commentSetup(t *testing.T) (*application.Service, string, string, string) {
	t.Helper()
	svc, pool := freshService(t)
	svc.Comments = svc.Store.(application.CommentStore)
	station := testUUID(9001)
	acc1 := seedAccount(t, pool, 1, "active")
	acc2 := seedAccount(t, pool, 2, "active")
	return svc, station, acc1, acc2
}

func TestPGCommentReplyOwnershipFlow(t *testing.T) {
	svc, station, acc1, acc2 := commentSetup(t)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "Preço bom ⛽")
	if err != nil {
		t.Fatalf("SubmitComment: %v", err)
	}
	if top.Depth != 0 {
		t.Errorf("top comment must be depth 0: %+v", top)
	}
	reply, err := svc.Reply(ctx, acc2, station, "GASOLINE_REGULAR", top.ID, "Concordo")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if reply.Depth != 1 || reply.ParentID != top.ID {
		t.Errorf("reply wrong: %+v", reply)
	}
	if _, err := svc.Reply(ctx, acc2, station, "GASOLINE_REGULAR", reply.ID, "Nested"); !errors.Is(err, domain.ErrParentInvalid) {
		t.Errorf("reply-to-reply must refuse, got %v", err)
	}
	if _, err := svc.EditComment(ctx, acc2, top.ID, "Hijack", 1); !errors.Is(err, domain.ErrNotAuthor) {
		t.Errorf("cross-author edit must refuse, got %v", err)
	}
	edited, err := svc.EditComment(ctx, acc1, top.ID, "Preço ótimo ⛽", 1)
	if err != nil {
		t.Fatalf("EditComment: %v", err)
	}
	if edited.Revision != 2 {
		t.Errorf("edit must bump revision, got %+v", edited)
	}
	if _, err := svc.EditComment(ctx, acc1, top.ID, "Stale", 1); !errors.Is(err, domain.ErrStaleRevision) {
		t.Errorf("stale revision must refuse, got %v", err)
	}
	if err := svc.DeleteComment(ctx, acc2, top.ID); !errors.Is(err, domain.ErrNotAuthor) {
		t.Errorf("cross-author delete must refuse, got %v", err)
	}
	if err := svc.DeleteComment(ctx, acc1, top.ID); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := svc.ViewComment(ctx, top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("deleted view must be not-found, got %v", err)
	}
	if _, err := svc.Reply(ctx, acc2, station, "GASOLINE_REGULAR", top.ID, "Late"); !errors.Is(err, domain.ErrParentInvalid) {
		t.Errorf("reply to tombstoned parent must refuse, got %v", err)
	}
	// The surviving reply still reads with its author alias only.
	view, err := svc.ViewComment(ctx, reply.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Alias == "" || view.Text != "Concordo" {
		t.Errorf("reply view must carry alias and verbatim text, got %+v", view)
	}
}

func TestPGPagingWalkRealDB(t *testing.T) {
	svc, station, acc1, _ := commentSetup(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "note"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page must carry cursor, got %+v", first)
	}
	second, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("second page must carry cursor, got %+v", second)
	}
	third, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", second.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Items) != 1 || third.NextCursor != "" {
		t.Errorf("last page must end cleanly, got %+v", third)
	}
	seen := map[string]bool{}
	for _, page := range []application.CommentPage{first, second, third} {
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Errorf("duplicate item %v across pages", item.ID)
			}
			seen[item.ID] = true
			if item.Alias == "" {
				t.Errorf("paged item must carry alias: %+v", item)
			}
		}
	}
	if len(seen) != 5 {
		t.Errorf("walk must cover 5 items, got %d", len(seen))
	}
	if _, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", "!!!tampered!!!", 2); err == nil {
		t.Error("tampered cursor must refuse")
	}
}

func TestPGConcurrentSameParentReplies(t *testing.T) {
	svc, station, acc1, acc2 := commentSetup(t)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	const racers = 16
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			acc := acc1
			if i%2 == 1 {
				acc = acc2
			}
			_, errs[i] = svc.Reply(ctx, acc, station, "GASOLINE_REGULAR", top.ID, "reply")
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent reply: %v", err)
		}
	}
	page, err := svc.ListReplies(ctx, top.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != racers {
		t.Errorf("all %d replies must persist, got %d", racers, len(page.Items))
	}
}

func TestPGUnsafeTextRoundTripsVerbatim(t *testing.T) {
	svc, station, acc1, _ := commentSetup(t)
	ctx := context.Background()

	raw := `<script>alert("x")</script> & "quotes"`
	got, err := svc.SubmitComment(ctx, acc1, station, "GASOLINE_REGULAR", raw)
	if err != nil {
		t.Fatalf("markup text must store: %v", err)
	}
	if got.Text != raw {
		t.Errorf("stored text must be verbatim, got %q", got.Text)
	}
	view, err := svc.ViewComment(ctx, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Text != raw {
		t.Errorf("view text must be verbatim (no rendering server-side), got %q", view.Text)
	}
}
