package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

// runCorrections is the operator review port for community corrections.
// Every decision requires an explicit reason and writes the append-only
// audit row; the CLI never edits the database directly.
func runCorrections(dsn string, timeout time.Duration, list int, id, decision, reason, decider, evidenceRaw string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pool: %w", err)
	}
	defer pool.Close()
	service := directoryapp.CorrectionService{
		Store: directoryadapters.CorrectionStore{
			Pool: pool,
			Repo: directoryadapters.NewRepository(pool),
		},
		Clock: time.Now,
		NewID: directoryadapters.NewUUIDv4,
	}
	if list > 0 {
		rows, err := service.ListPending(ctx, list)
		if err != nil {
			return err
		}
		for _, row := range rows {
			raw, err := json.Marshal(map[string]any{
				"id": row.ID, "station_id": row.StationID, "field": row.Field,
				"status": row.Status, "created_at": row.CreatedAt.UTC().Format(time.RFC3339),
				"client_submission_id": row.ClientSubmissionID,
			})
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
		}
		return nil
	}
	if id == "" || decision == "" || reason == "" || decider == "" {
		return fmt.Errorf("--corrections-decide requires --decision, --reason and --decider")
	}
	var evidence map[string]any
	if err := json.Unmarshal([]byte(evidenceRaw), &evidence); err != nil {
		return fmt.Errorf("--evidence must be a JSON object")
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	if err := service.Decide(ctx, id, decision, reason, "staff", decider, raw); err != nil {
		return err
	}
	fmt.Println(`{"status":"decided"}`)
	return nil
}
