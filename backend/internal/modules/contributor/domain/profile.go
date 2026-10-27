package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalid          = errors.New("contributor: invalid input")
	ErrNotFound         = errors.New("contributor: not-found")
	ErrConflict         = errors.New("contributor: conflict")
	ErrQuota            = errors.New("contributor: quota")
	ErrOverrideRequired = errors.New("contributor: override-required")
	ErrForbidden        = errors.New("contributor: forbidden")
	ErrUnavailable      = errors.New("contributor: unavailable")
	ErrClaimExpired     = errors.New("contributor: claim-expired")
	ErrClaimState       = errors.New("contributor: claim-state")
)

const (
	VisibilityPublic = "PUBLIC"
	VisibilityHidden = "HIDDEN"
	MaxLinks         = 2
	MaxDisplayRunes  = 40
	MinDisplayRunes  = 2
)

// Link is the client-provided platform + handle pair. URLs are never accepted.
type Link struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
}

type PublicLink struct {
	Platform string `json:"platform"`
	Handle   string `json:"handle"`
	URL      string `json:"url"`
}

type Profile struct {
	AccountID         string
	DisplayName       string
	Links             []Link
	Visibility        string
	PublicConsent     bool
	ShowRolesPublicly bool
	HiddenByAdmin     bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PublicContributor struct {
	AccountID        string
	DisplayName      string
	Links            []PublicLink
	GitHubLogin      string
	ContributorSince time.Time
	Roles            []string
}

type platformRule struct {
	pattern *regexp.Regexp
	url     func(string) string
}

var platforms = map[string]platformRule{
	"github":    {regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`), func(h string) string { return "https://github.com/" + h }},
	"gitlab":    {regexp.MustCompile(`^[A-Za-z0-9_.-]{2,255}$`), func(h string) string { return "https://gitlab.com/" + h }},
	"linkedin":  {regexp.MustCompile(`^[A-Za-z0-9-]{3,100}$`), func(h string) string { return "https://www.linkedin.com/in/" + h }},
	"x":         {regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`), func(h string) string { return "https://x.com/" + h }},
	"instagram": {regexp.MustCompile(`^[A-Za-z0-9_.]{1,30}$`), func(h string) string { return "https://www.instagram.com/" + h }},
	"youtube":   {regexp.MustCompile(`^[A-Za-z0-9_.-]{3,30}$`), func(h string) string { return "https://www.youtube.com/@" + h }},
	"bluesky":   {regexp.MustCompile(`^[a-z0-9.-]{3,253}$`), func(h string) string { return "https://bsky.app/profile/" + h }},
}

// NormalizeDisplayName applies NFC and trimming, then validates the rune set.
func NormalizeDisplayName(raw string) (string, bool) {
	name := strings.TrimSpace(norm.NFC.String(raw))
	runes := []rune(name)
	if len(runes) < MinDisplayRunes || len(runes) > MaxDisplayRunes {
		return "", false
	}
	for _, r := range runes {
		if !(unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r) ||
			r == ' ' || r == '.' || r == '\'' || r == '-') {
			return "", false
		}
	}
	return name, true
}

// ValidateLinks enforces the allowlist, handle rules, one per platform and
// the two-link maximum. Handles are returned unchanged (no slicing).
func ValidateLinks(links []Link) ([]Link, bool) {
	if len(links) > MaxLinks {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]Link, 0, len(links))
	for _, link := range links {
		rule, ok := platforms[link.Platform]
		if !ok || seen[link.Platform] || !rule.pattern.MatchString(link.Handle) {
			return nil, false
		}
		if link.Platform == "bluesky" && !strings.Contains(link.Handle, ".") {
			return nil, false
		}
		if link.Platform == "github" && !ValidGitHubLogin(link.Handle) {
			return nil, false
		}
		seen[link.Platform] = true
		out = append(out, link)
	}
	return out, true
}

func PublicLinks(links []Link) []PublicLink {
	out := make([]PublicLink, 0, len(links))
	for _, link := range links {
		rule, ok := platforms[link.Platform]
		if !ok {
			continue
		}
		out = append(out, PublicLink{Platform: link.Platform, Handle: link.Handle, URL: rule.url(link.Handle)})
	}
	return out
}

func ValidVisibility(raw string) bool {
	return raw == VisibilityPublic || raw == VisibilityHidden
}
