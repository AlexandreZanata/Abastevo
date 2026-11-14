package adapters

import (
	"context"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
)

// Every fact here comes from the actual source under the caller's retained root
// lock. Missing relations/providers are errors; a successful empty query is a
// known missing meter, never an always-none runtime provider.
func checkActualActivation(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, ref fleet.AssetRef, profile fleet.Profile, unitID string) error {
	queries := q.New(db)
	facts := fleet.ActivationFacts{}
	if unitID != "" {
		if err := queries.AssetAdmitUnit(ctx, q.AssetAdmitUnitParams{OrgID: scope.OrgID().String(), UnitID: unitID}); err != nil {
			return err
		}
		unit, err := fleet.NewUnitRef(scope, unitID)
		if err != nil {
			return err
		}
		facts.Unit = &unit
	}
	if profile.Kind != fleet.Implement {
		now, err := meterDBNow(ctx, queries)
		if err != nil {
			return err
		}
		at, _ := meterTime(now)
		rows, err := queries.ActualInServiceAssetMeters(ctx, q.ActualInServiceAssetMetersParams{OrgID: scope.OrgID().String(), AssetID: ref.ID().Value(), At: at})
		if err != nil {
			return err
		}
		for _, row := range rows {
			fact, err := fleet.NewMeterFact(ref, row, true)
			if err != nil {
				return err
			}
			facts.Meters = append(facts.Meters, fact)
		}
	}
	asset, err := fleet.NewDraft(ref, profile, ownedCatalog{categoryID: profile.Category})
	if err != nil {
		return err
	}
	return asset.CheckActivation(scope, facts)
}
