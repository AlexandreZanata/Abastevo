package jobs

import (
	"context"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fakeBulletinGenerator struct {
	calls int
	err   error
}

func (f *fakeBulletinGenerator) Generate(ctx context.Context) (application.BulletinReport, error) {
	f.calls++
	return application.BulletinReport{Created: 1}, f.err
}

func TestBulletinGenerateHandlerIdentityAndPayload(t *testing.T) {
	gen := &fakeBulletinGenerator{}
	h := BulletinGenerate{Generator: gen}
	if h.Kind() != ForumBulletinGenerateKind || h.Version() != 1 {
		t.Fatalf("identity %s/%d", h.Kind(), h.Version())
	}
	if err := h.Handle(context.Background(), platformjobs.Job{Payload: []byte(`{"version":1,"revision_id":"rev-1"}`)}); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if gen.calls != 1 {
		t.Fatalf("generator calls %d", gen.calls)
	}
	if err := h.Handle(context.Background(), platformjobs.Job{Payload: []byte(`{"version":2}`)}); err == nil {
		t.Fatal("expected version refusal")
	}
	if err := h.Handle(context.Background(), platformjobs.Job{Payload: []byte(`not-json`)}); err == nil {
		t.Fatal("expected malformed payload refusal")
	}
}
