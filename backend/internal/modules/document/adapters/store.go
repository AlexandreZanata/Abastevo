// Package adapters persists private document containers through ordinary
// scoped transactions and binds them to the private original service.
package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/document"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

const (
	createdAction = "document.upload.created"
	storedAction  = "document.upload.stored"
	targetType    = "document"
)

// Store implements the document repository over forced-RLS org_documents.
type Store struct {
	Pool      *pgxpool.Pool
	Authority app.Authority
	Audit     auditing.TransactionRunner
	Storage   StorageReservations
	// Targets is the closed registry of linkable kinds (EMP-DOC-003).
	Targets app.Targets
}

type StorageReservations interface {
	LockStorageReservation(context.Context, pgx.Tx, kernel.OrgScope) error
	CheckStorageReservation(context.Context, pgx.Tx, kernel.OrgScope, int64) error
}

var _ app.Repository = (*Store)(nil)

// AuditDefinitions registers the document upload vocabulary. Content access
// keeps the orgmedia sensitive-access actions (EMP-PRIV-005).
func AuditDefinitions() []audit.Definition {
	definitions := []audit.Definition{{Action: createdAction, TargetType: targetType}, {Action: storedAction, TargetType: targetType}, {Action: linkedAction, TargetType: targetType}}
	return append(definitions, safetyDefinitions()...)
}

func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }

func (s *Store) ready(scope kernel.OrgScope, actor app.Actor, id string) error {
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !validID(id) {
		return app.ErrNotFound
	}
	return nil
}

// storeError keeps bounded authority and document errors and maps database
// refusals; anything else is unavailable.
func storeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return app.ErrUnavailable
	}
	// Coverage denials keep their registered permission key.
	if errors.Is(err, permissions.ErrPermissionDenied) || errors.Is(err, permissions.ErrResourceNotFound) {
		return err
	}
	for _, known := range []error{app.ErrNotFound, app.ErrConflict, app.ErrOperationReused, app.ErrInvalid, app.ErrUnavailable,
		access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete,
		access.ErrUnauthenticated, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrLimitReached} {
		if errors.Is(err, known) {
			return known
		}
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrOperationReused
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505", "23514", "55000":
			return app.ErrConflict
		case "23503":
			return app.ErrNotFound
		}
	}
	return app.ErrUnavailable
}

func (s *Store) Create(ctx context.Context, scope kernel.OrgScope, actor app.Actor, command app.Create) (app.View, bool, error) {
	if err := s.ready(scope, actor, command.DocumentID); err != nil {
		return app.View{}, false, err
	}
	if err := command.Validate(scope); err != nil {
		return app.View{}, false, err
	}
	org := scope.OrgID().String()
	event := audit.Event{OperationID: command.OperationID, ActorID: actor.AccountID, Action: createdAction, TargetType: targetType,
		TargetID: command.DocumentID, RequestHash: command.Digest(scope, actor)}
	var view app.View
	var replayed bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		// A new document is the member's own record (B-BR-EMP-220).
		if err := s.Authority.CheckDocumentTx(ctx, tx, scope, actor, app.AttachPolicy(), app.Facts{TargetID: org, AuthoredBy: actor.MembershipID}); err != nil {
			return err
		}
		if s.Storage != nil {
			if err := s.Storage.LockStorageReservation(ctx, tx, scope); err != nil {
				return err
			}
		}
		var err error
		replayed, err = s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			if s.Storage != nil {
				if err := s.Storage.CheckStorageReservation(ctx, tx, scope, command.Original.Bytes); err != nil {
					return err
				}
			}
			if err := q.New(tx).InsertDocument(ctx, q.InsertDocumentParams{OrgID: org, ID: command.DocumentID, Kind: command.Original.Kind,
				OperationID: command.OperationID, CreatedBy: actor.MembershipID, OriginalSha256: command.Original.SHA256,
				OriginalBytes: command.Original.Bytes, OriginalType: command.Original.MediaType}); err != nil {
				return err
			}
			// EMP-DOC-014: the per-document closure job of the upload session.
			return enqueueSafety(ctx, tx, scope, command.DocumentID, ExpiryPurpose, time.Now().Add(app.UploadWindow+app.UploadGrace))
		})
		if err != nil {
			return err
		}
		row, err := q.New(tx).DocumentForShare(ctx, q.DocumentForShareParams{OrgID: org, ID: command.DocumentID})
		if err != nil {
			return err
		}
		if row.OperationID != command.OperationID || row.CreatedBy != actor.MembershipID {
			return app.ErrOperationReused
		}
		view, err = project(scope, q.DocumentForUpdateRow(row))
		return err
	})
	if err != nil {
		return app.View{}, false, storeError(ctx, err)
	}
	return view, replayed, nil
}

