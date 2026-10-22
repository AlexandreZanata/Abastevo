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

type Record struct {
	Dataset   string          `json:"dataset"`
	ID        string          `json:"id"`
	VehicleID string          `json:"vehicle_id"`
	Occurred  string          `json:"occurred_at"`
	Revision  int64           `json:"revision"`
	Voided    bool            `json:"voided"`
	Payload   json.RawMessage `json:"payload"`
}

type ListStore interface {
	List(ctx context.Context, accountID, dataset, vehicleID, category, from, to, afterTime, afterID string, limit int) ([]Record, error)
	Delete(ctx context.Context, accountID, dataset, id string) (int64, error)
}

type OdometerStore interface {
	OpenSegment(ctx context.Context, accountID, vehicleID, reason string) (int64, error)
	DeclareGap(ctx context.Context, accountID, vehicleID, gapID, afterID, beforeID, note string) (string, error)
}

type ConsentStatus struct {
	Revision int64    `json:"revision"`
	Datasets []string `json:"datasets"`
}

type ConsentStore interface {
	SetConsent(ctx context.Context, accountID string, datasets []string, enabled bool) (ConsentStatus, error)
	EraseAll(ctx context.Context, accountID string) error
}

type ReportTotals struct {
	PurchaseCentavos int64             `json:"purchase_centavos"`
	ExpenseCentavos  int64             `json:"expense_centavos"`
	ByVehicle        map[string]int64  `json:"by_vehicle"`
	ByCategory       map[string]int64  `json:"by_category"`
	RecordCount      int64             `json:"record_count"`
	Coverage         map[string]string `json:"coverage"`
}

type Report struct {
	Year        int          `json:"year"`
	Month       int          `json:"month"`
	Timezone    string       `json:"timezone"`
	Totals      ReportTotals `json:"totals"`
	Revision    int64        `json:"revision"`
	GeneratedAt string       `json:"generated_at"`
}

type ReportStore interface {
	BuildReport(ctx context.Context, accountID string, year, month int, timezone string) (Report, error)
	GetReport(ctx context.Context, accountID string, year, month int, timezone string) (Report, error)
}

// ConsumptionRecord is internal projection evidence; it is never a public DTO.
type ConsumptionRecord struct {
	ID, VehicleID string
	Payload       []byte
	SegmentNo     int64
	HasGap        bool
}
