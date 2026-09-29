package jobs

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestNoServiceRefuses(t *testing.T) {
	h := Geocode{}
	if err := h.Handle(context.Background(), jobs.Job{}); err == nil {
		t.Error("nil service accepted")
	}
	if h.Kind() != "geocode-station" || h.Version() != 1 {
		t.Errorf("identity = %q/%d", h.Kind(), h.Version())
	}
}
