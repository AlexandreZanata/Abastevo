// Package adapters persists organization export jobs through ordinary scoped
// transactions, stores private files, mints capabilities and runs the export
// queue handlers (EMP-RPT-001).
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TargetType is the audit target vocabulary of an export job; it equals the
// EXPORT_FILE sensitive-access target type.
const TargetType = "export"

// RequestedAction is the audit action of a job creation (BR-RPT-005).
const RequestedAction = "rpt.export.requested"

// AuditDefinitions registers the request event and the EXPORT_FILE part of
// the sensitive-access vocabulary.
func AuditDefinitions() []audit.Definition {
	return append([]audit.Definition{{Action: RequestedAction, TargetType: TargetType}}, audit.AccessDefinitions(audit.ExportFile)...)
}

// Store is the export repository and worker store.
type Store struct {
	Pool      *pgxpool.Pool
	Authority app.Authority
	Audit     auditing.TransactionRunner
}

var (
	_ app.Repository  = (*Store)(nil)
	_ app.WorkerStore = (*Store)(nil)
)

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

func (s *Store) ready(write bool) bool {
	return s != nil && s.Pool != nil && !missing(s.Authority) && (!write || !missing(s.Audit))
}

// storeError keeps only registered refusals; provider details stay private.
func storeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrOperationReused
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired,
		access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound, app.ErrOperationReused, app.ErrNotFound,
		app.ErrUnavailable, app.ErrUnauthenticated, app.ErrQueueFull, app.ErrNotReady, app.ErrExpired, app.ErrRightsLost, app.ErrBusy, app.ErrSettled, app.ErrFormatUnavailable, domain.ErrInvalid} {
		if errors.Is(err, known) {
			return err
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return app.ErrUnavailable
}

func validCall(scope kernel.OrgScope, actor app.Actor, member, export string) error {
	if scope.Validate() != nil || !domain.ValidID(member) || export != "" && !domain.ValidID(export) {
		return domain.ErrInvalid
	}
	if actor.Validate() != nil {
		return app.ErrUnauthenticated
	}
	return nil
}

func instant(v pgtype.Timestamptz) time.Time {
	if !v.Valid || v.InfinityModifier != pgtype.Finite {
		return time.Time{}
	}
	return v.Time.UTC()
}

type viewRow struct {
	ID, Kind, Parameters, ParametersDigest, State, FailureCode, ObjectSha256, MediaType string
	Attempts, MaxAttempts                                                               int16
	ObjectBytes, RowCount                                                               int64
	RequestedAt, AsOf, ReadyAt, ExpiresAt, FailedAt, ExpiredAt                          pgtype.Timestamptz
}

func viewOf(r viewRow) (app.View, error) {
	v := app.View{ExportID: r.ID, Kind: r.Kind, Parameters: []byte(r.Parameters), ParametersDigest: r.ParametersDigest, State: domain.State(r.State),
		Attempts: int(r.Attempts), MaxAttempts: int(r.MaxAttempts), FailureCode: domain.FailureCode(r.FailureCode), SHA256: r.ObjectSha256,
		RequestedAt: instant(r.RequestedAt), AsOf: instant(r.AsOf), ReadyAt: instant(r.ReadyAt), ExpiresAt: instant(r.ExpiresAt), FailedAt: instant(r.FailedAt), ExpiredAt: instant(r.ExpiredAt)}
	if r.ObjectSha256 != "" {
		v.MediaType, v.Bytes, v.Rows = r.MediaType, r.ObjectBytes, r.RowCount
	}
	if v.Validate() != nil {
		return app.View{}, app.ErrUnavailable
	}
	return v, nil
}

// authorizeRequest proves rpt.export.run for a request and then, in the same
// transaction, the data permission the kind declares (EMP-AUTH-011).
func (s *Store) authorizeRequest(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, member string, c app.Command) error {
	if err := s.Authority.CheckExportTx(ctx, tx, scope, actor, member, app.ActionRequest); err != nil {
		return err
	}
	return s.Authority.CheckDataTx(ctx, tx, scope, actor, member, c.ReadPermission())
}

func (s *Store) Request(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, exportID string, c app.Command) (app.View, bool, error) {
	if err := validCall(scope, actor, member, exportID); err != nil {
		return app.View{}, false, err
	}
	if !c.Valid() {
		return app.View{}, false, domain.ErrInvalid
	}
	if !s.ready(true) {
		return app.View{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	requester := app.Requester{AccountID: actor.AccountID, MembershipID: member}
	digest := c.Digest(scope, requester)
	var view app.View
	replayed := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorizeRequest(ctx, tx, scope, actor, member, c); err != nil {
			return err
		}
		queries := q.New(tx)
		if err := queries.ExportRequesterLock(ctx, q.ExportRequesterLockParams{OrgID: org, MembershipID: member}); err != nil {
			return err
		}
		prior, err := queries.ExportByOperation(ctx, q.ExportByOperationParams{OrgID: org, OperationID: c.OperationID()})
		switch {
		case err == nil:
			if prior.RequesterAccountID != actor.AccountID || prior.RequesterMembershipID != member || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			if view, err = s.readTx(ctx, tx, scope, member, prior.ID); err != nil {
				return err
			}
			replayed = true
			return s.authorizeRequest(ctx, tx, scope, actor, member, c)
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		open, err := queries.CountOpenExports(ctx, q.CountOpenExportsParams{OrgID: org, MembershipID: member})
		if err != nil {
			return err
		}
		if open >= domain.OpenJobLimit {
			return app.ErrQueueFull
		}
		event := audit.Event{OperationID: c.OperationID(), ActorID: actor.AccountID, Action: RequestedAction, TargetType: TargetType, TargetID: exportID, RequestHash: digest}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			limits := c.Limits()
			if err := q.New(tx).InsertExport(ctx, q.InsertExportParams{OrgID: org, ID: exportID, OperationID: c.OperationID(), RequesterAccountID: actor.AccountID, RequesterMembershipID: member,
				Kind: c.Kind(), Parameters: string(c.Parameters()), ParametersDigest: c.ParametersDigest(), RequestHash: digest, MediaType: c.MediaType(),
				RowLimit: limits.Rows, ByteLimit: limits.Bytes, TimeLimitMs: int32(limits.Duration / time.Millisecond), RetentionSeconds: int32(c.Retention() / time.Second), MaxAttempts: domain.MaxAttempts}); err != nil {
				return err
			}
			return enqueue(ctx, tx, BuildKind, scope, exportID, BuildAttempts, time.Time{})
		})
		if err != nil {
			return err
		}
		if repeated {
			// The operation was recorded without a job: another module's event.
			return app.ErrOperationReused
		}
		if view, err = s.readTx(ctx, tx, scope, member, exportID); err != nil {
			return err
		}
		return s.authorizeRequest(ctx, tx, scope, actor, member, c)
	})
	if err != nil {
		return app.View{}, false, storeError(ctx, err)
	}
	return view, replayed, nil
}

