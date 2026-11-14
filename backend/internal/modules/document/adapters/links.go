package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/document"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

const linkedAction = "document.link.created"

var _ app.LinkRepository = (*Store)(nil)

// linkRow is the common projection of the link queries.
type linkRow struct {
	DocumentID, TargetKind, TargetID, OperationID, CreatedBy, StorageState string
	CreatedAt                                                              pgtype.Timestamptz
}

func (s *Store) linkPorts() error {
	if s == nil || s.Authority == nil || s.Audit == nil || s.Targets == nil {
		return app.ErrUnavailable
	}
	return nil
}

// LinkTx is the EMP-DOC-003 link port inside the caller's scoped transaction.
// It checks both endpoints, writes at most one row and one audit event, and
// leaves commit or rollback to the caller.
func (s *Store) LinkTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, command app.Link) (app.LinkView, bool, error) {
	if err := s.linkPorts(); err != nil || tx == nil {
		return app.LinkView{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil {
		return app.LinkView{}, false, app.ErrNotFound
	}
	if err := command.Validate(scope); err != nil {
		return app.LinkView{}, false, err
	}
	policy, resolver, err := s.Targets.Policy(command.Target.Kind, access.OperationalWrite)
	if err != nil {
		return app.LinkView{}, false, err
	}
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return app.LinkView{}, false, app.ErrUnavailable
	}
	view, replayed, err := s.link(ctx, tx, scope, actor, command, policy, resolver)
	if err != nil {
		return app.LinkView{}, false, storeError(ctx, err)
	}
	return view, replayed, nil
}

// Link runs LinkTx in its own scoped transaction.
func (s *Store) Link(ctx context.Context, scope kernel.OrgScope, actor app.Actor, command app.Link) (app.LinkView, bool, error) {
	if err := s.linkPorts(); err != nil || s.Pool == nil {
		return app.LinkView{}, false, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil {
		return app.LinkView{}, false, app.ErrNotFound
	}
	var view app.LinkView
	var replayed bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		view, replayed, err = s.LinkTx(ctx, tx, scope, actor, command)
		return err
	})
	if err != nil {
		return app.LinkView{}, false, storeError(ctx, err)
	}
	return view, replayed, nil
}

