package jobs

import (
	"context"
	"errors"
	"testing"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestSweepHandleRunsOnePass(t *testing.T) {
	ran := false
	h := Sweep{Run: func(context.Context) (privacyapp.SweepReport, error) {
		ran = true
		return privacyapp.SweepReport{Checked: 2, Purged: 1}, nil
	}}
	if err := h.Handle(context.Background(), jobs.Job{Kind: "privacy-training-sweep", Payload: []byte(`{"version":1}`)}); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if !ran {
		t.Error("sweep did not run")
	}
	if h.Kind() != "privacy-training-sweep" || h.Version() != 1 {
		t.Errorf("kind/version = %q/%d", h.Kind(), h.Version())
	}
}

func TestSweepHandleRejects(t *testing.T) {
	if err := (Sweep{}).Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
		t.Error("unconfigured sweep accepted")
	}
	if err := (Sweep{}).Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":2}`)}); err == nil {
		t.Error("version 2 accepted")
	}
	if err := (Sweep{}).Handle(context.Background(), jobs.Job{Payload: []byte("not json")}); err == nil {
		t.Error("non-JSON accepted")
	}
	failing := Sweep{Run: func(context.Context) (privacyapp.SweepReport, error) {
		return privacyapp.SweepReport{}, errors.New("inventory down")
	}}
	if err := failing.Handle(context.Background(), jobs.Job{Payload: []byte(`{"version":1}`)}); err == nil {
		t.Error("inventory outage accepted")
	}
}
