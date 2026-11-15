package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/document"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// MediaAuthority is the orgmedia Authority: the document row owns the private
// original's identity (EDR-152) and current document authority gates every
// transfer. Upload is creator-bound and needs a pending document; issue and
// read need a STORED document and doc.document.view_content coverage.
type MediaAuthority struct {
	Pool      *pgxpool.Pool
	Authority app.Authority
}

var _ mediaapp.Authority = MediaAuthority{}

func mediaError(err error) error {
	var denied *permissions.PermissionDenial
	switch {
	case errors.As(err, &denied), errors.Is(err, permissions.ErrPermissionDenied):
		return media.ErrForbidden
	case errors.Is(err, media.ErrNotFound), errors.Is(err, pgx.ErrNoRows), errors.Is(err, access.ErrNoMembership),
		errors.Is(err, permissions.ErrResourceNotFound), errors.Is(err, app.ErrNotFound):
		return media.ErrNotFound
	}
	return media.ErrUnavailable
}

func (m MediaAuthority) Resolve(ctx context.Context, scope kernel.OrgScope, member media.Member, id string, intent media.Intent) (media.Object, error) {
	actor, ok := app.ActorFrom(ctx)
	if !ok || m.Pool == nil || m.Authority == nil {
		return media.Object{}, media.ErrUnavailable
	}
	if actor.Member() != member || scope.Validate() != nil || !validID(id) {
		return media.Object{}, media.ErrNotFound
	}
	policy := app.ContentPolicy()
	if intent == media.Upload {
		policy = app.AttachPolicy()
	} else if intent != media.Issue && intent != media.Read {
		return media.Object{}, media.ErrUnavailable
	}
	var object media.Object
	err := database.WithOrgTx(ctx, m.Pool, scope, func(tx pgx.Tx) error {
		row, err := q.New(tx).DocumentForShare(ctx, q.DocumentForShareParams{OrgID: scope.OrgID().String(), ID: id})
		if err != nil {
			return err
		}
		storage := domain.State(row.StorageState)
		// An upload is the creator's pending session only (B-BR-EMP-220).
		if intent == media.Upload && (row.CreatedBy != member.MembershipID || storage != domain.Uploading) {
			return media.ErrNotFound
		}
		if err := m.Authority.CheckDocumentTx(ctx, tx, scope, actor, policy, app.Facts{TargetID: id, AuthoredBy: row.CreatedBy}); err != nil {
			return err
		}
		object = media.Object{OrgID: scope.OrgID().String(), DocumentID: row.ID, SHA256: row.OriginalSha256, MIME: row.OriginalType,
			Bytes: row.OriginalBytes, Revision: row.StorageRevision, Synthetic: row.DataMode == app.DataMode, Ready: storage == domain.Stored}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return media.Object{}, media.ErrUnavailable
		}
		return media.Object{}, mediaError(err)
	}
	return object, nil
}

// CheckAccess rechecks current authority and the unchanged stored identity.
// Grant revocation is the member's current coverage plus the access log's
// member-bound issuance, which the database requires for every served read.
func (m MediaAuthority) CheckAccess(ctx context.Context, scope kernel.OrgScope, member media.Member, object media.Object, grant string) error {
	if !media.ValidID(grant) {
		return media.ErrNotFound
	}
	current, err := m.Resolve(ctx, scope, member, object.DocumentID, media.Read)
	if err != nil {
		return err
	}
	if !current.Ready || !object.SameIdentity(current) {
		return media.ErrNotFound
	}
	return nil
}

// UnavailableObjects stands in for unconfigured private storage: every byte
// transfer is unavailable, so no original is accepted or served.
type UnavailableObjects struct{}

var (
	_ mediaapp.Objects  = UnavailableObjects{}
	_ app.SafetyObjects = UnavailableObjects{}
)

func (UnavailableObjects) Put(context.Context, kernel.OrgScope, media.Object, []byte) error {
	return media.ErrUnavailable
}
func (UnavailableObjects) Get(context.Context, kernel.OrgScope, media.Object) ([]byte, error) {
	return nil, media.ErrUnavailable
}
func (UnavailableObjects) Delete(context.Context, kernel.OrgScope, media.Object) error {
	return media.ErrUnavailable
}