func enqueue(ctx context.Context, tx pgx.Tx, kind string, scope kernel.OrgScope, exportID string, attempts int32, notBefore time.Time) error {
	payload, err := json.Marshal(Payload{Version: 1, OrgID: scope.OrgID().String(), ExportID: exportID})
	if err != nil {
		return err
	}
	_, err = jobs.Enqueue(ctx, tx, kind, payload, Dedupe(kind, scope, exportID), attempts, notBefore)
	return err
}

func (s *Store) readTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, member, exportID string) (app.View, error) {
	row, err := q.New(tx).ExportForRequester(ctx, q.ExportForRequesterParams{OrgID: scope.OrgID().String(), ID: exportID, MembershipID: member})
	if err != nil {
		return app.View{}, err
	}
	return viewOf(viewRow{ID: row.ID, Kind: row.Kind, Parameters: row.Parameters, ParametersDigest: row.ParametersDigest, State: row.State, FailureCode: row.FailureCode,
		ObjectSha256: row.ObjectSha256, MediaType: row.MediaType, Attempts: row.Attempts, MaxAttempts: row.MaxAttempts, ObjectBytes: row.ObjectBytes, RowCount: row.RowCount,
		RequestedAt: row.RequestedAt, AsOf: row.AsOf, ReadyAt: row.ReadyAt, ExpiresAt: row.ExpiresAt, FailedAt: row.FailedAt, ExpiredAt: row.ExpiredAt})
}

func (s *Store) Read(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, exportID string) (app.View, error) {
	if s == nil {
		return app.View{}, app.ErrUnavailable
	}
	if err := validCall(scope, actor, member, exportID); err != nil {
		return app.View{}, err
	}
	if !s.ready(false) {
		return app.View{}, app.ErrUnavailable
	}
	var view app.View
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckExportTx(ctx, tx, scope, actor, member, app.ActionRead); err != nil {
			return err
		}
		var err error
		view, err = s.readTx(ctx, tx, scope, member, exportID)
		return err
	})
	if err != nil {
		return app.View{}, storeError(ctx, err)
	}
	return view, nil
}

