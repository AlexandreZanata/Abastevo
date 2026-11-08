package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	forumsql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func topicQuotaSubjectHourly(accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("forum:quota:topic:hour:%s", accountID)))
	return hex.EncodeToString(sum[:])
}

func topicQuotaSubjectDaily(accountID string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("forum:quota:topic:day:%s", accountID)))
	return hex.EncodeToString(sum[:])
}

func mapTopicRowToDomain(r forumsql.ForumPost) *domain.Post {
	var publicAlias *string
	if r.PublicAlias.Valid {
		cp := r.PublicAlias.String
		publicAlias = &cp
	}

	var correction *string
	if r.Correction.Valid {
		cp := r.Correction.String
		correction = &cp
	}

	var fuelProduct *string
	if r.FuelProduct.Valid {
		cp := r.FuelProduct.String
		fuelProduct = &cp
	}

	var stationID *string
	if r.StationID.Valid {
		cp := uuidString(r.StationID)
		stationID = &cp
	}

	var region *domain.Region
	if r.RegionUf.Valid {
		var ibge *int
		if r.RegionMunicipalityIbge.Valid {
			val := int(r.RegionMunicipalityIbge.Int32)
			ibge = &val
		}
		region = &domain.Region{
			UF:               r.RegionUf.String,
			MunicipalityIBGE: ibge,
		}
	}

	var firstPubAt *time.Time
	if r.FirstPublishedAt.Valid {
		t := r.FirstPublishedAt.Time.UTC()
		firstPubAt = &t
	}

	return &domain.Post{
		ID:                  uuidString(r.ID),
		Kind:                domain.PostKind(r.Kind),
		Category:            domain.Category(r.Category),
		Title:               r.Title,
		Excerpt:             r.Excerpt,
		BodyText:            r.BodyText,
		AuthorAccountID:     uuidString(r.AuthorAccountID),
		PublicAlias:         publicAlias,
		FirstPublishedAt:    firstPubAt,
		LocalCreatedAt:      r.LocalCreatedAt.Time.UTC(),
		LocalUpdatedAt:      r.LocalUpdatedAt.Time.UTC(),
		FirstPublicationSeq: r.FirstPublicationSeq,
		PublicationState:    domain.PublicationState(r.PublicationState),
		Visibility:          domain.Visibility(r.Visibility),
		Revision:            int(r.Revision),
		CommentsEnabled:     r.CommentsEnabled,
		Correction:          correction,
		Region:              region,
		FuelProduct:         fuelProduct,
		StationID:           stationID,
	}
}

