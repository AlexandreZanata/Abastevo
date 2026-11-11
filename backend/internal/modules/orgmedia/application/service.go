// Package application owns bounded private transfers, not document persistence.
package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// Authority verifies the current session, membership, exact record/content
// permission and synthetic declaration/state. CheckAccess also checks individual
// grant revocation. The owning AUTH/DOC implementations must fail closed.
type Authority interface {
	Resolve(context.Context, kernel.OrgScope, domain.Member, string, domain.Intent) (domain.Object, error)
	CheckAccess(context.Context, kernel.OrgScope, domain.Member, domain.Object, string) error
}
type Objects interface {
	Put(context.Context, kernel.OrgScope, domain.Object, []byte) error
	Get(context.Context, kernel.OrgScope, domain.Object) ([]byte, error)
}

// Inspector returns the type established by content, never by a filename.
// Missing/unsupported format inspection refuses; it must not transform originals.
type Inspector interface {
	Inspect(context.Context, []byte) (string, error)
}
type Tokens interface {
	Issue(kernel.OrgScope, domain.Member, domain.Object, string, time.Time) (Access, error)
	Verify(kernel.OrgScope, domain.Member, domain.Object, string, time.Time) (string, error)
}

// Audit commits the bounded access attempt before any token or bytes leave API.
// RequestID is server-generated for each actual access, including repeated reads.
type Audit interface {
	Record(context.Context, kernel.OrgScope, domain.Member, domain.Object, string, string, domain.Intent) error
}
type Access struct {
	Token     string    `json:"capability"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (Access) String() string       { return "orgmedia: access redacted" }
func (Access) GoString() string     { return "orgmedia: access redacted" }
func (Access) LogValue() slog.Value { return slog.StringValue("orgmedia: access redacted") }

type Content struct {
	Bytes []byte
	MIME  string
}

func (Content) String() string       { return "orgmedia: content redacted" }
func (Content) GoString() string     { return "orgmedia: content redacted" }
func (Content) LogValue() slog.Value { return slog.StringValue("orgmedia: content redacted") }

type Config struct {
	Enabled   bool
	Authority Authority
	Objects   Objects
	Inspector Inspector
	Tokens    Tokens
	Audit     Audit
	Now       func() time.Time
	NewID     func() (string, error)
}
type Service struct {
	config    Config
	transfers chan struct{}
}

func NewService(config Config) (*Service, error) {
	if config.Authority == nil || config.Objects == nil || config.Inspector == nil || config.Tokens == nil || config.Audit == nil || config.NewID == nil {
		return nil, domain.ErrUnavailable
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{config: config, transfers: make(chan struct{}, 4)}, nil
}
func safeAuthorityError(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	return domain.ErrUnavailable
}
func (service *Service) resolve(ctx context.Context, scope kernel.OrgScope, member domain.Member, id string, intent domain.Intent) (domain.Object, error) {
	if ctx.Err() != nil {
		return domain.Object{}, domain.ErrUnavailable
	}
	if scope.Validate() != nil || member.Validate() != nil || !domain.ValidID(id) {
		return domain.Object{}, domain.ErrNotFound
	}
	object, err := service.config.Authority.Resolve(ctx, scope, member, id, intent)
	if ctx.Err() != nil {
		return domain.Object{}, domain.ErrUnavailable
	}
	if err != nil {
		return domain.Object{}, safeAuthorityError(err)
	}
	if object.Validate(scope, id) != nil || (intent != domain.Upload && !object.Ready) {
		return domain.Object{}, domain.ErrNotFound
	}
	return object, nil
}
func (service *Service) enter() (func(), error) {
	if !service.config.Enabled {
		return nil, domain.ErrDisabled
	}
	select {
	case service.transfers <- struct{}{}:
		return func() { <-service.transfers }, nil
	default:
		return nil, domain.ErrUnavailable
	}
}
func (service *Service) newID() (string, error) {
	id, err := service.config.NewID()
	if err != nil || !domain.ValidID(id) {
		return "", domain.ErrUnavailable
	}
	return id, nil
}

// Upload bounds bytes. Its HTTP owner also sets the request read deadline, as in
// the existing ingress gateway; context alone cannot interrupt an arbitrary Reader.
func (service *Service) Upload(ctx context.Context, scope kernel.OrgScope, member domain.Member, id string, body io.Reader) error {
	release, err := service.enter()
	if err != nil {
		return err
	}
	defer release()
	if body == nil {
		return domain.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	object, err := service.resolve(ctx, scope, member, id, domain.Upload)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(body, object.Bytes+1))
	if err != nil {
		return domain.ErrIntegrity
	}
	if err := object.CheckSnapshot(raw); err != nil {
		return err
	}
	if err := service.inspect(ctx, object, raw); err != nil {
		return err
	}
	current, err := service.resolve(ctx, scope, member, id, domain.Upload)
	if err != nil {
		return err
	}
	if !object.SameIdentity(current) {
		return domain.ErrNotFound
	}
	if err := service.config.Objects.Put(ctx, scope, object, raw); err != nil {
		return domain.ErrUnavailable
	}
	// A failed metadata/authority recheck never acknowledges a ready document.
	current, err = service.resolve(ctx, scope, member, id, domain.Upload)
	if err != nil {
		return err
	}
	if !object.SameIdentity(current) {
		return domain.ErrNotFound
	}
	return nil
}

func (service *Service) inspect(ctx context.Context, object domain.Object, raw []byte) error {
	mime, err := service.config.Inspector.Inspect(ctx, raw)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUnsupported):
			return domain.ErrUnsupported
		case errors.Is(err, domain.ErrTooLarge):
			return domain.ErrTooLarge
		case errors.Is(err, domain.ErrIntegrity):
			return domain.ErrIntegrity
		}
		return domain.ErrUnavailable
	}
	if mime != object.MIME {
		return domain.ErrUnsupported
	}
	// An inspector must not substitute or sanitize the accepted snapshot.
	if err := object.CheckSnapshot(raw); err != nil {
		return err
	}
	return nil
}
func (service *Service) Issue(ctx context.Context, scope kernel.OrgScope, member domain.Member, id string) (Access, error) {
	if !service.config.Enabled {
		return Access{}, domain.ErrDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	object, err := service.resolve(ctx, scope, member, id, domain.Issue)
	if err != nil {
		return Access{}, err
	}
	accessID, err := service.newID()
	if err != nil {
		return Access{}, err
	}
	if err := service.config.Authority.CheckAccess(ctx, scope, member, object, accessID); err != nil {
		return Access{}, safeAuthorityError(err)
	}
	access, err := service.config.Tokens.Issue(scope, member, object, accessID, service.config.Now())
	if err != nil {
		return Access{}, domain.ErrUnavailable
	}
	if err := service.config.Audit.Record(ctx, scope, member, object, accessID, accessID, domain.Issue); err != nil {
		return Access{}, domain.ErrUnavailable
	}
	if err := service.recheck(ctx, scope, member, object, access.Token, accessID); err != nil {
		return Access{}, err
	}
	return access, nil
}
func (service *Service) recheck(ctx context.Context, scope kernel.OrgScope, member domain.Member, object domain.Object, token, accessID string) error {
	current, err := service.resolve(ctx, scope, member, object.DocumentID, domain.Read)
	if err != nil {
		return err
	}
	if !object.SameIdentity(current) {
		return domain.ErrNotFound
	}
	if err := service.config.Authority.CheckAccess(ctx, scope, member, current, accessID); err != nil {
		return safeAuthorityError(err)
	}
	verified, err := service.config.Tokens.Verify(scope, member, current, token, service.config.Now())
	if err != nil || verified != accessID {
		return domain.ErrNotFound
	}
	if ctx.Err() != nil {
		return domain.ErrUnavailable
	}
	return nil
}
func (service *Service) Open(ctx context.Context, scope kernel.OrgScope, member domain.Member, id, token string) (Content, error) {
	release, err := service.enter()
	if err != nil {
		return Content{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	object, err := service.resolve(ctx, scope, member, id, domain.Read)
	if err != nil {
		return Content{}, err
	}
	accessID, err := service.config.Tokens.Verify(scope, member, object, token, service.config.Now())
	if err != nil || !domain.ValidID(accessID) {
		return Content{}, domain.ErrNotFound
	}
	if err := service.config.Authority.CheckAccess(ctx, scope, member, object, accessID); err != nil {
		return Content{}, safeAuthorityError(err)
	}
	raw, err := service.config.Objects.Get(ctx, scope, object)
	if err != nil {
		return Content{}, domain.ErrUnavailable
	}
	if err := object.CheckSnapshot(raw); err != nil {
		return Content{}, domain.ErrIntegrity
	}
	raw = bytes.Clone(raw)
	if err := service.inspect(ctx, object, raw); err != nil {
		return Content{}, err
	}
	requestID, err := service.newID()
	if err != nil {
		return Content{}, err
	}
	if err := service.config.Audit.Record(ctx, scope, member, object, accessID, requestID, domain.Read); err != nil {
		return Content{}, domain.ErrUnavailable
	}
	if err := service.recheck(ctx, scope, member, object, token, accessID); err != nil {
		return Content{}, err
	}
	return Content{Bytes: raw, MIME: object.MIME}, nil
}
