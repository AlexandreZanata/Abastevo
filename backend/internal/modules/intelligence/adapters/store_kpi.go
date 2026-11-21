package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	intel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	plan "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/plan"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) KPIOverview(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, q app.KPIOverviewQuery) (app.KPIOverviewView, error) {
	org := scope.OrgID().String()
	dim := q.Dimension
	if dim == "" {
		dim = "ORG"
	}
	return withOrgReadTx(ctx, s, scope, actor, member, org, app.KpiPolicy(), false, func(tx pgx.Tx) (app.KPIOverviewView, error) {
		var fuelSpendApproved, fuelSpendPending, fuelCountApproved, fuelCountPending int64
		if err := tx.QueryRow(ctx, `
			SELECT
			  COALESCE(SUM(company_centavos) FILTER(WHERE review_state IN('APPROVED','SUBMITTED')), 0),
			  COALESCE(SUM(company_centavos) FILTER(WHERE review_state='SUBMITTED'), 0),
			  COUNT(*) FILTER(WHERE review_state IN('APPROVED','SUBMITTED')),
			  COUNT(*) FILTER(WHERE review_state='SUBMITTED')
			FROM (
			  SELECT company_centavos, review_state FROM org_expenses WHERE org_id=$1::uuid AND origin_kind='FUELING'
			  UNION ALL
			  SELECT company_centavos, review_state FROM org_operating_cost_entries WHERE org_id=$1::uuid AND category='FUEL'
			) x`, org).Scan(&fuelSpendApproved, &fuelSpendPending, &fuelCountApproved, &fuelCountPending); err != nil {
			return app.KPIOverviewView{}, err
		}
		var maintSpend int64
		var maintCount int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(company_centavos),0), COUNT(*)
			FROM org_operating_cost_entries
			WHERE org_id=$1::uuid AND category='MAINTENANCE' AND review_state='APPROVED'`, org).Scan(&maintSpend, &maintCount); err != nil {
			return app.KPIOverviewView{}, err
		}
		var totalOpSpend int64
		var totalOpCount int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(company_centavos),0), COUNT(*)
			FROM (
			  SELECT company_centavos FROM org_expenses WHERE org_id=$1::uuid AND review_state IN('APPROVED','SUBMITTED')
			  UNION ALL
			  SELECT company_centavos FROM org_operating_cost_entries WHERE org_id=$1::uuid AND operating_cost AND review_state='APPROVED'
			) x`, org).Scan(&totalOpSpend, &totalOpCount); err != nil {
			return app.KPIOverviewView{}, err
		}
		var openFindings, pendingReview, syncReview, unrecDocs int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_findings WHERE org_id=$1::uuid AND state IN('OPEN','IN_REVIEW')`, org).Scan(&openFindings); err != nil {
			return app.KPIOverviewView{}, err
		}
		if err := tx.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM org_sync_review_items i
			WHERE i.org_id=$1::uuid
			  AND NOT EXISTS (
			    SELECT 1 FROM org_sync_review_resolutions r
			    WHERE r.org_id=i.org_id AND r.review_item_id=i.id
			  )`, org).Scan(&syncReview); err != nil {
			return app.KPIOverviewView{}, err
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_work_orders WHERE org_id=$1::uuid AND state IN('REQUESTED','QUOTED')`, org).Scan(&pendingReview); err != nil {
			return app.KPIOverviewView{}, err
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_documents WHERE org_id=$1::uuid AND storage_state='STORED'`, org).Scan(&unrecDocs); err != nil {
			return app.KPIOverviewView{}, err
		}

		// Distance usage per asset using VALID odometer readings
		rRows, err := tx.Query(ctx, `
			SELECT m.asset_id::text, MIN(r.value), MAX(r.value), COUNT(*)
			FROM org_meter_readings r JOIN org_meters m ON m.org_id=r.org_id AND m.id=r.meter_id
			WHERE r.org_id=$1::uuid AND m.kind='ODOMETER' AND r.state='VALID'
			GROUP BY m.asset_id`, org)
		if err != nil {
			return app.KPIOverviewView{}, err
		}
		var totalDistM int64
		hasDistBasis := false
		for rRows.Next() {
			var aid string
			var minV, maxV, cnt int64
			if err := rRows.Scan(&aid, &minV, &maxV, &cnt); err != nil {
				rRows.Close()
				return app.KPIOverviewView{}, err
			}
			if cnt >= 2 && maxV > minV {
				totalDistM += maxV - minV
				hasDistBasis = true
			}
		}
		if err := rRows.Err(); err != nil {
			rRows.Close()
			return app.KPIOverviewView{}, err
		}
		rRows.Close()

		// Availability from ACTIVE assets minus merged maintenance holds over 30d window
		var activeAssets int64
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_assets WHERE org_id=$1::uuid AND state='ACTIVE'`, org).Scan(&activeAssets); err != nil {
			return app.KPIOverviewView{}, err
		}
		activeSec := activeAssets * 30 * 86400
		var unavailSec int64
		wRows, err := tx.Query(ctx, `SELECT hold_started_at, COALESCE(hold_ended_at, clock_timestamp()) FROM org_work_orders WHERE org_id=$1::uuid AND hold_started_at IS NOT NULL`, org)
		if err != nil {
			return app.KPIOverviewView{}, err
		}
		var windows []plan.HoldWindow
		for wRows.Next() {
			var st, en time.Time
			if err := wRows.Scan(&st, &en); err != nil {
				wRows.Close()
				return app.KPIOverviewView{}, err
			}
			windows = append(windows, plan.HoldWindow{Start: st, End: en})
		}
		if err := wRows.Err(); err != nil {
			wRows.Close()
			return app.KPIOverviewView{}, err
		}
		wRows.Close()
		now := time.Now().UTC()
		unavailSec = plan.UnavailableSeconds(windows, now.Add(-30*24*time.Hour), now)
		avail := intel.ComputeAvailability(activeSec, unavailSec)

		// Closed intervals / fuelings consumption summary
		var ivIntervals []intel.RatioInterval
		var totalVolMilli int64
		fRows, err := tx.Query(ctx, `
			SELECT asset_id::text, product, SUM(quantity)
			FROM (
			  SELECT asset_id, product, quantity FROM org_fuelings WHERE org_id=$1::uuid
			  UNION ALL
			  SELECT asset_id, product, quantity_milli AS quantity FROM org_internal_fuelings WHERE org_id=$1::uuid AND review_state='APPROVED'
			) f GROUP BY asset_id, product`, org)
		if err != nil {
			return app.KPIOverviewView{}, err
		}
		for fRows.Next() {
			var aid, prod string
			var qty int64
			if err := fRows.Scan(&aid, &prod, &qty); err != nil {
				fRows.Close()
				return app.KPIOverviewView{}, err
			}
			totalVolMilli += qty
			if hasDistBasis && totalDistM > 0 && len(ivIntervals) == 0 {
				ivIntervals = append(ivIntervals, intel.RatioInterval{AssetID: aid, Product: prod, DistanceM: totalDistM, QuantityMilli: qty, CostCentavos: fuelSpendApproved})
			}
		}
		if err := fRows.Err(); err != nil {
			fRows.Close()
			return app.KPIOverviewView{}, err
		}
		fRows.Close()
		cons := intel.ComputeDistanceConsumption(ivIntervals)

		// Stored price comparisons
		cmpRows, err := tx.Query(ctx, `
			SELECT id::text,fueling_id::text,reference_source,methodology_version,status,insufficiency_reason,place_rule,product,price_condition,coverage_stations,difference_cents,difference_per_unit_milli
			FROM org_price_comparisons WHERE org_id=$1::uuid ORDER BY computed_at DESC LIMIT 50`, org)
		if err != nil {
			return app.KPIOverviewView{}, err
		}
		var cmps []app.PriceComparisonView
		var priceDiffSum int64
		var comparedN, totalCmp int
		for cmpRows.Next() {
			var cv app.PriceComparisonView
			if err := cmpRows.Scan(&cv.ID, &cv.FuelingID, &cv.ReferenceSource, &cv.MethodologyVersion, &cv.Status, &cv.InsufficiencyReason, &cv.PlaceRule, &cv.Product, &cv.PriceCondition, &cv.CoverageStations, &cv.DifferenceCents, &cv.DifferencePerUnitMilli); err != nil {
				cmpRows.Close()
				return app.KPIOverviewView{}, err
			}
			totalCmp++
			if cv.Status == "COMPUTED" && cv.DifferenceCents != nil {
				comparedN++
				priceDiffSum += *cv.DifferenceCents
				cv.DisplayNote = "Diferença em relação à referência para revisão operacional"
			}
			cmps = append(cmps, cv)
		}
		if err := cmpRows.Err(); err != nil {
			cmpRows.Close()
			return app.KPIOverviewView{}, err
		}
		cmpRows.Close()

		values := []app.KPIValueView{
			{IndicatorID: "IND-001", NamePT: "Gasto com combustível", Status: "COMPUTED", DisplayValue: strconv.FormatInt(fuelSpendApproved, 10), Unit: "centavos", Numerator: fuelSpendApproved, Denominator: 1, ApprovedCount: int(fuelCountApproved), PendingCount: int(fuelCountPending), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-002", NamePT: "Volume por produto", Status: "COMPUTED", DisplayValue: strconv.FormatInt(totalVolMilli, 10), Unit: "mL", Numerator: totalVolMilli, Denominator: 1, ApprovedCount: int(fuelCountApproved), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-008", NamePT: "Custo operacional total", Status: "COMPUTED", DisplayValue: strconv.FormatInt(totalOpSpend, 10), Unit: "centavos", Numerator: totalOpSpend, Denominator: 1, ApprovedCount: int(totalOpCount), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-011", NamePT: "Custo de manutenção", Status: "COMPUTED", DisplayValue: strconv.FormatInt(maintSpend, 10), Unit: "centavos", Numerator: maintSpend, Denominator: 1, ApprovedCount: int(maintCount), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-012", NamePT: "Disponibilidade da frota", Status: avail.Status, InsufficiencyReason: string(avail.Reason), ValueRational: avail.Ratio, DisplayValue: avail.Percent1DP, Unit: "%", Numerator: avail.AvailableSeconds, Denominator: avail.ActiveSeconds, MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-013", NamePT: "Registros aguardando revisão", Status: "COMPUTED", DisplayValue: strconv.Itoa(pendingReview), Unit: "count", Numerator: int64(pendingReview), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-014", NamePT: "Documentos não conciliados", Status: "COMPUTED", DisplayValue: strconv.Itoa(unrecDocs), Unit: "count", Numerator: int64(unrecDocs), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-015", NamePT: "Envios offline aguardando decisão", Status: "COMPUTED", DisplayValue: strconv.Itoa(syncReview), Unit: "count", Numerator: int64(syncReview), MethodVersion: intel.KPIVersion},
			{IndicatorID: "IND-019", NamePT: "Achados em aberto", Status: "COMPUTED", DisplayValue: strconv.Itoa(openFindings), Unit: "count", Numerator: int64(openFindings), MethodVersion: intel.KPIVersion},
		}
		if cons.Status == "COMPUTED" {
			values = append(values, app.KPIValueView{IndicatorID: "IND-003", NamePT: "Consumo (km/L)", Status: "COMPUTED", ValueRational: cons.KmPerUnitRat, DisplayValue: cons.KmPerUnit2DP, Unit: "km/L", Numerator: cons.DistanceM, Denominator: cons.QuantityMilli, SampleSmall: cons.SmallSample, MethodVersion: intel.KPIVersion})
		} else {
			values = append(values, app.KPIValueView{IndicatorID: "IND-003", NamePT: "Consumo (km/L)", Status: "INSUFFICIENT_DATA", InsufficiencyReason: string(cons.Reason), Unit: "km/L", MethodVersion: intel.KPIVersion})
		}
		if hasDistBasis && totalDistM > 0 {
			values = append(values, app.KPIValueView{IndicatorID: "IND-017", NamePT: "Distância percorrida", Status: "COMPUTED", DisplayValue: strconv.FormatInt(totalDistM, 10), Unit: "m", Numerator: totalDistM, MethodVersion: intel.KPIVersion})
		} else {
			values = append(values, app.KPIValueView{IndicatorID: "IND-017", NamePT: "Distância percorrida", Status: "INSUFFICIENT_DATA", InsufficiencyReason: string(intel.ReasonNoDistanceBasis), Unit: "m", MethodVersion: intel.KPIVersion})
		}
		if comparedN > 0 {
			values = append(values, app.KPIValueView{IndicatorID: "IND-016", NamePT: fmt.Sprintf("Diferença em relação à referência (%d de %d comparados)", comparedN, totalCmp), Status: "COMPUTED", DisplayValue: strconv.FormatInt(priceDiffSum, 10), Unit: "centavos", Numerator: priceDiffSum, ApprovedCount: comparedN, MethodVersion: intel.PriceMethodologyVersion})
		} else {
			values = append(values, app.KPIValueView{IndicatorID: "IND-016", NamePT: "Diferença em relação à referência", Status: "INSUFFICIENT_DATA", InsufficiencyReason: string(intel.ReasonNoReference), Unit: "centavos", MethodVersion: intel.PriceMethodologyVersion})
		}

		// Ensure all registered IND-001..IND-020 appear in values deterministically.
		present := make(map[string]app.KPIValueView, len(values))
		for _, v := range values {
			present[v.IndicatorID] = v
		}
		specs := intel.Indicators()
		complete := make([]app.KPIValueView, 0, len(specs))
		for _, spec := range specs {
			if existing, ok := present[spec.ID]; ok {
				complete = append(complete, existing)
				continue
			}
			reason := intel.ReasonNoClosedInterval
			switch spec.ID {
			case "IND-004", "IND-018":
				reason = intel.ReasonNoHourBasis
			case "IND-005", "IND-009", "IND-020":
				reason = intel.ReasonNoDistanceBasis
			case "IND-006", "IND-010":
				reason = intel.ReasonZeroDenominator
			case "IND-007":
				reason = intel.ReasonZeroBaseline
			}
			complete = append(complete, app.KPIValueView{
				IndicatorID:         spec.ID,
				NamePT:              spec.NamePT,
				Status:              "INSUFFICIENT_DATA",
				InsufficiencyReason: string(reason),
				Unit:                spec.Unit,
				MethodVersion:       intel.KPIVersion,
			})
		}

		return app.KPIOverviewView{
			Dimension:        dim,
			IncludePending:   q.IncludePending,
			Registry:         specs,
			Values:           complete,
			PriceComparisons: cmps,
		}, nil
	})
}

func (s *Store) ListFindings(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, forReview bool) ([]app.FindingView, error) {
	org := scope.OrgID().String()
	policy := app.FindingReadPolicy()
	if forReview {
		policy = app.FindingResolvePolicy(false)
	}
	return withOrgReadTx(ctx, s, scope, actor, member, org, policy, false, func(tx pgx.Tx) ([]app.FindingView, error) {
		rows, err := tx.Query(ctx, `
			SELECT id::text,finding_type,subject_key,COALESCE(asset_id::text,''),COALESCE(author_membership_id::text,''),
			       severity,state,method_version,title,non_proof_note,evidence_json,
			       COALESCE(assigned_membership_id::text,''),COALESCE(decided_by_membership_id::text,''),
			       decision_reason,followup_action,COALESCE(run_id::text,''),COALESCE(superseded_by_run_id::text,''),revision,created_at
			FROM org_findings WHERE org_id=$1::uuid ORDER BY created_at DESC, id LIMIT 200`, org)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []app.FindingView{}
		for rows.Next() {
			var f app.FindingView
			var cr time.Time
			if err := rows.Scan(&f.ID, &f.FindingType, &f.SubjectKey, &f.AssetID, &f.AuthorID, &f.Severity, &f.State, &f.MethodVersion,
				&f.Title, &f.NonProofNote, &f.Evidence, &f.AssignedID, &f.DecidedByID, &f.DecisionReason, &f.FollowupAction,
				&f.RunID, &f.SupersededByRun, &f.Revision, &cr); err != nil {
				return nil, err
			}
			f.CreatedAt = cr.UTC().Format(time.RFC3339Nano)
			out = append(out, f)
		}
		return out, rows.Err()
	})
}

func (s *Store) DecideFinding(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.DecideFindingInput) (map[string]any, bool, error) {
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
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.FindingResolvePolicy(true), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "find.decide", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		switch in.Action {
		case "BENCHMARK":
			// EMP-REL-010 (VAL-37): Repeatable synthetic fleet capacity benchmark inside scoped transaction.
			count := in.BenchmarkSyntheticCount
			if count <= 0 || count > 5000 {
				count = 500
			}
			start := time.Now()
			sum := int64(0)
			for i := 0; i < count; i++ {
				iv := intel.ComputeDistanceConsumption([]intel.RatioInterval{{Product: "DIESEL_S10", DistanceM: 612000 + int64(i), QuantityMilli: 180000, CostCentavos: 116820}})
				if iv.Status == "COMPUTED" {
					sum += iv.DistanceM
				}
			}
			elapsedMS := time.Since(start).Milliseconds()
			out = map[string]any{"synthetic_intervals": count, "total_distance_m": sum, "elapsed_ms": elapsedMS, "mode": "SYNTHETIC_HOST_BENCHMARK"}
		case "RAISE_OR_DETECT":
			fType := in.SyntheticSignalKind
			if fType == "" {
				fType = "FT-001"
			}
			sev := in.Severity
			if sev == "" {
				sev = "MEDIUM"
			}
			subj := in.SubjectKey
			if subj == "" {
				subj = in.OperationID
			}
			fid, _ := s.NewID()
			var existingID string
			err := tx.QueryRow(ctx, `SELECT id::text FROM org_findings WHERE org_id=$1::uuid AND finding_type=$2 AND subject_key=$3 AND method_version='findings-v1' AND state IN('OPEN','IN_REVIEW')`,
				org, fType, subj).Scan(&existingID)
			if err == nil {
				out = map[string]any{"finding_id": existingID, "deduplicated": true}
			} else {
				sig := intel.FindingSignal{Type: intel.FindingType(fType), Severity: intel.FindingSeverity(sev), Title: "Diferença operacional para revisão", NonProof: "Sinal para revisão; não altera registros nem comprova irregularidade."}
				if fType == "FT-006" {
					sig.Title = "Preço registrado acima da referência comparável"
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO org_findings(org_id,id,finding_type,subject_key,asset_id,author_membership_id,severity,state,method_version,title,non_proof_note,evidence_json)
					VALUES($1::uuid,$2::uuid,$3,$4,$5::uuid,$6::uuid,$7,'OPEN','findings-v1',$8,$9,'{"source":"detector"}'::jsonb)`,
					org, fid, fType, subj, optText(in.AssetID), optText(in.AuthorID), sev, sig.Title, sig.NonProof); err != nil {
					return mapPGErr(err)
				}
				out = map[string]any{"finding_id": fid, "deduplicated": false, "state": "OPEN", "severity": sev}
			}
		case "REPROCESS":
			runID, _ := s.NewID()
			pStart := time.Now().UTC().AddDate(0, -1, 0)
			pEnd := time.Now().UTC()
			if in.PeriodStart != "" {
				if p, err := parseDay(in.PeriodStart); err == nil {
					pStart = p
				}
			}
			if in.PeriodEnd != "" {
				if p, err := parseDay(in.PeriodEnd); err == nil {
					pEnd = p
				}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO org_finding_runs(org_id,id,operation_id,method_version,period_start,period_end,state,requested_by_membership_id)
				VALUES($1::uuid,$2::uuid,$3::uuid,'findings-v1',$4,$5,'RUNNING',$6::uuid)`,
				org, runID, in.OperationID, pStart, pEnd, member); err != nil {
				return mapPGErr(err)
			}
			tag, err := tx.Exec(ctx, `
				UPDATE org_findings SET state='SUPERSEDED', superseded_by_run_id=$2::uuid, revision=revision+1, updated_at=clock_timestamp()
				WHERE org_id=$1::uuid AND state IN('OPEN','IN_REVIEW')`, org, runID)
			if err != nil {
				return mapPGErr(err)
			}
			supCount := int(tag.RowsAffected())
			_, err = tx.Exec(ctx, `UPDATE org_finding_runs SET state='COMPLETED', superseded_count=$3, completed_at=clock_timestamp() WHERE org_id=$1::uuid AND id=$2::uuid`,
				org, runID, supCount)
			if err != nil {
				return mapPGErr(err)
			}
			out = map[string]any{"run_id": runID, "state": "COMPLETED", "superseded_count": supCount}
		default:
			if !validUUID(in.FindingID) {
				return app.ErrInvalid
			}
			var curState string
			var authorID pgtype.Text
			var rev int64
			err := tx.QueryRow(ctx, `SELECT state, COALESCE(author_membership_id::text,''), revision FROM org_findings WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`,
				org, in.FindingID).Scan(&curState, &authorID, &rev)
			if err != nil {
				return mapPGErr(err)
			}
			if in.Revision != nil && rev != *in.Revision {
				return app.ErrConflict
			}
			switch in.Action {
			case "TAKE":
				if curState != "OPEN" && curState != "IN_REVIEW" {
					return app.ErrFindingClosed
				}
				if authorID.String != "" && authorID.String == member {
					return app.ErrFindingSelfResolve
				}
				_, err = tx.Exec(ctx, `UPDATE org_findings SET state='IN_REVIEW', assigned_membership_id=$3::uuid, revision=revision+1, updated_at=clock_timestamp() WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.FindingID, member)
				out = map[string]any{"finding_id": in.FindingID, "state": "IN_REVIEW", "revision": rev + 1}
			case "RELEASE":
				if curState != "IN_REVIEW" {
					return app.ErrConflict
				}
				_, err = tx.Exec(ctx, `UPDATE org_findings SET state='OPEN', assigned_membership_id=NULL, revision=revision+1, updated_at=clock_timestamp() WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.FindingID)
				out = map[string]any{"finding_id": in.FindingID, "state": "OPEN", "revision": rev + 1}
			case "CONFIRM", "EXPLAIN", "FALSE_POSITIVE":
				if errV := intel.ValidateFindingDecision(intel.FindingState(curState), authorID.String, member, in.Reason); errV != nil {
					return errV
				}
				nextState := map[string]string{
					"CONFIRM":        "CONFIRMED",
					"EXPLAIN":        "EXPLAINED",
					"FALSE_POSITIVE": "FALSE_POSITIVE",
				}[in.Action]
				_, err = tx.Exec(ctx, `
					UPDATE org_findings
					SET state=$3, decided_by_membership_id=$4::uuid, decision_reason=$5, followup_action=$6, revision=revision+1, updated_at=clock_timestamp()
					WHERE org_id=$1::uuid AND id=$2::uuid`,
					org, in.FindingID, nextState, member, strings.TrimSpace(in.Reason), in.FollowupAction)
				out = map[string]any{"finding_id": in.FindingID, "state": nextState, "revision": rev + 1}
			default:
				return app.ErrInvalid
			}
			if err != nil {
				return mapPGErr(err)
			}
		}
		if err := commandSave(ctx, tx, org, "find.decide", in.OperationID, org, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "find.finding.decided", "finding", org, in.Action)
	})
	return out, replayed, err
}

func (s *Store) SaveFindingSettings(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.FindingSettingsInput) (map[string]any, bool, error) {
	if !s.ready(true) {
		return nil, false, app.ErrUnavailable
	}
	cfg := intel.FindingSettings{
		DuplicateWindowMinutes:  in.DuplicateWindowMinutes,
		MaxSpeedKmh:             in.MaxSpeedKmh,
		TankTolerancePercent:    in.TankTolerancePercent,
		OutlierThresholdPercent: in.OutlierThresholdPercent,
		DocQtyTolerancePercent:  in.DocQtyTolerancePercent,
		DocDateToleranceDays:    in.DocDateToleranceDays,
		PriceThresholdPercent:   in.PriceThresholdPercent,
		PriceMinDiffCents:       in.PriceMinDiffCents,
	}
	if err := cfg.Validate(); err != nil {
		return nil, false, app.ErrInvalid
	}
	org := scope.OrgID().String()
	reqHash, err := app.HashRequest(in)
	if err != nil {
		return nil, false, err
	}
	var out map[string]any
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, org, app.SettingsManagePolicy(), false); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "find.settings", in.OperationID, reqHash); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		typesJSON, _ := json.Marshal(in.EnabledTypes)
		if len(in.EnabledTypes) == 0 {
			typesJSON = []byte(`["FT-001","FT-002","FT-003","FT-004","FT-005","FT-006"]`)
		}
		refSrc := "OFFICIAL"
		if in.ReferenceSource != "" {
			refSrc = in.ReferenceSource
		}
		var rev int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO org_finding_settings(org_id,duplicate_window_minutes,max_speed_kmh,tank_tolerance_percent,outlier_threshold_percent,doc_quantity_tolerance_percent,doc_date_tolerance_days,price_threshold_percent,price_min_diff_cents,reference_source,enabled_types_json)
			VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)
			ON CONFLICT(org_id) DO UPDATE SET
			 duplicate_window_minutes=EXCLUDED.duplicate_window_minutes, max_speed_kmh=EXCLUDED.max_speed_kmh,
			 tank_tolerance_percent=EXCLUDED.tank_tolerance_percent, outlier_threshold_percent=EXCLUDED.outlier_threshold_percent,
			 doc_quantity_tolerance_percent=EXCLUDED.doc_quantity_tolerance_percent, doc_date_tolerance_days=EXCLUDED.doc_date_tolerance_days,
			 price_threshold_percent=EXCLUDED.price_threshold_percent, price_min_diff_cents=EXCLUDED.price_min_diff_cents,
			 reference_source=EXCLUDED.reference_source, enabled_types_json=EXCLUDED.enabled_types_json,
			 revision=org_finding_settings.revision+1, updated_at=clock_timestamp()
			RETURNING revision`,
			org, cfg.DuplicateWindowMinutes, cfg.MaxSpeedKmh, cfg.TankTolerancePercent, cfg.OutlierThresholdPercent,
			cfg.DocQtyTolerancePercent, cfg.DocDateToleranceDays, cfg.PriceThresholdPercent, cfg.PriceMinDiffCents, refSrc, typesJSON).Scan(&rev); err != nil {
			return mapPGErr(err)
		}
		out = map[string]any{"revision": rev, "reference_source": refSrc}
		if err := commandSave(ctx, tx, org, "find.settings", in.OperationID, org, reqHash, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, member, "find.settings.updated", "organization", org, refSrc)
	})
	return out, replayed, err
}

