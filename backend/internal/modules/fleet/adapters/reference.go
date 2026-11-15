package adapters

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5/pgtype"
)

// ReferenceAssetsDBTX reads the minimised reference entities of the assets
// assigned to the membership at the given database instant (EMP-OFFL-006,
// EDR-216): FLEET-006 periods intersected with the membership period, the
// in-service meters with their open segment and the accepted tank products.
// It reads nothing else and grants nothing; the caller checks coverage.
func (s *AssetStore) ReferenceAssetsDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, member string, at time.Time) ([]syncapp.ReferenceAssetSource, error) {
	db = syncapp.Borrow(db)
	if s == nil || db == nil || !validID(member) || at.IsZero() || database.ValidateOrgDBTX(ctx, db, scope) != nil {
		return nil, syncapp.ErrUnavailable
	}
	org := scope.OrgID().String()
	rows, err := db.Query(ctx, `SELECT a.id::text,a.revision,a.kind,a.usage_basis,a.state,a.name,a.fleet_number,a.plate,COALESCE(a.unit_id::text,''),x.kind,x.valid_to
 FROM org_assets a JOIN LATERAL(SELECT s.kind,s.valid_to FROM org_asset_assignments s JOIN org_memberships m ON(m.org_id,m.employee_id)=(s.org_id,s.employee_id)
  WHERE s.org_id=a.org_id AND s.asset_id=a.id AND m.id=$2::uuid AND m.started_at IS NOT NULL AND m.started_at<=$3 AND(m.ended_at IS NULL OR $3<m.ended_at)
   AND s.valid_from<=$3 AND(s.valid_to IS NULL OR $3<s.valid_to) ORDER BY s.valid_from DESC,s.id LIMIT 1) x ON true
 WHERE a.org_id=$1::uuid ORDER BY a.id LIMIT $4`, org, member, at, syncapp.ReferenceMaxEntities+1)
	if err != nil {
		return nil, syncapp.ErrUnavailable
	}
	defer rows.Close()
	var out []syncapp.ReferenceAssetSource
	index := map[string]int{}
	var ids []string
	for rows.Next() {
		var v syncapp.ReferenceAssetSource
		var until pgtype.Timestamptz
		if err := rows.Scan(&v.Asset.ID, &v.Asset.Revision, &v.Asset.Kind, &v.Asset.UsageBasis, &v.Asset.State, &v.Asset.Name, &v.Asset.FleetNumber, &v.Asset.Plate, &v.UnitID, &v.Asset.Assignment.Kind, &until); err != nil {
			return nil, syncapp.ErrUnavailable
		}
		if until.Valid {
			text := orgtime.FormatInstant(until.Time)
			v.Asset.Assignment.ValidTo = &text
		}
		v.Asset.Catalog, v.Asset.Products, v.Asset.Meters = syncapp.ReferenceCatalog, []string{}, []syncapp.ReferenceMeter{}
		index[v.Asset.ID] = len(out)
		ids = append(ids, v.Asset.ID)
		out = append(out, v)
	}
	if rows.Err() != nil || len(out) > syncapp.ReferenceMaxEntities {
		return nil, syncapp.ErrUnavailable
	}
	if len(out) == 0 {
		return out, nil
	}
	meters, err := db.Query(ctx, `SELECT m.asset_id::text,m.id::text,m.kind,m.modulus,m.installed_at,m.installed_submicro,g.id::text FROM org_meters m
 JOIN org_meter_segments g ON(g.org_id,g.meter_id)=(m.org_id,m.id) AND g.closed_at IS NULL
 WHERE m.org_id=$1::uuid AND m.asset_id=ANY($2::uuid[]) AND m.removed_at IS NULL ORDER BY m.asset_id,m.id,g.id`, org, ids)
	if err != nil {
		return nil, syncapp.ErrUnavailable
	}
	for meters.Next() {
		var asset string
		var m syncapp.ReferenceMeter
		var modulus pgtype.Int8
		var installed pgtype.Timestamptz
		var sub int16
		if err := meters.Scan(&asset, &m.ID, &m.Kind, &modulus, &installed, &sub, &m.SegmentID); err != nil || !installed.Valid {
			meters.Close()
			return nil, syncapp.ErrUnavailable
		}
		if modulus.Valid {
			value := modulus.Int64
			m.Modulus = &value
		}
		m.InstalledAt = orgtime.FormatInstant(installed.Time.Add(time.Duration(sub)))
		out[index[asset]].Asset.Meters = append(out[index[asset]].Asset.Meters, m)
	}
	meters.Close()
	if meters.Err() != nil {
		return nil, syncapp.ErrUnavailable
	}
	products, err := db.Query(ctx, `SELECT DISTINCT asset_id::text,product FROM org_asset_tank_products WHERE org_id=$1::uuid AND asset_id=ANY($2::uuid[]) ORDER BY 1,2`, org, ids)
	if err != nil {
		return nil, syncapp.ErrUnavailable
	}
	defer products.Close()
	for products.Next() {
		var asset, product string
		if err := products.Scan(&asset, &product); err != nil {
			return nil, syncapp.ErrUnavailable
		}
		out[index[asset]].Asset.Products = append(out[index[asset]].Asset.Products, product)
	}
	if products.Err() != nil {
		return nil, syncapp.ErrUnavailable
	}
	return out, nil
}

var _ syncapp.ReferenceAssets = (*AssetStore)(nil)
