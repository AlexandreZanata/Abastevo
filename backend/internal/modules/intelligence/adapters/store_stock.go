package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	stock "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fuelstock/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productUnit(product string) (string, bool) {
	switch product {
	case "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "GASOLINE_PREMIUM_GRADE", "ETHANOL", "DIESEL_S500", "DIESEL_S10":
		return "L", true
	case "CNG":
		return "M3", true
	case "ELECTRIC_DC", "ELECTRIC_AC":
		return "KWH", true
	}
	return "", false
}

func (s *Store) ListTanks(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string) ([]app.TankView, error) {
	org := scope.OrgID().String()
	return withOrgReadTx(ctx, s, scope, actor, member, org, app.TankPolicy(false), false, func(tx pgx.Tx) ([]app.TankView, error) {
		rows, err := tx.Query(ctx, `
			SELECT id::text, COALESCE(unit_id::text,''), name, product, unit, capacity_milli, quantity_milli, value_centavos, allow_negative, state, revision, updated_at
			FROM org_fuel_tanks WHERE org_id=$1::uuid ORDER BY created_at, id`, org)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []app.TankView{}
		for rows.Next() {
			var t app.TankView
			var up time.Time
			if err := rows.Scan(&t.ID, &t.UnitID, &t.Name, &t.Product, &t.Unit, &t.CapacityMilli, &t.QuantityMilli, &t.ValueCentavos, &t.AllowNegative, &t.State, &t.Revision, &up); err != nil {
				return nil, err
			}
			t.UpdatedAt = up.UTC().Format(time.RFC3339Nano)
			out = append(out, t)
		}
		return out, rows.Err()
	})
}

