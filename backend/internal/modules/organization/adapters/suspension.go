package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	admin "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type SuspensionStore struct {
	Roots app.TransactionRepository
	Proof app.FreshOperatorProof
	NewID func() (string, error)
}

var _ app.SuspensionRepository = (*SuspensionStore)(nil)
var _ app.SuspensionIntakeRepository = (*SuspensionStore)(nil)

func suspensionError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "P8402":
			return d.ErrConflict
		case "P8403":
			return admin.ErrUnauthorized
		}
		return app.ErrUnavailable
	}
	for _, known := range []error{admin.ErrUnauthorized, admin.ErrUnavailable, d.ErrInvalid, d.ErrScope, d.ErrConflict, d.ErrClosed, d.ErrTransition, app.ErrNotFound, app.ErrUnavailable} {
		if errors.Is(e, known) {
			return known
		}
	}
	return app.ErrUnavailable
}
func lockSuspensionRoot(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope) error {
	if e := database.ValidateOrgTx(ctx, tx, scope); e != nil {
		return app.ErrUnavailable
	}
	_, e := q.New(tx).LockSuspensionRoot(ctx, scope.OrgID().String())
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return suspensionError(ctx, e)
}
func (s *SuspensionStore) ChangeSuspensionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, sessionID string, c app.SuspensionCommand) (app.SuspensionChange, error) {
	if scope.Validate() != nil {
		return app.SuspensionChange{}, d.ErrScope
	}
	if e := c.Validate(); e != nil {
		return app.SuspensionChange{}, e
	}
	if s == nil || s.Roots == nil || s.Proof == nil || s.NewID == nil || tx == nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	// Admin MutateFresh already holds the current session lock. Install only the
	// selected root's transaction-local scope and require ordinary READ COMMITTED.
	var prior string
	if e := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('anpfuel.org_id',true),'')`).Scan(&prior); e != nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	if prior != "" && prior != scope.OrgID().String() {
		return app.SuspensionChange{}, d.ErrScope
	}
	if _, e := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, scope.OrgID().String()); e != nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	if e := lockSuspensionRoot(ctx, tx, scope); e != nil {
		return app.SuspensionChange{}, e
	}
	if e := s.Proof.ConfirmFreshTx(ctx, tx, sessionID); e != nil {
		return app.SuspensionChange{}, suspensionError(ctx, e)
	}
	root, e := s.Roots.GetTx(ctx, tx, scope)
	if e != nil {
		return app.SuspensionChange{}, suspensionError(ctx, e)
	}
	before, e := root.Snapshot(scope)
	if e != nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	queries := q.New(tx)
	current, openErr := queries.GetOpenSuspensionPeriod(ctx, scope.OrgID().String())
	if openErr != nil && !errors.Is(openErr, pgx.ErrNoRows) {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	var changed d.Organization
	if c.Action == app.SuspendOrganization {
		changed, e = root.SetSuspension(scope, c.ExpectedRevision, c.Category)
		if e == nil && openErr == nil {
			return app.SuspensionChange{}, app.ErrUnavailable
		}
	} else {
		changed, e = root.ResolveSuspension(scope, c.ExpectedRevision, c.Category)
		if e == nil && (openErr != nil || current.Reason != string(before.SuspensionReason)) {
			return app.SuspensionChange{}, app.ErrUnavailable
		}
	}
	if e != nil {
		return app.SuspensionChange{}, e
	}
	after, e := changed.Snapshot(scope)
	if e != nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	saved, e := s.Roots.SaveTx(ctx, tx, scope, c.ExpectedRevision, changed)
	if e != nil {
		return app.SuspensionChange{}, suspensionError(ctx, e)
	}
	if !saved {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	var id string
	var at pgtype.Timestamptz
	if c.Action == app.SuspendOrganization {
		id, e = s.NewID()
		if e != nil || legalIdentity(scope, id) != nil {
			return app.SuspensionChange{}, app.ErrUnavailable
		}
		at, e = queries.OpenSuspensionPeriod(ctx, q.OpenSuspensionPeriodParams{OrgID: scope.OrgID().String(), ID: id, Reason: string(c.Category), Revision: after.Revision, SessionID: sessionID})
	} else {
		var row q.ResolveSuspensionPeriodRow
		row, e = queries.ResolveSuspensionPeriod(ctx, q.ResolveSuspensionPeriodParams{OrgID: scope.OrgID().String(), Revision: after.Revision, SessionID: sessionID, Reason: string(c.Category)})
		id, at = row.ID, row.EndedAt
	}
	if e != nil {
		return app.SuspensionChange{}, suspensionError(ctx, e)
	}
	if !settingsInstant(at) {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	kind := "CAUSE_OPENED"
	if c.Action == app.ReactivateOrganization {
		kind = "CAUSE_RESOLVED"
	}
	if e := queries.InsertSuspensionNotice(ctx, q.InsertSuspensionNoticeParams{OrgID: scope.OrgID().String(), Revision: after.Revision, PeriodID: id, Kind: kind}); e != nil {
		return app.SuspensionChange{}, suspensionError(ctx, e)
	}
	if _, e := tx.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, prior); e != nil {
		return app.SuspensionChange{}, app.ErrUnavailable
	}
	return app.SuspensionChange{Before: before, After: after, PeriodID: id, EffectiveAt: at.Time.UTC()}, nil
}
func (s *SuspensionStore) AssessSuspensionTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, declared, serverOpened time.Time) (app.SuspensionIntake, error) {
	if s == nil || s.Roots == nil {
		return app.SuspensionIntake{}, app.ErrUnavailable
	}
	if e := lockSuspensionRoot(ctx, tx, scope); e != nil {
		return app.SuspensionIntake{}, e
	}
	root, e := s.Roots.GetTx(ctx, tx, scope)
	if e != nil {
		return app.SuspensionIntake{}, suspensionError(ctx, e)
	}
	v, e := root.Snapshot(scope)
	if e != nil {
		return app.SuspensionIntake{}, app.ErrUnavailable
	}
	if v.SuspensionReason != "" {
		open, e := q.New(tx).GetOpenSuspensionPeriod(ctx, scope.OrgID().String())
		if e != nil || open.Reason != string(v.SuspensionReason) {
			return app.SuspensionIntake{}, app.ErrUnavailable
		}
	}
	at, e := q.New(tx).SuspensionIntakeClock(ctx)
	if e != nil || !settingsInstant(at) {
		return app.SuspensionIntake{}, app.ErrUnavailable
	}
	earliest := declared
	if !serverOpened.IsZero() && serverOpened.Before(earliest) {
		earliest = serverOpened
	}
	rows, e := q.New(tx).SuspensionIntakePeriods(ctx, q.SuspensionIntakePeriodsParams{OrgID: scope.OrgID().String(), Earliest: pgtype.Timestamptz{Time: earliest, Valid: true}})
	if e != nil {
		return app.SuspensionIntake{}, app.ErrUnavailable
	}
	periods := make([]d.SuspensionPeriod, 0, len(rows))
	for _, row := range rows {
		if !settingsInstant(row.StartedAt) || row.EndedAt.Valid && !settingsInstant(row.EndedAt) {
			return app.SuspensionIntake{}, app.ErrUnavailable
		}
		p := d.SuspensionPeriod{StartedAt: row.StartedAt.Time}
		if row.EndedAt.Valid {
			p.EndedAt = row.EndedAt.Time
		}
		periods = append(periods, p)
	}
	assessment, e := d.AssessSuspension(v.State, periods, d.SuspensionIntakeFacts{DeclaredAt: declared, ReceivedAt: at.Time, ServerOpenedAt: serverOpened})
	if e != nil {
		return app.SuspensionIntake{}, app.ErrUnavailable
	}
	return app.SuspensionIntake{Assessment: assessment, ReceivedAt: at.Time.UTC(), RootRevision: v.Revision}, nil
}
