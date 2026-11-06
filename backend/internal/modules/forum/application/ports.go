package application

import (
	"context"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/jackc/pgx/v5"
)

// Clock abstracts system time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock implements Clock using time.Now().UTC().
type SystemClock struct{}

// Now returns current UTC time.
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

// IDGenerator abstracts UUID generation.
type IDGenerator interface {
	NewID() string
}

// ListPublicPostsQuery represents input parameters for feed queries.
type ListPublicPostsQuery struct {
	Category *domain.Category
	Kind     *domain.PostKind
	Limit    int
	Cursor   *string
}

// CategoriesResult represents categories with current taxonomy version.
type CategoriesResult struct {
	Items           []domain.CategoryItem
	TaxonomyVersion int
}

// CategoryReader defines read access to forum categories.
type CategoryReader interface {
	ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error)
}

// PostReader defines read access to forum posts and feed snapshots.
type PostReader interface {
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
	ListPublicPosts(ctx context.Context, query ListPublicPostsQuery) (domain.FeedPage, error)
	GetFeedSnapshot(ctx context.Context) (string, error)
}

// FeedItem represents a candidate publication parsed from an external feed.
type FeedItem struct {
	GUID        string
	Title       string
	Link        string
	Excerpt     string
	PublishedAt time.Time
	Categories  []string
}

// FetchRequest represents bounded parameters for a conditional feed fetch.
type FetchRequest struct {
	SourceID     string
	URL          string
	ETag         string
	LastModified string
}

// FetchResponse represents the result of a conditional feed fetch.
type FetchResponse struct {
	NotModified  bool
	StatusCode   int
	ETag         string
	LastModified string
	Body         []byte
}

// SourceFetcher abstracts safe network retrieval of external candidate feeds.
type SourceFetcher interface {
	Fetch(ctx context.Context, req FetchRequest) (FetchResponse, error)
}

// FeedParser abstracts decoding and sanitization of raw RSS/Atom feeds.
type FeedParser interface {
	Parse(raw []byte, now time.Time) ([]FeedItem, error)
}

// SourceCheckpoint represents conditional GET state and execution status for a source.
type SourceCheckpoint struct {
	SourceID            string
	ETag                string
	LastModified        string
	LastRunID           *string
	LastSuccessAt       *time.Time
	LastStatus          string
	ConsecutiveFailures int
	UpdatedAt           time.Time
}

// SourceRun represents an audit record of a candidate ingestion execution.
type SourceRun struct {
	ID              string
	SourceID        string
	Status          string // 'RUNNING', 'SUCCESS', 'NOT_MODIFIED', 'FAILED', 'BLOCKED'
	ETag            string
	LastModified    string
	ItemsDiscovered int
	ItemsIngested   int
	ErrorMessage    string
	StartedAt       time.Time
	CompletedAt     *time.Time
}

// IngestionStore defines persistence operations for candidate feed collection.
type IngestionStore interface {
	GetSourceByID(ctx context.Context, id string) (*domain.Source, error)
	ListEnabledSources(ctx context.Context) ([]domain.Source, error)
	GetSourceCheckpoint(ctx context.Context, sourceID string) (SourceCheckpoint, error)
	UpsertSourceCheckpoint(ctx context.Context, cp SourceCheckpoint) error
	RecordSourceRun(ctx context.Context, run SourceRun) error
	HasPostByExternalID(ctx context.Context, sourceID, externalID string) (bool, error)
	HasPostByUrlHash(ctx context.Context, sourceID, urlHash string) (bool, error)
	InsertNewsCandidatePost(ctx context.Context, post *domain.Post) error
}

// Quota policy constants for forum user topics per B-BR-FOR-11.
const (
	OperationForumTopicHourly = "forum:topic:hour"
	OperationForumTopicDaily  = "forum:topic:day"
	MaxLiveTopicsPerAccount   = 20
)

// CreateTopicCommand represents validated parameters to create an authored topic.
type CreateTopicCommand struct {
	AuthorAccountID string
	Category        domain.Category
	Title           string
	BodyText        string
	PublicAlias     *string
	Region          *domain.Region
	FuelProduct     *string
	StationID       *string
	OperationID     string
	Guard           func(context.Context, pgx.Tx) error
}

// EditTopicCommand represents validated parameters to CAS-edit an existing topic.
type EditTopicCommand struct {
	TopicID          string
	AuthorAccountID  string
	ExpectedRevision int
	Category         domain.Category
	Title            string
	BodyText         string
	Region           *domain.Region
	FuelProduct      *string
	StationID        *string
	Guard            func(context.Context, pgx.Tx) error
}

