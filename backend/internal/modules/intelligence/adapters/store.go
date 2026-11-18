// Package adapters implements F6 PostgreSQL persistence, KPI calculations,
// findings detection, audit viewing, and PDF/ZIP/accounting export formats.
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/intelligence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrMeterRequired   = errors.New("intelligence: meter required")
	ErrQuoteRequired   = errors.New("intelligence: quote required")
	ErrControlDisabled = errors.New("intelligence: licence expiry control is disabled")
	ErrExportTooLarge  = errors.New("intelligence: export exceeds limit")
)

type Authority interface {
	CheckManagementTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, member string, target string, policy access.Policy, ownTarget bool) error
}

type Store struct {
	Pool      *pgxpool.Pool
	Authority Authority
	Audit     auditing.TransactionRunner
	NewID     func() (string, error)
}

var _ app.ManagementRepository = (*Store)(nil)

func AuditDefinitions() []audit.Definition {
	actions := []struct {
		action, target string
	}{
		{"stock.tank.saved", "tank"},
		{"stock.movement.recorded", "tank"},
		{"maint.plan.saved", "maintenanceplan"},
		{"maint.occurrence.updated", "maintenanceplan"},
		{"maint.workorder.updated", "workorder"},
		{"maint.workorder.reviewed", "workorder"},
		{"maint.compliance.updated", "complianceitem"},
		{"fleet.asset.document.saved", "asset"},
		{"find.finding.decided", "finding"},
		{"find.settings.updated", "organization"},
		{"find.run.completed", "findingrun"},
		{"org.group.grant.updated", "organization"},
		{"f6.governance.updated", "organization"},
	}
	out := make([]audit.Definition, len(actions))
	for i, a := range actions {
		out[i] = audit.Definition{Action: a.action, TargetType: a.target}
	}
	return out
}

func (s *Store) ready(write bool) bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.NewID != nil && (!write || s.Audit != nil)
}

func validUUID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func optText(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Store) appendAudit(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, _ string, action, targetType, targetID, reason string) error {
	opID, err := s.NewID()
	if err != nil {
		return app.ErrUnavailable
	}
	sum := sha256.Sum256([]byte(action + ":" + targetType + ":" + targetID + ":" + reason))
	ev := audit.Event{
		OperationID: opID,
		ActorID:     actor.AccountID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		RequestHash: hex.EncodeToString(sum[:]),
	}
	_, err = s.Audit.RecordChangeTx(ctx, tx, scope, ev, func(context.Context, pgx.Tx) error { return nil })
	return err
}

// commandCheck checks idempotency in org_f6_commands; returns (cachedResult, replayed, error).
func commandCheck(ctx context.Context, tx pgx.Tx, org, scopeKind, opID, reqHash string) (json.RawMessage, bool, error) {
	if !validUUID(opID) {
		return nil, false, app.ErrInvalid
	}
	var existingHash string
	var result json.RawMessage
	err := tx.QueryRow(ctx, `SELECT request_hash, result_json FROM org_f6_commands WHERE org_id=$1::uuid AND scope_kind=$2 AND operation_id=$3::uuid`, org, scopeKind, opID).Scan(&existingHash, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, app.ErrUnavailable
	}
	if existingHash != reqHash {
		return nil, false, app.ErrOperationReused
	}
	return result, true, nil
}

func commandSave(ctx context.Context, tx pgx.Tx, org, scopeKind, opID, entityID, reqHash string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return app.ErrUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO org_f6_commands(org_id,scope_kind,operation_id,entity_id,request_hash,result_json) VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5,$6::jsonb)`,
		org, scopeKind, opID, entityID, reqHash, raw)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return app.ErrOperationReused
		}
		return app.ErrUnavailable
	}
	return nil
}

func mapPGErr(err error) error {
	if err == nil {
		return nil
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23503":
			return app.ErrNotFound
		case "23505", "55000":
			return app.ErrConflict
		case "23514":
			if containsAny(pe.Message, "conflict", "revision") {
				return app.ErrConflict
			}
			return app.ErrInvalid
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

func withOrgReadTx[T any](ctx context.Context, s *Store, scope kernel.OrgScope, actor app.Actor, member, target string, policy access.Policy, ownTarget bool, fn func(pgx.Tx) (T, error)) (T, error) {
	var zero T
	if !s.ready(false) {
		return zero, app.ErrUnavailable
	}
	var out T
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckManagementTx(ctx, tx, scope, actor, member, target, policy, ownTarget); err != nil {
			return err
		}
		var e error
		out, e = fn(tx)
		return mapPGErr(e)
	})
	return out, err
}