// CreateTopic atomically enforces rate quotas, live topic budget, verifies idempotency,
// inserts a new PENDING_REVIEW user topic into forum_posts, and logs an audited change.
func (s *Store) CreateTopic(ctx context.Context, cmd application.CreateTopicCommand) (application.TopicResult, error) {
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return application.TopicResult{}, domain.ErrMissingAuthor
	}
	opUUID, err := mustUUID(cmd.OperationID)
	if err != nil {
		return application.TopicResult{}, fmt.Errorf("forum: invalid operation id: %w", err)
	}

	bodySHA := sha256Hex(cmd.Title + "\n\n" + cmd.BodyText)

	// Idempotency check: see if (author_account_id, author_operation_id) already exists
	existing, err := s.queries.GetTopicByOperationID(ctx, forumsql.GetTopicByOperationIDParams{
		AuthorAccountID:   authorUUID,
		AuthorOperationID: opUUID,
	})
	if err == nil {
		if existing.BodySha256 == bodySHA {
			return application.TopicResult{
				Topic:    mapTopicRowToDomain(existing),
				Replayed: true,
			}, nil
		}
		return application.TopicResult{}, domain.ErrConflictingOperation
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return application.TopicResult{}, fmt.Errorf("forum: check existing topic: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return application.TopicResult{}, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if cmd.Guard != nil {
		if err := cmd.Guard(ctx, tx); err != nil {
			return application.TopicResult{}, err
		}
	}

	qtx := s.queries.WithTx(tx)

	// 1. Quota check (if limiter configured)
	if err := checkQuota(ctx, tx, s.quotas.TopicHourly, topicQuotaSubjectHourly(cmd.AuthorAccountID)); err != nil {
		return application.TopicResult{}, err
	}
	if err := checkQuota(ctx, tx, s.quotas.TopicDaily, topicQuotaSubjectDaily(cmd.AuthorAccountID)); err != nil {
		return application.TopicResult{}, err
	}

	// 2. Check live topic count for author across forum (max 20 live)
	liveCount, err := qtx.CountLiveTopicsByAuthor(ctx, authorUUID)
	if err != nil {
		return application.TopicResult{}, fmt.Errorf("forum: count live topics: %w", err)
	}
	if liveCount >= application.MaxLiveTopicsPerAccount {
		return application.TopicResult{}, domain.ErrTopicLiveQuotaExceeded
	}

	// 3. Optional fields mapping
	var publicAlias pgtype.Text
	if cmd.PublicAlias != nil && *cmd.PublicAlias != "" {
		publicAlias = pgtype.Text{String: *cmd.PublicAlias, Valid: true}
	}

	var regionUf pgtype.Text
	var regionIBGE pgtype.Int4
	if cmd.Region != nil {
		if cmd.Region.UF != "" {
			regionUf = pgtype.Text{String: cmd.Region.UF, Valid: true}
		}
		if cmd.Region.MunicipalityIBGE != nil {
			regionIBGE = pgtype.Int4{Int32: int32(*cmd.Region.MunicipalityIBGE), Valid: true}
		}
	}

	var fuelProduct pgtype.Text
	if cmd.FuelProduct != nil && *cmd.FuelProduct != "" {
		fuelProduct = pgtype.Text{String: *cmd.FuelProduct, Valid: true}
	}

	var stationUUID pgtype.UUID
	if cmd.StationID != nil && *cmd.StationID != "" {
		stUUID, err := mustUUID(*cmd.StationID)
		if err != nil {
			return application.TopicResult{}, domain.ErrInvalidStationID
		}
		stationUUID = stUUID
	}

	// 4. Insert topic
	topicUUID := newUUIDBytes()
	now := time.Now().UTC()
	created, err := qtx.InsertTopic(ctx, forumsql.InsertTopicParams{
		ID:                     topicUUID,
		Category:               string(cmd.Category),
		Title:                  cmd.Title,
		Excerpt:                "",
		BodyText:               cmd.BodyText,
		AuthorAccountID:        authorUUID,
		PublicAlias:            publicAlias,
		LocalCreatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		LocalUpdatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
		CommentsEnabled:        true,
		RegionUf:               regionUf,
		RegionMunicipalityIbge: regionIBGE,
		FuelProduct:            fuelProduct,
		StationID:              stationUUID,
		AuthorOperationID:      opUUID,
		BodySha256:             bodySHA,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "author_op") {
			_ = tx.Rollback(ctx)
			existing, readErr := s.queries.GetTopicByOperationID(ctx, forumsql.GetTopicByOperationIDParams{
				AuthorAccountID:   authorUUID,
				AuthorOperationID: opUUID,
			})
			if readErr == nil {
				if existing.BodySha256 == bodySHA {
					return application.TopicResult{
						Topic:    mapTopicRowToDomain(existing),
						Replayed: true,
					}, nil
				}
				return application.TopicResult{}, domain.ErrConflictingOperation
			}
		}
		return application.TopicResult{}, fmt.Errorf("forum: insert topic: %w", err)
	}

	// 5. Record change in forum_changes
	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "POST",
		TargetID:       topicUUID,
		Revision:       1,
		Action:         "CREATE",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-submission",
		PriorState:     "",
		NewState:       string(domain.StatePendingReview),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return application.TopicResult{}, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return application.TopicResult{}, fmt.Errorf("forum: commit tx: %w", err)
	}

	return application.TopicResult{
		Topic:    mapTopicRowToDomain(created),
		Replayed: false,
	}, nil
}

// EditTopic updates topic content under an optimistic concurrency CAS check.
// An edited topic returns to PENDING_REVIEW state per B-BR-FOR-09.
func (s *Store) EditTopic(ctx context.Context, cmd application.EditTopicCommand) (*domain.Post, error) {
	topicUUID, err := mustUUID(cmd.TopicID)
	if err != nil {
		return nil, domain.ErrTopicNotFound
	}
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if cmd.Guard != nil {
		if err := cmd.Guard(ctx, tx); err != nil {
			return nil, err
		}
	}

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetTopicByIDForUpdate(ctx, topicUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTopicNotFound
		}
		return nil, fmt.Errorf("forum: lock topic: %w", err)
	}

	if existing.PublicationState == string(domain.StateRemoved) {
		return nil, domain.ErrTopicAlreadyRemoved
	}
	if existing.AuthorAccountID != authorUUID {
		return nil, domain.ErrTopicAuthorMismatch
	}
	if existing.Revision != int32(cmd.ExpectedRevision) {
		return nil, domain.ErrRevisionMismatch
	}

	var regionUf pgtype.Text
	var regionIBGE pgtype.Int4
	if cmd.Region != nil {
		if cmd.Region.UF != "" {
			regionUf = pgtype.Text{String: cmd.Region.UF, Valid: true}
		}
		if cmd.Region.MunicipalityIBGE != nil {
			regionIBGE = pgtype.Int4{Int32: int32(*cmd.Region.MunicipalityIBGE), Valid: true}
		}
	}

	var fuelProduct pgtype.Text
	if cmd.FuelProduct != nil && *cmd.FuelProduct != "" {
		fuelProduct = pgtype.Text{String: *cmd.FuelProduct, Valid: true}
	}

	var stationUUID pgtype.UUID
	if cmd.StationID != nil && *cmd.StationID != "" {
		stUUID, err := mustUUID(*cmd.StationID)
		if err != nil {
			return nil, domain.ErrInvalidStationID
		}
		stationUUID = stUUID
	}

	now := time.Now().UTC()
	bodySHA := sha256Hex(cmd.Title + "\n\n" + cmd.BodyText)
	updated, err := qtx.UpdateTopicCAS(ctx, forumsql.UpdateTopicCASParams{
		ID:                     topicUUID,
		AuthorAccountID:        authorUUID,
		ExpectedRevision:       int32(cmd.ExpectedRevision),
		Title:                  cmd.Title,
		Excerpt:                "",
		BodyText:               cmd.BodyText,
		Category:               string(cmd.Category),
		RegionUf:               regionUf,
		RegionMunicipalityIbge: regionIBGE,
		FuelProduct:            fuelProduct,
		StationID:              stationUUID,
		BodySha256:             bodySHA,
		LocalUpdatedAt:         pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("forum: update topic: %w", err)
	}

	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "POST",
		TargetID:       topicUUID,
		Revision:       updated.Revision,
		Action:         "EDIT",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-edit",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StatePendingReview),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapTopicRowToDomain(updated), nil
}

