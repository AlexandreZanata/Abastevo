// Package domain owns the community safety rules (B-BR-SAFE-01…09):
// reporting and blocking the author of public user-generated content.
// A user is only ever named through a content reference; the server
// derives the author and never returns account identifiers. Standard
// library only: no I/O, no clock, no persistence.
package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ContentKind names a user-attributed public surface (B-BR-SAFE-01).
type ContentKind string

// The three content kinds that can name an author.
const (
	KindForumPost      ContentKind = "FORUM_POST"
	KindForumComment   ContentKind = "FORUM_COMMENT"
	KindStationComment ContentKind = "STATION_COMMENT"
)

// ReportReason is the closed user-report reason set (B-BR-SAFE-04).
type ReportReason string

// User report reasons.
const (
	ReasonSpam          ReportReason = "SPAM"
	ReasonHarassment    ReportReason = "HARASSMENT"
	ReasonHate          ReportReason = "HATE"
	ReasonImpersonation ReportReason = "IMPERSONATION"
	ReasonIllegal       ReportReason = "ILLEGAL"
	ReasonOther         ReportReason = "OTHER"
)

// Bounds frozen by B-BR-SAFE-04…07.
const (
	MaxDetailScalars = 200
	MaxLabelScalars  = 64
	MaxActiveBlocks  = 500
	BlockDailyQuota  = 50
	ReportDailyQuota = 10
	MaxHiddenRefs    = 100
	quotaWindowSecs  = 86400
)

var (
	// ErrInvalid marks malformed input (400 safety.bad-request).
	ErrInvalid = errors.New("safety: invalid input")
	// ErrNotFound marks content or blocks that cannot be named (404).
	ErrNotFound = errors.New("safety: not found")
	// ErrSelfTarget refuses naming the actor's own content (400).
	ErrSelfTarget = errors.New("safety: own content")
	// ErrBlockLimit refuses a block beyond the active cap (409).
	ErrBlockLimit = errors.New("safety: active block limit reached")
	// ErrRateLimited is matched by every QuotaError (429).
	ErrRateLimited = errors.New("safety: rate limited")
)

// QuotaError carries the retry delay of an exhausted rolling quota.
type QuotaError struct {
	RetryAfterSeconds int
}

func (e *QuotaError) Error() string { return ErrRateLimited.Error() }

// Is lets errors.Is(err, ErrRateLimited) match every quota refusal.
func (e *QuotaError) Is(target error) bool { return target == ErrRateLimited }

// ContentRef is one public content reference, with a canonical
// lower-case UUID.
type ContentRef struct {
	Kind ContentKind
	ID   string
}

// RawRef is an unvalidated wire reference.
type RawRef struct {
	Kind string
	ID   string
}

// Author is the server-derived author of a content reference. Label is
// the public alias the content shows (possibly empty); AccountID never
// leaves the server.
type Author struct {
	AccountID string
	Label     string
}

// ParseContentKind accepts exactly the three wire values.
func ParseContentKind(raw string) (ContentKind, error) {
	switch k := ContentKind(raw); k {
	case KindForumPost, KindForumComment, KindStationComment:
		return k, nil
	default:
		return "", ErrInvalid
	}
}

// ParseContentRef validates a kind and a UUID-shaped id.
func ParseContentRef(kind, id string) (ContentRef, error) {
	k, err := ParseContentKind(kind)
	if err != nil {
		return ContentRef{}, err
	}
	canonical, ok := canonicalUUID(id)
	if !ok {
		return ContentRef{}, ErrInvalid
	}
	return ContentRef{Kind: k, ID: canonical}, nil
}

// ParseID validates a UUID-shaped identifier and returns it lower-case.
func ParseID(id string) (string, error) {
	canonical, ok := canonicalUUID(id)
	if !ok {
		return "", ErrInvalid
	}
	return canonical, nil
}