// load reads one row under the requested lock and requires the creator for
// creator-bound session commands.
func (s *Store) load(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, id string, update bool) (q.DocumentForUpdateRow, error) {
	params := q.DocumentForUpdateParams{OrgID: scope.OrgID().String(), ID: id}
	var row q.DocumentForUpdateRow
	var err error
	if update {
		row, err = q.New(tx).DocumentForUpdate(ctx, params)
	} else {
		var shared q.DocumentForShareRow
		shared, err = q.New(tx).DocumentForShare(ctx, q.DocumentForShareParams(params))
		row = q.DocumentForUpdateRow(shared)
	}
	if err != nil {
		return q.DocumentForUpdateRow{}, err
	}
	if row.CreatedBy != actor.MembershipID {
		return q.DocumentForUpdateRow{}, app.ErrNotFound
	}
	if err := s.Authority.CheckDocumentTx(ctx, tx, scope, actor, app.AttachPolicy(), app.Facts{TargetID: id, AuthoredBy: row.CreatedBy}); err != nil {
		return q.DocumentForUpdateRow{}, err
	}
	return row, nil
}

func (s *Store) Pending(ctx context.Context, scope kernel.OrgScope, actor app.Actor, id string) (app.View, error) {
	if err := s.ready(scope, actor, id); err != nil {
		return app.View{}, err
	}
	var view app.View
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		row, err := s.load(ctx, tx, scope, actor, id, false)
		if err != nil {
			return err
		}
		view, err = project(scope, row)
		return err
	})
	if err != nil {
		return app.View{}, storeError(ctx, err)
	}
	return view, nil
}

// Facts resolves the authoring membership for the guarded route resolver. It
// grants nothing: orgaccess applies coverage after positive permission.
func (s *Store) Facts(ctx context.Context, scope kernel.OrgScope, id string) (app.Facts, error) {
	if s == nil || s.Pool == nil {
		return app.Facts{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || !validID(id) {
		return app.Facts{}, app.ErrNotFound
	}
	var creator string
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		creator, err = q.New(tx).DocumentCreator(ctx, q.DocumentCreatorParams{OrgID: scope.OrgID().String(), ID: id})
		return err
	})
	if err != nil {
		return app.Facts{}, storeError(ctx, err)
	}
	if !validID(creator) {
		return app.Facts{}, app.ErrUnavailable
	}
	return app.Facts{TargetID: id, AuthoredBy: creator}, nil
}

// project rebuilds the view and refuses any row the closed vocabulary or the
// EMP-DOC-001 container would not accept.
func project(scope kernel.OrgScope, row q.DocumentForUpdateRow) (app.View, error) {
	if _, err := domain.New(scope, row.ID, row.OriginalSha256); err != nil {
		return app.View{}, app.ErrUnavailable
	}
	states := [...]struct {
		dimension domain.Dimension
		raw       string
	}{{domain.Storage, row.StorageState}, {domain.Extraction, row.ExtractionState}, {domain.Structure, row.StructureState},
		{domain.Conference, row.ConferenceState}, {domain.Reconciliation, row.ReconciliationState}, {domain.Fiscal, row.FiscalState}}
	parsed := make([]domain.State, len(states))
	for i, state := range states {
		value, err := domain.ParseState(state.dimension, state.raw)
		if err != nil {
			return app.View{}, app.ErrUnavailable
		}
		parsed[i] = value
	}
	if row.Kind != app.KindReceipt || row.DataMode != app.DataMode || !validID(row.OperationID) || !validID(row.CreatedBy) ||
		!row.CreatedAt.Valid || !row.UpdatedAt.Valid {
		return app.View{}, app.ErrUnavailable
	}
	view := app.View{ID: row.ID, Kind: row.Kind, DataMode: row.DataMode, OperationID: row.OperationID, CreatedBy: row.CreatedBy,
		Original:  app.Declaration{Kind: row.Kind, SHA256: row.OriginalSha256, Bytes: row.OriginalBytes, MediaType: row.OriginalType},
		Statuses:  domain.Statuses{Storage: parsed[0], Extraction: parsed[1], Structure: parsed[2], Conference: parsed[3], Reconciliation: parsed[4], Fiscal: parsed[5]},
		Revisions: app.Revisions{Storage: row.StorageRevision, Extraction: row.ExtractionRevision, Structure: row.StructureRevision, Conference: row.ConferenceRevision, Fiscal: row.FiscalRevision},
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC()}
	if consulted := timestamp(row.FiscalConsultedAt); consulted != nil {
		view.FiscalConsultedAt = consulted
	}
	return view, nil
}

func timestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time.UTC()
	return &at
}
