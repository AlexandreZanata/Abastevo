package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type Erasure struct{ Reconcile func(context.Context) error }

func (Erasure) Kind() string { return "account-erasure-reconcile" }
func (Erasure) Version() int { return 1 }
func (e Erasure) Handle(ctx context.Context, job jobs.Job) error {
	var dto struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &dto); err != nil {
		return err
	}
	if dto.Version != 1 || e.Reconcile == nil {
		return errors.New("account erasure: invalid or unconfigured job")
	}
	return e.Reconcile(ctx)
}
