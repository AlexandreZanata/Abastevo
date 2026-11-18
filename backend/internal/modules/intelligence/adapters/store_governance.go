package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListConsolidation(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string) (map[string]any, error) {
	org := scope.OrgID().String()
	return withOrgReadTx(ctx, s, scope, actor, member, org, app.GroupManagePolicy(false), false, func(tx pgx.Tx) (map[string]any, error) {
		grants := []app.ConsolidationGrantView{}
		rows, err := tx.Query(ctx, `
			SELECT id::text, counterpart_org_id::text, allowed_datasets_json, state, proposed_by_org_id::text, valid_until, revision
			FROM org_consolidation_grants WHERE org_id=$1::uuid ORDER BY created_at DESC, id`, org)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var g app.ConsolidationGrantView
			var dsRaw []byte
			var validUntil time.Time
			if err := rows.Scan(&g.ID, &g.CounterpartOrgID, &dsRaw, &g.State, &g.ProposedByOrgID, &validUntil, &g.Revision); err != nil {
				rows.Close()
				return nil, err
			}
			_ = json.Unmarshal(dsRaw, &g.AllowedDatasets)
			g.ValidUntil = validUntil.UTC().Format(time.RFC3339Nano)
			grants = append(grants, g)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}

		recurring := []map[string]any{}
		if rRows, err := tx.Query(ctx, `SELECT id::text, asset_id::text, kind, recurrence_key, period_month, amount_centavos, expense_id::text, state, occurred_at FROM org_recurring_and_fine_costs WHERE org_id=$1::uuid ORDER BY occurred_at DESC, id`, org); err != nil {
			return nil, err
		} else {
			for rRows.Next() {
				var id, assetID, kind, recKey, pm, expID, st string
				var amt int64
				var at time.Time
				if err := rRows.Scan(&id, &assetID, &kind, &recKey, &pm, &amt, &expID, &st, &at); err != nil {
					rRows.Close()
					return nil, err
				}
				recurring = append(recurring, map[string]any{"id": id, "asset_id": assetID, "kind": kind, "recurrence_key": recKey, "period_month": pm, "amount_centavos": amt, "expense_id": expID, "state": st, "occurred_at": at.UTC().Format(time.RFC3339Nano)})
			}
			rRows.Close()
			if err := rRows.Err(); err != nil {
				return nil, err
			}
		}

		payables := []map[string]any{}
		if pRows, err := tx.Query(ctx, `SELECT id::text, kind, reference_code, period_month, payable_ids_json, total_centavos, settled_centavos, state, revision FROM org_payable_statements WHERE org_id=$1::uuid ORDER BY created_at DESC, id`, org); err != nil {
			return nil, err
		} else {
			for pRows.Next() {
				var id, kind, ref, pm, st string
				var pidsRaw []byte
				var tot, settled, rev int64
				if err := pRows.Scan(&id, &kind, &ref, &pm, &pidsRaw, &tot, &settled, &st, &rev); err != nil {
					pRows.Close()
					return nil, err
				}
				var pids []string
				_ = json.Unmarshal(pidsRaw, &pids)
				payables = append(payables, map[string]any{"id": id, "kind": kind, "reference_code": ref, "period_month": pm, "payable_ids": pids, "total_centavos": tot, "settled_centavos": settled, "state": st, "revision": rev})
			}
			pRows.Close()
			if err := pRows.Err(); err != nil {
				return nil, err
			}
		}

		schedules := []map[string]any{}
		if sRows, err := tx.Query(ctx, `SELECT id::text, owner_membership_id::text, report_kind, cadence, recipient_membership_ids_json, state, pause_reason, revision FROM org_report_schedules WHERE org_id=$1::uuid ORDER BY created_at DESC, id`, org); err != nil {
			return nil, err
		} else {
			for sRows.Next() {
				var id, ownerID, rKind, cad, st, reason string
				var recsRaw []byte
				var rev int64
				if err := sRows.Scan(&id, &ownerID, &rKind, &cad, &recsRaw, &st, &reason, &rev); err != nil {
					sRows.Close()
					return nil, err
				}
				var recs []string
				_ = json.Unmarshal(recsRaw, &recs)
				schedules = append(schedules, map[string]any{"id": id, "owner_membership_id": ownerID, "report_kind": rKind, "cadence": cad, "recipient_membership_ids": recs, "state": st, "pause_reason": reason, "revision": rev})
			}
			sRows.Close()
			if err := sRows.Err(); err != nil {
				return nil, err
			}
		}

		retention := []map[string]any{}
		if hRows, err := tx.Query(ctx, `SELECT id::text, record_kind, category, COALESCE(target_id::text,''), retention_days, hold_active, responsible_context, state, reason FROM org_retention_and_holds WHERE org_id=$1::uuid ORDER BY created_at DESC, id`, org); err != nil {
			return nil, err
		} else {
			for hRows.Next() {
				var id, rk, cat, tid, rc, st, reason string
				var rdays *int
				var active bool
				if err := hRows.Scan(&id, &rk, &cat, &tid, &rdays, &active, &rc, &st, &reason); err != nil {
					hRows.Close()
					return nil, err
				}
				retention = append(retention, map[string]any{"id": id, "record_kind": rk, "category": cat, "target_id": tid, "retention_days": rdays, "hold_active": active, "responsible_context": rc, "state": st, "reason": reason})
			}
			hRows.Close()
			if err := hRows.Err(); err != nil {
				return nil, err
			}
		}

		customRoles := []map[string]any{}
		if cRows, err := tx.Query(ctx, `SELECT id::text, key, name, permissions_json, state, revision FROM org_custom_roles WHERE org_id=$1::uuid ORDER BY key`, org); err != nil {
			return nil, err
		} else {
			for cRows.Next() {
				var id, key, name, st string
				var permsRaw []byte
				var rev int64
				if err := cRows.Scan(&id, &key, &name, &permsRaw, &st, &rev); err != nil {
					cRows.Close()
					return nil, err
				}
				var perms []string
				_ = json.Unmarshal(permsRaw, &perms)
				customRoles = append(customRoles, map[string]any{"id": id, "key": key, "name": name, "permissions": perms, "state": st, "revision": rev})
			}
			cRows.Close()
			if err := cRows.Err(); err != nil {
				return nil, err
			}
		}

		voluntary := []map[string]any{}
		if vRows, err := tx.Query(ctx, `SELECT id::text, fueling_id::text, station_id, product, unit, unit_price_milli, price_condition, published_at FROM org_voluntary_price_publications WHERE org_id=$1::uuid ORDER BY published_at DESC, id`, org); err != nil {
			return nil, err
		} else {
			for vRows.Next() {
				var id, fid, sid, prod, unit, cond string
				var price int64
				var at time.Time
				if err := vRows.Scan(&id, &fid, &sid, &prod, &unit, &price, &cond, &at); err != nil {
					vRows.Close()
					return nil, err
				}
				// Public price publication carries zero org or driver identity.
				voluntary = append(voluntary, map[string]any{"id": id, "fueling_id": fid, "station_id": sid, "product": prod, "unit": unit, "unit_price_milli": price, "price_condition": cond, "published_at": at.UTC().Format(time.RFC3339Nano)})
			}
			vRows.Close()
			if err := vRows.Err(); err != nil {
				return nil, err
			}
		}

		return map[string]any{
			"grants":                 grants,
			"recurring_costs":        recurring,
			"payable_statements":     payables,
			"report_schedules":       schedules,
			"retention_holds":        retention,
			"custom_roles":           customRoles,
			"voluntary_publications": voluntary,
		}, nil
	})
}

