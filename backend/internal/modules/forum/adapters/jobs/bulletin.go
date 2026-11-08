package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const ForumBulletinGenerateKind = "forum-bulletin-generate"

// BulletinGeneratePayload identifies the accepted-revision trigger. The
// generator re-reads the current accepted pair and is idempotent.
type BulletinGeneratePayload struct {
	Version    int    `json:"version"`
	RevisionID string `json:"revision_id,omitempty"`
}

// BulletinGenerator is the bounded application generator port.
type BulletinGenerator interface {
	Generate(ctx context.Context) (application.BulletinReport, error)
}

// BulletinGenerate is the worker handler for deterministic bulletin drafts.
type BulletinGenerate struct {
	Generator BulletinGenerator
}

func (BulletinGenerate) Kind() string { return ForumBulletinGenerateKind }
func (BulletinGenerate) Version() int { return 1 }

func (h BulletinGenerate) Handle(ctx context.Context, job platformjobs.Job) error {
	var payload BulletinGeneratePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("forum-bulletin-generate: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("forum-bulletin-generate: unsupported payload version %d", payload.Version)
	}
	if h.Generator == nil {
		return fmt.Errorf("forum-bulletin-generate: generator not initialized")
	}
	_, err := h.Generator.Generate(ctx)
	return err
}
