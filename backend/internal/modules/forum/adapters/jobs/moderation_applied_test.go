package jobs

import (
	"context"
	"encoding/json"
	"testing"

	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fakeSettler struct {
	caseID, status string
	calls          int
}

func (f *fakeSettler) SettleReportsByCase(_ context.Context, caseID, status string) (int64, error) {
	f.caseID, f.status = caseID, status
	f.calls++
	return 1, nil
}

func moderationJob(t *testing.T, p ModerationAppliedPayload) platformjobs.Job {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return platformjobs.Job{Kind: ForumModerationAppliedKind, Payload: raw}
}

func TestModerationApplied_SettlesReportsByAction(t *testing.T) {
	const caseID = "cccccccc-0000-4ccc-8ccc-000000000001"
	cases := []struct {
		action, status string
		calls          int
	}{
		{"INVALIDATE", "RESOLVED", 1},
		{"RESOLVE", "RESOLVED", 1},
		{"DISMISS", "DISMISSED", 1},
		{"REVIEW", "", 0},
	}
	for _, tc := range cases {
		settler := &fakeSettler{}
		h := ModerationApplied{Reports: settler}
		err := h.Handle(context.Background(), moderationJob(t, ModerationAppliedPayload{
			Version: 1, CaseID: caseID, TargetType: "FORUM_COMMENT", TargetID: "x", ActionID: "a", Action: tc.action,
		}))
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.action, err)
		}
		if settler.calls != tc.calls || settler.status != tc.status {
			t.Fatalf("%s: calls=%d status=%q, want %d %q", tc.action, settler.calls, settler.status, tc.calls, tc.status)
		}
		if tc.calls == 1 && settler.caseID != caseID {
			t.Fatalf("%s: settled case %q", tc.action, settler.caseID)
		}
	}
}

func TestModerationApplied_RejectsInvalidPayloads(t *testing.T) {
	h := ModerationApplied{Reports: &fakeSettler{}}
	bad := []ModerationAppliedPayload{
		{Version: 2, CaseID: "c", TargetType: "FORUM_POST", Action: "RESOLVE"},
		{Version: 1, CaseID: "", TargetType: "FORUM_POST", Action: "RESOLVE"},
		{Version: 1, CaseID: "c", TargetType: "OBSERVATION", Action: "RESOLVE"},
	}
	for _, p := range bad {
		if err := h.Handle(context.Background(), moderationJob(t, p)); err == nil {
			t.Fatalf("expected error for %+v", p)
		}
	}
	if err := h.Handle(context.Background(), platformjobs.Job{Payload: []byte("{")}); err == nil {
		t.Fatal("expected error for malformed payload")
	}
	if err := (ModerationApplied{}).Handle(context.Background(), moderationJob(t, ModerationAppliedPayload{
		Version: 1, CaseID: "c", TargetType: "FORUM_POST", Action: "DISMISS",
	})); err == nil {
		t.Fatal("expected error without a settler")
	}
}
