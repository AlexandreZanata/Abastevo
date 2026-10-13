package application

import (
	"context"
	"encoding/json"
	"time"
)

type Command struct {
	OperationID      string          `json:"operation_id"`
	RuleID           string          `json:"rule_id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Delete           bool            `json:"delete"`
	Enabled          *bool           `json:"enabled,omitempty"`
	Payload          json.RawMessage `json:"payload"`
}

type Batch struct {
	Commands []Command `json:"commands"`
}

type CommandResult struct {
	OperationID string `json:"operation_id"`
	RuleID      string `json:"rule_id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed"`
}

type BatchResult struct {
	Results []CommandResult `json:"results"`
}

type RuleRecord struct {
	RuleID   string          `json:"rule_id"`
	Revision int64           `json:"revision"`
	Enabled  bool            `json:"enabled"`
	Payload  json.RawMessage `json:"payload"`
}

type InboxItem struct {
	InboxID string          `json:"inbox_id"`
	RuleID  string          `json:"rule_id"`
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
}

type RuleStore interface {
	Apply(ctx context.Context, accountID string, batch Batch) (BatchResult, error)
	ListRules(ctx context.Context, accountID string) ([]RuleRecord, error)
	ListInbox(ctx context.Context, accountID string, limit int) ([]InboxItem, error)
	EraseAll(ctx context.Context, accountID string) error
}

type EvaluateResult struct {
	Candidates int    `json:"candidates"`
	NextCursor string `json:"next_cursor"`
}

type EvaluateStore interface {
	Evaluate(ctx context.Context, accountID, ruleID, afterStationID string, limit int, now time.Time) (EvaluateResult, error)
}