func (s *Store) link(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, command app.Link, policy access.Policy, resolver app.TargetResolver) (app.LinkView, bool, error) {
	org := scope.OrgID().String()
	queries := q.New(tx)
	// The document row lock serializes the link writes of one document, so a
	// duplicate under any operation sees the committed first link.
	row, err := queries.DocumentForUpdate(ctx, q.DocumentForUpdateParams{OrgID: org, ID: command.DocumentID})
	if err != nil {
		return app.LinkView{}, false, err
	}
	if err := s.Authority.CheckDocumentTx(ctx, tx, scope, actor, app.AttachPolicy(), app.Facts{TargetID: row.ID, AuthoredBy: row.CreatedBy}); err != nil {
		return app.LinkView{}, false, err
	}
	// EMP-DOC-001 decides the owned same-organization link; it grants no
	// access and recognizes no cost (EDR-135).
	document, err := domain.New(scope, row.ID, row.OriginalSha256)
	if err != nil {
		return app.LinkView{}, false, app.ErrUnavailable
	}
	target, err := domain.NewTarget(scope, command.Target.Kind, command.Target.ID)
	if err != nil {
		return app.LinkView{}, false, app.ErrInvalid
	}
	if _, err := document.Link(target); err != nil {
		return app.LinkView{}, false, app.ErrInvalid
	}
	switch domain.State(row.StorageState) {
	case domain.StorageFailed, domain.Replaced, domain.Deleted:
		return app.LinkView{}, false, app.ErrConflict
	}
	binding, err := s.target(ctx, tx, scope, actor, policy, resolver, command.Target)
	if err != nil {
		return app.LinkView{}, false, err
	}
	if prior, err := queries.DocumentLinkByOperation(ctx, q.DocumentLinkByOperationParams{OrgID: org, OperationID: command.OperationID}); err == nil {
		if prior.DocumentID != command.DocumentID || prior.TargetKind != string(command.Target.Kind) || prior.TargetID != command.Target.ID || prior.CreatedBy != actor.MembershipID {
			return app.LinkView{}, false, app.ErrOperationReused
		}
		view, err := projectLink(scope, linkRow(prior))
		return view, true, err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.LinkView{}, false, err
	}
	pair := q.DocumentLinkByTargetParams{OrgID: org, DocumentID: command.DocumentID, TargetKind: string(command.Target.Kind), TargetID: command.Target.ID}
	if existing, err := queries.DocumentLinkByTarget(ctx, pair); err == nil {
		view, err := projectLink(scope, linkRow(existing))
		return view, true, err
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return app.LinkView{}, false, err
	}
	event := audit.Event{OperationID: command.OperationID, ActorID: actor.AccountID, Action: linkedAction, TargetType: targetType,
		TargetID: command.DocumentID, RequestHash: command.Digest(scope, actor)}
	insert := q.InsertDocumentLinkParams{OrgID: org, DocumentID: command.DocumentID, TargetKind: string(command.Target.Kind), TargetID: command.Target.ID,
		OperationID: command.OperationID, CreatedBy: actor.MembershipID}
	reference := pgtype.Text{String: command.Target.ID, Valid: true}
	switch binding {
	case app.BindAsset:
		insert.AssetID = reference
	case app.BindEmployee:
		insert.EmployeeID = reference
	case app.BindMeterReading:
		insert.ReadingID = reference
	case app.BindHeldMeterReading:
		insert.HeldReadingID = reference
	default:
		return app.LinkView{}, false, app.ErrUnavailable
	}
	replayed, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		return q.New(tx).InsertDocumentLink(ctx, insert)
	})
	if err != nil {
		return app.LinkView{}, false, err
	}
	if replayed {
		// An identical event without its link row cannot be produced here.
		return app.LinkView{}, false, app.ErrUnavailable
	}
	created, err := queries.DocumentLinkByTarget(ctx, pair)
	if err != nil {
		return app.LinkView{}, false, err
	}
	view, err := projectLink(scope, linkRow(created))
	return view, false, err
}

