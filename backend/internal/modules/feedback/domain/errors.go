package domain

import "errors"

// Stable feedback verdict codes shared by fixtures, handlers and audit
// (P14-T01). Every refusal carries an explainable reason; display
// strings stay out of the domain.
var (
	ErrTextEmpty           = errors.New("feedback: comment text is empty")
	ErrTextTooLong         = errors.New("feedback: comment text exceeds 280 scalars")
	ErrTextInvalidEncoding = errors.New("feedback: comment text is not valid UTF-8")
	ErrRatingOutOfRange    = errors.New("feedback: rating outside 1-5")
	ErrAgreementNegative   = errors.New("feedback: negative vote count")
	ErrTargetInvalid       = errors.New("feedback: blank station or product")
	ErrGateRequired        = errors.New("feedback: account gate required")
	ErrStatsMissing        = errors.New("feedback: aggregate missing after write")
	ErrRatingNotFound      = errors.New("feedback: rating not found")
	ErrCommentNotFound     = errors.New("feedback: comment not found")
	ErrNotAuthor           = errors.New("feedback: only the author edits")
	ErrStaleRevision       = errors.New("feedback: stale revision")
	ErrParentInvalid       = errors.New("feedback: invalid reply target")
	ErrAuthorForbidden     = errors.New("feedback: author account not active")
	ErrSessionInvalid      = errors.New("feedback: valid account session required")
)

// VerdictOK is the shared success code.
const VerdictOK = "ok"

// VerdictCode maps a domain error to the stable fixture/audit code.
func VerdictCode(err error) string {
	switch {
	case err == nil:
		return VerdictOK
	case errors.Is(err, ErrTextEmpty):
		return "text-empty"
	case errors.Is(err, ErrTextTooLong):
		return "text-too-long"
	case errors.Is(err, ErrTextInvalidEncoding):
		return "text-invalid-encoding"
	case errors.Is(err, ErrRatingOutOfRange):
		return "rating-out-of-range"
	case errors.Is(err, ErrAgreementNegative):
		return "agreement-negative"
	case errors.Is(err, ErrTargetInvalid):
		return "target-invalid"
	case errors.Is(err, ErrGateRequired):
		return "gate-required"
	case errors.Is(err, ErrStatsMissing):
		return "stats-missing"
	case errors.Is(err, ErrRatingNotFound):
		return "rating-not-found"
	case errors.Is(err, ErrCommentNotFound):
		return "comment-not-found"
	case errors.Is(err, ErrNotAuthor):
		return "not-author"
	case errors.Is(err, ErrStaleRevision):
		return "stale-revision"
	case errors.Is(err, ErrParentInvalid):
		return "parent-invalid"
	case errors.Is(err, ErrAuthorForbidden):
		return "author-forbidden"
	case errors.Is(err, ErrSessionInvalid):
		return "session-invalid"
	default:
		return "invalid-value"
	}
}
