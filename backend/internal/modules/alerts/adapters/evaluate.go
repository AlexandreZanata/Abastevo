package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/alerts"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	alertsdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func deterministicInboxID(dedupe string) string {
	sum := sha256.Sum256([]byte(dedupe))
	var b [16]byte
	copy(b[:], sum[:16])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" + hexed[16:20] + "-" + hexed[20:32]
}

func (s *Store) Evaluate(ctx context.Context, accountID, ruleID, afterStationID string, limit int, now time.Time) (application.EvaluateResult, error) {
	if !validID(ruleID) || limit < 1 || limit > 100 || now.IsZero() {
		return application.EvaluateResult{}, errInvalid()
	}
	if afterStationID != "" && !validID(afterStationID) {
		return application.EvaluateResult{}, errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.EvaluateResult{}, errUnavailable()
	}
	defer rollback(tx)
	if s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return application.EvaluateResult{}, err
		}
	}
	query := q.New(tx)
	stored, err := query.GetRule(ctx, q.GetRuleParams{AccountID: uid(accountID), RuleID: uid(ruleID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.EvaluateResult{}, errNotFound()
		}
		return application.EvaluateResult{}, errUnavailable()
	}
	if stored.Deleted || !stored.Enabled {
		return application.EvaluateResult{}, errNotFound()
	}
	rule, _, err := parseRule(stored.Payload)
	if err != nil {
		return application.EvaluateResult{}, errUnavailable()
	}
	if len(rule.Target.StationIDs) == 0 {
		return application.EvaluateResult{}, nil
	}
	stations := append([]string{}, rule.Target.StationIDs...)
	if afterStationID != "" {
		filtered := stations[:0]
		passed := false
		for _, station := range stations {
			if passed {
				filtered = append(filtered, station)
			}
			if station == afterStationID {
				passed = true
			}
		}
		stations = filtered
	}
	stationIDs := make([]pgtype.UUID, 0, len(stations))
	for _, station := range stations {
		stationIDs = append(stationIDs, uid(station))
	}
	candidates := 0
	lastStation := ""
	sources := map[alertsdomain.Source]bool{}
	for _, p := range rule.Predicates {
		sources[p.Source] = true
	}
	for source := range sources {
		for _, p := range rule.Predicates {
			if p.Source != source {
				continue
			}
			prices, baselines, truncated, err := s.snapshotPrices(ctx, query, source, string(p.Fuel), string(p.Unit), stationIDs, now, limit)
			if err != nil {
				return application.EvaluateResult{}, err
			}
			for _, price := range prices {
				var baseline *alertsdomain.Price
				for _, base := range baselines {
					if base.StationID == price.StationID {
						baseline = &base.Price
						break
					}
				}
				matched, _ := alertsdomain.Evaluate(rule, price.Price, baseline, now, nil)
				if !matched {
					continue
				}
				if price.Condition != p.Condition {
					continue
				}
				dedupe := accountID + "|" + ruleID + "|" + string(source) + "|" + price.StationID + "|" + string(p.Fuel) + "|" + string(p.Unit) + "|" + string(price.Condition) + "|" + price.ID
				inboxID := deterministicInboxID(dedupe)
				payload, _ := json.Marshal(map[string]any{"rule_id": ruleID, "source": string(source), "station_id": price.StationID, "price_id": price.ID, "milli": price.Price.Milli, "at": price.Price.At.Format(time.RFC3339), "dedupe": dedupe})
				n, err := query.InsertInboxCandidate(ctx, q.InsertInboxCandidateParams{AccountID: uid(accountID), InboxID: uid(inboxID), RuleID: uid(ruleID), Payload: payload})
				if err != nil {
					return application.EvaluateResult{}, errUnavailable()
				}
				if n > 0 {
					candidates++
					if _, err = jobs.Enqueue(ctx, tx, "alerts-dispatch", payload, inboxID, 5, now); err != nil {
						return application.EvaluateResult{}, errUnavailable()
					}
				}
				lastStation = price.StationID
			}
			_ = truncated
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.EvaluateResult{}, errUnavailable()
	}
	next := ""
	if candidates >= limit {
		next = lastStation
	}
	return application.EvaluateResult{Candidates: candidates, NextCursor: next}, nil
}

type snapshotPrice struct {
	ID        string
	StationID string
	Condition kernel.ConditionKind
	Price     alertsdomain.Price
}

func (s *Store) snapshotPrices(ctx context.Context, query *q.Queries, source alertsdomain.Source, fuel, unit string, stations []pgtype.UUID, now time.Time, limit int) ([]snapshotPrice, []snapshotPrice, bool, error) {
	out := []snapshotPrice{}
	bases := []snapshotPrice{}
	truncated := false
	if source == alertsdomain.Community {
		rows, err := query.CommunityPrices(ctx, q.CommunityPricesParams{Column1: stations, FuelProduct: fuel, Unit: unit, ReceivedAt: pgTimestamptz(now.Add(-24 * time.Hour)), Limit: int32(limit + 1)})
		if err != nil {
			return nil, nil, false, errUnavailable()
		}
		if len(rows) > limit {
			rows = rows[:limit]
			truncated = true
		}
		for _, row := range rows {
			out = append(out, snapshotPrice{ID: idString(row.ID), StationID: idString(row.StationID), Condition: kernel.ConditionKind(row.ConditionKind), Price: alertsdomain.Price{Source: source, Fuel: kernel.Product(row.FuelProduct), Unit: kernel.Unit(row.Unit), Condition: kernel.ConditionKind(row.ConditionKind), Milli: row.AmountMilliBrl, At: row.ReceivedAt.Time}})
		}
		baseRows, err := query.CommunityBaseline(ctx, q.CommunityBaselineParams{Column1: stations, FuelProduct: fuel, Unit: unit, ReceivedAt: pgTimestamptz(now.Add(-24 * time.Hour))})
		if err != nil {
			return nil, nil, false, errUnavailable()
		}
		for _, row := range baseRows {
			bases = append(bases, snapshotPrice{ID: idString(row.ID), StationID: idString(row.StationID), Condition: kernel.ConditionKind(row.ConditionKind), Price: alertsdomain.Price{Source: source, Fuel: kernel.Product(row.FuelProduct), Unit: kernel.Unit(row.Unit), Condition: kernel.ConditionKind(row.ConditionKind), Milli: row.AmountMilliBrl, At: row.ReceivedAt.Time}})
		}
		return out, bases, truncated, nil
	}
	rows, err := query.OfficialPrices(ctx, q.OfficialPricesParams{Column1: stations, FuelProduct: fuel, Unit: unit, CollectedOn: pgDate(now.AddDate(0, 0, -8)), Limit: int32(limit + 1)})
	if err != nil {
		return nil, nil, false, errUnavailable()
	}
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}
	for _, row := range rows {
		at := time.Date(int(row.CollectedOn.Time.Year()), row.CollectedOn.Time.Month(), row.CollectedOn.Time.Day(), 12, 0, 0, 0, time.UTC)
		out = append(out, snapshotPrice{ID: idString(row.ID), StationID: idString(row.StationID), Condition: kernel.Standard, Price: alertsdomain.Price{Source: source, Fuel: kernel.Product(row.FuelProduct), Unit: kernel.Unit(row.Unit), Condition: kernel.Standard, Milli: row.AmountMilliBrl, At: at}})
	}
	return out, bases, truncated, nil
}

var _ application.EvaluateStore = (*Store)(nil)
