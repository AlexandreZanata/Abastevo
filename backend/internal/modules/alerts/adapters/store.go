package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/alerts"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	alertsdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxRules = 20

type Store struct {
	Pool   *pgxpool.Pool
	Paid   func(context.Context, pgx.Tx, string) error
	Sender application.PushProvider
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func uid(s string) pgtype.UUID { var id pgtype.UUID; _ = id.Scan(s); return id }

func rollback(tx pgx.Tx) { _ = tx.Rollback(context.Background()) }

func validID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}

func parseRule(raw json.RawMessage) (alertsdomain.Rule, []byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return alertsdomain.Rule{}, nil, errInvalid()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var wire struct {
		Predicates []struct {
			Kind           string `json:"kind"`
			Fuel           string `json:"fuel"`
			Unit           string `json:"unit"`
			Condition      string `json:"condition"`
			Source         string `json:"source"`
			ThresholdMilli int64  `json:"threshold_milli"`
			DropMilli      int64  `json:"drop_milli"`
			DropPercentBP  int64  `json:"drop_percent_bp"`
			BaselineHours  int64  `json:"baseline_hours"`
		} `json:"predicates"`
		Operator string `json:"operator"`
		Target   struct {
			StationIDs []string `json:"station_ids"`
			City       string   `json:"city"`
		} `json:"target"`
		Schedule struct {
			Days          []int  `json:"days"`
			WindowStart   string `json:"window_start"`
			WindowEnd     string `json:"window_end"`
			Timezone      string `json:"timezone"`
			QuietStart    string `json:"quiet_start"`
			QuietEnd      string `json:"quiet_end"`
			CooldownHours int64  `json:"cooldown_hours"`
		} `json:"schedule"`
	}
	if err := dec.Decode(&wire); err != nil {
		return alertsdomain.Rule{}, nil, errInvalid()
	}
	if _, err := dec.Token(); err == nil {
		return alertsdomain.Rule{}, nil, errInvalid()
	}
	rule := alertsdomain.Rule{Operator: wire.Operator, Target: alertsdomain.Target{StationIDs: wire.Target.StationIDs, City: wire.Target.City}, Schedule: alertsdomain.Schedule{Days: wire.Schedule.Days, WindowStart: wire.Schedule.WindowStart, WindowEnd: wire.Schedule.WindowEnd, Timezone: wire.Schedule.Timezone, QuietStart: wire.Schedule.QuietStart, QuietEnd: wire.Schedule.QuietEnd, CooldownHours: wire.Schedule.CooldownHours}}
	for _, p := range wire.Predicates {
		rule.Predicates = append(rule.Predicates, alertsdomain.Predicate{Kind: alertsdomain.PredicateKind(p.Kind), Fuel: kernelProduct(p.Fuel), Unit: kernelUnit(p.Unit), Condition: kernelCondition(p.Condition), Source: alertsdomain.Source(p.Source), ThresholdMilli: p.ThresholdMilli, DropMilli: p.DropMilli, DropPercentBP: p.DropPercentBP, BaselineHours: p.BaselineHours})
	}
	normalized, err := alertsdomain.NormalizeRule(rule)
	if err != nil {
		return alertsdomain.Rule{}, nil, errInvalid()
	}
	canonical, _ := json.Marshal(normalized)
	return normalized, canonical, nil
}

func (s *Store) Apply(ctx context.Context, accountID string, batch application.Batch) (application.BatchResult, error) {
	if len(batch.Commands) < 1 || len(batch.Commands) > 50 {
		return application.BatchResult{}, errInvalid()
	}
	raw, _ := json.Marshal(batch)
	if len(raw) > 65536 {
		return application.BatchResult{}, errInvalid()
	}
	for _, c := range batch.Commands {
		if !validID(c.OperationID) || !validID(c.RuleID) || c.ExpectedRevision < 0 {
			return application.BatchResult{}, errInvalid()
		}
		if c.Delete && c.Enabled != nil {
			return application.BatchResult{}, errInvalid()
		}
		if c.Delete {
			if len(bytes.TrimSpace(c.Payload)) > 0 && string(bytes.TrimSpace(c.Payload)) != "null" {
				return application.BatchResult{}, errInvalid()
			}
		} else if c.Enabled != nil && len(bytes.TrimSpace(c.Payload)) == 0 {
			continue
		} else if len(bytes.TrimSpace(c.Payload)) == 0 {
			return application.BatchResult{}, errInvalid()
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return application.BatchResult{}, errUnavailable()
	}
	defer rollback(tx)
	needsPaid := false
	for _, c := range batch.Commands {
		if !c.Delete {
			needsPaid = true
		}
	}
	if needsPaid && s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return application.BatchResult{}, err
		}
	}
	query := q.New(tx)
	result := application.BatchResult{Results: []application.CommandResult{}}
	for _, c := range batch.Commands {
		hashInput, _ := json.Marshal(struct {
			Account string
			Command application.Command
		}{accountID, c})
		sum := sha256.Sum256(hashInput)
		hash := hex.EncodeToString(sum[:])
		old, err := query.GetAlertOperation(ctx, q.GetAlertOperationParams{AccountID: uid(accountID), OperationID: uid(c.OperationID)})
		if err == nil {
			if old.PayloadHash != hash {
				return application.BatchResult{}, errConflict()
			}
			var receipt application.CommandResult
			if json.Unmarshal(old.Receipt, &receipt) != nil {
				return application.BatchResult{}, errUnavailable()
			}
			receipt.Replayed = true
			result.Results = append(result.Results, receipt)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return application.BatchResult{}, errUnavailable()
		}
		revision, err := s.applyOne(ctx, query, accountID, c)
		if err != nil {
			return application.BatchResult{}, err
		}
		receipt := application.CommandResult{OperationID: c.OperationID, RuleID: c.RuleID, Revision: revision, Status: "accepted"}
		receiptBytes, _ := json.Marshal(receipt)
		if err = query.SaveAlertOperation(ctx, q.SaveAlertOperationParams{AccountID: uid(accountID), OperationID: uid(c.OperationID), PayloadHash: hash, Receipt: receiptBytes}); err != nil {
			return application.BatchResult{}, errUnavailable()
		}
		result.Results = append(result.Results, receipt)
	}
	if needsPaid && s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return application.BatchResult{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.BatchResult{}, errUnavailable()
	}
	return result, nil
}

