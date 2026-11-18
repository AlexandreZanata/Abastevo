package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	plan "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/plan"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func parseDay(raw string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(raw))
}

func (s *Store) ListPlans(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, assetID string) ([]app.PlanView, []app.ComplianceView, error) {
	org := scope.OrgID().String()
	type out struct {
		plans []app.PlanView
		comp  []app.ComplianceView
	}
	v, err := withOrgReadTx(ctx, s, scope, actor, member, org, app.PlanPolicy(false), false, func(tx pgx.Tx) (out, error) {
		rows, err := tx.Query(ctx, `
			SELECT p.id::text, p.asset_id::text, p.name, p.rebase_rule, p.blocking, p.state, p.revision,
			       COALESCE(o.id::text,''), COALESCE(o.state,''), o.due_odometer_m, o.due_hour_s, o.due_day,
			       o.remaining_m, o.remaining_s, o.days_to_due, o.projected_due_day, COALESCE(o.deciding_trigger,''), COALESCE(o.reading_revision,0)
			FROM org_maintenance_plans p
			LEFT JOIN LATERAL (
			  SELECT * FROM org_maintenance_occurrences o WHERE o.org_id=p.org_id AND o.plan_id=p.id ORDER BY o.sequence DESC LIMIT 1
			) o ON true
			WHERE p.org_id=$1::uuid AND ($2::text='' OR p.asset_id=$2::uuid)
			ORDER BY p.created_at, p.id`, org, assetID)
		if err != nil {
			return out{}, err
		}
		var res out
		for rows.Next() {
			var p app.PlanView
			var dueDay, projDay pgtype.Date
			var remM, remS, dueM, dueS *int64
			var daysTo *int32
			if err := rows.Scan(&p.ID, &p.AssetID, &p.Name, &p.RebaseRule, &p.Blocking, &p.State, &p.Revision,
				&p.OccurrenceID, &p.OccurrenceState, &dueM, &dueS, &dueDay, &remM, &remS, &daysTo, &projDay, &p.DecidingTrigger, &p.ReadingRevision); err != nil {
				rows.Close()
				return out{}, err
			}
			p.DueOdometerM, p.DueHourS, p.RemainingM, p.RemainingS = dueM, dueS, remM, remS
			if daysTo != nil {
				d := int(*daysTo)
				p.DaysToDue = &d
			}
			if dueDay.Valid {
				p.DueDay = dueDay.Time.Format("2006-01-02")
			}
			if projDay.Valid {
				p.ProjectedDueDay = projDay.Time.Format("2006-01-02")
			}
			res.plans = append(res.plans, p)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return out{}, err
		}
		rows.Close()
		cRows, err := tx.Query(ctx, `
			SELECT id::text, subject_kind, subject_id::text, item_kind, expiry_day, lead_days, state, COALESCE(document_id::text,''), licence_category, revision
			FROM org_compliance_items WHERE org_id=$1::uuid ORDER BY expiry_day, id`, org)
		if err != nil {
			return out{}, err
		}
		for cRows.Next() {
			var c app.ComplianceView
			var exp time.Time
			if err := cRows.Scan(&c.ID, &c.SubjectKind, &c.SubjectID, &c.ItemKind, &exp, &c.LeadDays, &c.State, &c.DocumentID, &c.LicenceCategory, &c.Revision); err != nil {
				cRows.Close()
				return out{}, err
			}
			c.ExpiryDay = exp.Format("2006-01-02")
			res.comp = append(res.comp, c)
		}
		err = cRows.Err()
		cRows.Close()
		return res, err
	})
	return v.plans, v.comp, err
}