// ParseReportReason accepts exactly the frozen reason codes.
func ParseReportReason(raw string) (ReportReason, error) {
	switch r := ReportReason(raw); r {
	case ReasonSpam, ReasonHarassment, ReasonHate, ReasonImpersonation, ReasonIllegal, ReasonOther:
		return r, nil
	default:
		return "", ErrInvalid
	}
}

// NormalizeDetail trims optional private report text and refuses
// malformed encoding, NUL, invisible format characters and more than
// MaxDetailScalars scalars. Blank input becomes "".
func NormalizeDetail(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", ErrInvalid
	}
	clean := strings.TrimSpace(raw)
	if utf8.RuneCountInString(clean) > MaxDetailScalars {
		return "", ErrInvalid
	}
	for _, r := range clean {
		if r == 0 || unicode.Is(unicode.Cf, r) {
			return "", ErrInvalid
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", ErrInvalid
		}
	}
	return clean, nil
}

// CheckTarget refuses authorless content as missing and the actor's
// own content as a self target (B-BR-SAFE-02).
func CheckTarget(actorAccountID string, a Author) error {
	if a.AccountID == "" {
		return ErrNotFound
	}
	if strings.EqualFold(a.AccountID, actorAccountID) {
		return ErrSelfTarget
	}
	return nil
}

// AdmitBlock applies the active block cap (B-BR-SAFE-05). The daily
// quota of new blocks (BlockDailyQuota) is a transactional window
// limiter in the adapter, because unblocking deletes rows and a row
// count would let block/unblock churn escape it. Replays of an existing
// block never reach either check.
func AdmitBlock(activeBlocks int) error {
	if activeBlocks >= MaxActiveBlocks {
		return ErrBlockLimit
	}
	return nil
}

// AdmitReport applies the rolling daily quota of new user reports
// (B-BR-SAFE-04). Replays of an existing report never reach it.
func AdmitReport(reportsLastDay int) error {
	if reportsLastDay >= ReportDailyQuota {
		return &QuotaError{RetryAfterSeconds: quotaWindowSecs}
	}
	return nil
}

// ParseHiddenQuery validates 1..MaxHiddenRefs references, all or none,
// and removes duplicates while keeping the first-seen order.
func ParseHiddenQuery(raw []RawRef) ([]ContentRef, error) {
	if len(raw) == 0 || len(raw) > MaxHiddenRefs {
		return nil, ErrInvalid
	}
	seen := make(map[ContentRef]bool, len(raw))
	out := make([]ContentRef, 0, len(raw))
	for _, r := range raw {
		ref, err := ParseContentRef(r.Kind, r.ID)
		if err != nil {
			return nil, err
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out, nil
}

// HiddenSubset returns the references whose resolved author is in the
// blocker's blocked set, in request order (B-BR-SAFE-06). Unknown
// references have no author and are never hidden.
func HiddenSubset(refs []ContentRef, authors map[ContentRef]string, blocked map[string]bool) []ContentRef {
	out := make([]ContentRef, 0)
	if len(blocked) == 0 {
		return out
	}
	for _, ref := range refs {
		if id, ok := authors[ref]; ok && blocked[strings.ToLower(id)] {
			out = append(out, ref)
		}
	}
	return out
}

// BlockLabel keeps the public alias as a single bounded line
// (B-BR-SAFE-07); control characters become spaces.
func BlockLabel(alias string) string {
	mapped := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, alias)
	clean := strings.Join(strings.Fields(mapped), " ")
	if utf8.RuneCountInString(clean) > MaxLabelScalars {
		clean = string([]rune(clean)[:MaxLabelScalars])
	}
	return clean
}

// CaseDetail is the fixed, PII-free moderation case detail: it names the
// content reference only, never reporter identity or free text
// (B-BR-SAFE-04).
func CaseDetail(ref ContentRef) string {
	return "community user report via " + string(ref.Kind) + " " + ref.ID
}

func canonicalUUID(s string) (string, bool) {
	if len(s) != 36 {
		return "", false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return "", false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return "", false
			}
		}
	}
	return strings.ToLower(s), true
}
