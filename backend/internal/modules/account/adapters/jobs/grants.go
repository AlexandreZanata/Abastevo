package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// GrantRevocationKind is the recurring reconcile of the provider grant
// revocation queue (P13-T05): deleted accounts' sealed Sign in with Apple
// refresh tokens are revoked at the provider.
const GrantRevocationKind = "account-provider-revoke"

// GrantRevocation runs one bounded reconcile per job.
type GrantRevocation struct{ Reconcile func(context.Context) error }

func (GrantRevocation) Kind() string { return GrantRevocationKind }
func (GrantRevocation) Version() int { return 1 }
func (g GrantRevocation) Handle(ctx context.Context, job jobs.Job) error {
	var dto struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &dto); err != nil {
		return err
	}
	if dto.Version != 1 || g.Reconcile == nil {
		return errors.New("account grant revocation: invalid or unconfigured job")
	}
	return g.Reconcile(ctx)
}