func (s *Store) SaveTank(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.SaveTankInput) (app.TankView, bool, error) {
	if !s.ready(true) {
		return app.TankView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return app.TankView{}, false, err
	}
	unit, ok := productUnit(in.Product)
	if !ok || (in.Unit != "" && in.Unit != unit) || strings.TrimSpace(in.Name) == "" || in.CapacityMilli <= 0 {
		return app.TankView{}, false, app.ErrInvalid
	}
	var out app.TankView
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.TankPolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "stock.tank.save", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var up time.Time
		if in.TankID == "" {
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			err = tx.QueryRow(ctx, `
				INSERT INTO org_fuel_tanks(org_id,id,unit_id,name,product,unit,capacity_milli,allow_negative)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8)
				RETURNING id::text,COALESCE(unit_id::text,''),name,product,unit,capacity_milli,quantity_milli,value_centavos,allow_negative,state,revision,updated_at`,
				org, id, optText(in.UnitID), strings.TrimSpace(in.Name), in.Product, unit, in.CapacityMilli, in.AllowNegative,
			).Scan(&out.ID, &out.UnitID, &out.Name, &out.Product, &out.Unit, &out.CapacityMilli, &out.QuantityMilli, &out.ValueCentavos, &out.AllowNegative, &out.State, &out.Revision, &up)
			if err != nil {
				return mapPGErr(err)
			}
		} else {
			if !validUUID(in.TankID) || in.Revision == nil {
				return app.ErrInvalid
			}
			nextState := "ACTIVE"
			if in.State != "" {
				nextState = in.State
			}
			err = tx.QueryRow(ctx, `
				UPDATE org_fuel_tanks SET name=$4, capacity_milli=$5, allow_negative=$6, state=$7, revision=revision+1
				WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3
				RETURNING id::text,COALESCE(unit_id::text,''),name,product,unit,capacity_milli,quantity_milli,value_centavos,allow_negative,state,revision,updated_at`,
				org, in.TankID, *in.Revision, strings.TrimSpace(in.Name), in.CapacityMilli, in.AllowNegative, nextState,
			).Scan(&out.ID, &out.UnitID, &out.Name, &out.Product, &out.Unit, &out.CapacityMilli, &out.QuantityMilli, &out.ValueCentavos, &out.AllowNegative, &out.State, &out.Revision, &up)
			if errors.Is(err, pgx.ErrNoRows) {
				var exists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_fuel_tanks WHERE org_id=$1::uuid AND id=$2::uuid)`, org, in.TankID).Scan(&exists)
				if !exists {
					return app.ErrNotFound
				}
				return app.ErrConflict
			}
			if err != nil {
				return mapPGErr(err)
			}
		}
		out.UpdatedAt = up.UTC().Format(time.RFC3339Nano)
		if err := commandSave(ctx, tx, org, "stock.tank.save", in.OperationID, out.ID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "stock.tank.saved", "tank", out.ID, "")
	})
	return out, replayed, err
}

func (s *Store) ListMovements(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, tankID string) ([]app.MovementView, []app.ReconciliationView, error) {
	org := scope.OrgID().String()
	type res struct {
		mov []app.MovementView
		rec []app.ReconciliationView
	}
	v, err := withOrgReadTx(ctx, s, scope, actor, member, org, app.MovementPolicy(false), false, func(tx pgx.Tx) (res, error) {
		rows, err := tx.Query(ctx, `
			SELECT id::text,tank_id::text,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,
			       COALESCE(asset_id::text,''),COALESCE(counterpart_tank_id::text,''),COALESCE(fueling_id::text,''),COALESCE(expense_id::text,''),reason,occurred_at
			FROM org_fuel_tank_movements
			WHERE org_id=$1::uuid AND ($2::text='' OR tank_id=$2::uuid)
			ORDER BY recorded_at DESC, id LIMIT 200`, org, tankID)
		if err != nil {
			return res{}, err
		}
		var out res
		for rows.Next() {
			var m app.MovementView
			var occ time.Time
			if err := rows.Scan(&m.ID, &m.TankID, &m.Kind, &m.Product, &m.Unit, &m.DeltaQuantityMilli, &m.AmountCentavos, &m.PoolQuantityAfterMilli, &m.PoolValueAfterCentavos,
				&m.AssetID, &m.CounterpartTankID, &m.FuelingID, &m.ExpenseID, &m.Reason, &occ); err != nil {
				rows.Close()
				return res{}, err
			}
			m.OccurredAt = occ.UTC().Format(time.RFC3339Nano)
			if m.Kind == "DISPENSE" {
				m.RecognizedExpenseCentavos = m.AmountCentavos
			}
			out.mov = append(out.mov, m)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return res{}, err
		}
		rows.Close()
		rRows, err := tx.Query(ctx, `
			SELECT id::text,tank_id::text,measured_milli,recorded_milli,variance_milli,state,COALESCE(adjustment_movement_id::text,''),reason,measured_at
			FROM org_fuel_tank_reconciliations
			WHERE org_id=$1::uuid AND ($2::text='' OR tank_id=$2::uuid)
			ORDER BY recorded_at DESC, id LIMIT 100`, org, tankID)
		if err != nil {
			return res{}, err
		}
		for rRows.Next() {
			var rec app.ReconciliationView
			var meas time.Time
			if err := rRows.Scan(&rec.ID, &rec.TankID, &rec.MeasuredMilli, &rec.RecordedMilli, &rec.VarianceMilli, &rec.State, &rec.AdjustmentMovementID, &rec.Reason, &meas); err != nil {
				rRows.Close()
				return res{}, err
			}
			rec.MeasuredAt = meas.UTC().Format(time.RFC3339Nano)
			out.rec = append(out.rec, rec)
		}
		err = rRows.Err()
		rRows.Close()
		return out, err
	})
	return v.mov, v.rec, err
}

func (s *Store) RecordMovement(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.RecordMovementInput) (map[string]any, bool, error) {
	if !s.ready(true) {
		return nil, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return nil, false, err
	}
	if !validUUID(in.TankID) {
		return nil, false, app.ErrInvalid
	}
	occAt := time.Now().UTC()
	if in.OccurredAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.OccurredAt)
		if err != nil {
			return nil, false, app.ErrInvalid
		}
		occAt = parsed.UTC()
	}
	var out map[string]any
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.MovementPolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "stock.movement.record", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var product, unit string
		var unitID pgtype.Text
		var qtyMilli, valCents, rev int64
		var allowNeg bool
		err := tx.QueryRow(ctx, `SELECT COALESCE(unit_id::text,''), product, unit, quantity_milli, value_centavos, allow_negative, revision FROM org_fuel_tanks WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, in.TankID).
			Scan(&unitID, &product, &unit, &qtyMilli, &valCents, &allowNeg, &rev)
		if err != nil {
			return mapPGErr(err)
		}
		pool := stock.Pool{Product: product, Unit: unit, QuantityMilli: qtyMilli, ValueCentavos: valCents, AllowNegative: allowNeg}
		movID, err := s.NewID()
		if err != nil {
			return app.ErrUnavailable
		}
		switch in.Kind {
		case "RECEIPT":
			next, outcome, err := pool.Receive(in.Product, in.QuantityMilli, in.AmountCentavos)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
				org, in.TankID, rev, next.QuantityMilli, next.ValueCentavos); err != nil {
				return mapPGErr(err)
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,supplier_id,document_id,reason,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'RECEIPT',$5,$6,$7,$8,$9,$10,$11::uuid,$12::uuid,$13,$14::uuid,$15)`,
				org, movID, in.OperationID, in.TankID, product, unit, in.QuantityMilli, in.AmountCentavos, next.QuantityMilli, next.ValueCentavos, optText(in.SupplierID), optText(in.DocumentID), in.Reason, member, occAt)
			if err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"movement": app.MovementView{
				ID: movID, TankID: in.TankID, Kind: "RECEIPT", Product: product, Unit: unit,
				DeltaQuantityMilli: in.QuantityMilli, AmountCentavos: in.AmountCentavos,
				PoolQuantityAfterMilli: outcome.AfterQuantityMilli, PoolValueAfterCentavos: outcome.AfterValueCentavos,
				OccurredAt: occAt.Format(time.RFC3339Nano), RecognizedExpenseCentavos: 0,
			}}
		case "DISPENSE":
			if !validUUID(in.AssetID) {
				return app.ErrInvalid
			}
			next, outcome, err := pool.Dispense(in.Product, in.QuantityMilli)
			if err != nil {
				return err
			}
			var assetUnit, assetCC pgtype.Text
			if err := tx.QueryRow(ctx, `SELECT (SELECT unit_id::text FROM org_asset_unit_periods WHERE org_id=$1::uuid AND asset_id=$2::uuid AND valid_from<=$3 AND(valid_to IS NULL OR $3<valid_to) LIMIT 1), (SELECT cost_centre_id::text FROM org_asset_cost_centre_periods WHERE org_id=$1::uuid AND asset_id=$2::uuid AND valid_from<=$3 AND(valid_to IS NULL OR $3<valid_to) LIMIT 1)`, org, in.AssetID, occAt).Scan(&assetUnit, &assetCC); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
				org, in.TankID, rev, next.QuantityMilli, next.ValueCentavos); err != nil {
				return mapPGErr(err)
			}
			fuelID, _ := s.NewID()
			expID, _ := s.NewID()
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,asset_id,fueling_id,expense_id,reason,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'DISPENSE',$5,$6,$7,$8,$9,$10,$11::uuid,$12::uuid,$13::uuid,$14,$15::uuid,$16)`,
				org, movID, in.OperationID, in.TankID, product, unit, -in.QuantityMilli, outcome.MovedCentavos, next.QuantityMilli, next.ValueCentavos, in.AssetID, fuelID, expID, in.Reason, member, occAt); err != nil {
				return mapPGErr(err)
			}
			fillKind := "FULL"
			if in.FillKind == "PARTIAL" || in.FillKind == "UNKNOWN" {
				fillKind = in.FillKind
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_internal_fuelings(org_id,id,operation_id,tank_id,movement_id,asset_id,unit_id,cost_centre_id,product,unit,quantity_milli,cost_centavos,fill,occurred_at,reporting_day,author_membership_id,review_state)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7::uuid,$8::uuid,$9,$10,$11,$12,$13,$14,$15::date,$16::uuid,'APPROVED')`,
				org, fuelID, in.OperationID, in.TankID, movID, in.AssetID, optText(assetUnit.String), optText(assetCC.String), product, unit, in.QuantityMilli, outcome.MovedCentavos, fillKind, occAt, occAt.Format("2006-01-02"), member); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_operating_cost_entries(org_id,id,origin_kind,source_id,movement_id,asset_id,unit_id,cost_centre_id,category,operating_cost,paid_centavos,company_centavos,reporting_day,occurred_at,review_state)
				VALUES($1::uuid,$2::uuid,'INTERNAL_STOCK_DISPENSE',$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7::uuid,'FUEL',true,$8,$8,$9::date,$10,'APPROVED')`,
				org, expID, fuelID, movID, in.AssetID, optText(assetUnit.String), optText(assetCC.String), outcome.MovedCentavos, occAt.Format("2006-01-02"), occAt); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{
				"movement": app.MovementView{
					ID: movID, TankID: in.TankID, Kind: "DISPENSE", Product: product, Unit: unit,
					DeltaQuantityMilli: -in.QuantityMilli, AmountCentavos: outcome.MovedCentavos,
					PoolQuantityAfterMilli: outcome.AfterQuantityMilli, PoolValueAfterCentavos: outcome.AfterValueCentavos,
					AssetID: in.AssetID, FuelingID: fuelID, ExpenseID: expID,
					OccurredAt: occAt.Format(time.RFC3339Nano), RecognizedExpenseCentavos: outcome.RecognizedExpenseCentavos,
				},
			}
		case "TRANSFER":
			if !validUUID(in.CounterpartTankID) || in.CounterpartTankID == in.TankID {
				return app.ErrInvalid
			}
			var dProd, dUnit string
			var dQty, dVal, dRev int64
			var dAllow bool
			if err := tx.QueryRow(ctx, `SELECT product,unit,quantity_milli,value_centavos,allow_negative,revision FROM org_fuel_tanks WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, in.CounterpartTankID).
				Scan(&dProd, &dUnit, &dQty, &dVal, &dAllow, &dRev); err != nil {
				return mapPGErr(err)
			}
			dst := stock.Pool{Product: dProd, Unit: dUnit, QuantityMilli: dQty, ValueCentavos: dVal, AllowNegative: dAllow}
			nSrc, nDst, outcome, err := stock.Transfer(pool, dst, in.Product, in.QuantityMilli)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
				org, in.TankID, rev, nSrc.QuantityMilli, nSrc.ValueCentavos); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
				org, in.CounterpartTankID, dRev, nDst.QuantityMilli, nDst.ValueCentavos); err != nil {
				return mapPGErr(err)
			}
			inID, _ := s.NewID()
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,counterpart_tank_id,reason,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'TRANSFER_OUT',$5,$6,$7,$8,$9,$10,$11::uuid,$12,$13::uuid,$14)`,
				org, movID, in.OperationID, in.TankID, product, unit, -in.QuantityMilli, outcome.MovedCentavos, nSrc.QuantityMilli, nSrc.ValueCentavos, in.CounterpartTankID, in.Reason, member, occAt); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,counterpart_tank_id,reason,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'TRANSFER_IN',$5,$6,$7,$8,$9,$10,$11::uuid,$12,$13::uuid,$14)`,
				org, inID, in.OperationID, in.CounterpartTankID, product, unit, in.QuantityMilli, outcome.MovedCentavos, nDst.QuantityMilli, nDst.ValueCentavos, in.TankID, in.Reason, member, occAt); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"transfer_centavos": outcome.MovedCentavos, "source_after_milli": nSrc.QuantityMilli, "destination_after_milli": nDst.QuantityMilli, "recognized_expense_centavos": 0}
		case "ADJUST":
			next, outcome, err := pool.Adjust(in.DeltaMilli, in.Reason)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
				org, in.TankID, rev, next.QuantityMilli, next.ValueCentavos); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,reason,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'ADJUSTMENT',$5,$6,$7,$8,$9,$10,$11,$12::uuid,$13)`,
				org, movID, in.OperationID, in.TankID, product, unit, in.DeltaMilli, outcome.MovedCentavos, next.QuantityMilli, next.ValueCentavos, in.Reason, member, occAt); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"after_quantity_milli": next.QuantityMilli, "after_value_centavos": next.ValueCentavos}
		case "RECONCILE":
			variance := in.MeasuredMilli - qtyMilli
			recState := "EXPLAINED"
			var adjID any
			if in.ApplyAdjustment && variance != 0 {
				next, outcome, err := pool.Adjust(variance, in.Reason)
				if err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE org_fuel_tanks SET quantity_milli=$4,value_centavos=$5,revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3`,
					org, in.TankID, rev, next.QuantityMilli, next.ValueCentavos); err != nil {
					return mapPGErr(err)
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_fuel_tank_movements(org_id,id,operation_id,tank_id,kind,product,unit,delta_quantity_milli,amount_centavos,pool_quantity_after_milli,pool_value_after_centavos,reason,author_membership_id,occurred_at)
					VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'ADJUSTMENT',$5,$6,$7,$8,$9,$10,$11,$12::uuid,$13)`,
					org, movID, in.OperationID, in.TankID, product, unit, variance, outcome.MovedCentavos, next.QuantityMilli, next.ValueCentavos, in.Reason, member, occAt); err != nil {
					return mapPGErr(err)
				}
				recState = "ADJUSTED"
				adjID = movID
			} else if strings.TrimSpace(in.Reason) == "" {
				recState = "OPEN"
			}
			recID, _ := s.NewID()
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_fuel_tank_reconciliations(org_id,id,operation_id,tank_id,measured_milli,recorded_milli,variance_milli,state,adjustment_movement_id,reason,author_membership_id,measured_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9::uuid,$10,$11::uuid,$12)`,
				org, recID, in.OperationID, in.TankID, in.MeasuredMilli, qtyMilli, variance, recState, adjID, in.Reason, member, occAt); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"reconciliation": app.ReconciliationView{
				ID: recID, TankID: in.TankID, MeasuredMilli: in.MeasuredMilli, RecordedMilli: qtyMilli,
				VarianceMilli: variance, State: recState, Reason: in.Reason, MeasuredAt: occAt.Format(time.RFC3339Nano),
			}}
		default:
			return app.ErrInvalid
		}
		if err := commandSave(ctx, tx, org, "stock.movement.record", in.OperationID, in.TankID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "stock.movement.recorded", "tank", in.TankID, in.Kind)
	})
	return out, replayed, err
}
