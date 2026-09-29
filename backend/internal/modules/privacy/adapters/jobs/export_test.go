package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestExportBuildDelegatesOneRequest(t *testing.T) {
	var got string
	h := ExportBuild{Build: func(_ context.Context, requestID string) (string, bool, error) {
		got = requestID
		return requestID, false, nil
	}}
	payload := jobs.Job{Payload: []byte(`{"version":1,"request_id":"e1"}`)}
	if err := h.Handle(context.Background(), payload); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if got != "e1" {
		t.Errorf("request = %q", got)
	}
	if h.Kind() != "privacy-export-build" || h.Version() != 1 {
		t.Errorf("identity = %q v%d", h.Kind(), h.Version())
	}
}

func TestExportBuildRejectsBadPayload(t *testing.T) {
	h := ExportBuild{Build: func(context.Context, string) (string, bool, error) {
		return "", false, nil
	}}
	for _, raw := range []string{
		`not json`, `{"version":2,"request_id":"e1"}`,
		`{"version":1}`, `{"version":1,"request_id":""}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(raw)}); err == nil {
			t.Errorf("payload accepted: %s", raw)
		}
	}
}

func TestExportBuildPropagatesBuildFailure(t *testing.T) {
	h := ExportBuild{Build: func(context.Context, string) (string, bool, error) {
		return "", false, errors.New("inventory down")
	}}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1,"request_id":"e1"}`)}); err == nil {
		t.Error("build failure accepted")
	}
}