// RemoveTopic performs an authenticated tombstone removal on an authored topic.
// Removal is terminal; content is wiped and cannot be restored.
func (s *Store) RemoveTopic(ctx context.Context, cmd application.RemoveTopicCommand) (*domain.Post, error) {
	topicUUID, err := mustUUID(cmd.TopicID)
	if err != nil {
		return nil, domain.ErrTopicNotFound
	}
	authorUUID, err := mustUUID(cmd.AuthorAccountID)
	if err != nil {
		return nil, domain.ErrMissingAuthor
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if cmd.Guard != nil {
		if err := cmd.Guard(ctx, tx); err != nil {
			return nil, err
		}
	}

	qtx := s.queries.WithTx(tx)

	existing, err := qtx.GetTopicByIDForUpdate(ctx, topicUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTopicNotFound
		}
		return nil, fmt.Errorf("forum: lock topic: %w", err)
	}

	if existing.PublicationState == string(domain.StateRemoved) {
		return nil, domain.ErrTopicAlreadyRemoved
	}
	if existing.AuthorAccountID != authorUUID {
		return nil, domain.ErrTopicAuthorMismatch
	}
	if existing.Revision != int32(cmd.ExpectedRevision) {
		return nil, domain.ErrRevisionMismatch
	}

	now := time.Now().UTC()
	updated, err := qtx.RemoveTopicTombstone(ctx, forumsql.RemoveTopicTombstoneParams{
		ID:               topicUUID,
		AuthorAccountID:  authorUUID,
		ExpectedRevision: int32(cmd.ExpectedRevision),
		LocalUpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("forum: remove topic: %w", err)
	}

	changeID := newUUIDBytes()
	if err := qtx.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeID,
		TargetType:     "POST",
		TargetID:       topicUUID,
		Revision:       updated.Revision,
		Action:         "REMOVE",
		ActorKind:      "AUTHOR",
		ActorAccountID: authorUUID,
		AdminAuditRef:  "",
		PolicyVersion:  1,
		ReasonCode:     "author-removal",
		PriorState:     existing.PublicationState,
		NewState:       string(domain.StateRemoved),
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return nil, fmt.Errorf("forum: record change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("forum: commit tx: %w", err)
	}

	return mapTopicRowToDomain(updated), nil
}

// GetTopicByID retrieves an authored topic by ID.
func (s *Store) GetTopicByID(ctx context.Context, id string) (*domain.Post, error) {
	topicUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrTopicNotFound
	}
	row, err := s.queries.GetTopicByID(ctx, topicUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTopicNotFound
		}
		return nil, fmt.Errorf("forum: get topic: %w", err)
	}
	return mapTopicRowToDomain(row), nil
}

// ListTopicsByAccount retrieves all topics authored by the specified account ID,
// including unapproved (PENDING_REVIEW), published, and removed topics, sorted by ID.
func (s *Store) ListTopicsByAccount(ctx context.Context, accountID string) ([]domain.Post, error) {
	accUUID, err := mustUUID(accountID)
	if err != nil {
		return nil, domain.ErrTargetInvalid
	}

	rows, err := s.queries.ListTopicsByAuthor(ctx, accUUID)
	if err != nil {
		return nil, fmt.Errorf("forum: list topics by author: %w", err)
	}

	out := make([]domain.Post, 0, len(rows))
	for _, row := range rows {
		p := mapTopicRowToDomain(row)
		out = append(out, *p)
	}
	return out, nil
}
