package domain

import "errors"

// Sentinel errors for forum domain rules.
var (
	ErrInvalidUTF8              = errors.New("forum: invalid utf-8 encoding")
	ErrTitleEmpty               = errors.New("forum: title cannot be empty")
	ErrTitleTooLong             = errors.New("forum: title exceeds 120 unicode characters")
	ErrExcerptTooLong           = errors.New("forum: excerpt exceeds 400 unicode characters")
	ErrBodyTooLong              = errors.New("forum: body exceeds 4000 unicode characters")
	ErrInvalidCategory          = errors.New("forum: invalid category")
	ErrInvalidPostKind          = errors.New("forum: invalid post kind")
	ErrInvalidPublicationState  = errors.New("forum: invalid publication state")
	ErrInvalidTransition        = errors.New("forum: invalid publication state transition")
	ErrRemovedIsTerminal        = errors.New("forum: removed post is in terminal state")
	ErrNotPubliclyVisible       = errors.New("forum: post is not publicly visible")
	ErrMissingSource            = errors.New("forum: missing source for non-topic post")
	ErrMissingSourceDate        = errors.New("forum: news post requires source publication date")
	ErrMissingRights            = errors.New("forum: unapproved rights status for publication")
	ErrMissingAuthor            = errors.New("forum: topic requires author account id")
	ErrTopicCannotHaveSource    = errors.New("forum: topic post cannot have external source id")
	ErrInvalidURL               = errors.New("forum: invalid or non-https canonical url")
	ErrInvalidRegion            = errors.New("forum: invalid geographic region or ibge code")
	ErrRevisionMismatch         = errors.New("forum: revision mismatch")
	ErrForumDisabled            = errors.New("forum: forum subsystem is disabled")
	ErrPostNotFound             = errors.New("forum: post not found")
	ErrInvalidCursor            = errors.New("forum: invalid cursor")
	ErrFeedOversized            = errors.New("forum: feed exceeds maximum byte size")
	ErrFeedMalformed            = errors.New("forum: feed is malformed or invalid xml")
	ErrFeedSecurityRefusal      = errors.New("forum: feed contains forbidden entity declaration or excessive depth")
	ErrFeedItemOversized        = errors.New("forum: feed item metadata exceeds size limit")
	ErrEgressForbidden          = errors.New("forum: target host or address violates egress policy")
	ErrRedirectDisallowed       = errors.New("forum: redirect target rejected")
	ErrCommentBodyEmpty         = errors.New("forum: comment body cannot be empty")
	ErrCommentBodyTooLong       = errors.New("forum: comment body exceeds 280 unicode characters")
	ErrInvalidCommentDepth      = errors.New("forum: comment depth exceeds maximum allowed (max depth 1)")
	ErrCrossPostParent          = errors.New("forum: parent comment belongs to a different post")
	ErrParentCommentNotFound    = errors.New("forum: parent comment not found")
	ErrParentCommentNotVisible  = errors.New("forum: parent comment is not publicly visible")
	ErrPostCommentsDisabled     = errors.New("forum: comments are disabled for this post")
	ErrPostNotCommentable       = errors.New("forum: post is not in published/visible state for comments")
	ErrConflictingOperation     = errors.New("forum: conflicting operation with same id but different content")
	ErrCommentLiveQuotaExceeded = errors.New("forum: account live comment budget exceeded")
	ErrCommentNotFound          = errors.New("forum: comment not found")
	ErrCommentAuthorMismatch    = errors.New("forum: comment author mismatch")
	ErrCommentAlreadyRemoved    = errors.New("forum: comment is already removed")
	ErrTargetInvalid            = errors.New("forum: invalid account id or target")
	ErrReportInvalid            = errors.New("forum: invalid report")
	ErrReportQuotaExceeded      = errors.New("forum: report quota exceeded")
	ErrSelfReportForbidden      = errors.New("forum: cannot report own content")
	ErrReportTargetMissing      = errors.New("forum: reported target not found")
	ErrTopicTitleEmpty          = errors.New("forum: topic title cannot be empty")
	ErrTopicTitleTooLong        = errors.New("forum: topic title exceeds 120 unicode characters")
	ErrTopicBodyEmpty           = errors.New("forum: topic body cannot be empty")
	ErrTopicBodyTooLong         = errors.New("forum: topic body exceeds 4000 unicode characters")
	ErrTopicHourlyQuotaExceeded = errors.New("forum: topic hourly quota exceeded")
	ErrTopicDailyQuotaExceeded  = errors.New("forum: topic daily quota exceeded")
	ErrTopicLiveQuotaExceeded   = errors.New("forum: account live topic budget exceeded")
	ErrTopicNotFound            = errors.New("forum: topic not found")
	ErrTopicAuthorMismatch      = errors.New("forum: topic author mismatch")
	ErrTopicAlreadyRemoved      = errors.New("forum: topic is already removed")
	ErrTopicKindSpoof           = errors.New("forum: invalid topic kind or external source on topic")
	ErrInvalidFuelProduct       = errors.New("forum: invalid fuel product")
	ErrInvalidStationID         = errors.New("forum: invalid station id")
)

const (
	VerdictOK = "ok"
)

