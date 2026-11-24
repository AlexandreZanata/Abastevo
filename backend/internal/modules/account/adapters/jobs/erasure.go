package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type Erasure struct {
	Reconcile          func(context.Context) error
	PurgeEmailCodes    func(context.Context) error
	PurgeRecoveryAudit func(context.Context) error
}

func (Erasure) Kind() string { return "account-erasure-reconcile" }
func (Erasure) Version() int { return 1 }
func (e Erasure) Handle(ctx context.Context, job jobs.Job) error {
	var dto struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &dto); err != nil {
		return err
	}
	if dto.Version != 1 || e.Reconcile == nil || e.PurgeEmailCodes == nil || e.PurgeRecoveryAudit == nil {
		return errors.New("account erasure: invalid or unconfigured job")
	}
	// Keep each bounded cleanup independent so a failure in one
	// category does not defer the other categories until a retry.
	return errors.Join(e.Reconcile(ctx), e.PurgeEmailCodes(ctx), e.PurgeRecoveryAudit(ctx))
}
