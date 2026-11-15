package adapters

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Queue kinds. Each runs on its own serial dispatcher (BR-RPT-009).
const (
	BuildKind   = "org-export-build"
	CleanupKind = "org-export-cleanup"
	// BuildAttempts leaves the export row's own budget (three) room to
	// settle FAILED before the queue would park the job.
	BuildAttempts   int32 = 6
	CleanupAttempts int32 = 10
)

// errPayload parks a malformed envelope: it can never become valid.
var errPayload = errors.New("orgexport: unsupported export payload")

// Payload is the reference-only versioned envelope (BR-PRIV-003).
type Payload struct {
	Version  int    `json:"version"`
	OrgID    string `json:"org_id"`
	ExportID string `json:"export_id"`
}

// Dedupe returns the queue de-duplication key of a kind and job.
func Dedupe(kind string, scope kernel.OrgScope, exportID string) string {
	if kind != BuildKind && kind != CleanupKind || scope.Validate() != nil || !domain.ValidID(exportID) {
		return ""
	}
	return kind + ":" + scope.OrgID().String() + ":" + exportID
}

// ParsePayload accepts only the exact version-1 envelope.
func ParsePayload(raw []byte) (kernel.OrgScope, string, error) {
	var p Payload
	if httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || !domain.ValidID(p.OrgID) || !domain.ValidID(p.ExportID) {
		return kernel.OrgScope{}, "", errPayload
	}
	id, err := kernel.ParseOrgID(p.OrgID)
	if err != nil {
		return kernel.OrgScope{}, "", errPayload
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}, "", errPayload
	}
	return scope, p.ExportID, nil
}

// BuildHandler runs Worker.Build for one queued job.
type BuildHandler struct{ Worker app.Worker }

// CleanupHandler runs Worker.Cleanup for one queued job.
type CleanupHandler struct{ Worker app.Worker }

var (
	_ jobs.Handler = BuildHandler{}
	_ jobs.Handler = CleanupHandler{}
)

func (BuildHandler) Kind() string { return BuildKind }
func (BuildHandler) Version() int { return 1 }

// Handle reports only static module errors to the queue, never provider text.
func (h BuildHandler) Handle(ctx context.Context, job jobs.Job) error {
	if job.Kind != BuildKind {
		return errPayload
	}
	scope, export, err := ParsePayload(job.Payload)
	if err != nil || job.DedupeKey != "" && job.DedupeKey != Dedupe(BuildKind, scope, export) {
		return errPayload
	}
	return queueError(h.Worker.Build(ctx, scope, export))
}

func (CleanupHandler) Kind() string { return CleanupKind }
func (CleanupHandler) Version() int { return 1 }

func (h CleanupHandler) Handle(ctx context.Context, job jobs.Job) error {
	if job.Kind != CleanupKind {
		return errPayload
	}
	scope, export, err := ParsePayload(job.Payload)
	if err != nil || job.DedupeKey != "" && job.DedupeKey != Dedupe(CleanupKind, scope, export) {
		return errPayload
	}
	return queueError(h.Worker.Cleanup(ctx, scope, export))
}

// queueError keeps the queue's last_error to registered static text.
func queueError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{app.ErrBusy, app.ErrNotReady, app.ErrUnavailable, app.ErrRightsLost, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return app.ErrUnavailable
}
