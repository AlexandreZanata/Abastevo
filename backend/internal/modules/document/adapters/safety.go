package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/document"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// Upload safety vocabulary (EMP-DOC-014). Terminal events carry the closed
// reason code; the stored action keeps its EMP-DOC-002 name.
const (
	receivedAction    = "document.upload.received"
	quarantinedAction = "document.upload.quarantined"
	failedAction      = "document.upload.failed"
)

var _ app.SafetyRepository = (*Store)(nil)

func safetyDefinitions() []audit.Definition {
	failures := []string{}
	for _, reason := range app.Reasons() {
		if _, to, _ := reason.Transition(); to == domain.StorageFailed {
			failures = append(failures, reason.Code())
		}
	}
	return []audit.Definition{{Action: receivedAction, TargetType: targetType},
		{Action: quarantinedAction, TargetType: targetType, Reasons: []string{app.UnsafeContent.Code()}, RequireReason: true},
		{Action: failedAction, TargetType: targetType, Reasons: failures, RequireReason: true}}
}

// storageDigest pins one storage step of the declared original.
func storageDigest(action, org, id string, revision int64, row q.DocumentSafetyRow, reason app.Reason) string {
	raw, _ := json.Marshal([]any{action + ".v1", org, id, revision, row.OriginalSha256, row.OriginalBytes, row.OriginalType, string(reason)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// step writes one storage transition by compare-and-set on its revision.
func step(ctx context.Context, tx pgx.Tx, org, id string, from, to domain.State, revision int64, reason app.Reason) error {
	n, err := q.New(tx).SettleStorage(ctx, q.SettleStorageParams{OrgID: org, ID: id, FromState: string(from), ToState: string(to),
		ExpectedRevision: revision, Reason: pgtype.Text{String: string(reason), Valid: reason != ""}})
	if err != nil {
		return err
	}
	if n != 1 {
		return app.ErrConflict
	}
	return nil
}

// allowed asks the EMP-DOC-001 domain for one storage step.
func allowed(from, to domain.State) bool {
	targets, err := domain.Allowed(domain.Storage, from)
	if err != nil {
		return false
	}
	for _, target := range targets {
		if target == to {
			return true
		}
	}
	return false
}

func outcomeAction(to domain.State) string {
	switch to {
	case domain.Stored:
		return storedAction
	case domain.Quarantined:
		return quarantinedAction
	}
	return failedAction
}

// MarkReceived records UPLOADING -> RECEIVED for the creator's verified,
// stored bytes with one document.upload.received event and queues the
// safety check (EMP-DOC-014). RECEIVED or STORED is a replay.
func (s *Store) MarkReceived(ctx context.Context, scope kernel.OrgScope, actor app.Actor, id, operation string) (app.View, bool, error) {
	if err := s.ready(scope, actor, id); err != nil {
		return app.View{}, false, err
	}
	if !validID(operation) {
		return app.View{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var view app.View
	replayed := false
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		row, err := s.load(ctx, tx, scope, actor, id, true)
		if err != nil {
			return err
		}
		switch domain.State(row.StorageState) {
		case domain.Received, domain.Stored:
			replayed = true
			view, err = project(scope, row)
			return err
		case domain.Uploading:
		default:
			return app.ErrConflict
		}
		if !allowed(domain.Uploading, domain.Received) {
			return app.ErrConflict
		}
		event := audit.Event{OperationID: operation, ActorID: actor.AccountID, Action: receivedAction, TargetType: targetType, TargetID: id,
			RequestHash: storageDigest(receivedAction, org, id, row.StorageRevision, safetyRow(row), "")}
		if _, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			if err := step(ctx, tx, org, id, domain.Uploading, domain.Received, row.StorageRevision, ""); err != nil {
				return err
			}
			return enqueueSafety(ctx, tx, scope, id, CheckPurpose, time.Now().Add(app.CheckDelay))
		}); err != nil {
			return err
		}
		current, err := q.New(tx).DocumentForUpdate(ctx, q.DocumentForUpdateParams{OrgID: org, ID: id})
		if err != nil {
			return err
		}
		view, err = project(scope, current)
		return err
	})
	if err != nil {
		return app.View{}, false, storeError(ctx, err)
	}
	return view, replayed, nil
}

// Refuse closes the creator's pending document whose verified bytes were
// refused by content, through RECEIVED, with one event carrying the reason.
// Only the ingress content verdicts close a session this way.
func (s *Store) Refuse(ctx context.Context, scope kernel.OrgScope, actor app.Actor, id string, reason app.Reason, operation string) (app.View, error) {
	if err := s.ready(scope, actor, id); err != nil {
		return app.View{}, err
	}
	if reason != app.UnsafeContent && reason != app.UnsupportedContent {
		return app.View{}, app.ErrInvalid
	}
	if !validID(operation) {
		return app.View{}, app.ErrUnavailable
	}
	_, to, _ := reason.Transition()
	org := scope.OrgID().String()
	var view app.View
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		row, err := s.load(ctx, tx, scope, actor, id, true)
		if err != nil {
			return err
		}
		if domain.State(row.StorageState) != domain.Uploading || !allowed(domain.Uploading, domain.Received) || !allowed(domain.Received, to) {
			return app.ErrConflict
		}
		action := outcomeAction(to)
		event := audit.Event{OperationID: operation, ActorID: actor.AccountID, Action: action, TargetType: targetType, TargetID: id, ReasonCode: reason.Code(),
			RequestHash: storageDigest(action, org, id, row.StorageRevision, safetyRow(row), reason)}
		if _, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			if err := step(ctx, tx, org, id, domain.Uploading, domain.Received, row.StorageRevision, ""); err != nil {
				return err
			}
			return step(ctx, tx, org, id, domain.Received, to, row.StorageRevision+1, reason)
		}); err != nil {
			return err
		}
		current, err := q.New(tx).DocumentForUpdate(ctx, q.DocumentForUpdateParams{OrgID: org, ID: id})
		if err != nil {
			return err
		}
		view, err = project(scope, current)
		return err
	})
	if err != nil {
		return app.View{}, storeError(ctx, err)
	}
	return view, nil
}