// target applies the owner's policy, resolving the record only after a
// positive permission; the resolver's own refusal is kept.
func (s *Store) target(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, policy access.Policy, resolver app.TargetResolver, ref app.TargetRef) (app.Binding, error) {
	var binding app.Binding
	var resolveErr error
	err := s.Authority.CheckTargetTx(ctx, tx, scope, actor, policy, func(ctx context.Context) (app.Facts, error) {
		resolved, err := resolver.ResolveDocumentTargetTx(ctx, tx, scope, ref.Kind, ref.ID)
		if err == nil {
			kind, ok := resolved.Binding.Kind()
			if !ok || kind != ref.Kind || resolved.AuthoredBy != "" && !validID(resolved.AuthoredBy) {
				err = app.ErrUnavailable
			}
		}
		if err != nil {
			switch {
			case errors.Is(err, app.ErrNotFound), errors.Is(err, app.ErrConflict):
			default:
				err = app.ErrUnavailable
			}
			resolveErr = err
			return app.Facts{}, err
		}
		binding = resolved.Binding
		return app.Facts{TargetID: ref.ID, AuthoredBy: resolved.AuthoredBy}, nil
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	if err != nil {
		return "", err
	}
	if binding == "" {
		return "", app.ErrUnavailable
	}
	return binding, nil
}

// DocumentLinks pages one document's links under the link read policy.
func (s *Store) DocumentLinks(ctx context.Context, scope kernel.OrgScope, actor app.Actor, documentID string, filter app.DocumentLinkFilter) (app.LinkPage, error) {
	if err := s.linkPorts(); err != nil || s.Pool == nil {
		return app.LinkPage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !validID(documentID) {
		return app.LinkPage{}, app.ErrNotFound
	}
	if err := filter.Validate(scope); err != nil {
		return app.LinkPage{}, err
	}
	org := scope.OrgID().String()
	var page app.LinkPage
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		creator, err := queries.DocumentCreator(ctx, q.DocumentCreatorParams{OrgID: org, ID: documentID})
		if err != nil {
			return err
		}
		if err := s.Authority.CheckDocumentTx(ctx, tx, scope, actor, app.LinkReadPolicy(), app.Facts{TargetID: documentID, AuthoredBy: creator}); err != nil {
			return err
		}
		params := q.DocumentLinksByDocumentParams{OrgID: org, DocumentID: documentID, RowLimit: int32(filter.Limit + 1)}
		if filter.After != nil {
			params.AfterKind = pgtype.Text{String: string(filter.After.Kind), Valid: true}
			params.AfterTarget = pgtype.Text{String: filter.After.ID, Valid: true}
		}
		rows, err := queries.DocumentLinksByDocument(ctx, params)
		if err != nil {
			return err
		}
		items := make([]linkRow, len(rows))
		for i, row := range rows {
			items[i] = linkRow(row)
		}
		page, err = projectPage(scope, items, filter.Limit)
		return err
	})
	if err != nil {
		return app.LinkPage{}, storeError(ctx, err)
	}
	return page, nil
}

// TargetLinks pages one target's links. The subject needs current attach
// authority and the owner's permission covering the target (EDR-163).
func (s *Store) TargetLinks(ctx context.Context, scope kernel.OrgScope, actor app.Actor, target app.TargetRef, filter app.TargetLinkFilter) (app.LinkPage, error) {
	if err := s.linkPorts(); err != nil || s.Pool == nil {
		return app.LinkPage{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil {
		return app.LinkPage{}, app.ErrNotFound
	}
	if err := target.Validate(scope); err != nil {
		return app.LinkPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.LinkPage{}, err
	}
	policy, resolver, err := s.Targets.Policy(target.Kind, access.Read)
	if err != nil {
		return app.LinkPage{}, err
	}
	org := scope.OrgID().String()
	var page app.LinkPage
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckDocumentTx(ctx, tx, scope, actor, app.LinkReadPolicy(), app.Facts{TargetID: org, AuthoredBy: actor.MembershipID}); err != nil {
			return err
		}
		if _, err := s.target(ctx, tx, scope, actor, policy, resolver, target); err != nil {
			return err
		}
		params := q.DocumentLinksByTargetParams{OrgID: org, TargetKind: string(target.Kind), TargetID: target.ID, RowLimit: int32(filter.Limit + 1)}
		if filter.AfterDocument != "" {
			params.AfterDocument = pgtype.Text{String: filter.AfterDocument, Valid: true}
		}
		rows, err := q.New(tx).DocumentLinksByTarget(ctx, params)
		if err != nil {
			return err
		}
		items := make([]linkRow, len(rows))
		for i, row := range rows {
			items[i] = linkRow(row)
		}
		page, err = projectPage(scope, items, filter.Limit)
		return err
	})
	if err != nil {
		return app.LinkPage{}, storeError(ctx, err)
	}
	return page, nil
}

func projectPage(scope kernel.OrgScope, rows []linkRow, limit int) (app.LinkPage, error) {
	page := app.LinkPage{More: len(rows) > limit}
	if page.More {
		rows = rows[:limit]
	}
	page.Items = make([]app.LinkView, 0, len(rows))
	for _, row := range rows {
		view, err := projectLink(scope, row)
		if err != nil {
			return app.LinkPage{}, err
		}
		page.Items = append(page.Items, view)
	}
	return page, nil
}

// projectLink refuses any row outside the closed vocabulary.
func projectLink(scope kernel.OrgScope, row linkRow) (app.LinkView, error) {
	ref := app.TargetRef{Kind: domain.TargetKind(row.TargetKind), ID: row.TargetID}
	storage, err := domain.ParseState(domain.Storage, row.StorageState)
	if err != nil || ref.Validate(scope) != nil || !validID(row.DocumentID) || !validID(row.OperationID) || !validID(row.CreatedBy) || !row.CreatedAt.Valid {
		return app.LinkView{}, app.ErrUnavailable
	}
	return app.LinkView{DocumentID: row.DocumentID, Target: ref, DocumentStorage: storage, OperationID: row.OperationID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time.UTC()}, nil
}