func (s *Store) SaveConsolidation(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.ConsolidationSaveInput) (map[string]any, bool, error) {
	if !s.ready(true) {
		return nil, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return nil, false, err
	}
	policy := app.GroupManagePolicy(true)
	auditAction := "org.group.grant.updated"
	auditTarget := "organization"
	if in.Action != "PROPOSE" && in.Action != "ACCEPT" && in.Action != "END" && in.Action != "DECLINE" {
		policy = app.SettingsManagePolicy()
		auditAction = "f6.governance.updated"
	}

	var out map[string]any
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, policy, false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "f6.governance", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}

		entityID := org
		switch in.Action {
		case "PROPOSE":
			if !validUUID(in.CounterpartOrgID) || in.CounterpartOrgID == org || len(in.AllowedDatasets) == 0 || strings.TrimSpace(in.ProposalCode) == "" {
				return app.ErrInvalid
			}
			validUntil := time.Now().UTC().Add(30 * 24 * time.Hour)
			if in.ValidUntil != "" {
				t, err := time.Parse(time.RFC3339, in.ValidUntil)
				if err != nil {
					return app.ErrInvalid
				}
				validUntil = t.UTC()
			}
			sum := sha256.Sum256([]byte(in.ProposalCode))
			codeHash := hex.EncodeToString(sum[:])
			dsRaw, _ := json.Marshal(in.AllowedDatasets)
			grantID, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = grantID
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_consolidation_grants(org_id,id,operation_id,counterpart_org_id,allowed_datasets_json,proposal_code_hash,state,proposed_by_org_id,valid_until)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::jsonb,$6,'PROPOSED',$1::uuid,$7)`,
				org, grantID, in.OperationID, in.CounterpartOrgID, dsRaw, codeHash, validUntil); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"grant_id": grantID, "state": "PROPOSED", "revision": int64(1)}

		case "ACCEPT", "END", "DECLINE":
			if !validUUID(in.GrantID) || in.Revision == nil {
				return app.ErrInvalid
			}
			entityID = in.GrantID
			var state, hash string
			var rev int64
			if err := tx.QueryRow(ctx, `SELECT state, proposal_code_hash, revision FROM org_consolidation_grants WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, in.GrantID).
				Scan(&state, &hash, &rev); err != nil {
				return mapPGErr(err)
			}
			if rev != *in.Revision {
				return app.ErrConflict
			}
			nextState := ""
			switch in.Action {
			case "ACCEPT":
				if state != "PROPOSED" {
					return app.ErrConflict
				}
				sum := sha256.Sum256([]byte(in.ProposalCode))
				if in.ProposalCode == "" || hex.EncodeToString(sum[:]) != hash {
					return app.ErrInvalid
				}
				nextState = "ACTIVE"
			case "DECLINE":
				if state != "PROPOSED" {
					return app.ErrConflict
				}
				nextState = "DECLINED"
			case "END":
				if state != "ACTIVE" && state != "PROPOSED" {
					return app.ErrConflict
				}
				nextState = "ENDED"
			}
			if _, err := tx.Exec(ctx, `UPDATE org_consolidation_grants SET state=$3, revision=revision+1, updated_at=clock_timestamp() WHERE org_id=$1::uuid AND id=$2::uuid`,
				org, in.GrantID, nextState); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"grant_id": in.GrantID, "state": nextState, "revision": rev + 1}

		case "RECURRING_COST":
			if !validUUID(in.AssetID) || strings.TrimSpace(in.RecurrenceKey) == "" || len(in.PeriodMonth) != 7 || in.AmountCentavos < 0 {
				return app.ErrInvalid
			}
			kind := strings.ToUpper(strings.TrimSpace(in.Kind))
			cat := "OTHER"
			switch kind {
			case "FINE":
				cat = "FINE"
			case "LEASE":
				cat = "LEASE"
			case "INSURANCE":
				cat = "INSURANCE"
			case "RECURRING_OTHER":
				cat = "OTHER"
			default:
				return app.ErrInvalid
			}
			at := time.Now().UTC()
			if in.OccurredAt != "" {
				parsed, err := time.Parse(time.RFC3339, in.OccurredAt)
				if err != nil {
					return app.ErrInvalid
				}
				at = parsed.UTC()
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			expID, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_recurring_and_fine_costs(org_id,id,operation_id,asset_id,kind,recurrence_key,period_month,amount_centavos,expense_id,state,author_membership_id,occurred_at)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9::uuid,'APPROVED',$10::uuid,$11)`,
				org, id, in.OperationID, in.AssetID, kind, in.RecurrenceKey, in.PeriodMonth, in.AmountCentavos, expID, member, at); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_operating_cost_entries(org_id,id,origin_kind,source_id,recurring_cost_id,asset_id,category,operating_cost,paid_centavos,company_centavos,reporting_day,occurred_at,review_state)
				VALUES($1::uuid,$2::uuid,'FINE_OR_RECURRING',$3::uuid,$3::uuid,$4::uuid,$5,true,$6,$6,$7::date,$8,'APPROVED')`,
				org, expID, id, in.AssetID, cat, in.AmountCentavos, at.Format("2006-01-02"), at); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"recurring_cost_id": id, "expense_id": expID, "kind": kind, "period_month": in.PeriodMonth, "amount_centavos": in.AmountCentavos}

		case "PAYABLE_STATEMENT":
			kind := strings.ToUpper(strings.TrimSpace(in.Kind))
			if (kind != "CARD_STATEMENT" && kind != "SUPPLIER_INVOICE") || strings.TrimSpace(in.ReferenceCode) == "" || len(in.PeriodMonth) != 7 || in.AmountCentavos < 0 {
				return app.ErrInvalid
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			pids := in.PayableIDs
			if pids == nil {
				pids = []string{}
			}
			pidsRaw, _ := json.Marshal(pids)
			state := "OPEN"
			settled := int64(0)
			if len(pids) > 0 && in.Reason == "SETTLE" {
				state = "SETTLED"
				settled = in.AmountCentavos
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_payable_statements(org_id,id,operation_id,kind,supplier_id,reference_code,period_month,payable_ids_json,total_centavos,settled_centavos,state)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,$7,$8::jsonb,$9,$10,$11)`,
				org, id, in.OperationID, kind, optText(in.SupplierID), in.ReferenceCode, in.PeriodMonth, pidsRaw, in.AmountCentavos, settled, state); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"statement_id": id, "kind": kind, "state": state, "total_centavos": in.AmountCentavos, "settled_centavos": settled}

		case "REPORT_SCHEDULE":
			cad := strings.ToUpper(strings.TrimSpace(in.Cadence))
			if strings.TrimSpace(in.ReportKind) == "" || (cad != "DAILY" && cad != "WEEKLY" && cad != "MONTHLY") {
				return app.ErrInvalid
			}
			recs := in.RecipientIDs
			if recs == nil {
				recs = []string{}
			}
			state := "ACTIVE"
			pauseReason := ""
			for _, rID := range recs {
				if !validUUID(rID) {
					return app.ErrInvalid
				}
				var mState string
				err := tx.QueryRow(ctx, `SELECT state FROM org_memberships WHERE org_id=$1::uuid AND id=$2::uuid`, org, rID).Scan(&mState)
				if errors.Is(err, pgx.ErrNoRows) || mState != "ACTIVE" {
					state = "PAUSED"
					pauseReason = "RECIPIENT_REVOKED"
				} else if err != nil {
					return mapPGErr(err)
				}
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			recsRaw, _ := json.Marshal(recs)
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_report_schedules(org_id,id,operation_id,owner_membership_id,report_kind,cadence,recipient_membership_ids_json,state,pause_reason)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7::jsonb,$8,$9)`,
				org, id, in.OperationID, member, in.ReportKind, cad, recsRaw, state, pauseReason); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"schedule_id": id, "report_kind": in.ReportKind, "cadence": cad, "state": state, "pause_reason": pauseReason}

		case "RETENTION_POLICY", "LEGAL_HOLD", "DELETION_REPLAY", "DSR_ROUTE", "ERASURE_TOMBSTONE":
			if strings.TrimSpace(in.Category) == "" {
				return app.ErrInvalid
			}
			if in.TargetID != "" && !validUUID(in.TargetID) {
				return app.ErrInvalid
			}
			recordKind := ""
			state := "RECORDED"
			holdActive := false
			var purgedAt *time.Time
			var expiresAt *time.Time
			if in.ExpiresAt != "" {
				t, err := time.Parse(time.RFC3339, in.ExpiresAt)
				if err != nil {
					return app.ErrInvalid
				}
				utc := t.UTC()
				expiresAt = &utc
			}
			switch in.Action {
			case "RETENTION_POLICY":
				recordKind = "POLICY"
				state = "ACTIVE"
				if in.RetentionDays == nil || *in.RetentionDays < 1 {
					return app.ErrInvalid
				}
			case "LEGAL_HOLD":
				recordKind = "LEGAL_HOLD"
				holdActive = in.HoldActive
				if holdActive {
					state = "HELD"
				} else {
					state = "RELEASED"
				}
			case "DELETION_REPLAY":
				recordKind = "DELETION_LEDGER"
				var held int
				if in.TargetID != "" {
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_retention_and_holds WHERE org_id=$1::uuid AND record_kind='LEGAL_HOLD' AND category=$2 AND target_id=$3::uuid AND hold_active=true`, org, in.Category, in.TargetID).Scan(&held); err != nil {
						return mapPGErr(err)
					}
				} else {
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM org_retention_and_holds WHERE org_id=$1::uuid AND record_kind='LEGAL_HOLD' AND category=$2 AND hold_active=true`, org, in.Category).Scan(&held); err != nil {
						return mapPGErr(err)
					}
				}
				if held > 0 {
					return app.ErrHoldOpen
				}
				now := time.Now().UTC()
				purgedAt = &now
				state = "PURGED"
			case "DSR_ROUTE":
				recordKind = "DSR_REQUEST"
				state = "ROUTED"
			case "ERASURE_TOMBSTONE":
				recordKind = "ERASURE_TOMBSTONE"
				state = "ERASED_PSEUDONYMIZED"
			}
			respCtx := "ORGANIZATION"
			if in.ResponsibleCtx == "PLATFORM" {
				respCtx = "PLATFORM"
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			replayHash := reqHash
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_retention_and_holds(org_id,id,operation_id,record_kind,category,target_id,retention_days,hold_active,expires_at,purged_at,replay_hash,responsible_context,state,reason,actor_membership_id)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15::uuid)`,
				org, id, in.OperationID, recordKind, in.Category, optText(in.TargetID), in.RetentionDays, holdActive, expiresAt, purgedAt, replayHash, respCtx, state, in.Reason, member); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"record_id": id, "record_kind": recordKind, "state": state, "responsible_context": respCtx, "hold_active": holdActive, "replay_hash": replayHash}

		case "CUSTOM_ROLE":
			key := strings.TrimSpace(in.RoleKey)
			name := strings.TrimSpace(in.RoleName)
			if key == "" || name == "" || len(in.Permissions) == 0 {
				return app.ErrInvalid
			}
			cat := permissions.SystemCatalog()
			for _, permStr := range in.Permissions {
				if permStr == "org.group.manage" || permStr == "org.lifecycle.manage" {
					return app.ErrInvalid
				}
				if _, err := cat.Lookup(permissions.Owner, permissions.Permission(permStr)); err != nil {
					return app.ErrInvalid
				}
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			permsRaw, _ := json.Marshal(in.Permissions)
			var rev int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO org_custom_roles(org_id,id,operation_id,key,name,permissions_json)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6::jsonb)
				ON CONFLICT(org_id,key) DO UPDATE SET name=EXCLUDED.name, permissions_json=EXCLUDED.permissions_json, revision=org_custom_roles.revision+1, updated_at=clock_timestamp()
				RETURNING id::text, revision`,
				org, id, in.OperationID, key, name, permsRaw).Scan(&entityID, &rev); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"custom_role_id": entityID, "key": key, "name": name, "permissions": in.Permissions, "revision": rev}

		case "VOLUNTARY_PRICE":
			if !validUUID(in.FuelingID) || strings.TrimSpace(in.StationID) == "" || strings.TrimSpace(in.Product) == "" || strings.TrimSpace(in.Unit) == "" || in.UnitPriceMilli <= 0 || strings.TrimSpace(in.PriceCondition) == "" {
				return app.ErrInvalid
			}
			id, err := s.NewID()
			if err != nil {
				return app.ErrUnavailable
			}
			entityID = id
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_voluntary_price_publications(org_id,id,operation_id,fueling_id,station_id,product,unit,unit_price_milli,price_condition,published_by_membership_id)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10::uuid)`,
				org, id, in.OperationID, in.FuelingID, in.StationID, in.Product, in.Unit, in.UnitPriceMilli, in.PriceCondition, member); err != nil {
				return mapPGErr(err)
			}
			// Public contribution stays unlinked from enterprise org/driver identity.
			out = map[string]any{"publication_id": id, "station_id": in.StationID, "product": in.Product, "unit_price_milli": in.UnitPriceMilli, "anonymized": true}

		default:
			return app.ErrInvalid
		}

		if err := commandSave(ctx, tx, org, "f6.governance", in.OperationID, entityID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, auditAction, auditTarget, entityID, in.Action)
	})
	return out, replayed, err
}
