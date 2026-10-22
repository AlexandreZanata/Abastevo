package application

import "context"

type LedgerRecord struct {
	SegmentNo int64
	HasGap    bool
	Dataset   string
	ID        string
	VehicleID string
	Occurred  string
	Payload   []byte
}

type LedgerReader interface {
	ListRecords(ctx context.Context, accountID, dataset, vehicleID, from, to string, limit int) ([]LedgerRecord, error)
}

type Side struct {
	VehicleID string `json:"vehicle_id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Timezone  string `json:"timezone"`
	Method    string `json:"method"`
}

type Comparison struct {
	Baseline          string `json:"baseline"`
	Current           string `json:"current"`
	Compatible        bool   `json:"compatible"`
	Issue             string `json:"issue"`
	BaselineKmPerUnit string `json:"baseline_km_per_unit"`
	CurrentKmPerUnit  string `json:"current_km_per_unit"`
	ChangePercent     string `json:"change_percent"`
}

type CompareStore interface {
	Compare(ctx context.Context, accountID string, baseline, current Side) (Comparison, error)
}