func (s *Store) applyOne(ctx context.Context, query *q.Queries, accountID string, c application.Command) (int64, error) {
	if c.Delete {
		old, err := query.GetRule(ctx, q.GetRuleParams{AccountID: uid(accountID), RuleID: uid(c.RuleID)})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, errConflict()
			}
			return 0, errUnavailable()
		}
		if old.Deleted {
			return old.Revision, nil
		}
		revision, err := query.DeleteRule(ctx, q.DeleteRuleParams{AccountID: uid(accountID), RuleID: uid(c.RuleID)})
		if err != nil {
			return 0, errUnavailable()
		}
		return revision, nil
	}
	var canonical []byte
	if len(bytes.TrimSpace(c.Payload)) > 0 {
		rule, bytes, err := parseRule(c.Payload)
		if err != nil {
			return 0, err
		}
		canonical = bytes
		for _, station := range rule.Target.StationIDs {
			if _, err = query.RuleStation(ctx, uid(station)); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return 0, errInvalid()
				}
				return 0, errUnavailable()
			}
		}
	}
	old, err := query.GetRule(ctx, q.GetRuleParams{AccountID: uid(accountID), RuleID: uid(c.RuleID)})
	if err == nil {
		if old.Deleted || old.Revision != c.ExpectedRevision {
			return 0, errConflict()
		}
		if c.Enabled != nil && len(canonical) == 0 {
			revision, err := query.SetRuleEnabled(ctx, q.SetRuleEnabledParams{AccountID: uid(accountID), RuleID: uid(c.RuleID), Enabled: *c.Enabled})
			if err != nil {
				return 0, errUnavailable()
			}
			return revision, nil
		}
		enabled := old.Enabled
		if c.Enabled != nil {
			enabled = *c.Enabled
		}
		row, err := query.SaveRule(ctx, q.SaveRuleParams{AccountID: uid(accountID), RuleID: uid(c.RuleID), Payload: canonical, Enabled: enabled})
		if err != nil {
			return 0, errUnavailable()
		}
		return row.Revision, nil
	} else if errors.Is(err, pgx.ErrNoRows) {
		if c.ExpectedRevision != 0 {
			return 0, errConflict()
		}
		count, err := query.CountRules(ctx, uid(accountID))
		if err != nil {
			return 0, errUnavailable()
		}
		if count >= maxRules {
			return 0, errQuota()
		}
		enabled := true
		if c.Enabled != nil {
			enabled = *c.Enabled
		}
		row, err := query.SaveRule(ctx, q.SaveRuleParams{AccountID: uid(accountID), RuleID: uid(c.RuleID), Payload: canonical, Enabled: enabled})
		if err != nil {
			return 0, errUnavailable()
		}
		return row.Revision, nil
	}
	return 0, errUnavailable()
}

func (s *Store) ListRules(ctx context.Context, accountID string) ([]application.RuleRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, errUnavailable()
	}
	defer rollback(tx)
	rows, err := q.New(tx).ListRules(ctx, uid(accountID))
	if err != nil {
		return nil, errUnavailable()
	}
	result := make([]application.RuleRecord, 0, len(rows))
	for _, row := range rows {
		result = append(result, application.RuleRecord{RuleID: idString(row.RuleID), Revision: row.Revision, Enabled: row.Enabled, Payload: row.Payload})
	}
	return result, nil
}

func (s *Store) ListInbox(ctx context.Context, accountID string, limit int) ([]application.InboxItem, error) {
	if limit < 1 || limit > 100 {
		return nil, errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, errUnavailable()
	}
	defer rollback(tx)
	rows, err := q.New(tx).ListInbox(ctx, q.ListInboxParams{AccountID: uid(accountID), Limit: int32(limit)})
	if err != nil {
		return nil, errUnavailable()
	}
	result := make([]application.InboxItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, application.InboxItem{InboxID: idString(row.InboxID), RuleID: idString(row.RuleID), Status: row.Status, Payload: row.Payload})
	}
	return result, nil
}

func (s *Store) EraseAll(ctx context.Context, accountID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if err = query.EraseAlertRules(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.EraseAlertInbox(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = query.PurgeAlertOperations(ctx, uid(accountID)); err != nil {
		return errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return errUnavailable()
	}
	return nil
}

var _ application.RuleStore = (*Store)(nil)