func (s *Store) ListAuditEvents(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, q app.AuditQuery) ([]app.AuditEventView, error) {
	org := scope.OrgID().String()
	lim := q.Limit
	if lim <= 0 || lim > 200 {
		lim = 100
	}
	return withOrgReadTx(ctx, s, scope, actor, member, org, app.AuditReadPolicy(), false, func(tx pgx.Tx) ([]app.AuditEventView, error) {
		rows, err := tx.Query(ctx, `
			SELECT operation_id::text, 'AUDIT', action, actor_id::text, target_type, target_id::text, COALESCE(reason_code,''), recorded_at
			FROM org_audit_events
			WHERE org_id=$1::uuid
			  AND ($2::text='' OR actor_id=NULLIF($2,'')::uuid)
			  AND ($3::text='' OR action=$3)
			  AND ($4::text='' OR target_type=$4)
			ORDER BY recorded_at DESC, operation_id LIMIT $5`, org, strings.TrimSpace(q.ActorID), strings.TrimSpace(q.Action), strings.TrimSpace(q.TargetType), lim)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []app.AuditEventView{}
		for rows.Next() {
			var ev app.AuditEventView
			var at time.Time
			if err := rows.Scan(&ev.ID, &ev.Category, &ev.Action, &ev.ActorID, &ev.TargetType, &ev.TargetID, &ev.Reason, &at); err != nil {
				return nil, err
			}
			ev.OccurredAt = at.UTC().Format(time.RFC3339Nano)
			out = append(out, ev)
		}
		return out, rows.Err()
	})
}
