package http

import (
	"context"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// DeviceSessions is the document owner of the signed attachment session
// (EMP-OFFL-005, EDR-270): the verified device source becomes the android
// actor of EMP-DOC-002/003; refusals keep the bounded private envelope and a
// permission refusal its registered key.
type DeviceSessions struct{ Service app.Service }

var _ syncapp.AttachmentSessions = DeviceSessions{}

func (d DeviceSessions) OpenAttachment(ctx context.Context, source syncapp.Source, in syncapp.AttachmentDeclaration) (syncapp.AttachmentSession, error) {
	if source.Scope.Validate() != nil {
		return syncapp.AttachmentSession{}, orgapi.NotFound
	}
	kind, err := domain.ParseTargetKind(in.TargetKind)
	if err != nil {
		return syncapp.AttachmentSession{}, orgapi.ValidationFailed
	}
	actor := app.Actor{AccountID: source.Actor.AccountID, MembershipID: source.MembershipID, Channel: "android", ContributorID: source.Actor.ContributorID, DeviceKeyID: source.Actor.KeyID}
	session, err := d.Service.OpenDevice(ctx, source.Scope, actor, app.DeviceUpload{
		Create: app.Create{OperationID: in.DeclarationID, DocumentID: in.DocumentID, Original: app.Declaration{Kind: in.Kind, SHA256: in.SHA256, Bytes: in.Bytes, MediaType: in.MediaType}},
		Link:   app.Link{OperationID: in.LinkID, DocumentID: in.DocumentID, Target: app.TargetRef{Kind: kind, ID: in.TargetID}},
	})
	if err != nil {
		return syncapp.AttachmentSession{}, mapped(err, string(app.AttachPolicy().Permission))
	}
	out := syncapp.AttachmentSession{DocumentID: session.View.ID, Storage: string(session.View.Statuses.Storage), StorageRevision: session.View.Revisions.Storage}
	if session.Upload != nil {
		out.Capability, out.ExpiresAt = session.Upload.Token, session.Upload.ExpiresAt
	}
	return out, nil
}
