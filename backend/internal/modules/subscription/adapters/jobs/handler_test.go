package jobs

import (
	"context"
	platform "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"strings"
	"testing"
)

func TestJobEnvelopeAndStableOperationID(t *testing.T) {
	called := false
	handler := Handler{Name: "subscription-refresh", RunWithID: func(_ context.Context, digest, id string) error {
		called = true
		if id != "stable-job" || len(digest) != 64 {
			t.Fatal("job binding lost")
		}
		return nil
	}}
	valid := `{"version":1,"digest":"` + strings.Repeat("a", 64) + `"}`
	for _, raw := range []string{`{"version":2,"digest":"x"}`, `{"version":1,"digest":"x"}`, `{"version":1,"digest":"` + strings.Repeat("a", 64) + `","token":"forbidden"}`} {
		if handler.Handle(context.Background(), platform.Job{Payload: []byte(raw)}) == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
	if called {
		t.Fatal("invalid job executed")
	}
	if err := handler.Handle(context.Background(), platform.Job{ID: "stable-job", Payload: []byte(valid)}); err != nil || !called {
		t.Fatal("valid digest job refused", err)
	}
}
