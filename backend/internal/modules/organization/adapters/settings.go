package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SettingsStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Audit        auditing.TransactionRunner
	Capabilities setupCapability
}

var _ app.SettingsRepository = (*SettingsStore)(nil)
var _ app.SettingsIntakeRepository = (*SettingsStore)(nil)

func SettingsAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "org.settings.changed", TargetType: "org.settings", Reasons: []string{"configuration"}, RequireReason: true}}
}
func (s *SettingsStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && s.Capabilities != nil
}
func (s *SettingsStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member string, write bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, scope.OrgID().String(), app.SettingsPolicy(write))
}
func settingsError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P8101":
			return d.ErrConflict
		case "P8102", "P8103":
			return d.ErrInvalid
		case "P8104", "23505":
			return app.ErrUnavailable
		}
	}
	// Preserve typed domain field errors; SQL details and member hints stay private.
	if d.SettingsField(e) != "" {
		return e
	}
	return legalError(ctx, e)
}
func settingsInstant(at pgtype.Timestamptz) bool {
	return at.Valid && at.InfinityModifier == pgtype.Finite && !at.Time.IsZero() && at.Time.UTC().Year() >= 1 && at.Time.UTC().Year() <= 9999
}
func settingsValue(scope kernel.OrgScope, row q.GetSettingsRevisionRow) (d.Settings, error) {
	if !settingsInstant(row.EffectiveAt) {
		return d.Settings{}, app.ErrUnavailable
	}
	values, e := DecodeSettingsValues(row.SettingsValues)
	if e != nil {
		return d.Settings{}, app.ErrUnavailable
	}
	result, e := d.RehydrateSettings(scope, d.SettingsSnapshot{OrgID: scope.OrgID(), Revision: row.Revision, EffectiveAt: row.EffectiveAt.Time, Values: values, Source: d.SettingsSource(row.Source), Author: d.SettingsAuthor{AccountID: row.AuthorAccountID, MembershipID: row.AuthorMembershipID}, Reason: row.Reason, OperationID: row.OperationID, SourceMembershipID: row.SourceMembershipID, SourceMembershipRevision: row.SourceMembershipRevision})
	if e != nil {
		return d.Settings{}, app.ErrUnavailable
	}
	return result, nil
}
func loadSettings(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, revision int64) (d.Settings, error) {
	row, e := q.New(tx).GetSettingsRevision(ctx, q.GetSettingsRevisionParams{OrgID: scope.OrgID().String(), Revision: revision})
	if e != nil {
		return d.Settings{}, e
	}
	return settingsValue(scope, row)
}
func (s *SettingsStore) ChangeSettings(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, command app.SettingsWrite) (app.SettingsResult, bool, error) {
	if e := legalCall(scope, actor, member); e != nil {
		return app.SettingsResult{}, false, e
	}
	v, e := command.Snapshot()
	if e != nil {
		return app.SettingsResult{}, false, e
	}
	if !s.ready() {
		return app.SettingsResult{}, false, app.ErrUnavailable
	}
	var result app.SettingsResult
	replay, readyToCommit := false, false
	digest := command.Digest(scope)
	e = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, actor, member, true); e != nil {
			return e
		}
		queries := q.New(tx)
		current, e := queries.EnsureSettings(ctx, scope.OrgID().String())
		if e != nil {
			return e
		}
		receipt, e := queries.SettingsCommandReceipt(ctx, q.SettingsCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if e == nil {
			if receipt.ActorID != actor.AccountID || receipt.RequestHash != digest {
				return app.ErrOperationReused
			}
			if receipt.SettingsRevision < 2 || receipt.ResultRootRevision < 1 || !settingsInstant(receipt.EffectiveAt) {
				return app.ErrUnavailable
			}
			result = app.SettingsResult{Revision: receipt.SettingsRevision, RootRevision: receipt.ResultRootRevision, EffectiveAt: receipt.EffectiveAt.Time.UTC()}
			replay = true
			readyToCommit = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if current != v.ExpectedRevision {
			return d.ErrConflict
		}
		old, e := loadSettings(ctx, tx, scope, current)
		if e != nil {
			return e
		}
		now, e := queries.CurrentOnboardingTime(ctx)
		if e != nil {
			return e
		}
		if !settingsInstant(now) {
			return app.ErrUnavailable
		}
		changed, e := old.Change(scope, v.ExpectedRevision, v.Values, d.SettingsAuthor{AccountID: actor.AccountID, MembershipID: member}, v.Reason, v.OperationID, now.Time)
		if e != nil {
			return e
		}
		snap, e := changed.Snapshot(scope)
		if e != nil {
			return e
		}
		raw, e := EncodeSettingsValues(snap.Values)
		if e != nil {
			return e
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: "org.settings.changed", TargetType: "org.settings", TargetID: scope.OrgID().String(), ReasonCode: "configuration", RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			if e := queries.InsertSettingsRevision(ctx, q.InsertSettingsRevisionParams{OrgID: scope.OrgID().String(), Revision: snap.Revision, SettingsValues: raw, ActorID: actor.AccountID, MemberID: member, Reason: v.Reason, OperationID: v.OperationID}); e != nil {
				return e
			}
			count, e := queries.PublishSettingsRevision(ctx, q.PublishSettingsRevisionParams{OrgID: scope.OrgID().String(), ExpectedRevision: v.ExpectedRevision})
			if e != nil {
				return e
			}
			if count != 1 {
				return d.ErrConflict
			}
			stored, e := loadSettings(ctx, tx, scope, snap.Revision)
			if e != nil {
				return e
			}
			actual, e := stored.Snapshot(scope)
			if e != nil {
				return e
			}
			root, e := queries.SettingsRootRevision(ctx, scope.OrgID().String())
			if e != nil {
				return e
			}
			if root < 1 {
				return app.ErrUnavailable
			}
			result = app.SettingsResult{Revision: actual.Revision, RootRevision: root, EffectiveAt: actual.EffectiveAt}
			return queries.InsertSettingsReceipt(ctx, q.InsertSettingsReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: actor.AccountID, RequestHash: digest, SettingsRevision: actual.Revision, ResultRootRevision: root})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrUnavailable
		}
		readyToCommit = true
		return nil
	})
	if e != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.SettingsResult{}, false, app.ErrUnavailable
		}
		return app.SettingsResult{}, false, settingsError(ctx, e)
	}
	return result, replay, nil
}
func (s *SettingsStore) ReadSettings(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, revision int64) (app.SettingsRead, error) {
	if e := legalCall(scope, actor, member); e != nil {
		return app.SettingsRead{}, e
	}
	if revision < 0 {
		return app.SettingsRead{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.SettingsRead{}, app.ErrUnavailable
	}
	var result app.SettingsRead
	readyToCommit := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, actor, member, false); e != nil {
			return e
		}
		current, e := q.New(tx).EnsureSettings(ctx, scope.OrgID().String())
		if e != nil {
			return e
		}
		if revision == 0 {
			revision = current
		}
		result.Settings, e = loadSettings(ctx, tx, scope, revision)
		if e != nil {
			return e
		}
		result.RootRevision, e = q.New(tx).SettingsRootRevision(ctx, scope.OrgID().String())
		if e != nil {
			return e
		}
		if result.RootRevision < 1 {
			return app.ErrUnavailable
		}
		readyToCommit = true
		return nil
	})
	if e != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.SettingsRead{}, app.ErrUnavailable
		}
		return app.SettingsRead{}, settingsError(ctx, e)
	}
	return result, nil
}
func (s *SettingsStore) ListSettings(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, f app.SettingsFilter) (app.SettingsPage, error) {
	if e := legalCall(scope, actor, member); e != nil {
		return app.SettingsPage{}, e
	}
	if e := f.Validate(); e != nil {
		return app.SettingsPage{}, e
	}
	if !s.ready() {
		return app.SettingsPage{}, app.ErrUnavailable
	}
	result := app.SettingsPage{Versions: []d.Settings{}}
	readyToCommit := false
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, actor, member, false); e != nil {
			return e
		}
		if _, e := q.New(tx).EnsureSettings(ctx, scope.OrgID().String()); e != nil {
			return e
		}
		rows, e := q.New(tx).ListSettingsRevisions(ctx, q.ListSettingsRevisionsParams{OrgID: scope.OrgID().String(), BeforeRevision: f.BeforeRevision, RowLimit: int32(f.Limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == f.Limit {
				prior, e := result.Versions[i-1].Snapshot(scope)
				if e != nil {
					return e
				}
				result.NextRevision = prior.Revision
				break
			}
			v, e := settingsValue(scope, q.GetSettingsRevisionRow(row))
			if e != nil {
				return e
			}
			result.Versions = append(result.Versions, v)
		}
		readyToCommit = true
		return nil
	})
	if e != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.SettingsPage{}, app.ErrUnavailable
		}
		return app.SettingsPage{}, settingsError(ctx, e)
	}
	return result, nil
}
func (s *SettingsStore) CaptureSettingsTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) (app.SettingsIntake, error) {
	if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
		return app.SettingsIntake{}, settingsError(ctx, e)
	}
	if _, e := q.New(tx).EnsureSettings(ctx, scope.OrgID().String()); e != nil {
		return app.SettingsIntake{}, settingsError(ctx, e)
	}
	clock, e := q.New(tx).SettingsIntakeClock(ctx, scope.OrgID().String())
	if e != nil {
		return app.SettingsIntake{}, settingsError(ctx, e)
	}
	if !settingsInstant(clock.ReceivedAt) {
		return app.SettingsIntake{}, app.ErrUnavailable
	}
	value, e := loadSettings(ctx, tx, scope, clock.Revision)
	if e != nil {
		return app.SettingsIntake{}, settingsError(ctx, e)
	}
	v, e := value.Snapshot(scope)
	if e != nil || clock.ReceivedAt.Time.Before(v.EffectiveAt) {
		return app.SettingsIntake{}, app.ErrUnavailable
	}
	return app.SettingsIntake{Settings: value, ReceivedAt: clock.ReceivedAt.Time.UTC()}, nil
}
func (s *SettingsStore) SettingsRevisionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, revision int64) (d.Settings, error) {
	if revision < 1 {
		return d.Settings{}, d.ErrInvalid
	}
	if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
		return d.Settings{}, settingsError(ctx, e)
	}
	v, e := loadSettings(ctx, tx, scope, revision)
	if e != nil {
		return d.Settings{}, settingsError(ctx, e)
	}
	return v, nil
}
