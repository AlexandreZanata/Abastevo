package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgconnector"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

const fleetWebhookKind = "org_connector.fleet_webhook"
const webhookAttempts = 3

var errWebhookRetry = errors.New("fleet webhook: retryable delivery")
var errWebhookState = errors.New("fleet webhook: delivery state unavailable")

type webhookJob struct {
	Version int    `json:"version"`
	OrgID   string `json:"org_id"`
	ID      string `json:"delivery_id"`
}

// FleetWebhooks schedules one job for each eligible destination in the vehicle
// command transaction. The worker rechecks authority before external delivery.
type FleetWebhooks struct{}

func (FleetWebhooks) QueueFleetLifecycle(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, operationID string) error {
	org := scope.OrgID().String()
	q := db.New(tx)
	destinations, err := q.ListWebhookDestinations(ctx, db.ListWebhookDestinationsParams{OrgID: org, OperationID: operationID})
	if err != nil {
		return err
	}
	for _, destination := range destinations {
		id, err := jobs.NewUUIDv4()
		if err != nil {
			return err
		}
		payload, err := json.Marshal(webhookJob{Version: 1, OrgID: org, ID: id})
		if err != nil {
			return err
		}
		key := fleetWebhookKind + ":" + org + ":" + destination.ConnectionID + ":" + operationID
		job, err := jobs.Enqueue(ctx, tx, fleetWebhookKind, payload, key, webhookAttempts, time.Time{})
		if err != nil {
			return err
		}
		if job.Created {
			if err := q.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{OrgID: org, ID: id, ConnectionID: destination.ConnectionID,
				OperationID: operationID, JobID: job.ID, ConnectionRevision: destination.Revision}); err != nil {
				return err
			}
		}
	}
	return nil
}

// WebhookEvent is the entire version 1 external payload.
type WebhookEvent struct {
	Version    int       `json:"version"`
	ID         string    `json:"delivery_id"`
	Event      string    `json:"event"`
	VehicleID  string    `json:"vehicle_id"`
	OccurredAt time.Time `json:"occurred_at"`
}

type WebhookResult string

const (
	WebhookDelivered  WebhookResult = "DELIVERED"
	WebhookRetry      WebhookResult = "RETRY"
	WebhookFailed     WebhookResult = "FAILED"
	WebhookSuppressed WebhookResult = "SUPPRESSED"
)

type WebhookSender interface {
	SendWebhook(context.Context, string, domain.CredentialRef, string, []byte) WebhookResult
}

type FleetWebhookWorker struct {
	Access Access
	Sender WebhookSender
}

func (FleetWebhookWorker) Kind() string { return fleetWebhookKind }
func (FleetWebhookWorker) Version() int { return 1 }

func (w FleetWebhookWorker) Handle(ctx context.Context, job jobs.Job) error {
	var payload webhookJob
	if json.Unmarshal(job.Payload, &payload) != nil || payload.Version != w.Version() ||
		!audit.ValidID(payload.ID) || !audit.ValidID(job.ID) || job.Kind != w.Kind() || job.LeaseToken < 1 {
		return errWebhookState
	}
	org, err := kernel.ParseOrgID(payload.OrgID)
	if err != nil || org.String() != payload.OrgID || w.Access.Pool == nil {
		return errWebhookState
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return errWebhookState
	}
	var endpoint string
	var credential domain.CredentialRef
	var body []byte
	var attempt int32
	var leaseEnds time.Time
	// Keep authorization and allocation in the same transaction. The socket
	// opens after commit under its own bounded deadline.
	err = database.WithOrgTx(ctx, w.Access.Pool, scope, func(tx pgx.Tx) error {
		q := db.New(tx)
		source, err := q.ClaimWebhookDelivery(ctx, db.ClaimWebhookDeliveryParams{OrgID: payload.OrgID, ID: payload.ID,
			JobID: job.ID, LeaseToken: job.LeaseToken})
		if errors.Is(err, pgx.ErrNoRows) {
			finished, checkErr := q.WebhookDeliveryComplete(ctx, db.WebhookDeliveryCompleteParams{OrgID: payload.OrgID, ID: payload.ID, JobID: job.ID})
			if checkErr == nil && finished.Valid && finished.Bool {
				return nil
			}
			return errWebhookState
		}
		if err != nil {
			return err
		}
		attempt = source.Attempts
		leaseEnds = source.LeaseExpiresAt.Time
		if w.Sender == nil {
			return nil
		}
		review, err := w.Access.checkActiveTx(ctx, tx, scope, domain.OutboundWebhook, source.ConnectionID, source.AssetID)
		if errors.Is(err, domain.ErrDenied) {
			return nil
		}
		if err != nil {
			return err
		}
		connection, err := q.GetConnection(ctx, db.GetConnectionParams{OrgID: payload.OrgID, ID: source.ConnectionID})
		if err != nil {
			return err
		}
		if connection.Revision != source.ConnectionRevision {
			return nil
		}
		endpoint, err = readWebhookEndpoint(ctx, q, payload.OrgID, review)
		if errors.Is(err, domain.ErrDenied) {
			return nil
		}
		if err != nil {
			return err
		}
		event := "fleet.vehicle.deactivated"
		if source.Command == "REACTIVATE" {
			event = "fleet.vehicle.reactivated"
		}
		body, err = json.Marshal(WebhookEvent{Version: 1, ID: payload.ID, Event: event, VehicleID: source.AssetID,
			OccurredAt: source.RecordedAt.Time.UTC()})
		if err != nil {
			return err
		}
		if int64(len(body)) <= review.MaxBytes && review.MaxItems >= 1 {
			credential = review.Credential
		}
		return nil
	})
	if err != nil {
		return errWebhookState
	}
	if attempt == 0 {
		return nil
	}
	outcome := WebhookSuppressed
	if credential.Reference() != "" {
		sendCtx, cancelSend := context.WithDeadline(ctx, leaseEnds.Add(-time.Second))
		outcome = WebhookRetry
		if sendCtx.Err() == nil {
			outcome = w.Sender.SendWebhook(sendCtx, endpoint, credential, payload.ID, body)
		}
		cancelSend()
	}
	if outcome == WebhookRetry && attempt >= webhookAttempts {
		outcome = WebhookFailed
	}
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	err = database.WithOrgTx(ackCtx, w.Access.Pool, scope, func(tx pgx.Tx) error {
		count, err := db.New(tx).FinishWebhookDelivery(ackCtx, db.FinishWebhookDeliveryParams{OrgID: payload.OrgID, ID: payload.ID,
			LeaseToken: job.LeaseToken, Outcome: string(outcome)})
		return requireOne(count, err)
	})
	if err != nil {
		return errWebhookState
	}
	if outcome == WebhookRetry {
		return errWebhookRetry
	}
	return nil
}

func readWebhookEndpoint(ctx context.Context, q *db.Queries, org string, review domain.Approval) (string, error) {
	endpoint, err := q.ReadWebhookEndpoint(ctx, db.ReadWebhookEndpointParams{OrgID: org, ApprovalID: review.ID, ApprovalRevision: review.Revision})
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !validWebhookEndpoint(endpoint) {
		return "", domain.ErrDenied
	}
	return endpoint, err
}
