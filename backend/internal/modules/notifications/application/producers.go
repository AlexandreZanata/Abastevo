package application

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

// ForumReplyFacts describes a reply that just became publicly visible on a
// forum topic, as the forum port reports it.
type ForumReplyFacts struct {
	TopicID                string
	TopicTitle             string
	TopicAuthorAccountID   string
	CommentID              string
	CommentAuthorAccountID string
	// AuthorName is the public forum name shown on the topic for the reply.
	AuthorName string
}

// ContributionFacts describes a terminal admissibility decision of a
// community observation, as the community port reports it.
type ContributionFacts struct {
	// ContributorRef is the observation's attribution token.
	ContributorRef string
	ObservationID  string
	StationID      string
	StationName    string
	Fuel           string
	Validated      bool
	Reason         string
}

// Producers turns producer facts into notices inside the producing
// transaction. Skips (no recipient account, actor is the recipient) return
// nil so the event itself still commits; every other failure is returned
// and rolls the event back with its notice.
type Producers struct {
	Notifier Notifier
	Clock    func() time.Time
	// AccountOfContributor resolves the account bound to an attribution
	// token inside tx; found=false when no account owns it.
	AccountOfContributor func(ctx context.Context, tx pgx.Tx, contributorRef string) (accountID string, found bool, err error)
}

var errNotConfigured = errors.New("notifications: producer not configured")

func (p Producers) now() time.Time {
	if p.Clock != nil {
		return p.Clock()
	}
	return time.Now()
}

// ForumReply notifies the topic author about a visible reply, never the
// replier about their own reply.
func (p Producers) ForumReply(ctx context.Context, tx pgx.Tx, f ForumReplyFacts) error {
	if f.TopicAuthorAccountID == "" {
		return nil
	}
	if p.Notifier == nil {
		return errNotConfigured
	}
	event, err := domain.NewForumReply(f.TopicAuthorAccountID, f.CommentAuthorAccountID, domain.ForumReply{
		TopicID: f.TopicID, TopicTitle: f.TopicTitle, CommentID: f.CommentID, AuthorName: f.AuthorName,
	}, p.now())
	if errors.Is(err, domain.ErrSelfNotification) {
		return nil
	}
	if err != nil {
		return err
	}
	return p.Notifier.Notify(ctx, tx, event)
}

// ContributionDecided notifies the account that owns the observation's
// contributor identity. Contributors without an account get nothing.
func (p Producers) ContributionDecided(ctx context.Context, tx pgx.Tx, f ContributionFacts) error {
	if p.Notifier == nil || p.AccountOfContributor == nil {
		return errNotConfigured
	}
	if f.ContributorRef == "" {
		return nil
	}
	account, found, err := p.AccountOfContributor(ctx, tx, f.ContributorRef)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	event, err := domain.NewContributionDecision(account, f.Validated, domain.Contribution{
		ObservationID: f.ObservationID, StationID: f.StationID, StationName: f.StationName,
		Fuel: f.Fuel, Reason: f.Reason,
	}, p.now())
	if err != nil {
		return err
	}
	return p.Notifier.Notify(ctx, tx, event)
}
