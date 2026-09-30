//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
)

var modIDMu sync.Mutex
var modIDSeq int

func modUUID() string {
	modIDMu.Lock()
	defer modIDMu.Unlock()
	modIDSeq++
	return fmt.Sprintf("bbbbbbbb-2222-4222-8222-%012d", modIDSeq)
}

func moderationSetup(t *testing.T) (*application.Service, *pgxpool.Pool, string, string, string) {
	t.Helper()
	svc, pool := freshService(t)
	svc.Comments = svc.Store.(application.CommentStore)
	svc.Votes = svc.Store.(application.VoteStore)
	modStore := moderationadapters.NewStore(pool)
	svc.ReportQuota = func(context.Context, string, string) (time.Duration, error) {
		return 0, nil
	}
	svc.OpenCase = func(ctx context.Context, _, commentID, reason string) (string, error) {
		res, err := moderationapp.Open(ctx, moderationapp.Ports{
			Clock: time.Now,
			NewID: func() (string, error) { return modUUID(), nil },
			Store: modStore,
		}, moderationapp.OpenDTO{
			TargetType: moderationdomain.TargetComment,
			TargetID:   commentID,
			Reason:     reason,
		})
		if err != nil {
			return "", err
		}
		return res.CaseID, nil
	}
	station := testUUID(9001)
	author := seedAccount(t, pool, 1, "active")
	reporter := seedAccount(t, pool, 2, "active")
	return svc, pool, station, author, reporter
}

func TestPGReportFlagsAndHides(t *testing.T) {
	svc, _, station, author, reporter := moderationSetup(t)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, author, station, "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ReportComment(ctx, reporter, top.ID, "spam"); err != nil {
		t.Fatalf("ReportComment: %v", err)
	}
	raw, found, err := svc.Comments.GetComment(ctx, top.ID)
	if err != nil || !found {
		t.Fatalf("raw read: %+v %v %v", raw, found, err)
	}
	if raw.Visibility != domain.VisibilityFlagged {
		t.Errorf("report must flag, got %q", raw.Visibility)
	}
	// Flagged rows still read publicly.
	if _, err := svc.ViewComment(ctx, top.ID); err != nil {
		t.Errorf("flagged view must read, got %v", err)
	}
	if err := svc.SetVisibility(ctx, top.ID, domain.VisibilityHidden); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	// Hidden text leaks nowhere through reads…
	if _, err := svc.ViewComment(ctx, top.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("hidden view must be not-found, got %v", err)
	}
	page, err := svc.ListComments(ctx, station, "GASOLINE_REGULAR", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == top.ID {
			t.Errorf("hidden row leaked into list: %+v", item)
		}
	}
	// …but stays for audit through the raw store.
	raw, found, err = svc.Comments.GetComment(ctx, top.ID)
	if err != nil || !found || raw.Visibility != domain.VisibilityHidden {
		t.Errorf("audit read must keep hidden rows: %+v %v %v", raw, found, err)
	}
	if err := svc.SetVisibility(ctx, top.ID, domain.VisibilityVisible); err != nil {
		t.Fatalf("unhide: %v", err)
	}
	if _, err := svc.ViewComment(ctx, top.ID); err != nil {
		t.Errorf("unhidden view must read, got %v", err)
	}
}

func TestPGReporterDeletionKeepsCase(t *testing.T) {
	svc, pool, station, author, reporter := moderationSetup(t)
	ctx := context.Background()

	top, err := svc.SubmitComment(ctx, author, station, "GASOLINE_REGULAR", "Thread")
	if err != nil {
		t.Fatal(err)
	}
	caseID, err := svc.OpenCase(ctx, reporter, top.ID, "spam")
	if err != nil {
		t.Fatalf("OpenCase: %v", err)
	}
	// The reporter has no other footprint: deleting the account row
	// succeeds, proving reports pin no account (no FK, no reporter
	// column) and the case survives.
	if _, err := pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, reporter); err != nil {
		t.Fatalf("reporter delete must succeed: %v", err)
	}
	var targetType, targetID, status string
	err = pool.QueryRow(ctx,
		`SELECT target_type, target_id, status FROM moderation_cases WHERE id = $1`, caseID).
		Scan(&targetType, &targetID, &status)
	if err != nil {
		t.Fatalf("case must survive reporter deletion: %v", err)
	}
	if targetType != moderationdomain.TargetComment || targetID != top.ID || status != moderationdomain.StatusOpen {
		t.Errorf("case wrong: %v %v %v", targetType, targetID, status)
	}
	// Reporting with a deleted reporter fails closed at the gate.
	if err := svc.ReportComment(ctx, reporter, top.ID, "again"); err == nil {
		t.Error("deleted reporter must refuse")
	}
}
