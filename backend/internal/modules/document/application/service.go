package application

import (
	"context"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// Authority applies current membership, organization state and the policy's
// coverage over the supplied facts inside the caller's scoped transaction. It
// returns the bounded orgaccess errors; it never trusts a client descriptor.
type Authority interface {
	CheckDocumentTx(context.Context, pgx.Tx, kernel.OrgScope, Actor, access.Policy, Facts) error
}

// Repository persists document containers. Every method checks current
// authority in its own scoped transaction before reading or changing a row.
type Repository interface {
	Create(context.Context, kernel.OrgScope, Actor, Create) (View, bool, error)
	// Pending returns the creator's own document under current attach authority.
	Pending(context.Context, kernel.OrgScope, Actor, string) (View, error)
	// MarkStored records UPLOADING -> RECEIVED -> STORED after verified bytes.
	MarkStored(context.Context, kernel.OrgScope, Actor, string, string) (View, bool, error)
	// Facts is the guarded route resource resolver; it carries no authority.
	Facts(context.Context, kernel.OrgScope, string) (Facts, error)
}

// Media is the existing bounded private original service (EMP-PLAT-006).
type Media interface {
	Upload(context.Context, kernel.OrgScope, media.Member, string, io.Reader) error
	Issue(context.Context, kernel.OrgScope, media.Member, string) (mediaapp.Access, error)
	Open(context.Context, kernel.OrgScope, media.Member, string, string) (mediaapp.Content, error)
}

// UploadTokens mints and verifies the upload capability. Verify happens before
// any byte is read and returns the bound actor; authority is rechecked later.
type UploadTokens interface {
	Issue(kernel.OrgScope, Actor, string, time.Time) (UploadAccess, error)
	Verify(kernel.OrgScope, string, string, time.Time) (Actor, error)
}

type actorKey struct{}

// WithActor binds the verified actor to one media call so that the document
// media authority can recheck the same proof subject; nothing else reads it.
func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}
func ActorFrom(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(Actor)
	return actor, ok && actor.Validate() == nil
}

// Service is default-off. Without Media the session can still be declared,
// but bytes and content are unavailable (EDR-153).
type Service struct {
	Enabled    bool
	Repository Repository
	Media      Media
	Tokens     UploadTokens
	Now        func() time.Time
	NewID      func() (string, error)
}

func (s Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s Service) ready(scope kernel.OrgScope, actor Actor) error {
	if !s.Enabled {
		return ErrDisabled
	}
	if s.Repository == nil {
		return ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil {
		return ErrNotFound
	}
	return nil
}

// Create declares the document and returns a fresh upload capability while
// storage is still pending, also for an identical replay.
func (s Service) Create(ctx context.Context, scope kernel.OrgScope, actor Actor, command Create) (Created, error) {
	if err := s.ready(scope, actor); err != nil {
		return Created{}, err
	}
	if err := command.Validate(scope); err != nil {
		return Created{}, err
	}
	if s.Tokens == nil {
		return Created{}, ErrUnavailable
	}
	view, replayed, err := s.Repository.Create(ctx, scope, actor, command)
	if err != nil {
		return Created{}, err
	}
	out := Created{View: view, Replayed: replayed}
	if view.Statuses.Storage == domain.Uploading {
		upload, err := s.Tokens.Issue(scope, actor, view.ID, s.now())
		if err != nil {
			return Created{}, ErrUnavailable
		}
		out.Upload = &upload
	}
	return out, nil
}

// VerifyUpload authenticates the ingress capability before the body is read.
func (s Service) VerifyUpload(scope kernel.OrgScope, documentID, token string) (Actor, error) {
	if !s.Enabled {
		return Actor{}, ErrDisabled
	}
	if s.Tokens == nil {
		return Actor{}, ErrUnavailable
	}
	if scope.Validate() != nil || !validID(documentID) {
		return Actor{}, ErrNotFound
	}
	actor, err := s.Tokens.Verify(scope, documentID, token, s.now())
	if err != nil || actor.Validate() != nil {
		return Actor{}, ErrNotFound
	}
	return actor, nil
}

// Upload sends verified bytes through orgmedia and only then records STORED.
// A retry after STORED reads nothing and writes nothing.
func (s Service) Upload(ctx context.Context, scope kernel.OrgScope, actor Actor, documentID string, body io.Reader) (Stored, error) {
	if err := s.ready(scope, actor); err != nil {
		return Stored{}, err
	}
	if !validID(documentID) {
		return Stored{}, ErrNotFound
	}
	view, err := s.Repository.Pending(ctx, scope, actor, documentID)
	if err != nil {
		return Stored{}, err
	}
	switch view.Statuses.Storage {
	case domain.Stored:
		return Stored{View: view, Replayed: true}, nil
	case domain.Uploading:
	default:
		return Stored{}, ErrConflict
	}
	if s.Media == nil || s.NewID == nil {
		return Stored{}, ErrUnavailable
	}
	if err := s.Media.Upload(WithActor(ctx, actor), scope, actor.Member(), documentID, body); err != nil {
		return Stored{}, err
	}
	operation, err := s.NewID()
	if err != nil || !validID(operation) {
		return Stored{}, ErrUnavailable
	}
	view, replayed, err := s.Repository.MarkStored(ctx, scope, actor, documentID, operation)
	if err != nil {
		return Stored{}, err
	}
	return Stored{View: view, Replayed: replayed}, nil
}

// IssueContent records ISSUED before a member-bound capability is minted.
func (s Service) IssueContent(ctx context.Context, scope kernel.OrgScope, actor Actor, documentID string) (mediaapp.Access, error) {
	if err := s.ready(scope, actor); err != nil {
		return mediaapp.Access{}, err
	}
	if !validID(documentID) {
		return mediaapp.Access{}, ErrNotFound
	}
	if s.Media == nil {
		return mediaapp.Access{}, ErrUnavailable
	}
	return s.Media.Issue(WithActor(ctx, actor), scope, actor.Member(), documentID)
}

// ReadContent records SERVED before the original leaves the service.
func (s Service) ReadContent(ctx context.Context, scope kernel.OrgScope, actor Actor, documentID, token string) (mediaapp.Content, error) {
	if err := s.ready(scope, actor); err != nil {
		return mediaapp.Content{}, err
	}
	if !validID(documentID) {
		return mediaapp.Content{}, ErrNotFound
	}
	if s.Media == nil {
		return mediaapp.Content{}, ErrUnavailable
	}
	return s.Media.Open(WithActor(ctx, actor), scope, actor.Member(), documentID, token)
}
