package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// Upload safety (EMP-DOC-014, EDR-169/170): verified bytes are RECEIVED, and
// only the check over the stored object makes them STORED.
var (
	// ErrRetry means a check or an object removal did not finish; the
	// state written so far is truthful and a later attempt converges.
	ErrRetry = errors.New("document: safety check incomplete")
	// ErrNotDue means the upload session is still open.
	ErrNotDue = errors.New("document: upload session still open")
)

const (
	// UploadWindow bounds an upload session from its declaration: a
	// capability is minted only if it expires by then (B-BR-EMP-254).
	UploadWindow = 24 * time.Hour
	// UploadGrace covers a transfer that verified its capability just before
	// the close; the database refuses an earlier closure.
	UploadGrace = 5 * time.Minute
	// CheckDelay leaves the inline check time to finish before the queued
	// check of the same document runs.
	CheckDelay = time.Minute
)

// Reason is the closed reason of a QUARANTINED or FAILED document.
type Reason string

const (
	UnsafeContent      Reason = "UNSAFE_CONTENT"
	UnsupportedContent Reason = "UNSUPPORTED_CONTENT"
	IntegrityMismatch  Reason = "INTEGRITY_MISMATCH"
	CheckExhausted     Reason = "CHECK_ATTEMPTS_EXHAUSTED"
	UploadAbandoned    Reason = "UPLOAD_ABANDONED"
)

// Reasons returns the closed set in a stable order.
func Reasons() []Reason {
	return []Reason{UnsafeContent, UnsupportedContent, IntegrityMismatch, CheckExhausted, UploadAbandoned}
}

// Transition returns the storage step a reason closes; the empty reason is
// the passing check RECEIVED -> STORED.
func (r Reason) Transition() (from, to domain.State, ok bool) {
	switch r {
	case "":
		return domain.Received, domain.Stored, true
	case UnsafeContent:
		return domain.Received, domain.Quarantined, true
	case UnsupportedContent, IntegrityMismatch, CheckExhausted:
		return domain.Received, domain.StorageFailed, true
	case UploadAbandoned:
		return domain.Uploading, domain.StorageFailed, true
	}
	return "", "", false
}

// Code is the registered audit reason code of the reason.
func (r Reason) Code() string {
	switch r {
	case UnsafeContent:
		return "content.unsafe"
	case UnsupportedContent:
		return "content.unsupported"
	case IntegrityMismatch:
		return "integrity.mismatch"
	case CheckExhausted:
		return "check.exhausted"
	case UploadAbandoned:
		return "upload.abandoned"
	}
	return ""
}

// Outcome is the storage state a document converged to and its reason.
type Outcome struct {
	State  domain.State
	Reason Reason
}

// Subject is the server-owned row a check acts on. It carries no actor:
// a verdict judges the declared bytes, not a request.
type Subject struct {
	private
	// Object is the private original's identity from the row; never ready.
	Object   media.Object
	State    domain.State
	Reason   Reason
	Revision int64
	// Abandoned reports an UPLOADING session past its close and grace on the
	// database clock.
	Abandoned bool
}

// SafetyRepository persists system settlements in a scoped transaction.
// Settle is a compare-and-set: it writes only while the row is still at
// `from` and `revision`, and otherwise returns the current outcome unchanged.
type SafetyRepository interface {
	Subject(context.Context, kernel.OrgScope, string) (Subject, error)
	Settle(ctx context.Context, scope kernel.OrgScope, id string, from domain.State, revision int64, reason Reason, operation string) (Outcome, error)
}

// SafetyObjects reads a stored original under its organization key (bounded
// and digest-checked) and removes one with confirmed absence.
type SafetyObjects interface {
	Get(context.Context, kernel.OrgScope, media.Object) ([]byte, error)
	Delete(context.Context, kernel.OrgScope, media.Object) error
}

// Safety converges one document toward its terminal storage state. final
// reports the caller's last attempt: an incomplete check then fails as
// CHECK_ATTEMPTS_EXHAUSTED instead of asking for a retry.
type Safety interface {
	Converge(ctx context.Context, scope kernel.OrgScope, id string, final bool) (Outcome, error)
}

