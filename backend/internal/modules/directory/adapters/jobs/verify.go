package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// VerifySweep auto-verifies one bounded batch of pending suggestions:
// exact official matches approve and link; everything else stays
// pending for authorized review (never auto-rejection). Pins record as
// unknown-quality revisions through the ports; projection stays
// official-only.
type VerifySweep struct {
	Store       application.VerifyStore
	Resolve     func(ctx context.Context, cnpj, display string, address map[string]string) (stationID, municipality, state string, err error)
	RecordPin   func(ctx context.Context, stationID string, lat, lon float64, ref string) error
	NewID       func() (string, error)
	Batch       int
	AccountLive func(ctx context.Context, accountID string) (bool, error)
}

// Kind implements jobs.Handler.
func (VerifySweep) Kind() string { return "suggestion-verify" }

// Version implements jobs.Handler.
func (VerifySweep) Version() int { return 1 }

// Handle implements jobs.Handler.
func (v VerifySweep) Handle(ctx context.Context, job jobs.Job) error {
	var envelope struct {
		Version int `json:"version"`
		Batch   int `json:"batch"`
	}
	if err := json.Unmarshal(job.Payload, &envelope); err != nil {
		return fmt.Errorf("verify: bad payload: %w", err)
	}
	if envelope.Version != 1 {
		return fmt.Errorf("verify: unsupported payload version %d", envelope.Version)
	}
	batch := v.Batch
	if envelope.Batch > 0 {
		batch = envelope.Batch
	}
	if batch <= 0 {
		batch = 25
	}
	pending, err := v.Store.ListPending(ctx, batch)
	if err != nil {
		return err
	}
	ports := application.VerifyPorts{
		Store: v.Store, Resolve: v.Resolve, RecordPin: v.RecordPin, NewID: v.NewID,
		AccountLive: v.AccountLive,
	}
	if ports.NewID == nil {
		return errors.New("verify: id generator required")
	}
	for _, row := range pending {
		if _, err := application.AutoVerify(ctx, ports, row.ID); err != nil {
			return err
		}
	}
	return nil
}
