// Package domain holds the account notification model (B-BR-NOTIF-02,
// B-BR-NOTIF-03): the closed v1 kinds, their bounded public payloads, the
// event keys that make producers idempotent, the 30-day expiry and the
// list/mark limits. It is pure: no I/O, clock or randomness.
package domain

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// v1 kinds. The app skips kinds it does not know, so new kinds can ship
// server-first.
const (
	KindForumReply            = "forum.reply"
	KindContributionValidated = "contribution.validated"
	KindContributionRejected  = "contribution.rejected"
)

// Bounds from B-BR-NOTIF-02/03.
const (
	TTL              = 30 * 24 * time.Hour
	MaxTopicTitle    = 160
	MaxAuthorName    = 80
	MaxStationName   = 160
	MaxPayloadBytes  = 2048
	MaxEventKeyRunes = 200
	DefaultListLimit = 30
	MaxListLimit     = 50
	MaxMarkIDs       = 50
	// PurgeBatch bounds how many expired rows one write removes.
	PurgeBatch = 100
)

var (
	// ErrInvalid marks a malformed request or payload.
	ErrInvalid = errors.New("notifications: invalid input")
	// ErrSelfNotification marks an event whose actor is its recipient. A
	// producer never notifies the actor about their own action, so callers
	// skip it instead of failing the producing transaction.
	ErrSelfNotification = errors.New("notifications: actor is the recipient")
)

var (
	canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	wireProduct   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,31}$`)
	reasonCode    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

// Event is one notification ready to insert. The adapter assigns the row ID.
type Event struct {
	AccountID string
	Kind      string
	EventKey  string
	Payload   []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ForumReply is the forum.reply payload. AuthorName is the public forum
// name already shown on the topic for that reply; it may be empty.
type ForumReply struct {
	TopicID    string `json:"topic_id"`
	TopicTitle string `json:"topic_title"`
	CommentID  string `json:"comment_id"`
	AuthorName string `json:"author_name"`
}

// Contribution is the contribution.validated/rejected payload. Fuel is the
// existing wire product string; Reason is a stable code, rejected only.
type Contribution struct {
	ObservationID string `json:"observation_id"`
	StationID     string `json:"station_id"`
	StationName   string `json:"station_name"`
	Fuel          string `json:"fuel"`
	Reason        string `json:"reason,omitempty"`
}

// IsUUID reports a canonical lower-case UUID.
func IsUUID(s string) bool { return canonicalUUID.MatchString(s) }

// NewForumReply builds the notice for recipient (the topic author) about a
// reply by actor. Display text is cleaned and clipped to its bound so a
// long name never fails the producing transaction.
func NewForumReply(recipient, actor string, p ForumReply, now time.Time) (Event, error) {
	if !IsUUID(recipient) || !IsUUID(p.TopicID) || !IsUUID(p.CommentID) {
		return Event{}, ErrInvalid
	}
	if recipient == actor {
		return Event{}, ErrSelfNotification
	}
	p.TopicTitle = Clip(p.TopicTitle, MaxTopicTitle)
	p.AuthorName = Clip(p.AuthorName, MaxAuthorName)
	if p.TopicTitle == "" {
		return Event{}, ErrInvalid
	}
	return newEvent(recipient, KindForumReply, KindForumReply+":"+p.CommentID, p, now)
}

// NewContributionDecision builds the validated or rejected notice for the
// account that owns the observation. A rejection carries its stable reason.
func NewContributionDecision(recipient string, validated bool, p Contribution, now time.Time) (Event, error) {
	if !IsUUID(recipient) || !IsUUID(p.ObservationID) || !IsUUID(p.StationID) || !wireProduct.MatchString(p.Fuel) {
		return Event{}, ErrInvalid
	}
	p.StationName = Clip(p.StationName, MaxStationName)
	kind := KindContributionRejected
	if validated {
		kind = KindContributionValidated
		if p.Reason != "" {
			return Event{}, ErrInvalid
		}
	} else if !reasonCode.MatchString(p.Reason) {
		return Event{}, ErrInvalid
	}
	return newEvent(recipient, kind, kind+":"+p.ObservationID, p, now)
}

func newEvent(recipient, kind, key string, payload any, now time.Time) (Event, error) {
	if now.IsZero() || utf8.RuneCountInString(key) > MaxEventKeyRunes {
		return Event{}, ErrInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > MaxPayloadBytes {
		return Event{}, ErrInvalid
	}
	created := now.UTC().Truncate(time.Microsecond)
	return Event{
		AccountID: recipient, Kind: kind, EventKey: key, Payload: raw,
		CreatedAt: created, ExpiresAt: created.Add(TTL),
	}, nil
}

// Clip returns s as valid UTF-8 without control characters, with
// surrounding space trimmed and at most max runes.
func Clip(s string, max int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:max]))
}

// Limit resolves the requested page size: absent means the default, and
// anything outside 1..50 is invalid.
func Limit(requested *int) (int, error) {
	if requested == nil {
		return DefaultListLimit, nil
	}
	if *requested < 1 || *requested > MaxListLimit {
		return 0, ErrInvalid
	}
	return *requested, nil
}

// Selection is a validated mark-read request: either All, or 1..50
// distinct canonical notification IDs.
type Selection struct {
	All bool
	IDs []string
}

// NewSelection validates the mark-read target. Exactly one form is
// accepted; duplicates collapse.
func NewSelection(ids []string, all bool) (Selection, error) {
	if all {
		if len(ids) != 0 {
			return Selection{}, ErrInvalid
		}
		return Selection{All: true}, nil
	}
	if len(ids) < 1 || len(ids) > MaxMarkIDs {
		return Selection{}, ErrInvalid
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !IsUUID(id) {
			return Selection{}, ErrInvalid
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return Selection{IDs: out}, nil
}
