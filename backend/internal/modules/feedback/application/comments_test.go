package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func testCommentService(gate AccountGate) (*Service, *MemStore) {
	store := NewMemStore()
	var n int
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		Comments:      store,
		CheckAccount:  gate,
		StationExists: knownStations,
		IDGen: func() (string, error) {
			n++
			return "comment-" + string(rune('0'+n)), nil
		},
	}, store
}

func TestSubmitAndReplyHappy(t *testing.T) {
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Preço bom ⛽")
	if err != nil {
		t.Fatalf("SubmitComment: %v", err)
	}
	if top.Depth != 0 || top.ParentID != "" || top.Revision != 1 || top.Scalars != 11 {
		t.Errorf("top comment wrong: %+v", top)
	}
	reply, err := svc.Reply(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", top.ID, "Concordo")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if reply.Depth != 1 || reply.ParentID != top.ID {
		t.Errorf("reply wrong: %+v", reply)
	}
	if _, err := svc.Reply(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", reply.ID, "Nested"); !errors.Is(err, domain.ErrParentInvalid) {
		t.Errorf("reply-to-reply must refuse, got %v", err)
	}
	if _, err := svc.Reply(ctx, "acc-2", "station-1", "DIESEL_S10", top.ID, "Wrong fuel"); !errors.Is(err, domain.ErrParentInvalid) {
		t.Errorf("cross-target reply must refuse, got %v", err)
	}
	if _, err := svc.Reply(ctx, "acc-2", "station-1", "GASOLINE_REGULAR", "missing", "Ghost"); !errors.Is(err, domain.ErrParentInvalid) {
		t.Errorf("missing parent must refuse, got %v", err)
	}
}

func TestCommentTextRules(t *testing.T) {
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()

	for _, raw := range []string{"", "   ", strings.Repeat("x", 281), "bad\xffbytes"} {
		if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", raw); err == nil {
			t.Errorf("text %q must refuse", raw)
		}
	}
	if _, err := svc.SubmitComment(ctx, "", "station-1", "GASOLINE_REGULAR", "ok"); err == nil {
		t.Error("empty account must refuse")
	}
	if _, err := svc.SubmitComment(ctx, "acc-1", "unknown", "GASOLINE_REGULAR", "ok"); err == nil {
		t.Error("unknown station must refuse")
	}
	denied := errors.New("feedback: suspended test")
	svc, _ = testCommentService(func(context.Context, string) error { return denied })
	if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "ok"); !errors.Is(err, domain.ErrAuthorForbidden) {
		t.Errorf("gate failure must map to author-forbidden, got %v", err)
	}
	bare := &Service{
		Clock:    &fakeClock{now: 1_700_000_000},
		Store:    NewMemStore(),
		Comments: NewMemStore(),
		IDGen: func() (string, error) {
			return "comment-1", nil
		},
	}
	if _, err := bare.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "ok"); !errors.Is(err, domain.ErrGateRequired) {
		t.Errorf("missing ports must fail closed, got %v", err)
	}
}

func TestEditRevisionAndOwnership(t *testing.T) {
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Original")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := svc.EditComment(ctx, "acc-1", top.ID, "Edited", 1)
	if err != nil {
		t.Fatalf("EditComment: %v", err)
	}
	if edited.Revision != 2 || edited.Text != "Edited" {
		t.Errorf("edit must bump revision, got %+v", edited)
	}
	if _, err := svc.EditComment(ctx, "acc-1", top.ID, "Stale", 1); !errors.Is(err, domain.ErrStaleRevision) {
		t.Errorf("stale revision must refuse, got %v", err)
	}
	if _, err := svc.EditComment(ctx, "acc-2", top.ID, "Hijack", 2); !errors.Is(err, domain.ErrNotAuthor) {
		t.Errorf("cross-author edit must refuse, got %v", err)
	}
	if _, err := svc.EditComment(ctx, "acc-1", top.ID, strings.Repeat("x", 281), 2); err == nil {
		t.Error("oversized edit must refuse without bumping")
	}
	view, err := svc.ViewComment(ctx, top.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 2 || view.Alias == "" {
		t.Errorf("view must carry revision and alias, got %+v", view)
	}
	if err := svc.DeleteComment(ctx, "acc-2", top.ID); !errors.Is(err, domain.ErrNotAuthor) {
		t.Errorf("cross-author delete must refuse, got %v", err)
	}
	if err := svc.DeleteComment(ctx, "acc-1", top.ID); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
	if _, err := svc.ViewComment(ctx, top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("deleted view must be not-found, got %v", err)
	}
	if _, err := svc.EditComment(ctx, "acc-1", top.ID, "Resurrect", 2); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("deleted edit must be not-found, got %v", err)
	}
	if err := svc.DeleteComment(ctx, "acc-1", top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("second delete must be not-found, got %v", err)
	}
}

func TestListPagingWalk(t *testing.T) {
	svc, _ := testCommentService(allowAll)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "note"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page must carry cursor, got %+v", first)
	}
	second, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", first.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("second page must carry cursor, got %+v", second)
	}
	third, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", second.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Items) != 1 || third.NextCursor != "" {
		t.Errorf("last page must end cleanly, got %+v", third)
	}
	seen := map[string]bool{}
	for _, page := range []CommentPage{first, second, third} {
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Errorf("duplicate item %v across pages", item.ID)
			}
			seen[item.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("walk must cover 5 items, got %d", len(seen))
	}
	if _, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", "!!!tampered!!!", 2); err == nil {
		t.Error("tampered cursor must refuse")
	}
	empty, err := svc.ListComments(ctx, "station-1", "DIESEL_S10", "", 20)
	if err != nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Errorf("empty target must page empty, got %+v %v", empty, err)
	}
}