// Checker is the safety check shared by the ingress and the queued job.
type Checker struct {
	Repository SafetyRepository
	Objects    SafetyObjects
	Inspector  mediaapp.Inspector
	NewID      func() (string, error)
}

var _ Safety = Checker{}

// Converge acts on the current row only: RECEIVED runs the check, an
// abandoned UPLOADING session is closed, FAILED removes the object, and every
// other state is already terminal.
func (c Checker) Converge(ctx context.Context, scope kernel.OrgScope, id string, final bool) (Outcome, error) {
	if c.Repository == nil || c.Objects == nil || c.Inspector == nil || c.NewID == nil {
		return Outcome{}, ErrUnavailable
	}
	if scope.Validate() != nil || !validID(id) {
		return Outcome{}, ErrNotFound
	}
	subject, err := c.Repository.Subject(ctx, scope, id)
	switch {
	case errors.Is(err, ErrNotFound):
		return Outcome{}, ErrNotFound
	case err != nil:
		return Outcome{}, ErrRetry
	}
	current := Outcome{State: subject.State, Reason: subject.Reason}
	switch subject.State {
	case domain.Received:
		reason, err := c.verdict(ctx, scope, subject.Object)
		if err != nil {
			if !final {
				return current, ErrRetry
			}
			reason = CheckExhausted
		}
		return c.settle(ctx, scope, subject, reason)
	case domain.Uploading:
		if !subject.Abandoned {
			return current, ErrNotDue
		}
		return c.settle(ctx, scope, subject, UploadAbandoned)
	case domain.StorageFailed:
		return current, c.remove(ctx, scope, subject.Object)
	}
	return current, nil
}

// verdict judges what storage holds under the document key against the
// declaration. An error means the check could not complete and says nothing
// about the bytes.
func (c Checker) verdict(ctx context.Context, scope kernel.OrgScope, object media.Object) (Reason, error) {
	raw, err := c.Objects.Get(ctx, scope, object)
	switch {
	case errors.Is(err, media.ErrIntegrity), errors.Is(err, media.ErrTooLarge):
		return IntegrityMismatch, nil
	case err != nil:
		return "", ErrRetry
	}
	if object.CheckSnapshot(raw) != nil {
		return IntegrityMismatch, nil
	}
	mime, err := c.Inspector.Inspect(ctx, raw)
	switch {
	case errors.Is(err, media.ErrUnsafe):
		return UnsafeContent, nil
	case errors.Is(err, media.ErrUnsupported):
		return UnsupportedContent, nil
	case errors.Is(err, media.ErrTooLarge), errors.Is(err, media.ErrIntegrity):
		return IntegrityMismatch, nil
	case err != nil:
		return "", ErrRetry
	case mime != object.MIME:
		return UnsupportedContent, nil
	}
	// An inspector must not substitute or sanitize the stored snapshot.
	if object.CheckSnapshot(raw) != nil {
		return IntegrityMismatch, nil
	}
	return "", nil
}

// settle commits the reason's step by compare-and-set; whichever outcome the
// row holds afterwards is the answer, and a FAILED object is removed.
func (c Checker) settle(ctx context.Context, scope kernel.OrgScope, subject Subject, reason Reason) (Outcome, error) {
	current := Outcome{State: subject.State, Reason: subject.Reason}
	from, _, ok := reason.Transition()
	if !ok {
		return current, ErrUnavailable
	}
	operation, err := c.NewID()
	if err != nil || !validID(operation) {
		return current, ErrRetry
	}
	outcome, err := c.Repository.Settle(ctx, scope, subject.Object.DocumentID, from, subject.Revision, reason, operation)
	if err != nil {
		return current, ErrRetry
	}
	if outcome.State == domain.StorageFailed {
		return outcome, c.remove(ctx, scope, subject.Object)
	}
	return outcome, nil
}

// remove deletes the object of a FAILED document; a missing key converges.
func (c Checker) remove(ctx context.Context, scope kernel.OrgScope, object media.Object) error {
	if err := c.Objects.Delete(ctx, scope, object); err != nil {
		return ErrRetry
	}
	return nil
}
