package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestRetentionRunsSweepAndLogs(t *testing.T) {
	var logged []any
	h := Retention{
		Run: func(context.Context) (privacyapp.RetentionReport, error) {
			return privacyapp.RetentionReport{Categories: []privacyapp.CategoryReport{
				{Category: "challenges", Purged: 7},
			}}, nil
		},
		Log: func(_ string, args ...any) { logged = args },
	}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	joined := strings.Join(fmtArgs(logged), " ")
	if !strings.Contains(joined, "challenges/purged") {
		t.Errorf("log = %v", logged)
	}
	if h.Kind() != "privacy-retention" || h.Version() != 1 {
		t.Errorf("identity = %q v%d", h.Kind(), h.Version())
	}
}

func fmtArgs(args []any) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, fmt.Sprintf("%v", a))
	}
	return out
}

func TestRetentionRejectsBadPayload(t *testing.T) {
	h := Retention{Run: func(context.Context) (privacyapp.RetentionReport, error) {
		return privacyapp.RetentionReport{}, nil
	}}
	for _, raw := range []string{`not json`, `{"version":2}`, `{}`} {
		if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(raw)}); err == nil {
			t.Errorf("payload accepted: %s", raw)
		}
	}
}

func TestRetentionPropagatesSweepFailure(t *testing.T) {
	boom := errors.New("purge down")
	h := Retention{Run: func(context.Context) (privacyapp.RetentionReport, error) {
		return privacyapp.RetentionReport{}, boom
	}}
	if err := h.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); !errors.Is(err, boom) {
		t.Errorf("handle = %v", err)
	}
}