// RemoveTopicCommand represents parameters to tombstone an existing topic.
type RemoveTopicCommand struct {
	TopicID          string
	AuthorAccountID  string
	ExpectedRevision int
	Guard            func(context.Context, pgx.Tx) error
}

// TopicResult wraps a post with idempotent replay metadata.
type TopicResult struct {
	Topic    *domain.Post
	Replayed bool
}

// TopicStore defines persistence operations for user topics.
type TopicStore interface {
	CreateTopic(ctx context.Context, cmd CreateTopicCommand) (TopicResult, error)
	EditTopic(ctx context.Context, cmd EditTopicCommand) (*domain.Post, error)
	RemoveTopic(ctx context.Context, cmd RemoveTopicCommand) (*domain.Post, error)
	GetTopicByID(ctx context.Context, id string) (*domain.Post, error)
	ListTopicsByAccount(ctx context.Context, accountID string) ([]domain.Post, error)
}

// Quota policy constants for forum discussion comments per B-BR-FOR-11.
const (
	OperationForumCommentHourly = "forum:comment:hour"
	OperationForumCommentDaily  = "forum:comment:day"
	MaxLiveCommentsPerAccount   = 100
)

// CreateCommentCommand represents validated parameters to create a comment or reply.
type CreateCommentCommand struct {
	PostID          string
	AuthorAccountID string
	PublicAlias     string
	ParentID        *string
	BodyText        string
	OperationID     string
	Guard           func(context.Context, pgx.Tx) error
}

// EditCommentCommand represents validated parameters to CAS-edit an existing comment.
type EditCommentCommand struct {
	CommentID        string
	AuthorAccountID  string
	ExpectedRevision int
	BodyText         string
	Guard            func(context.Context, pgx.Tx) error
}

// RemoveCommentCommand represents parameters to tombstone an existing comment.
type RemoveCommentCommand struct {
	CommentID        string
	AuthorAccountID  string
	ExpectedRevision int
	Guard            func(context.Context, pgx.Tx) error
}

// ListPostCommentsQuery represents pagination parameters for top-level post comments.
type ListPostCommentsQuery struct {
	PostID         string
	Limit          int
	CursorSequence *int64
}

// ListCommentRepliesQuery represents pagination parameters for single-level replies.
type ListCommentRepliesQuery struct {
	ParentID       string
	Limit          int
	CursorSequence *int64
}

// CommentResult wraps a comment with idempotent replay metadata.
type CommentResult struct {
	Comment  *domain.Comment
	Replayed bool
}

// CommentStore defines persistence operations for reviewed discussion threads.
type CommentStore interface {
	CreateComment(ctx context.Context, cmd CreateCommentCommand) (CommentResult, error)
	EditComment(ctx context.Context, cmd EditCommentCommand) (*domain.Comment, error)
	RemoveComment(ctx context.Context, cmd RemoveCommentCommand) (*domain.Comment, error)
	GetCommentByID(ctx context.Context, id string) (*domain.Comment, error)
	ListCommentsByPost(ctx context.Context, query ListPostCommentsQuery) ([]domain.Comment, error)
	ListRepliesByParent(ctx context.Context, query ListCommentRepliesQuery) ([]domain.Comment, error)
	PublishComment(ctx context.Context, commentID string) (*domain.Comment, error)
	RejectComment(ctx context.Context, commentID string) (*domain.Comment, error)
	ListCommentsByAccount(ctx context.Context, accountID string) ([]domain.Comment, error)
	EraseAccountBatch(ctx context.Context, accountID string, limit int) (done bool, erasedCount int, err error)
}

// ReportCommand represents parameters to report a forum post or comment.
type ReportCommand struct {
	ReporterAccountID string
	TargetType        string // "FORUM_POST" or "FORUM_COMMENT"
	TargetID          string
	Reason            string
	Detail            string
	Guard             func(context.Context, pgx.Tx) error
}

// ReportStore defines persistence operations for forum reports and moderation triage.
type ReportStore interface {
	Report(ctx context.Context, cmd ReportCommand) (*domain.ReportReceipt, error)
}

// ModerationCaseOpener opens or converges a moderation case in an authorized transaction.
type ModerationCaseOpener func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (caseID string, replayed bool, err error)