func (s *Store) SavePlan(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.SavePlanInput) (map[string]any, bool, error) {
	if !s.ready(true) {
		return nil, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return nil, false, err
	}
	var out map[string]any
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.PlanPolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "maint.plan.save", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		switch in.Action {
		case "", "SAVE":
			if !validUUID(in.AssetID) || strings.TrimSpace(in.Name) == "" {
				return app.ErrInvalid
			}
			baseDay, err := parseDay(in.BaselineDay)
			if err != nil {
				return app.ErrInvalid
			}
			evalDay := time.Now().UTC()
			if in.EvalDay != "" {
				evalDay, err = parseDay(in.EvalDay)
				if err != nil {
					return app.ErrInvalid
				}
			}
			if in.DistanceIntervalM != nil {
				var hasOdo bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_meters WHERE org_id=$1::uuid AND asset_id=$2::uuid AND kind='ODOMETER' AND removed_at IS NULL)`, org, in.AssetID).Scan(&hasOdo); err != nil {
					return mapPGErr(err)
				}
				if !hasOdo {
					return ErrMeterRequired
				}
			}
			if in.HourIntervalS != nil {
				var hasHour bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_meters WHERE org_id=$1::uuid AND asset_id=$2::uuid AND kind='HOUR_METER' AND removed_at IS NULL)`, org, in.AssetID).Scan(&hasHour); err != nil {
					return mapPGErr(err)
				}
				if !hasHour {
					return ErrMeterRequired
				}
			}
			spec := plan.PlanSpec{
				DistanceIntervalM:  in.DistanceIntervalM,
				DistanceLeadM:      in.DistanceLeadM,
				HourIntervalS:      in.HourIntervalS,
				HourLeadS:          in.HourLeadS,
				TimeIntervalDays:   in.TimeIntervalDays,
				TimeIntervalMonths: in.TimeIntervalMonths,
				TimeLeadDays:       in.TimeLeadDays,
				BaselineDay:        baseDay,
				BaselineOdometerM:  in.BaselineOdometerM,
				BaselineHourS:      in.BaselineHourS,
			}
			evalRes, err := plan.Evaluate(plan.PlanEvalInput{Plan: spec, EvalDay: evalDay, CurrentOdoM: in.CurrentOdometerM, CurrentHourS: in.CurrentHourS})
			if err != nil {
				return app.ErrInvalid
			}
			rebase := "ACTUAL"
			if in.RebaseRule == "SCHEDULED" {
				rebase = "SCHEDULED"
			}
			planID, _ := s.NewID()
			occID, _ := s.NewID()
			var dueOdo, dueHour *int64
			if spec.DistanceIntervalM != nil && spec.BaselineOdometerM != nil {
				v := *spec.BaselineOdometerM + *spec.DistanceIntervalM
				dueOdo = &v
			}
			if spec.HourIntervalS != nil && spec.BaselineHourS != nil {
				v := *spec.BaselineHourS + *spec.HourIntervalS
				dueHour = &v
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_maintenance_plans(org_id,id,operation_id,asset_id,name,distance_interval_m,distance_lead_m,hour_interval_s,hour_lead_s,time_interval_days,time_interval_months,time_lead_days,baseline_day,baseline_odometer_m,baseline_hour_s,rebase_rule,blocking)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
				org, planID, in.OperationID, in.AssetID, strings.TrimSpace(in.Name), in.DistanceIntervalM, in.DistanceLeadM, in.HourIntervalS, in.HourLeadS,
				in.TimeIntervalDays, in.TimeIntervalMonths, in.TimeLeadDays, baseDay, in.BaselineOdometerM, in.BaselineHourS, rebase, in.Blocking); err != nil {
				return mapPGErr(err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_maintenance_occurrences(org_id,id,plan_id,asset_id,sequence,state,due_odometer_m,due_hour_s,due_day,remaining_m,remaining_s,days_to_due,deciding_trigger,reading_revision)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,1,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
				org, occID, planID, in.AssetID, string(evalRes.State), dueOdo, dueHour, evalRes.DueDay, evalRes.RemainingM, evalRes.RemainingS, evalRes.DaysToDue, string(evalRes.DecidingTrigger), in.ReadingRevision); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"plan_id": planID, "occurrence_id": occID, "state": string(evalRes.State), "deciding_trigger": string(evalRes.DecidingTrigger)}
		case "RECOMPUTE", "COMPLETE_OCCURRENCE", "SKIP_OCCURRENCE":
			if !validUUID(in.PlanID) {
				return app.ErrInvalid
			}
			var pSpec plan.PlanSpec
			var assetID, rebaseRule, occID, occState string
			var seq int
			var dueOdo, dueHour *int64
			err := tx.QueryRow(ctx, `
				SELECT p.asset_id::text, p.distance_interval_m, p.distance_lead_m, p.hour_interval_s, p.hour_lead_s,
				       p.time_interval_days, p.time_interval_months, p.time_lead_days, p.baseline_day, p.baseline_odometer_m, p.baseline_hour_s, p.rebase_rule,
				       o.id::text, o.sequence, o.state, o.due_odometer_m, o.due_hour_s
				FROM org_maintenance_plans p
				JOIN org_maintenance_occurrences o ON o.org_id=p.org_id AND o.plan_id=p.id
				WHERE p.org_id=$1::uuid AND p.id=$2::uuid AND o.state IN('UPCOMING','DUE','OVERDUE')
				FOR UPDATE OF o`, org, in.PlanID).
				Scan(&assetID, &pSpec.DistanceIntervalM, &pSpec.DistanceLeadM, &pSpec.HourIntervalS, &pSpec.HourLeadS,
					&pSpec.TimeIntervalDays, &pSpec.TimeIntervalMonths, &pSpec.TimeLeadDays, &pSpec.BaselineDay, &pSpec.BaselineOdometerM, &pSpec.BaselineHourS, &rebaseRule,
					&occID, &seq, &occState, &dueOdo, &dueHour)
			if err != nil {
				return mapPGErr(err)
			}
			evalDay := time.Now().UTC()
			if in.EvalDay != "" {
				if evalDay, err = parseDay(in.EvalDay); err != nil {
					return app.ErrInvalid
				}
			}
			if in.Action == "RECOMPUTE" {
				var winDay *time.Time
				if in.WindowStartDay != "" {
					wd, err := parseDay(in.WindowStartDay)
					if err != nil {
						return app.ErrInvalid
					}
					winDay = &wd
				}
				evalRes, err := plan.Evaluate(plan.PlanEvalInput{
					Plan: pSpec, EvalDay: evalDay, CurrentOdoM: in.CurrentOdometerM, CurrentHourS: in.CurrentHourS,
					WindowStartDay: winDay, WindowStartOdoM: in.WindowStartOdoM,
				})
				if err != nil {
					return app.ErrInvalid
				}
				if _, err := tx.Exec(ctx, `
					UPDATE org_maintenance_occurrences
					SET state=$3, remaining_m=$4, remaining_s=$5, days_to_due=$6, projected_due_day=$7, deciding_trigger=$8, reading_revision=$9, revision=revision+1, updated_at=clock_timestamp()
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, occID, string(evalRes.State), evalRes.RemainingM, evalRes.RemainingS, evalRes.DaysToDue, evalRes.ProjectedDueDay, string(evalRes.DecidingTrigger), in.ReadingRevision); err != nil {
					return mapPGErr(err)
				}
				proj := ""
				if evalRes.ProjectedDueDay != nil {
					proj = evalRes.ProjectedDueDay.Format("2006-01-02")
				}
				out = map[string]any{"occurrence_id": occID, "state": string(evalRes.State), "projected_due_day": proj, "deciding_trigger": string(evalRes.DecidingTrigger)}
			} else {
				targetState := "DONE"
				if in.Action == "SKIP_OCCURRENCE" {
					if strings.TrimSpace(in.Reason) == "" {
						return app.ErrInvalid
					}
					targetState = "SKIPPED"
				}
				if _, err := tx.Exec(ctx, `
					UPDATE org_maintenance_occurrences
					SET state=$3, completed_day=$4, completed_odometer_m=$5, completed_hour_s=$6, skip_reason=$7, revision=revision+1, updated_at=clock_timestamp()
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, occID, targetState, evalDay, in.CurrentOdometerM, in.CurrentHourS, in.Reason); err != nil {
					return mapPGErr(err)
				}
				nextID, _ := s.NewID()
				nextBaseDay := evalDay
				var nextOdo, nextHour, nextDueOdo, nextDueHour *int64
				if pSpec.DistanceIntervalM != nil && dueOdo != nil {
					comp := *dueOdo
					if in.CurrentOdometerM != nil {
						comp = *in.CurrentOdometerM
					}
					nextDue := plan.RebaseNextMeter(*dueOdo, comp, *pSpec.DistanceIntervalM, plan.RebaseRule(rebaseRule))
					nextOdo = &comp
					nextDueOdo = &nextDue
				}
				if pSpec.HourIntervalS != nil && dueHour != nil {
					comp := *dueHour
					if in.CurrentHourS != nil {
						comp = *in.CurrentHourS
					}
					nextDue := plan.RebaseNextMeter(*dueHour, comp, *pSpec.HourIntervalS, plan.RebaseRule(rebaseRule))
					nextHour = &comp
					nextDueHour = &nextDue
				}
				_ = nextOdo
				_ = nextHour
				var nextDueDay *time.Time
				if pSpec.TimeIntervalDays != nil || pSpec.TimeIntervalMonths != nil {
					d := plan.AddIntervalDay(nextBaseDay, pSpec.TimeIntervalMonths, pSpec.TimeIntervalDays)
					nextDueDay = &d
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_maintenance_occurrences(org_id,id,plan_id,asset_id,sequence,state,due_odometer_m,due_hour_s,due_day)
					VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,'UPCOMING',$6,$7,$8)`,
					org, nextID, in.PlanID, assetID, seq+1, nextDueOdo, nextDueHour, nextDueDay); err != nil {
					return mapPGErr(err)
				}
				out = map[string]any{"closed_occurrence_id": occID, "closed_state": targetState, "next_occurrence_id": nextID, "next_due_odometer_m": nextDueOdo, "next_due_hour_s": nextDueHour}
			}
		case "COMPLIANCE_TOGGLE_LICENCE":
			enabled := in.LicenceEnabled != nil && *in.LicenceEnabled
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_retention_and_holds(org_id,id,operation_id,record_kind,category,hold_active,state,reason,actor_membership_id)
				VALUES($1::uuid,$2::uuid,$3::uuid,'POLICY','DRIVER_LICENCE_CONTROL',$4,$5,'Licence expiry control toggled',$6::uuid)
				ON CONFLICT(org_id,id) DO UPDATE SET
				  operation_id=EXCLUDED.operation_id,
				  hold_active=EXCLUDED.hold_active,
				  state=EXCLUDED.state,
				  actor_membership_id=EXCLUDED.actor_membership_id,
				  updated_at=clock_timestamp()`,
				org, org, in.OperationID, enabled, map[bool]string{true: "ENABLED", false: "DISABLED"}[enabled], member); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"licence_control_enabled": enabled}
		case "COMPLIANCE_UPSERT":
			if !validUUID(in.SubjectID) || strings.TrimSpace(in.ItemKind) == "" {
				return app.ErrInvalid
			}
			if in.SubjectKind == "EMPLOYEE" {
				var licenceOn bool
				_ = tx.QueryRow(ctx, `SELECT COALESCE((SELECT hold_active FROM org_retention_and_holds WHERE org_id=$1::uuid AND record_kind='POLICY' AND category='DRIVER_LICENCE_CONTROL' ORDER BY created_at DESC LIMIT 1), false)`, org).Scan(&licenceOn)
				if !licenceOn {
					return ErrControlDisabled
				}
			}
			expDay, err := parseDay(in.ExpiryDay)
			if err != nil {
				return app.ErrInvalid
			}
			lead := in.LeadDays
			if lead <= 0 {
				lead = 30
			}
			evalDay := time.Now().UTC()
			if in.EvalDay != "" {
				evalDay, _ = parseDay(in.EvalDay)
			}
			st, _ := plan.EvaluateCompliance(expDay, evalDay, lead)
			id, _ := s.NewID()
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_compliance_items(org_id,id,operation_id,subject_kind,subject_id,item_kind,expiry_day,lead_days,state,document_id,licence_category)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5::uuid,$6,$7,$8,$9,$10::uuid,$11)`,
				org, id, in.OperationID, in.SubjectKind, in.SubjectID, in.ItemKind, expDay, lead, string(st), optText(in.DocumentID), in.LicenceCategory); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"compliance_item_id": id, "state": string(st)}
		case "INSURANCE_SAVE", "INSURANCE_CLAIM":
			id := in.PolicyID
			if id == "" {
				id, _ = s.NewID()
			}
			if in.Action == "INSURANCE_SAVE" {
				vf, err := parseDay(in.ValidFrom)
				if err != nil {
					return app.ErrInvalid
				}
				exp, err := parseDay(in.ExpiryDay)
				if err != nil {
					return app.ErrInvalid
				}
				assetsJSON, _ := json.Marshal(in.AssetIDs)
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_insurance_policies(org_id,id,operation_id,policy_number,insurer_name,valid_from,expiry_day,asset_ids_json,state)
					VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8::jsonb,'ACTIVE')`,
					org, id, in.OperationID, in.PolicyNumber, in.InsurerName, vf, exp, assetsJSON); err != nil {
					return mapPGErr(err)
				}
				out = map[string]any{"policy_id": id, "state": "ACTIVE"}
			} else {
				if _, err := tx.Exec(ctx, `
					UPDATE org_insurance_policies SET claim_state=$3, incident_report_id=$4::uuid, revision=revision+1, updated_at=clock_timestamp()
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, id, in.ClaimState, optText(in.IncidentReportID)); err != nil {
					return mapPGErr(err)
				}
				out = map[string]any{"policy_id": id, "claim_state": in.ClaimState}
			}
		default:
			return app.ErrInvalid
		}
		targetID := org
		if in.PlanID != "" {
			targetID = in.PlanID
		}
		if err := commandSave(ctx, tx, org, "maint.plan.save", in.OperationID, targetID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "maint.plan.saved", "maintenanceplan", targetID, in.Action)
	})
	return out, replayed, err
}

func (s *Store) SaveAssetDocument(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, assetID string, in app.AssetDocumentInput) (app.AssetDocumentView, bool, error) {
	if !s.ready(true) {
		return app.AssetDocumentView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return app.AssetDocumentView{}, false, err
	}
	var out app.AssetDocumentView
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, assetID, app.AssetDocumentManagePolicy(), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "fleet.asset.doc", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		if in.Action == "DETACH" {
			var cr time.Time
			err := tx.QueryRow(ctx, `
				UPDATE org_asset_document_links SET state='DETACHED', updated_at=clock_timestamp()
				WHERE org_id=$1::uuid AND id=$2::uuid AND asset_id=$3::uuid AND state='ACTIVE'
				RETURNING id::text,asset_id::text,document_id::text,kind,is_cover_photo,state,created_at`,
				org, in.LinkID, assetID).Scan(&out.ID, &out.AssetID, &out.DocumentID, &out.Kind, &out.IsCoverPhoto, &out.State, &cr)
			if err != nil {
				return mapPGErr(err)
			}
			out.CreatedAt = cr.UTC().Format(time.RFC3339Nano)
		} else {
			if !validUUID(in.DocumentID) || strings.TrimSpace(in.Kind) == "" {
				return app.ErrInvalid
			}
			var exp any
			if in.ExpiryDay != "" {
				d, err := parseDay(in.ExpiryDay)
				if err != nil {
					return app.ErrInvalid
				}
				exp = d
			}
			id, _ := s.NewID()
			var cr time.Time
			if err := tx.QueryRow(ctx, `
				INSERT INTO org_asset_document_links(org_id,id,operation_id,asset_id,document_id,kind,expiry_day,is_cover_photo,author_membership_id)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9::uuid)
				RETURNING id::text,asset_id::text,document_id::text,kind,is_cover_photo,state,created_at`,
				org, id, in.OperationID, assetID, in.DocumentID, in.Kind, exp, in.IsCoverPhoto, member).
				Scan(&out.ID, &out.AssetID, &out.DocumentID, &out.Kind, &out.IsCoverPhoto, &out.State, &cr); err != nil {
				return mapPGErr(err)
			}
			out.ExpiryDay = in.ExpiryDay
			out.CreatedAt = cr.UTC().Format(time.RFC3339Nano)
		}
		if err := commandSave(ctx, tx, org, "fleet.asset.doc", in.OperationID, assetID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "fleet.asset.document.saved", "asset", assetID, in.Action)
	})
	return out, replayed, err
}

func (s *Store) ListWorkOrders(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, forReview, mineOnly bool) ([]app.WorkOrderView, error) {
	org := scope.OrgID().String()
	policy := app.WorkOrderPolicy(false)
	target := org
	ownTarget := false
	if forReview {
		policy = app.WorkOrderReviewPolicy(false)
	} else if mineOnly {
		policy = app.MaintenanceRequestPolicy()
		target = member
		ownTarget = true
	}
	return withOrgReadTx(ctx, s, scope, actor, member, target, policy, ownTarget, func(tx pgx.Tx) ([]app.WorkOrderView, error) {
		rows, err := tx.Query(ctx, `
			SELECT id::text,number,asset_id::text,kind,origin_kind,COALESCE(origin_id::text,''),description,state,
			       requester_membership_id::text,COALESCE(reviewer_membership_id::text,''),selected_quote_cents,quote_valid_until,
			       approved_cents,tolerance_percent,parts_cents,service_cents,discount_cents,paid_cents,arithmetic_mismatch,
			       COALESCE(expense_id::text,''),hold_open,after_the_fact,revision,lines_json
			FROM org_work_orders
			WHERE org_id=$1::uuid AND ($2::bool=false OR requester_membership_id=$3::uuid)
			ORDER BY created_at DESC, id LIMIT 200`, org, mineOnly, member)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []app.WorkOrderView{}
		for rows.Next() {
			var w app.WorkOrderView
			var validUntil pgtype.Date
			var linesRaw []byte
			if err := rows.Scan(&w.ID, &w.Number, &w.AssetID, &w.Kind, &w.OriginKind, &w.OriginID, &w.Description, &w.State,
				&w.RequesterID, &w.ReviewerID, &w.SelectedQuoteCents, &validUntil, &w.ApprovedCents, &w.TolerancePercent,
				&w.PartsCents, &w.ServiceCents, &w.DiscountCents, &w.PaidCents, &w.ArithmeticMismatch, &w.ExpenseID,
				&w.HoldOpen, &w.AfterTheFact, &w.Revision, &linesRaw); err != nil {
				return nil, err
			}
			if validUntil.Valid {
				w.QuoteValidUntil = validUntil.Time.Format("2006-01-02")
			}
			_ = json.Unmarshal(linesRaw, &w.Lines)
			out = append(out, w)
		}
		return out, rows.Err()
	})
}

func (s *Store) SaveWorkOrder(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.SaveWorkOrderInput) (map[string]any, bool, error) {
	if !s.ready(true) {
		return nil, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return nil, false, err
	}
	var out map[string]any
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.WorkOrderPolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "maint.wo.save", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		switch in.Action {
		case "COMPONENT_SAVE":
			compID := in.ComponentID
			if compID == "" {
				compID, _ = s.NewID()
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_fleet_components(org_id,id,operation_id,asset_id,kind,serial_number,position,state,installed_at,installed_meter_value,work_order_id,reason)
					VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,'INSTALLED',clock_timestamp(),$8,$9::uuid,$10)`,
					org, compID, in.OperationID, optText(in.AssetID), in.ComponentKind, in.SerialNumber, in.Position, in.MeterValue, optText(in.WorkOrderID), in.Reason); err != nil {
					return mapPGErr(err)
				}
			} else {
				if _, err := tx.Exec(ctx, `
					UPDATE org_fleet_components SET state=$3, removed_at=clock_timestamp(), removed_meter_value=$4, reason=$5, revision=revision+1, updated_at=clock_timestamp()
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, compID, in.ComponentState, in.MeterValue, in.Reason); err != nil {
					return mapPGErr(err)
				}
			}
			out = map[string]any{"component_id": compID, "serial_number": in.SerialNumber, "state": in.ComponentState}
		case "PART_MOVEMENT":
			id, _ := s.NewID()
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_parts_stock_items(org_id,id,operation_id,code,name,unit,quantity_milli,value_centavos,work_order_id,last_movement_kind)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9::uuid,'RECEIPT')
				ON CONFLICT(org_id,code) DO UPDATE SET
				  quantity_milli = org_parts_stock_items.quantity_milli + EXCLUDED.quantity_milli,
				  value_centavos = org_parts_stock_items.value_centavos + EXCLUDED.value_centavos,
				  work_order_id = EXCLUDED.work_order_id,
				  revision = org_parts_stock_items.revision + 1,
				  updated_at = clock_timestamp()`,
				org, id, in.OperationID, in.PartCode, in.PartName, in.PartUnit, in.PartDeltaMilli, in.PartAmountCentavos, optText(in.WorkOrderID)); err != nil {
				return mapPGErr(err)
			}
			var q, v int64
			_ = tx.QueryRow(ctx, `SELECT quantity_milli,value_centavos FROM org_parts_stock_items WHERE org_id=$1::uuid AND code=$2`, org, in.PartCode).Scan(&q, &v)
			out = map[string]any{"part_code": in.PartCode, "quantity_milli": q, "value_centavos": v}
		case "", "CREATE":
			if !validUUID(in.AssetID) || strings.TrimSpace(in.Description) == "" {
				return app.ErrInvalid
			}
			kind := in.Kind
			if kind == "" {
				kind = "CORRECTIVE"
			}
			originKind := in.OriginKind
			if originKind == "" {
				originKind = "MANUAL"
			}
			tol := 10
			if in.TolerancePercent != nil {
				tol = *in.TolerancePercent
			}
			id, _ := s.NewID()
			var num int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO org_work_orders(org_id,id,operation_id,asset_id,kind,origin_kind,origin_id,supplier_id,description,state,requester_membership_id,tolerance_percent,invoice_required,after_the_fact)
				VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7::uuid,$8::uuid,$9,'REQUESTED',$10::uuid,$11,$12,$13)
				RETURNING number`,
				org, id, in.OperationID, in.AssetID, kind, originKind, optText(in.OriginID), optText(in.SupplierID), strings.TrimSpace(in.Description), member, tol, in.InvoiceRequired, in.AfterTheFact).Scan(&num); err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"work_order_id": id, "number": num, "state": "REQUESTED", "revision": 1}
		default:
			if !validUUID(in.WorkOrderID) || in.Revision == nil {
				return app.ErrInvalid
			}
			var state, reqMember, assetID string
			var rev, approvedCents int64
			var appPtr *int64
			var tol int
			var holdOpen, invReq bool
			var invDoc pgtype.Text
			var completedAt pgtype.Timestamptz
			var originID pgtype.Text
			var originKind string
			err := tx.QueryRow(ctx, `
				SELECT asset_id::text, state, revision, requester_membership_id::text, approved_cents, tolerance_percent, hold_open, invoice_required, COALESCE(invoice_document_id::text,''), completed_at, origin_kind, COALESCE(origin_id::text,'')
				FROM org_work_orders WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, in.WorkOrderID).
				Scan(&assetID, &state, &rev, &reqMember, &appPtr, &tol, &holdOpen, &invReq, &invDoc, &completedAt, &originKind, &originID)
			if err != nil {
				return mapPGErr(err)
			}
			if rev != *in.Revision {
				return app.ErrConflict
			}
			if appPtr != nil {
				approvedCents = *appPtr
			}
			switch in.Action {
			case "QUOTE":
				if in.SelectedQuoteCents == nil {
					return ErrQuoteRequired
				}
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateQuoted) {
					return app.ErrConflict
				}
				var validDay any
				if in.QuoteValidUntil != "" {
					d, err := parseDay(in.QuoteValidUntil)
					if err != nil {
						return app.ErrInvalid
					}
					validDay = d
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET state='QUOTED', selected_quote_cents=$3, quote_valid_until=$4, supplier_id=COALESCE($5::uuid,supplier_id), revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID, *in.SelectedQuoteCents, validDay, optText(in.SupplierID))
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "QUOTED", "revision": rev + 1}
			case "APPROVE_DIRECT":
				if in.SelectedQuoteCents == nil {
					return ErrQuoteRequired
				}
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateApproved) || (in.DirectLimitCents != nil && *in.SelectedQuoteCents > *in.DirectLimitCents) {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET state='APPROVED', selected_quote_cents=$3, approved_cents=$3, revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID, *in.SelectedQuoteCents)
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "APPROVED", "revision": rev + 1}
			case "SCHEDULE":
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateScheduled) {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET state='SCHEDULED', scheduled_at=clock_timestamp(), revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID)
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "SCHEDULED", "revision": rev + 1}
			case "START":
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateInExecution) {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET state='IN_EXECUTION', hold_open=true, hold_started_at=COALESCE(hold_started_at,clock_timestamp()), started_at=COALESCE(started_at,clock_timestamp()), revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID)
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "IN_EXECUTION", "hold_open": true, "revision": rev + 1}
			case "COMPLETE":
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateCompleted) {
					return app.ErrConflict
				}
				var lines []plan.WorkOrderLine
				for _, l := range in.Lines {
					lines = append(lines, plan.WorkOrderLine{Kind: plan.LineKind(l.Kind), QuantityMilli: l.QuantityMilli, UnitPriceMilli: l.UnitPriceMilli})
				}
				paid := int64(0)
				if in.PaidCents != nil {
					paid = *in.PaidCents
				}
				rec, errRec := plan.EvaluateWorkOrder(lines, approvedCents, tol, in.DiscountCents, paid)
				if errRec != nil {
					return app.ErrInvalid
				}
				linesRaw, _ := json.Marshal(in.Lines)
				_, err = tx.Exec(ctx, `
					UPDATE org_work_orders
					SET state='COMPLETED', lines_json=$3::jsonb, parts_cents=$4, service_cents=$5, discount_cents=$6, paid_cents=$7, arithmetic_mismatch=$8, invoice_document_id=COALESCE($9::uuid,invoice_document_id), completed_at=clock_timestamp(), revision=revision+1
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID, linesRaw, rec.PartsCentavos, rec.ServiceCentavos, in.DiscountCents, paid, rec.ArithmeticMismatch, optText(in.InvoiceDocumentID))
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "COMPLETED", "parts_cents": rec.PartsCentavos, "service_cents": rec.ServiceCentavos, "arithmetic_mismatch": rec.ArithmeticMismatch, "revision": rev + 1}
			case "RELEASE_HOLD":
				if !holdOpen {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET hold_open=false, hold_ended_at=clock_timestamp(), revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID)
				out = map[string]any{"work_order_id": in.WorkOrderID, "hold_open": false, "revision": rev + 1}
			case "CLOSE":
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateClosed) {
					return app.ErrConflict
				}
				if holdOpen {
					return app.ErrHoldOpen
				}
				if invReq && invDoc.String == "" && in.InvoiceDocumentID == "" {
					return app.ErrInvoiceRequired
				}
				var lines []plan.WorkOrderLine
				var linesRaw []byte
				var paidCents, discountCents int64
				if err := tx.QueryRow(ctx, `SELECT lines_json, COALESCE(paid_cents,0), discount_cents FROM org_work_orders WHERE org_id=$1::uuid AND id=$2::uuid`, org, in.WorkOrderID).
					Scan(&linesRaw, &paidCents, &discountCents); err != nil {
					return mapPGErr(err)
				}
				var decoded []app.WorkOrderLineInput
				_ = json.Unmarshal(linesRaw, &decoded)
				for _, l := range decoded {
					lines = append(lines, plan.WorkOrderLine{Kind: plan.LineKind(l.Kind), QuantityMilli: l.QuantityMilli, UnitPriceMilli: l.UnitPriceMilli})
				}
				rec, errRec := plan.EvaluateWorkOrder(lines, approvedCents, tol, discountCents, paidCents)
				if errRec != nil {
					return app.ErrInvalid
				}
				if appPtr != nil && rec.OverrunExceeded {
					return app.ErrOverrunDenied
				}
				compTime := time.Now().UTC()
				if completedAt.Valid {
					compTime = completedAt.Time.UTC()
				}
				periodMonth := time.Date(compTime.Year(), compTime.Month(), 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				var periodState string
				if err := tx.QueryRow(ctx, `SELECT state FROM org_finance_periods WHERE org_id=$1::uuid AND period_month=$2::date`, org, periodMonth).Scan(&periodState); err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return mapPGErr(err)
				} else if err == nil && periodState == "LOCKED" {
					return orgapi.PeriodLocked
				}
				expID, _ := s.NewID()
				if _, err := tx.Exec(ctx, `UPDATE org_work_orders SET state='CLOSED', expense_id=$3::uuid, closed_at=clock_timestamp(), revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID, expID); err != nil {
					return mapPGErr(err)
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_operating_cost_entries(org_id,id,origin_kind,source_id,work_order_id,asset_id,category,operating_cost,paid_centavos,company_centavos,reporting_day,occurred_at,review_state)
					VALUES($1::uuid,$2::uuid,'WORK_ORDER',$3::uuid,$3::uuid,$4::uuid,'MAINTENANCE',true,$5,$5,$6::date,$7,'APPROVED')`,
					org, expID, in.WorkOrderID, assetID, paidCents, compTime.Format("2006-01-02"), compTime); err != nil {
					return mapPGErr(err)
				}
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "CLOSED", "expense_id": expID, "recognized_expense_cents": paidCents, "revision": rev + 1}
			case "CANCEL":
				if !plan.CanTransitionWorkOrder(plan.WorkOrderState(state), plan.WOStateCancelled) || strings.TrimSpace(in.Reason) == "" {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_work_orders SET state='CANCELLED', hold_open=false, hold_ended_at=COALESCE(hold_ended_at,clock_timestamp()), reason=$3, revision=revision+1 WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.WorkOrderID, in.Reason)
				out = map[string]any{"work_order_id": in.WorkOrderID, "state": "CANCELLED", "revision": rev + 1}
			default:
				return app.ErrInvalid
			}
			if err != nil {
				return mapPGErr(err)
			}
		}
		if err := commandSave(ctx, tx, org, "maint.wo.save", in.OperationID, org, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "maint.workorder.updated", "workorder", org, in.Action)
	})
	return out, replayed, err
}

func (s *Store) ReviewWorkOrder(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.ReviewWorkOrderInput) (app.WorkOrderView, bool, error) {
	if !s.ready(true) {
		return app.WorkOrderView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return app.WorkOrderView{}, false, err
	}
	if !validUUID(in.WorkOrderID) {
		return app.WorkOrderView{}, false, app.ErrInvalid
	}
	var out app.WorkOrderView
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.WorkOrderReviewPolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "maint.wo.review", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var reqMember, state string
		var rev int64
		var quoteCents *int64
		var validUntil pgtype.Date
		if err := tx.QueryRow(ctx, `SELECT requester_membership_id::text,state,revision,selected_quote_cents,quote_valid_until FROM org_work_orders WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`, org, in.WorkOrderID).
			Scan(&reqMember, &state, &rev, &quoteCents, &validUntil); err != nil {
			return mapPGErr(err)
		}
		if rev != in.Revision {
			return app.ErrConflict
		}
		var validPtr *time.Time
		if validUntil.Valid {
			t := validUntil.Time
			validPtr = &t
		}
		if err := plan.ValidateReview(reqMember, member, validPtr, time.Now().UTC()); err != nil {
			return err
		}
		var nextState string
		var approved *int64
		switch in.Decision {
		case "APPROVE":
			if state != "QUOTED" && state != "REQUESTED" {
				return app.ErrConflict
			}
			nextState = "APPROVED"
			approved = quoteCents
			if in.ApprovedCents != nil {
				approved = in.ApprovedCents
			}
		case "RETURN":
			if state != "QUOTED" || strings.TrimSpace(in.Reason) == "" {
				return app.ErrConflict
			}
			nextState = "QUOTED"
		case "REJECT":
			if strings.TrimSpace(in.Reason) == "" {
				return app.ErrInvalid
			}
			nextState = "REJECTED"
		default:
			return app.ErrInvalid
		}
		if err := tx.QueryRow(ctx, `
			UPDATE org_work_orders SET state=$3, reviewer_membership_id=$4::uuid, approved_cents=COALESCE($5,approved_cents), reason=$6, revision=revision+1
			WHERE org_id=$1::uuid AND id=$2::uuid
			RETURNING id::text,number,asset_id::text,kind,origin_kind,COALESCE(origin_id::text,''),description,state,
			          requester_membership_id::text,COALESCE(reviewer_membership_id::text,''),selected_quote_cents,approved_cents,tolerance_percent,revision`,
			org, in.WorkOrderID, nextState, member, approved, in.Reason).
			Scan(&out.ID, &out.Number, &out.AssetID, &out.Kind, &out.OriginKind, &out.OriginID, &out.Description, &out.State,
				&out.RequesterID, &out.ReviewerID, &out.SelectedQuoteCents, &out.ApprovedCents, &out.TolerancePercent, &out.Revision); err != nil {
			return mapPGErr(err)
		}
		if err := commandSave(ctx, tx, org, "maint.wo.review", in.OperationID, in.WorkOrderID, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "maint.workorder.reviewed", "workorder", in.WorkOrderID, in.Decision)
	})
	return out, replayed, err
}

var _ = errors.Is
