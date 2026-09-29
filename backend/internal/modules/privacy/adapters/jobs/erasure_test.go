package jobs

import (
	"context"
	"errors"
	"testing"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestErasureDelegatesOneRequest(t *testing.T) {
	var gotID, gotReason string
	h := Erasure{
		Load: func(context.Context, string) (privacydomain.Request, error) {
			return privacydomain.Request{
				ID: "d1", ContributorID: "c1", ClientSubmissionID: "erase-1",
				Type: privacydomain.TypeDeletion, Status: privacydomain.StatusRequested,
			}, nil
		},
		Erase: func(_ context.Context, contributorID string, dto privacyapp.EraseDTO) (privacyapp.ErasureReport, error) {
			gotID, gotReason = contributorID, dto.Reason
			return privacyapp.ErasureReport{}, nil
		},
	}
	payload := jobs.Job{Payload: []byte(`{"version":1,"request_id":"d1","reason":"owner request"}`)}
	if err := h.Handle(context.Background(), payload); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if gotID != "c1" || gotReason != "owner request" {
		t.Errorf("delegated = %q %q", gotID, gotReason)
	}
	if h.Kind() != "privacy-erasure" || h.Version() != 1 {
		t.Errorf("identity = %q v%d", h.Kind(), h.Version())
	}
}

func TestErasureReplaysTerminal(t *testing.T) {
	called := false
	h := Erasure{
		Load: func(context.Context, string) (privacydomain.Request, error) {
			return privacydomain.Request{Status: privacydomain.StatusReady}, nil
		},
		Erase: func(context.Context, string, privacyapp.EraseDTO) (privacyapp.ErasureReport, error) {
			called = true
			return privacyapp.ErasureReport{}, nil
		},
	}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1,"request_id":"d1"}`)}); err != nil {
		t.Fatalf("terminal replay = %v", err)
	}
	if called {
		t.Error("terminal request re-executed")
	}
}

func TestErasureRejectsBadPayload(t *testing.T) {
	h := Erasure{
		Load: func(context.Context, string) (privacydomain.Request, error) {
			return privacydomain.Request{}, nil
		},
		Erase: func(context.Context, string, privacyapp.EraseDTO) (privacyapp.ErasureReport, error) {
			return privacyapp.ErasureReport{}, nil
		},
	}
	for _, raw := range []string{
		`not json`, `{"version":2,"request_id":"d1"}`,
		`{"version":1}`, `{"version":1,"request_id":""}`,
	} {
		if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(raw)}); err == nil {
			t.Errorf("payload accepted: %s", raw)
		}
	}
}

func TestErasurePropagatesFailure(t *testing.T) {
	h := Erasure{
		Load: func(context.Context, string) (privacydomain.Request, error) {
			return privacydomain.Request{Status: privacydomain.StatusRequested}, nil
		},
		Erase: func(context.Context, string, privacyapp.EraseDTO) (privacyapp.ErasureReport, error) {
			return privacyapp.ErasureReport{}, errors.New("community down")
		},
	}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1,"request_id":"d1"}`)}); err == nil {
		t.Error("erase failure accepted")
	}
}