func (s *Store) List(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, filter app.Filter) (app.Page, error) {
	if err := validCall(scope, actor, member, ""); err != nil {
		return app.Page{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.Page{}, err
	}
	if !s.ready(false) {
		return app.Page{}, app.ErrUnavailable
	}
	var page app.Page
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckExportTx(ctx, tx, scope, actor, member, app.ActionRead); err != nil {
			return err
		}
		params := q.ListExportsForRequesterParams{OrgID: scope.OrgID().String(), MembershipID: member, State: string(filter.State), RowLimit: int32(filter.Limit + 1), AfterID: "00000000-0000-0000-0000-000000000000"}
		if !filter.After.Zero() {
			params.HasAfter, params.AfterID = true, filter.After.ExportID
			params.AfterAt = pgtype.Timestamptz{Time: filter.After.RequestedAt, Valid: true}
		}
		rows, err := q.New(tx).ListExportsForRequester(ctx, params)
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				page.Next = app.KeyOf(page.Exports[i-1])
				break
			}
			v, err := viewOf(viewRow{ID: row.ID, Kind: row.Kind, Parameters: row.Parameters, ParametersDigest: row.ParametersDigest, State: row.State, FailureCode: row.FailureCode,
				ObjectSha256: row.ObjectSha256, MediaType: row.MediaType, Attempts: row.Attempts, MaxAttempts: row.MaxAttempts, ObjectBytes: row.ObjectBytes, RowCount: row.RowCount,
				RequestedAt: row.RequestedAt, AsOf: row.AsOf, ReadyAt: row.ReadyAt, ExpiresAt: row.ExpiresAt, FailedAt: row.FailedAt, ExpiredAt: row.ExpiredAt})
			if err != nil {
				return err
			}
			page.Exports = append(page.Exports, v)
		}
		return nil
	})
	if err != nil {
		return app.Page{}, storeError(ctx, err)
	}
	return page, nil
}

func (s *Store) Deliverable(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, exportID string, formats app.Registry) (domain.Object, error) {
	if err := validCall(scope, actor, member, exportID); err != nil {
		return domain.Object{}, err
	}
	if !s.ready(false) {
		return domain.Object{}, app.ErrUnavailable
	}
	var object domain.Object
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckExportTx(ctx, tx, scope, actor, member, app.ActionRead); err != nil {
			return err
		}
		row, err := q.New(tx).ExportForRequester(ctx, q.ExportForRequesterParams{OrgID: scope.OrgID().String(), ID: exportID, MembershipID: member})
		if err != nil {
			return err
		}
		// The file carries the kind's data: its declared read permission is
		// checked again now, before the state is disclosed (EMP-AUTH-011).
		read, admitted := formats.ReadPermission(row.Kind)
		if !admitted {
			return app.ErrFormatUnavailable
		}
		if err := s.Authority.CheckDataTx(ctx, tx, scope, actor, member, read); err != nil {
			return err
		}
		switch domain.State(row.State) {
		case domain.Ready:
			// Expiry is enforced at read time too (BR-PRIV-009).
			if !row.Live {
				return app.ErrExpired
			}
		case domain.Expired:
			return app.ErrExpired
		default:
			return app.ErrNotReady
		}
		object = domain.Object{OrgID: scope.OrgID().String(), ExportID: row.ID, SHA256: row.ObjectSha256, MediaType: row.MediaType, Bytes: row.ObjectBytes, Rows: row.RowCount}
		if object.Validate(scope) != nil {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return domain.Object{}, storeError(ctx, err)
	}
	return object, nil
}

