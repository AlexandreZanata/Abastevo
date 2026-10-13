package application

import (
	"context"
	"encoding/json"
)

type Command struct {
	OperationID      string          `json:"operation_id"`
	Dataset          string          `json:"dataset"`
	ID               string          `json:"id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Delete           bool            `json:"delete"`
	Void             bool            `json:"void"`
	Payload          json.RawMessage `json:"payload"`
}

type Batch struct {
	Commands []Command `json:"commands"`
}

type CommandResult struct {
	OperationID string `json:"operation_id"`
	Dataset     string `json:"dataset"`
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed"`
}

type BatchResult struct {
	Results []CommandResult `json:"results"`
}

type LedgerStore interface {
	Apply(ctx context.Context, accountID string, batch Batch) (BatchResult, error)
}

type OdometerStore interface {
	OpenSegment(ctx context.Context, accountID, vehicleID, reason string) (int64, error)
	DeclareGap(ctx context.Context, accountID, vehicleID, gapID, afterID, beforeID, note string) (string, error)
}
