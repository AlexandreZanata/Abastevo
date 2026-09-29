package jobs

import (
	"context"
	"errors"
	"testing"

	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestHandleRejectsBadPayload(t *testing.T) {
	h := Validate{Run: func(context.Context, string, string) (string, error) { return "VALIDATED", nil }}
	for name, raw := range map[string]string{
		"malformed":  `{`,
		"version":    `{"version":2,"observation_id":"o1"}`,
		"empty-id":   `{"version":1,"observation_id":"  "}`,
		"missing-id": `{"version":1}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(raw)}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (Validate{}).Handle(context.Background(), jobs.Job{ID: "job-1", Payload: []byte(`{"version":1,"observation_id":"o1"}`)}); err == nil {
		t.Error("nil orchestration accepted")
	}
}

func TestHandleDelegatesCommandProof(t *testing.T) {
	var gotID, gotCmd string
	h := Validate{Run: func(_ context.Context, id, cmd string) (string, error) {
		gotID, gotCmd = id, cmd
		return "VALIDATED", nil
	}}
	job := jobs.Job{ID: "job-9", Payload: []byte(`{"version":1,"observation_id":"obs-1"}`)}
	if err := h.Handle(context.Background(), job); err != nil {
		t.Fatalf("handle = %v", err)
	}
	// The persisted job ID is the command proof for the claim.
	if gotID != "obs-1" || gotCmd != "job-9" {
		t.Errorf("delegated = %q/%q", gotID, gotCmd)
	}
}

func TestHandlePropagatesPendingAndTransient(t *testing.T) {
	pending := Validate{Run: func(context.Context, string, string) (string, error) {
		return "VALIDATING", communityapp.ErrPending
	}}
	if err := pending.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"observation_id":"o"}`)}); !errors.Is(err, communityapp.ErrPending) {
		t.Errorf("pending = %v", err)
	}
	boom := errors.New("directory down")
	failing := Validate{Run: func(context.Context, string, string) (string, error) { return "", boom }}
	if err := failing.Handle(context.Background(), jobs.Job{ID: "j", Payload: []byte(`{"version":1,"observation_id":"o"}`)}); !errors.Is(err, boom) {
		t.Errorf("transient = %v", err)
	}
}
