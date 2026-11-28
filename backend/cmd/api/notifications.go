package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	forumapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	notificationsadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters"
	notificationshttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/adapters/http"
	notificationsapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/application"
)

// forumReplyNotices turns a reply's first publication into a notice for the
// topic author inside the publishing transaction (B-BR-NOTIF-02).
func forumReplyNotices(notifier notificationsapp.Notifier) forumapp.ReplyNotifier {
	producers := notificationsapp.Producers{Notifier: notifier, Clock: time.Now}
	return func(ctx context.Context, tx pgx.Tx, n forumapp.ReplyNotice) error {
		return producers.ForumReply(ctx, tx, notificationsapp.ForumReplyFacts{
			TopicID: n.PostID, TopicTitle: n.PostTitle, TopicAuthorAccountID: n.PostAuthorAccountID,
			CommentID: n.CommentID, CommentAuthorAccountID: n.CommentAuthorAccountID, AuthorName: n.AuthorName,
		})
	}
}

// notificationRoutes mounts the owner inbox API (B-BR-NOTIF-03): the list
// is a private session read and marking read is a signed write bound to
// the account; the handler collapses every session or proof failure into
// one generic authentication error.
func notificationRoutes(router chi.Router, store *notificationsadapters.Store, sessions func(context.Context, string, string) (string, error), signedOwner func(*http.Request, string) (string, string, error), cursorSecret []byte, origin string) {
	notificationshttp.Handler{
		Service:      notificationsapp.Service{Inbox: store, Clock: time.Now},
		Sessions:     sessions,
		SignedOwner:  signedOwner,
		CursorSecret: cursorSecret,
		Origin:       origin,
	}.RegisterRoutes(router)
}
