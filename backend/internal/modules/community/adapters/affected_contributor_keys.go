package adapters

import (
	"context"
	"errors"
	"strings"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// AffectedContributorKeys pages every exact price key affected by this owner's
// anchors or confirmations. The internal cursor is stable and contains no GPS.
func (s *Store) AffectedContributorKeys(ctx context.Context, ref string, after *domain.PriceKey, limit int) ([]domain.PriceKey, error) {
	if strings.TrimSpace(ref) == "" || len(ref) > 512 || limit < 1 || limit > 100 {
		return nil, errors.New("community: invalid trust recompute page")
	}
	params := community.AffectedContributorKeysParams{ContributorRef: ref, PageLimit: int32(limit)}
	if after != nil {
		if _, err := mustUUID(after.StationID); err != nil {
			return nil, err
		}
		params.HasCursor, params.AfterStationID, params.AfterProduct, params.AfterUnit, params.AfterCondition, params.AfterQualifier = true, after.StationID, after.Product, after.Unit, after.ConditionKind, after.Qualifier
	}
	rows, err := community.New(s.pool).AffectedContributorKeys(ctx, params)
	if err != nil {
		return nil, err
	}
	keys := make([]domain.PriceKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, domain.PriceKey{StationID: uuidString(row.StationID), Product: row.FuelProduct, Unit: row.Unit, ConditionKind: row.ConditionKind, Qualifier: row.QualifierKey})
	}
	return keys, nil
}