func (s *Store) workerReady(scope kernel.OrgScope, exportID string) error {
	if s == nil || s.Pool == nil {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || !domain.ValidID(exportID) {
		return domain.ErrInvalid
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, scope kernel.OrgScope, exportID, lease string, ttl time.Duration) (app.Claim, error) {
	if err := s.workerReady(scope, exportID); err != nil {
		return app.Claim{}, err
	}
	if !domain.ValidID(lease) || ttl < time.Second || ttl > time.Hour {
		return app.Claim{}, domain.ErrInvalid
	}
	var claim app.Claim
	org := scope.OrgID().String()
	abandoned := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		row, err := queries.ClaimExport(ctx, q.ClaimExportParams{OrgID: org, ID: exportID, Lease: lease, LeaseSeconds: int32(ttl / time.Second)})
		if errors.Is(err, pgx.ErrNoRows) {
			abandoned, err = s.unclaimable(ctx, tx, scope, exportID)
			return err
		}
		if err != nil {
			return err
		}
		limits := domain.Limits{Rows: row.RowLimit, Bytes: row.ByteLimit, Duration: time.Duration(row.TimeLimitMs) * time.Millisecond}
		claim = app.Claim{ExportID: exportID, Requester: app.Requester{AccountID: row.RequesterAccountID, MembershipID: row.RequesterMembershipID}, Kind: row.Kind,
			Parameters: []byte(row.Parameters), AsOf: instant(row.AsOf), Limits: limits, Retention: time.Duration(row.RetentionSeconds) * time.Second,
			Attempts: int(row.Attempts), MaxAttempts: int(row.MaxAttempts), Lease: lease}
		if row.ObjectSha256 != "" {
			claim.Provisional = &domain.Object{OrgID: org, ExportID: exportID, SHA256: row.ObjectSha256, MediaType: row.MediaType, Bytes: row.ObjectBytes, Rows: row.RowCount}
			if claim.Provisional.Validate(scope) != nil {
				return app.ErrUnavailable
			}
		}
		if claim.Requester.Validate() != nil || limits.Validate() != nil || claim.AsOf.IsZero() {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.Claim{}, storeError(ctx, err)
	}
	if abandoned {
		return app.Claim{}, app.ErrSettled
	}
	return claim, nil
}

// unclaimable explains a claim that found no row: settled, busy or missing.
// A job abandoned on its last attempt (lease expired, no attempt left) is
// settled FAILED here and reported true so the transaction commits it.
func (s *Store) unclaimable(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, exportID string) (bool, error) {
	queries := q.New(tx)
	state, err := queries.ExportWorkState(ctx, q.ExportWorkStateParams{OrgID: scope.OrgID().String(), ID: exportID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, app.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	switch domain.State(state.State) {
	case domain.Ready, domain.Expired, domain.Failed:
		return false, app.ErrSettled
	case domain.Running:
		if !state.Live && state.Attempts >= state.MaxAttempts {
			hasObject, err := queries.ExhaustAbandonedExport(ctx, q.ExhaustAbandonedExportParams{OrgID: scope.OrgID().String(), ID: exportID})
			if errors.Is(err, pgx.ErrNoRows) {
				return false, app.ErrBusy
			}
			if err != nil {
				return false, err
			}
			if hasObject {
				if err := enqueue(ctx, tx, CleanupKind, scope, exportID, CleanupAttempts, time.Time{}); err != nil {
					return false, err
				}
			}
			return true, nil
		}
	}
	return false, app.ErrBusy
}

func (s *Store) CheckRequester(ctx context.Context, scope kernel.OrgScope, c app.Claim, read permission.Permission) error {
	if err := s.workerReady(scope, c.ExportID); err != nil {
		return err
	}
	if missing(s.Authority) || c.Requester.Validate() != nil {
		return app.ErrUnavailable
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		return s.Authority.CheckRequesterTx(ctx, tx, scope, c.Requester, c.Kind, read)
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, app.ErrRightsLost):
		return app.ErrRightsLost
	case ctx.Err() != nil:
		return ctx.Err()
	}
	return app.ErrUnavailable
}

func (s *Store) fenced(ctx context.Context, scope kernel.OrgScope, c app.Claim, work func(*q.Queries) (bool, error)) error {
	if err := s.workerReady(scope, c.ExportID); err != nil {
		return err
	}
	if !domain.ValidID(c.Lease) {
		return domain.ErrInvalid
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		held, err := work(q.New(tx))
		if err != nil {
			return err
		}
		if !held {
			return app.ErrBusy
		}
		return nil
	})
	return storeError(ctx, err)
}

func (s *Store) Provisional(ctx context.Context, scope kernel.OrgScope, c app.Claim, o domain.Object) error {
	if o.ExportID != c.ExportID || o.Validate(scope) != nil {
		return domain.ErrInvalid
	}
	return s.fenced(ctx, scope, c, func(queries *q.Queries) (bool, error) {
		n, err := queries.RecordProvisionalExport(ctx, q.RecordProvisionalExportParams{OrgID: o.OrgID, ID: o.ExportID, Lease: c.Lease, ObjectSha256: o.SHA256, ObjectBytes: o.Bytes, RowCount: o.Rows, MediaType: o.MediaType})
		return n == 1, err
	})
}

func (s *Store) Publish(ctx context.Context, scope kernel.OrgScope, c app.Claim, o domain.Object) error {
	if o.ExportID != c.ExportID || o.Validate(scope) != nil {
		return domain.ErrInvalid
	}
	if err := s.workerReady(scope, c.ExportID); err != nil {
		return err
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		expires, err := q.New(tx).PublishExport(ctx, q.PublishExportParams{OrgID: o.OrgID, ID: o.ExportID, Lease: c.Lease, ObjectSha256: o.SHA256, ObjectBytes: o.Bytes, RowCount: o.Rows, MediaType: o.MediaType})
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrBusy
		}
		if err != nil {
			return err
		}
		at := instant(expires)
		if at.IsZero() {
			return app.ErrUnavailable
		}
		// The cleanup that expires the job and deletes its file is scheduled
		// in the same transaction, at the expiry instant.
		return enqueue(ctx, tx, CleanupKind, scope, o.ExportID, CleanupAttempts, at)
	})
	return storeError(ctx, err)
}

