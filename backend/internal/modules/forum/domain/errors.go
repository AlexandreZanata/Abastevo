package domain

import "errors"

// Sentinel errors for forum domain rules.
var (
	ErrInvalidUTF8             = errors.New("forum: invalid utf-8 encoding")
	ErrTitleEmpty              = errors.New("forum: title cannot be empty")
	ErrTitleTooLong            = errors.New("forum: title exceeds 120 unicode characters")
	ErrExcerptTooLong          = errors.New("forum: excerpt exceeds 400 unicode characters")
	ErrBodyTooLong             = errors.New("forum: body exceeds 4000 unicode characters")
	ErrInvalidCategory         = errors.New("forum: invalid category")
	ErrInvalidPostKind         = errors.New("forum: invalid post kind")
	ErrInvalidPublicationState = errors.New("forum: invalid publication state")
	ErrInvalidTransition       = errors.New("forum: invalid publication state transition")
	ErrRemovedIsTerminal       = errors.New("forum: removed post is in terminal state")
	ErrNotPubliclyVisible      = errors.New("forum: post is not publicly visible")
	ErrMissingSource           = errors.New("forum: missing source for non-topic post")
	ErrMissingSourceDate       = errors.New("forum: news post requires source publication date")
	ErrMissingRights           = errors.New("forum: unapproved rights status for publication")
	ErrMissingAuthor           = errors.New("forum: topic requires author account id")
	ErrTopicCannotHaveSource   = errors.New("forum: topic post cannot have external source id")
	ErrInvalidURL              = errors.New("forum: invalid or non-https canonical url")
	ErrInvalidRegion           = errors.New("forum: invalid geographic region or ibge code")
	ErrRevisionMismatch        = errors.New("forum: revision mismatch")
	ErrForumDisabled           = errors.New("forum: forum subsystem is disabled")
	ErrPostNotFound            = errors.New("forum: post not found")
	ErrInvalidCursor           = errors.New("forum: invalid cursor")
	ErrFeedOversized           = errors.New("forum: feed exceeds maximum byte size")
	ErrFeedMalformed           = errors.New("forum: feed is malformed or invalid xml")
	ErrFeedSecurityRefusal     = errors.New("forum: feed contains forbidden entity declaration or excessive depth")
	ErrFeedItemOversized       = errors.New("forum: feed item metadata exceeds size limit")
	ErrEgressForbidden         = errors.New("forum: target host or address violates egress policy")
	ErrRedirectDisallowed      = errors.New("forum: redirect target rejected")
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
	default:
		return "unknown-error"
	}
}
