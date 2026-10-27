package domain

import "strings"

// ValidGitHubLogin implements GitHub's login rules: 1–39 characters,
// alphanumeric or hyphen, no leading/trailing hyphen and no double hyphen.
func ValidGitHubLogin(raw string) bool {
	if len(raw) < 1 || len(raw) > 39 {
		return false
	}
	if raw[0] == '-' || raw[len(raw)-1] == '-' || strings.Contains(raw, "--") {
		return false
	}
	for _, c := range raw {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// NormalizeGitHubLogin returns the case-insensitive comparison form.
func NormalizeGitHubLogin(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }
