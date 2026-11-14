package jobs

import (
	"context"

	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
)

// ForumState is the editorial forum change detector kept in the durable
// export state (B-BR-SEO-08): a publish, edit, hide or source change moves
// it, so the no-op check rebuilds and the export content hash changes.
type ForumState struct {
	Posts       int64  `json:"posts"`
	Fingerprint string `json:"fingerprint"`
}

func forumState(fingerprint forumapp.EditorialExportFingerprint) *ForumState {
	return &ForumState{Posts: fingerprint.Posts, Fingerprint: fingerprint.Hash}
}

// currentForum reads the forum fingerprint when a forum reader is wired.
func currentForum(ctx context.Context, reader forumapp.EditorialExportReader) (*ForumState, error) {
	if reader == nil {
		return nil, nil
	}
	fingerprint, err := reader.EditorialFingerprint(ctx)
	if err != nil {
		return nil, err
	}
	return forumState(fingerprint), nil
}

func sameForum(left, right *ForumState) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
