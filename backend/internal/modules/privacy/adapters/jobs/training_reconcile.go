package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// TrainingReconcile materializes one bounded batch of durable revocation
// watermarks. Selection refuses immediately, independent of this job.
type TrainingReconcile struct {
	Run func(context.Context) (int64, error)
}

func (TrainingReconcile) Kind() string { return "privacy-training-reconcile" }
func (TrainingReconcile) Version() int { return 1 }
func (h TrainingReconcile) Handle(ctx context.Context, job jobs.Job) error {
	var dto struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &dto); err != nil {
		return err
	}
	if dto.Version != 1 || h.Run == nil {
		return errors.New("training reconcile: invalid or unconfigured job")
	}
	_, err := h.Run(ctx)
	return err
}
