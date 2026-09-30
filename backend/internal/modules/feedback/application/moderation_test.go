package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
)

func testModerationService() (*Service, *MemStore, *int, *[]string) {
	store := NewMemStore()
	var quotaCalls int
	var opened []string
	return &Service{
		Clock:         &fakeClock{now: 1_700_000_000},
		Store:         store,
		Comments:      store,
		Votes:         store,
		CheckAccount:  allowAll,
		StationExists: knownStations,
		ReportQuota: func(context.Context, string, string) (time.Duration, error) {
			quotaCalls++
			return 0, nil
		},
		OpenCase: func(_ context.Context, accountID, commentID, reason string) (string, error) {
			opened = append(opened, accountID+"/"+commentID+"/"+reason)
			return "case-1", nil
		},
		IDGen: func() (string, error) {
			return "comment-1", nil
		},
	}, store, &quotaCalls, &opened
}

func TestReportFlagsAndOpensCase(t *testing.T) {
	svc, store, quotaCalls, opened := testModerationService()
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReportComment(ctx, "acc-2", top.ID, "spam"); err != nil {
		t.Fatalf("ReportComment: %v", err)
	}
	if *quotaCalls != 1 || len(*opened) != 1 || !strings.Contains((*opened)[0], top.ID+"/spam") {
		t.Errorf("report must quota-check and open exactly one case, got %d %v", *quotaCalls, *opened)
	}
	store.mu.Lock()
	visibility := store.comments[top.ID].Visibility
	store.mu.Unlock()
	if visibility != domain.VisibilityFlagged {
		t.Errorf("first report must flag, got %q", visibility)
	}
	// Second report converges on the flag without error.
	if err := svc.ReportComment(ctx, "acc-2", top.ID, "spam again"); err != nil {
		t.Fatalf("repeat report: %v", err)
	}
	if len(*opened) != 2 {
		t.Errorf("repeat reports open converging cases, got %v", *opened)
	}
}

func TestReportValidation(t *testing.T) {
	svc, _, _, _ := testModerationService()
	ctx := context.Background()
	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"", "   ", strings.Repeat("x", 201)} {
		if err := svc.ReportComment(ctx, "acc-2", top.ID, reason); !errors.Is(err, domain.ErrReportInvalid) {
			t.Errorf("reason %q must refuse, got %v", reason, err)
		}
	}
	if err := svc.ReportComment(ctx, "acc-2", "missing", "spam"); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("missing target must be not-found, got %v", err)
	}
	denied := errors.New("feedback: suspended test")
	svc.CheckAccount = func(context.Context, string) error { return denied }
	if err := svc.ReportComment(ctx, "acc-2", top.ID, "spam"); !errors.Is(err, domain.ErrAuthorForbidden) {
		t.Errorf("gate failure must map to author-forbidden, got %v", err)
	}
	bare := &Service{Clock: &fakeClock{now: 1_700_000_000}, Store: NewMemStore(), Comments: NewMemStore(), Votes: NewMemStore()}
	if err := bare.ReportComment(ctx, "acc-2", top.ID, "spam"); !errors.Is(err, domain.ErrGateRequired) {
		t.Errorf("missing ports must fail closed, got %v", err)
	}
}

func TestReportQuotaDenies(t *testing.T) {
	svc, _, _, _ := testModerationService()
	svc.ReportQuota = func(context.Context, string, string) (time.Duration, error) {
		return 30 * time.Second, &domain.QuotaDeniedError{RetryAfterSeconds: 30}
	}
	ctx := context.Background()
	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	err = svc.ReportComment(ctx, "acc-2", top.ID, "spam")
	var denied *domain.QuotaDeniedError
	if !errors.As(err, &denied) || denied.RetryAfterSeconds != 30 {
		t.Errorf("quota denial must propagate with delay, got %v", err)
	}
}

func TestSetVisibilityLane(t *testing.T) {
	svc, _, _, _ := testModerationService()
	ctx := context.Background()
	top, err := svc.SubmitComment(ctx, "acc-1", "station-1", "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetVisibility(ctx, top.ID, domain.VisibilityHidden); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	if _, err := svc.ViewComment(ctx, top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("hidden view must be not-found, got %v", err)
	}
	page, err := svc.ListComments(ctx, "station-1", "GASOLINE_REGULAR", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Errorf("hidden rows must leave reads, got %+v", page)
	}
	if err := svc.SetVisibility(ctx, top.ID, domain.VisibilityVisible); err != nil {
		t.Fatalf("unhide: %v", err)
	}
	if _, err := svc.ViewComment(ctx, top.ID); err != nil {
		t.Errorf("unhidden view must read, got %v", err)
	}
	if err := svc.SetVisibility(ctx, top.ID, "removed"); !errors.Is(err, domain.ErrVisibilityInvalid) {
		t.Errorf("unknown visibility must refuse, got %v", err)
	}
	if err := svc.DeleteComment(ctx, "acc-1", top.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetVisibility(ctx, top.ID, domain.VisibilityVisible); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("tombstoned visibility must refuse, got %v", err)
	}
	if err := svc.SetVisibility(ctx, "missing", domain.VisibilityHidden); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("missing visibility must be not-found, got %v", err)
	}
}
