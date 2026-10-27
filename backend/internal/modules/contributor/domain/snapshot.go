package domain

import "time"

// Snapshot is the server-side GitHub contributors cache for one repository.
type Snapshot struct {
	Repo      string
	Logins    []string
	ETag      string
	FetchedAt time.Time
}

// Gist is the public gist ownership proof payload.
type Gist struct {
	Owner string
	Files []string
}

// GitHubLink is one active account ↔ GitHub login link.
type GitHubLink struct {
	AccountID string
	Login     string
}