// safetyRow carries the identity columns a storage digest needs.
func safetyRow(row q.DocumentForUpdateRow) q.DocumentSafetyRow {
	return q.DocumentSafetyRow{ID: row.ID, DataMode: row.DataMode, OperationID: row.OperationID, OriginalSha256: row.OriginalSha256,
		OriginalBytes: row.OriginalBytes, OriginalType: row.OriginalType, StorageState: row.StorageState, StorageRevision: row.StorageRevision}
}

// subject projects a row for the check and refuses anything the closed
// vocabulary or the private object identity would not accept.
func subject(scope kernel.OrgScope, row q.DocumentSafetyRow) (app.Subject, error) {
	state, err := domain.ParseState(domain.Storage, row.StorageState)
	if err != nil {
		return app.Subject{}, app.ErrUnavailable
	}
	reason := app.Reason(row.StorageReason)
	if _, _, ok := reason.Transition(); !ok {
		return app.Subject{}, app.ErrUnavailable
	}
	object := media.Object{OrgID: scope.OrgID().String(), DocumentID: row.ID, SHA256: row.OriginalSha256, MIME: row.OriginalType,
		Bytes: row.OriginalBytes, Revision: row.StorageRevision, Synthetic: row.DataMode == app.DataMode}
	if object.Validate(scope, row.ID) != nil || !validID(row.OperationID) {
		return app.Subject{}, app.ErrUnavailable
	}
	return app.Subject{Object: object, State: state, Reason: reason, Revision: row.StorageRevision, Abandoned: row.Abandoned}, nil
}

// Subject loads the server-owned identity and storage state of a document
// for the safety check. It carries and checks no actor: the verdict judges
// the declared bytes, and only this organization's row is visible.
func (s *Store) Subject(ctx context.Context, scope kernel.OrgScope, id string) (app.Subject, error) {
	if s == nil || s.Pool == nil {
		return app.Subject{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || !validID(id) {
		return app.Subject{}, app.ErrNotFound
	}
	var row q.DocumentSafetyRow
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		row, err = q.New(tx).DocumentSafety(ctx, q.DocumentSafetyParams{OrgID: scope.OrgID().String(), ID: id})
		return err
	})
	if err != nil {
		return app.Subject{}, storeError(ctx, err)
	}
	return subject(scope, row)
}

// Settle commits one system outcome by compare-and-set on the expected state
// and storage revision under the row lock. A row that moved on, or a session
// that is still open, returns the current outcome and writes nothing. The
// event is attributed to the creating account: the creator-bound upload
// involves no other account and the audit log has no system actor (EDR-170).
func (s *Store) Settle(ctx context.Context, scope kernel.OrgScope, id string, from domain.State, revision int64, reason app.Reason, operation string) (app.Outcome, error) {
	if s == nil || s.Pool == nil || s.Audit == nil {
		return app.Outcome{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || !validID(id) {
		return app.Outcome{}, app.ErrNotFound
	}
	start, to, ok := reason.Transition()
	if !ok || start != from || revision < 1 || !validID(operation) || !allowed(from, to) {
		return app.Outcome{}, app.ErrInvalid
	}
	org := scope.OrgID().String()
	var outcome app.Outcome
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		locked, err := queries.DocumentSafetyForUpdate(ctx, q.DocumentSafetyForUpdateParams{OrgID: org, ID: id})
		if err != nil {
			return err
		}
		row := q.DocumentSafetyRow(locked)
		current, err := subject(scope, row)
		if err != nil {
			return err
		}
		outcome = app.Outcome{State: current.State, Reason: current.Reason}
		if current.State != from || current.Revision != revision || (reason == app.UploadAbandoned && !current.Abandoned) {
			return nil
		}
		account, err := queries.DocumentCreatorAccount(ctx, q.DocumentCreatorAccountParams{OrgID: org, OperationID: row.OperationID, ID: id})
		if err != nil || !validID(account) {
			return app.ErrUnavailable
		}
		action := outcomeAction(to)
		event := audit.Event{OperationID: operation, ActorID: account, Action: action, TargetType: targetType, TargetID: id, ReasonCode: reason.Code(),
			RequestHash: storageDigest(action, org, id, revision, row, reason)}
		if _, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			return step(ctx, tx, org, id, from, to, revision, reason)
		}); err != nil {
			return err
		}
		outcome = app.Outcome{State: to, Reason: reason}
		return nil
	})
	if err != nil {
		return app.Outcome{}, storeError(ctx, err)
	}
	return outcome, nil
}
