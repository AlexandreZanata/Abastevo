package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// DeviceUpload is one signed device attachment (EMP-OFFL-005, B-BR-EMP-417):
// the EMP-DOC-002 declaration and the EMP-DOC-003 link of the record it
// documents, carried with their own operation identifiers.
type DeviceUpload struct {
	private
	Create Create
	Link   Link
}

// DeviceSession is the declared document's current state; Upload is set only
// while storage is UPLOADING inside its session and the link exists.
type DeviceSession struct {
	private
	View   View
	Upload *UploadAccess
}

// OpenDevice declares, links and grants the upload for an enrolled device
// actor. It never serves content. The whole command is validated before any
// write; a refused link withholds the capability, and the declared document
// then closes with its session (B-BR-EMP-254). A document already FAILED,
// REPLACED or DELETED answers its state without a link or capability.
func (s Service) OpenDevice(ctx context.Context, scope kernel.OrgScope, actor Actor, command DeviceUpload) (DeviceSession, error) {
	if !s.Enabled {
		return DeviceSession{}, ErrDisabled
	}
	if s.Repository == nil || s.Links == nil || s.Tokens == nil {
		return DeviceSession{}, ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || actor.Channel != "android" {
		return DeviceSession{}, ErrNotFound
	}
	if err := command.Create.Validate(scope); err != nil {
		return DeviceSession{}, err
	}
	declaration, document, link := command.Create.OperationID, command.Create.DocumentID, command.Link.OperationID
	if command.Link.DocumentID != document || link == declaration || link == document || declaration == document {
		return DeviceSession{}, ErrInvalid
	}
	if err := command.Link.Validate(scope); err != nil {
		return DeviceSession{}, err
	}
	created, err := s.Create(ctx, scope, actor, command.Create)
	if err != nil {
		return DeviceSession{}, err
	}
	switch created.View.Statuses.Storage {
	case domain.StorageFailed, domain.Replaced, domain.Deleted:
		return DeviceSession{View: created.View}, nil
	}
	if _, err := s.Link(ctx, scope, actor, command.Link); err != nil {
		return DeviceSession{}, err
	}
	return DeviceSession{View: created.View, Upload: created.Upload}, nil
}