func (s *Store) Release(ctx context.Context, scope kernel.OrgScope, c app.Claim) error {
	return s.fenced(ctx, scope, c, func(queries *q.Queries) (bool, error) {
		n, err := queries.ReleaseExport(ctx, q.ReleaseExportParams{OrgID: scope.OrgID().String(), ID: c.ExportID, Lease: c.Lease})
		return n == 1, err
	})
}

func (s *Store) Fail(ctx context.Context, scope kernel.OrgScope, c app.Claim, code domain.FailureCode) error {
	if !code.Valid() {
		return domain.ErrInvalid
	}
	if err := s.workerReady(scope, c.ExportID); err != nil {
		return err
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		hasObject, err := q.New(tx).FailExport(ctx, q.FailExportParams{OrgID: scope.OrgID().String(), ID: c.ExportID, Lease: c.Lease, FailureCode: string(code)})
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrBusy
		}
		if err != nil {
			return err
		}
		if hasObject {
			return enqueue(ctx, tx, CleanupKind, scope, c.ExportID, CleanupAttempts, time.Time{})
		}
		return nil
	})
	return storeError(ctx, err)
}

func (s *Store) Retire(ctx context.Context, scope kernel.OrgScope, exportID string) (app.Retirement, error) {
	if err := s.workerReady(scope, exportID); err != nil {
		return app.Retirement{}, err
	}
	var retirement app.Retirement
	org := scope.OrgID().String()
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		row, err := queries.LockExportForCleanup(ctx, q.LockExportForCleanupParams{OrgID: org, ID: exportID})
		if err != nil {
			return err
		}
		state := domain.State(row.State)
		if state == domain.Ready {
			if !row.Due {
				retirement.Pending = true
				return nil
			}
			if n, err := queries.ExpireExport(ctx, q.ExpireExportParams{OrgID: org, ID: exportID}); err != nil || n != 1 {
				return errors.Join(err, app.ErrUnavailable)
			}
			state = domain.Expired
		}
		if !state.Terminal() || row.ObjectSha256 == "" || row.Deleted {
			return nil
		}
		object := domain.Object{OrgID: org, ExportID: exportID, SHA256: row.ObjectSha256, MediaType: row.MediaType, Bytes: row.ObjectBytes, Rows: row.RowCount}
		if object.Validate(scope) != nil {
			return app.ErrUnavailable
		}
		retirement.Object = &object
		return nil
	})
	if err != nil {
		return app.Retirement{}, storeError(ctx, err)
	}
	return retirement, nil
}

func (s *Store) MarkDeleted(ctx context.Context, scope kernel.OrgScope, exportID string, o domain.Object) error {
	if err := s.workerReady(scope, exportID); err != nil {
		return err
	}
	if o.ExportID != exportID || o.Validate(scope) != nil {
		return domain.ErrInvalid
	}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		n, err := q.New(tx).MarkExportObjectDeleted(ctx, q.MarkExportObjectDeletedParams{OrgID: o.OrgID, ID: exportID, ObjectSha256: o.SHA256})
		if err != nil {
			return err
		}
		if n != 1 {
			return app.ErrSettled
		}
		return nil
	})
	if errors.Is(err, app.ErrSettled) {
		return nil // Already recorded by an earlier delivery of the cleanup.
	}
	return storeError(ctx, err)
}
