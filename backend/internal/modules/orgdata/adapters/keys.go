package adapters

import (
	"context"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	accounts "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var keyActions = map[app.Action]string{
	app.IssueKey: "data.key.issued", app.RotateKey: "data.key.rotated", app.RevokeKey: "data.key.revoked",
}

func KeyAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: keyActions[app.IssueKey], TargetType: "data_key"},
		{Action: keyActions[app.RotateKey], TargetType: "data_key"},
		{Action: keyActions[app.RevokeKey], TargetType: "data_key"},
	}
}

type KeyStore struct {
	Pool      *pgxpool.Pool
	Audit     auditing.TransactionRunner
	Authority app.KeyAuthority
	Verifier  domain.KeyVerifier
}

func keyView(row db.ReadKeyMetadataRow) domain.Key {
	key := domain.Key{ID: row.ID, Label: row.Label, State: domain.KeyState(row.State), Revision: row.Revision,
		IssuedAt: row.IssuedAt.Time.UTC(), ExpiresAt: row.ExpiresAt.Time.UTC(), OverlapUntil: row.OverlapUntil.Time.UTC()}
	key.Scopes = make([]domain.Scope, 0, len(row.Scopes))
	for _, scope := range row.Scopes {
		key.Scopes = append(key.Scopes, domain.Scope(scope))
	}
	return key
}

func (s KeyStore) WriteKey(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string, input app.Command) (app.KeyResult, error) {
	if s.Pool == nil || s.Audit == nil || s.Authority == nil {
		return app.KeyResult{}, app.ErrUnavailable
	}
	input.ExpiresAt = input.ExpiresAt.UTC().Truncate(time.Microsecond)
	input.OverlapUntil = input.OverlapUntil.UTC().Truncate(time.Microsecond)
	if input.Validate() != nil || scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) {
		return app.KeyResult{}, domain.ErrInvalidKey
	}
	org := scope.OrgID().String()
	target := input.TargetID
	if input.Action == app.IssueKey {
		target = input.OperationID
	}
	resultID := input.OperationID
	if input.Action == app.RevokeKey {
		resultID = target
	}
	event := audit.Event{OperationID: input.OperationID, ActorID: actor.AccountID, Action: keyActions[input.Action],
		TargetType: "data_key", TargetID: target, RequestHash: input.Digest(scope)}
	var result app.KeyResult
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckKeyActionTx(ctx, tx, scope, actor, member, input.Action); err != nil {
			return err
		}
		queries := db.New(tx)
		now, err := queries.KeyClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			bearer, err := s.apply(ctx, db.New(tx), scope, input, now)
			result.Bearer = bearer
			return err
		})
		if err != nil {
			return err
		}
		result.Replayed = replayed
		row, err := queries.ReadKeyMetadata(ctx, db.ReadKeyMetadataParams{OrgID: org, ID: resultID})
		if err != nil {
			return err
		}
		result.Key = keyView(row)
		result.Key.State = result.Key.DisplayStateAt(now.Time)
		return nil
	})
	if err != nil {
		return app.KeyResult{}, err
	}
	return result, nil
}

func (s KeyStore) apply(ctx context.Context, q *db.Queries, scope kernel.OrgScope, input app.Command, now pgtype.Timestamptz) (domain.Bearer, error) {
	org := scope.OrgID().String()
	if input.Action != app.IssueKey {
		row, err := q.ReadKeyMetadata(ctx, db.ReadKeyMetadataParams{OrgID: org, ID: input.TargetID})
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Bearer{}, domain.ErrKeyDenied
		}
		if err != nil {
			return domain.Bearer{}, err
		}
		if row.Revision != input.ExpectedRevision {
			return domain.Bearer{}, domain.ErrKeyConflict
		}
		if input.Action == app.RevokeKey {
			changed, err := q.RevokeKey(ctx, db.RevokeKeyParams{OrgID: org, ID: input.TargetID,
				ExpectedRevision: input.ExpectedRevision, RevokedAt: now})
			return domain.Bearer{}, keyChanged(changed, err)
		}
		if input.OverlapUntil.IsZero() {
			input.OverlapUntil = now.Time
		}
		if err := keyView(row).CanRotate(input.Scopes, input.OverlapUntil, input.ExpiresAt, now.Time); err != nil {
			return domain.Bearer{}, err
		}
	}
	if !now.Time.Before(input.ExpiresAt) {
		return domain.Bearer{}, domain.ErrInvalidKey
	}
	secret, err := accounts.GenerateToken()
	if err != nil {
		return domain.Bearer{}, app.ErrUnavailable
	}
	bearer, err := domain.NewBearer(input.OperationID, secret)
	if err != nil {
		return domain.Bearer{}, err
	}
	digest, err := s.Verifier.Digest(scope, bearer)
	if err != nil {
		return domain.Bearer{}, err
	}
	scopes := make([]string, 0, len(input.Scopes))
	for _, scope := range input.Scopes {
		scopes = append(scopes, string(scope))
	}
	expires := pgtype.Timestamptz{Time: input.ExpiresAt, Valid: true}
	if err := q.InsertKey(ctx, db.InsertKeyParams{OrgID: org, ID: bearer.ID(), Label: input.Label,
		Scopes: scopes, Verifier: digest, IssuedAt: now, ExpiresAt: expires}); err != nil {
		return domain.Bearer{}, err
	}
	if input.Action == app.RotateKey {
		changed, err := q.RotateKey(ctx, db.RotateKeyParams{OrgID: org, ID: input.TargetID,
			ExpectedRevision: input.ExpectedRevision, ReplacementID: bearer.ID(),
			OverlapUntil: pgtype.Timestamptz{Time: input.OverlapUntil, Valid: true}})
		if err := keyChanged(changed, err); err != nil {
			return domain.Bearer{}, err
		}
	}
	return bearer, nil
}

func keyChanged(changed int64, err error) error {
	if err != nil {
		return err
	}
	if changed != 1 {
		return domain.ErrKeyConflict
	}
	return nil
}

func (s KeyStore) ListKeys(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, after string, limit int) (app.KeyPage, error) {
	if s.Pool == nil || s.Authority == nil || actor.Validate() != nil || !audit.ValidID(member) {
		return app.KeyPage{}, app.ErrUnavailable
	}
	if limit < 1 || limit > 100 || after != "" && !audit.ValidID(after) {
		return app.KeyPage{}, domain.ErrInvalidKey
	}
	page := app.KeyPage{Keys: make([]domain.Key, 0, limit)}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckKeyActionTx(ctx, tx, scope, actor, member, app.ListKeys); err != nil {
			return err
		}
		q := db.New(tx)
		now, err := q.KeyClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		rows, err := q.ListKeyMetadata(ctx, db.ListKeyMetadataParams{OrgID: scope.OrgID().String(), AfterID: after, RowLimit: int32(limit + 1)})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			page.AfterID = rows[limit-1].ID
		}
		for _, row := range rows {
			view := keyView(db.ReadKeyMetadataRow(row))
			view.State = view.DisplayStateAt(now.Time)
			page.Keys = append(page.Keys, view)
		}
		return nil
	})
	if err != nil {
		return app.KeyPage{}, err
	}
	return page, nil
}
