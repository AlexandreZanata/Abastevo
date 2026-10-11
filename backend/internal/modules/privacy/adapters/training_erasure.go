package adapters

import (
	"context"
	"time"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
)

// TrainingErasure wires account erasure over the training consent and
// derivative stores (CS10D): consent ends REVOKED, owned derivatives
// flip to tombstones, and owned copies delete immediately. It
// implements the application TrainingErasure port, so the erasure
// runner and the restore replay share one scope implementation.
type TrainingErasure struct {
	Clock       func() time.Time
	Attribution func(ctx context.Context, contributorID string) (string, error)
	Consents    *ConsentStore
	Terms       privacyapp.TermsProvider
	Inventory   *PromotionStore
	Objects     privacyapp.ObjectStore
}

var _ privacyapp.TrainingErasure = (*TrainingErasure)(nil)

// EraseContributor revokes one owner's training consent and purges
// its derivatives and bytes.
func (t TrainingErasure) EraseContributor(ctx context.Context, contributorID string) (int, int, error) {
	clock := t.Clock
	if clock == nil {
		clock = time.Now
	}
	return privacyapp.EraseTrainingContributor(ctx, privacyapp.TrainingErasePorts{
		Clock: clock, Attribution: t.Attribution,
		Consent: privacyapp.ConsentPorts{
			Clock: clock, Attribution: t.Attribution,
			Store: t.Consents, Terms: t.Terms,
		},
		Inventory: t.Inventory,
		Objects:   t.Objects,
	}, contributorID)
}
