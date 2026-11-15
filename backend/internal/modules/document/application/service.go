package application

import (
	"context"
	"errors"
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
	// CheckTargetTx applies a target owner's policy (EMP-DOC-003). The facts
	// are resolved only after a positive permission is established.
	CheckTargetTx(context.Context, pgx.Tx, kernel.OrgScope, Actor, access.Policy, func(context.Context) (Facts, error)) error
}

// Repository persists document containers. Every method checks current
// authority in its own scoped transaction before reading or changing a row.
type Repository interface {
	Create(context.Context, kernel.OrgScope, Actor, Create) (View, bool, error)
	// Pending returns the creator's own document under current attach authority.
	Pending(context.Context, kernel.OrgScope, Actor, string) (View, error)
	// MarkReceived records UPLOADING -> RECEIVED for the creator's verified,
	// stored bytes and queues their safety check; RECEIVED or later replays.
	MarkReceived(context.Context, kernel.OrgScope, Actor, string, string) (View, bool, error)
	// Refuse closes the creator's pending document whose verified bytes were
	// refused by content: UPLOADING -> RECEIVED -> QUARANTINED or FAILED.
	Refuse(ctx context.Context, scope kernel.OrgScope, actor Actor, id string, reason Reason, operation string) (View, error)
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
// but bytes and content are unavailable (EDR-153). Without Links the link
// surface is unavailable (EMP-DOC-003).
type Service struct {
	Enabled    bool
	Repository Repository
	Links      LinkRepository
	Media      Media
	// Safety is the check that alone makes received bytes STORED
	// (EMP-DOC-014); without it bytes are unavailable.
	Safety Safety
	Tokens UploadTokens
	Now    func() time.Time
	NewID  func() (string, error)
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
	// A capability never outlives the upload session (B-BR-EMP-254).
	if view.Statuses.Storage == domain.Uploading && !s.now().Add(UploadTTL).After(view.CreatedAt.Add(UploadWindow)) {
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

// Upload sends bytes through orgmedia's integrity gate and inspection; clean
// bytes are stored and RECEIVED, and only the safety check over the stored
// object makes them STORED (EMP-DOC-014). Content refused by a verified
// snapshot closes the session with its reason. A retry while RECEIVED runs
// the check without reading bytes; a retry after STORED reads and writes
// nothing.
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
	case domain.Received:
		return s.check(ctx, scope, actor, documentID, true)
	case domain.Uploading:
	default:
		return Stored{}, ErrConflict
	}
	if s.Media == nil || s.Safety == nil || s.NewID == nil {
		return Stored{}, ErrUnavailable
	}
	verdict := s.Media.Upload(WithActor(ctx, actor), scope, actor.Member(), documentID, body)
	reason, refused := contentReason(verdict)
	if verdict != nil && !refused {
		return Stored{}, verdict
	}
	operation, err := s.NewID()
	if err != nil || !validID(operation) {
		return Stored{}, ErrUnavailable
	}
	if refused {
		// The declared digest pins these bytes: no retry can succeed.
		if _, err := s.Repository.Refuse(ctx, scope, actor, documentID, reason, operation); err != nil {
			return Stored{}, err
		}
		return Stored{}, verdict
	}
	if _, _, err := s.Repository.MarkReceived(ctx, scope, actor, documentID, operation); err != nil {
		return Stored{}, err
	}
	return s.check(ctx, scope, actor, documentID, false)
}

// contentReason maps orgmedia's content verdicts on a verified snapshot.
func contentReason(err error) (Reason, bool) {
	switch {
	case errors.Is(err, media.ErrUnsafe):
		return UnsafeContent, true
	case errors.Is(err, media.ErrUnsupported):
		return UnsupportedContent, true
	}
	return "", false
}

// check runs the safety check now. A check that cannot finish leaves the
// received bytes to the queued job and answers the RECEIVED record.
func (s Service) check(ctx context.Context, scope kernel.OrgScope, actor Actor, id string, replayed bool) (Stored, error) {
	if s.Safety == nil {
		return Stored{}, ErrUnavailable
	}
	outcome, _ := s.Safety.Converge(ctx, scope, id, false)
	if refusal := refusalOf(outcome); refusal != nil {
		return Stored{}, refusal
	}
	view, err := s.Repository.Pending(ctx, scope, actor, id)
	if err != nil {
		return Stored{}, err
	}
	switch view.Statuses.Storage {
	case domain.Stored, domain.Received:
		return Stored{View: view, Replayed: replayed}, nil
	}
	return Stored{}, ErrConflict
}

// refusalOf answers a terminal check outcome with the refusal its reason names.
func refusalOf(outcome Outcome) error {
	switch {
	case outcome.State == domain.Quarantined:
		return media.ErrUnsafe
	case outcome.State != domain.StorageFailed:
		return nil
	case outcome.Reason == UnsupportedContent:
		return media.ErrUnsupported
	case outcome.Reason == IntegrityMismatch:
		return media.ErrIntegrity
	}
	return ErrConflict
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
