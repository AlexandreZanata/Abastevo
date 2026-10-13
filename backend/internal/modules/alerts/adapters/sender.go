package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/alerts"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/jackc/pgx/v5"
)

func validPushToken(token string) bool {
	if len(token) < 1 || len(token) > 256 {
		return false
	}
	for _, r := range token {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (s *Store) EnrollToken(ctx context.Context, accountID, token string) error {
	if !validPushToken(token) {
		return errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return errUnavailable()
	}
	defer rollback(tx)
	if err = q.New(tx).EnrollPushToken(ctx, q.EnrollPushTokenParams{AccountID: uid(accountID), Token: token}); err != nil {
		return errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return errUnavailable()
	}
	return nil
}

func (s *Store) RevokeToken(ctx context.Context, accountID, token string) error {
	if !validPushToken(token) {
		return errInvalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return errUnavailable()
	}
	defer rollback(tx)
	if _, err = q.New(tx).RevokePushToken(ctx, q.RevokePushTokenParams{AccountID: uid(accountID), Token: token}); err != nil {
		return errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return errUnavailable()
	}
	return nil
}

func (s *Store) SendInbox(ctx context.Context, accountID, inboxID, token string) (string, error) {
	if !validID(inboxID) || !validPushToken(token) {
		return "", errInvalid()
	}
	if s.Sender == nil {
		return "", errUnavailable()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	if s.Paid != nil {
		if err = s.Paid(ctx, tx, accountID); err != nil {
			return "", err
		}
	}
	revoked, err := query.GetPushToken(ctx, q.GetPushTokenParams{AccountID: uid(accountID), Token: token})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errNotFound()
		}
		return "", errUnavailable()
	}
	if revoked.Valid {
		return "", errNotFound()
	}
	row, err := query.GetInboxRow(ctx, q.GetInboxRowParams{AccountID: uid(accountID), InboxID: uid(inboxID)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errNotFound()
		}
		return "", errUnavailable()
	}
	if row.Status != "dispatched" {
		return "", errNotFound()
	}
	var candidate struct {
		RuleID    string `json:"rule_id"`
		Source    string `json:"source"`
		StationID string `json:"station_id"`
		Milli     int64  `json:"milli"`
	}
	if json.Unmarshal(row.Payload, &candidate) != nil || candidate.RuleID == "" {
		return "", errUnavailable()
	}
	payload := application.PushPayload{Title: "Seu alerta encontrou um preço", Rule: candidate.RuleID, Inbox: inboxID}
	payload.Body = "Preço encontrado para seu filtro. Confira os detalhes."
	if strings.Contains(payload.Body, token) || strings.Contains(payload.Title, token) {
		return "", errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return "", errUnavailable()
	}
	messageID, err := s.Sender.Send(ctx, token, payload)
	if err != nil {
		return "", err
	}
	if err = s.recordSend(ctx, accountID, inboxID, messageID); err != nil {
		return "", err
	}
	return messageID, nil
}

func (s *Store) recordSend(ctx context.Context, accountID, inboxID, messageID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return errUnavailable()
	}
	defer rollback(tx)
	query := q.New(tx)
	row, err := query.GetInboxRow(ctx, q.GetInboxRowParams{AccountID: uid(accountID), InboxID: uid(inboxID)})
	if err != nil {
		return errUnavailable()
	}
	var documented map[string]any
	if json.Unmarshal(row.Payload, &documented) != nil {
		documented = map[string]any{}
	}
	documented["provider_message_id"] = messageID
	recorded, _ := json.Marshal(documented)
	n, err := query.MarkInboxSent(ctx, q.MarkInboxSentParams{AccountID: uid(accountID), InboxID: uid(inboxID), Payload: recorded})
	if err != nil || n == 0 {
		return errUnavailable()
	}
	if err = tx.Commit(ctx); err != nil {
		return errUnavailable()
	}
	return nil
}

var _ application.DeliveryStore = (*Store)(nil)