// VerdictCode maps domain errors to stable machine-readable codes.
func VerdictCode(err error) string {
	if err == nil {
		return VerdictOK
	}
	switch {
	case errors.Is(err, ErrInvalidUTF8):
		return "invalid-utf8"
	case errors.Is(err, ErrTitleEmpty):
		return "title-empty"
	case errors.Is(err, ErrTitleTooLong):
		return "title-too-long"
	case errors.Is(err, ErrExcerptTooLong):
		return "excerpt-too-long"
	case errors.Is(err, ErrBodyTooLong):
		return "body-too-long"
	case errors.Is(err, ErrInvalidCategory):
		return "invalid-category"
	case errors.Is(err, ErrInvalidPostKind):
		return "invalid-post-kind"
	case errors.Is(err, ErrInvalidPublicationState):
		return "invalid-publication-state"
	case errors.Is(err, ErrInvalidTransition):
		return "invalid-transition"
	case errors.Is(err, ErrRemovedIsTerminal):
		return "removed-is-terminal"
	case errors.Is(err, ErrNotPubliclyVisible):
		return "not-publicly-visible"
	case errors.Is(err, ErrMissingSource):
		return "missing-source"
	case errors.Is(err, ErrMissingSourceDate):
		return "missing-source-date"
	case errors.Is(err, ErrMissingRights):
		return "missing-rights"
	case errors.Is(err, ErrMissingAuthor):
		return "missing-author"
	case errors.Is(err, ErrTopicCannotHaveSource):
		return "topic-cannot-have-source"
	case errors.Is(err, ErrInvalidURL):
		return "invalid-url"
	case errors.Is(err, ErrInvalidRegion):
		return "invalid-region"
	case errors.Is(err, ErrRevisionMismatch):
		return "revision-mismatch"
	case errors.Is(err, ErrForumDisabled):
		return "forum-disabled"
	case errors.Is(err, ErrPostNotFound):
		return "post-not-found"
	case errors.Is(err, ErrInvalidCursor):
		return "invalid-cursor"
	case errors.Is(err, ErrFeedOversized):
		return "feed-oversized"
	case errors.Is(err, ErrFeedMalformed):
		return "feed-malformed"
	case errors.Is(err, ErrFeedSecurityRefusal):
		return "feed-security-refusal"
	case errors.Is(err, ErrFeedItemOversized):
		return "feed-item-oversized"
	case errors.Is(err, ErrEgressForbidden):
		return "egress-forbidden"
	case errors.Is(err, ErrRedirectDisallowed):
		return "redirect-disallowed"
	case errors.Is(err, ErrCommentBodyEmpty):
		return "comment-body-empty"
	case errors.Is(err, ErrCommentBodyTooLong):
		return "comment-body-too-long"
	case errors.Is(err, ErrInvalidCommentDepth):
		return "invalid-comment-depth"
	case errors.Is(err, ErrCrossPostParent):
		return "cross-post-parent"
	case errors.Is(err, ErrParentCommentNotFound):
		return "parent-comment-not-found"
	case errors.Is(err, ErrParentCommentNotVisible):
		return "parent-comment-not-visible"
	case errors.Is(err, ErrPostCommentsDisabled):
		return "post-comments-disabled"
	case errors.Is(err, ErrPostNotCommentable):
		return "post-not-commentable"
	case errors.Is(err, ErrConflictingOperation):
		return "conflicting-operation"
	case errors.Is(err, ErrCommentLiveQuotaExceeded):
		return "comment-live-quota-exceeded"
	case errors.Is(err, ErrCommentNotFound):
		return "comment-not-found"
	case errors.Is(err, ErrCommentAuthorMismatch):
		return "comment-author-mismatch"
	case errors.Is(err, ErrCommentAlreadyRemoved):
		return "comment-already-removed"
	case errors.Is(err, ErrReportInvalid):
		return "report-invalid"
	case errors.Is(err, ErrReportQuotaExceeded):
		return "report-quota-exceeded"
	case errors.Is(err, ErrSelfReportForbidden):
		return "self-report-forbidden"
	case errors.Is(err, ErrReportTargetMissing):
		return "report-target-missing"
	case errors.Is(err, ErrTopicTitleEmpty):
		return "topic-title-empty"
	case errors.Is(err, ErrTopicTitleTooLong):
		return "topic-title-too-long"
	case errors.Is(err, ErrTopicBodyEmpty):
		return "topic-body-empty"
	case errors.Is(err, ErrTopicBodyTooLong):
		return "topic-body-too-long"
	case errors.Is(err, ErrTopicHourlyQuotaExceeded):
		return "topic-hourly-quota-exceeded"
	case errors.Is(err, ErrTopicDailyQuotaExceeded):
		return "topic-daily-quota-exceeded"
	case errors.Is(err, ErrTopicLiveQuotaExceeded):
		return "topic-live-quota-exceeded"
	case errors.Is(err, ErrTopicNotFound):
		return "topic-not-found"
	case errors.Is(err, ErrTopicAuthorMismatch):
		return "topic-author-mismatch"
	case errors.Is(err, ErrTopicAlreadyRemoved):
		return "topic-already-removed"
	case errors.Is(err, ErrTopicKindSpoof):
		return "topic-kind-spoof"
	case errors.Is(err, ErrInvalidFuelProduct):
		return "invalid-fuel-product"
	case errors.Is(err, ErrInvalidStationID):
		return "invalid-station-id"
	default:
		return "unknown-error"
	}
}
