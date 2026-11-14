//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

// BR-FLEET-011 stays intact: recording an acquisition is not an operation, so
// an unused DRAFT remains discardable; its receipts survive the discard.
func TestFleetLifecycleAcquiredDraftRemainsDiscardable(t *testing.T) {
	fx, s, _ := lifecycleFixture(t)
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	acquisition := lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.RecordAcquisition, created.AssetID, 1))
	acquired, _, err := lifecycleWrite(t, fx, s, acquisition)
	if err != nil || acquired.State != fleet.Draft || acquired.Revision != 2 {
		t.Fatal("draft acquisition", err)
	}
	gone, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.DiscardAsset, AssetID: created.AssetID, ExpectedRevision: 2}))
	if err != nil || !gone.Discarded {
		t.Fatal("acquired unused draft could not be discarded", err)
	}
	if replay, replayed, err := lifecycleWrite(t, fx, s, acquisition); err != nil || !replayed || replay != acquired {
		t.Fatal("acquisition receipt lost with the discarded draft", err)
	}
	if _, err := s.ListAssetLifecycle(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, fa.LifecycleFilter{Limit: 10}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("discarded draft history readable", err)
	}
}
