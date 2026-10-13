package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/alerts"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	alertsdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) Dispatch(ctx context.Context, now time.Time, limit int) (application.DispatchResult, error) {
	if now.IsZero() || limit < 1 || limit > 25 {
		return application.DispatchResult{}, errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.DispatchResult{}, errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	rows, err := query.DueInbox(ctx, int32(limit))
	if err != nil {
		return application.DispatchResult{}, errUnavailable()
	}
	result := application.DispatchResult{}
	for _, row := range rows {
		switch outcome, err := s.dispatchOne(ctx, tx, query, row, now); {
		case err != nil:
			return result, err
		case outcome == "dispatched":
			result.Dispatched++
		case outcome == "dropped" || outcome == "unknown":
			result.Dropped++
		default:
			result.Skipped++
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.DispatchResult{}, errUnavailable()
	}
	return result, nil
}

type dispatchCandidate struct {
	RuleID       string `json:"rule_id"`
	RuleRevision int64  `json:"rule_revision"`
	Source       string `json:"source"`
	StationID    string `json:"station_id"`
	PriceID      string `json:"price_id"`
	Milli        int64  `json:"milli"`
	At           string `json:"at"`
}

func (s *Store) dispatchOne(ctx context.Context, tx pgx.Tx, query *q.Queries, row q.DueInboxRow, now time.Time) (string, error) {
	accountID := idString(row.AccountID)
	var candidate dispatchCandidate
	if json.Unmarshal(row.Payload, &candidate) != nil || candidate.RuleID == "" {
		return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "unknown")
	}
	rule, err := query.GetRule(ctx, q.GetRuleParams{AccountID: uid(accountID), RuleID: uid(candidate.RuleID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "dropped")
		}
		return "", errUnavailable()
	}
	if rule.Deleted || !rule.Enabled {
		return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "dropped")
	}
	if candidate.RuleRevision != rule.Revision {
		return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "dropped")
	}
	if s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return "skipped", nil
		}
	}
	parsed, _, err := parseRule(rule.Payload)
	if err != nil {
		return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "unknown")
	}
	if !inDispatchWindow(parsed, now) {
		return "skipped", nil
	}
	if cooled, err := s.inCooldown(ctx, query, accountID, candidate.RuleID, parsed, now); err != nil {
		return "", err
	} else if cooled {
		return "skipped", nil
	}
	if fresh, err := s.recheckPrice(ctx, query, parsed, candidate, now); err != nil {
		return "", err
	} else if !fresh {
		return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "dropped")
	}
	return s.markInbox(ctx, query, accountID, row.InboxID, row.Payload, "dispatched")
}

func (s *Store) markInbox(ctx context.Context, query *q.Queries, accountID string, inboxID pgtype.UUID, payload []byte, status string) (string, error) {
	var documented map[string]any
	if json.Unmarshal(payload, &documented) != nil {
		documented = map[string]any{}
	}
	documented["dispatch_status"] = status
	recorded, _ := json.Marshal(documented)
	n, err := query.MarkInbox(ctx, q.MarkInboxParams{AccountID: uid(accountID), InboxID: inboxID, Status: status, Payload: recorded})
	if err != nil || n == 0 {
		return "", errUnavailable()
	}
	return status, nil
}

func inDispatchWindow(rule alertsdomain.Rule, now time.Time) bool {
	loc, err := time.LoadLocation(rule.Schedule.Timezone)
	if err != nil {
		return false
	}
	local := now.In(loc)
	weekday := int(local.Weekday())
	dayOK := false
	for _, d := range rule.Schedule.Days {
		if d == weekday {
			dayOK = true
		}
	}
	if !dayOK {
		return false
	}
	inside, err := alertsdomain.WindowContains(local.Hour()*60+local.Minute(), rule.Schedule.WindowStart, rule.Schedule.WindowEnd)
	if err != nil || !inside {
		return false
	}
	quietStart, quietEnd, err := quietBounds(rule.Schedule.QuietStart, rule.Schedule.QuietEnd)
	if err != nil {
		return false
	}
	quiet, err := alertsdomain.WindowContains(local.Hour()*60+local.Minute(), rule.Schedule.QuietStart, rule.Schedule.QuietEnd)
	if err != nil {
		return false
	}
	return quietStart == quietEnd || !quiet
}

func quietBounds(start, end string) (int, int, error) {
	parse := func(s string) (int, error) {
		if len(s) != 5 || s[2] != ':' {
			return 0, errInvalid()
		}
		hour, minute := 0, 0
		for _, r := range s[:2] {
			if r < '0' || r > '9' {
				return 0, errInvalid()
			}
			hour = hour*10 + int(r-'0')
		}
		for _, r := range s[3:] {
			if r < '0' || r > '9' {
				return 0, errInvalid()
			}
			minute = minute*10 + int(r-'0')
		}
		if hour > 23 || minute > 59 {
			return 0, errInvalid()
		}
		return hour*60 + minute, nil
	}
	from, err := parse(start)
	if err != nil {
		return 0, 0, err
	}
	to, err := parse(end)
	if err != nil {
		return 0, 0, err
	}
	return from, to, nil
}

func (s *Store) inCooldown(ctx context.Context, query *q.Queries, accountID, ruleID string, rule alertsdomain.Rule, now time.Time) (bool, error) {
	if rule.Schedule.CooldownHours <= 0 {
		return false, nil
	}
	last, err := query.LastDispatch(ctx, q.LastDispatchParams{AccountID: uid(accountID), RuleID: uid(ruleID)})
	if err != nil {
		return false, errUnavailable()
	}
	return last.Valid && now.Sub(last.Time) < time.Duration(rule.Schedule.CooldownHours)*time.Hour, nil
}

func (s *Store) recheckPrice(ctx context.Context, query *q.Queries, rule alertsdomain.Rule, candidate dispatchCandidate, now time.Time) (bool, error) {
	for _, p := range rule.Predicates {
		if string(p.Source) != candidate.Source {
			continue
		}
		if candidate.Source == "official" {
			rows, err := query.OfficialPrices(ctx, q.OfficialPricesParams{Column1: []pgtype.UUID{uid(candidate.StationID)}, FuelProduct: string(p.Fuel), Unit: string(p.Unit), CollectedOn: pgDate(now.AddDate(0, 0, -8)), Limit: 100})
			if err != nil {
				return false, errUnavailable()
			}
			for _, row := range rows {
				if idString(row.ID) == candidate.PriceID {
					return true, nil
				}
			}
			continue
		}
		rows, err := query.CommunityPrices(ctx, q.CommunityPricesParams{Column1: []pgtype.UUID{uid(candidate.StationID)}, FuelProduct: string(p.Fuel), Unit: string(p.Unit), ReceivedAt: pgTimestamptz(now.Add(-24 * time.Hour)), Limit: 100})
		if err != nil {
			return false, errUnavailable()
		}
		for _, row := range rows {
			if idString(row.ID) == candidate.PriceID {
				return true, nil
			}
		}
	}
	return false, nil
}

var _ application.DispatchStore = (*Store)(nil)
