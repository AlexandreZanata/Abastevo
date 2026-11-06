package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	forumsql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/forum"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

// QuotaChecker checks rate quotas within an open transaction.
type QuotaChecker interface {
	CheckTx(ctx context.Context, tx pgx.Tx, subject, operation string) (time.Duration, error)
}

// Store provides PostgreSQL persistence for Forum Abastevo.
type Store struct {
	pool       *pgxpool.Pool
	queries    *forumsql.Queries
	limiter    QuotaChecker
	CaseOpener application.ModerationCaseOpener
}

// SetQuotaChecker configures an optional transactional quota checker for rate limiting.
func (s *Store) SetQuotaChecker(limiter QuotaChecker) {
	s.limiter = limiter
}

// NewStore initializes a new Store backed by pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: forumsql.New(pool),
	}
}

// ListCategories returns the static forum taxonomy with current version.
func (s *Store) ListCategories(ctx context.Context) ([]domain.CategoryItem, int, error) {
	return domain.DefaultCategories(), domain.CurrentTaxonomyVersion, nil
}

// GetFeedSnapshot returns the current feed revision formatted as a string.
func (s *Store) GetFeedSnapshot(ctx context.Context) (string, error) {
	row, err := s.queries.GetFeedRevision(ctx)
	if err != nil {
		return "", fmt.Errorf("forum: get feed revision: %w", err)
	}
	return fmt.Sprintf("rev-%d", row.Revision), nil
}

// GetPostByID fetches a single post by ID including source metadata.
func (s *Store) GetPostByID(ctx context.Context, id string) (*domain.Post, error) {
	postUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrPostNotFound
	}

	row, err := s.queries.GetPostByID(ctx, postUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPostNotFound
		}
		return nil, fmt.Errorf("forum: get post by id: %w", err)
	}

	return mapPostRowToDomain(row), nil
}

// ListPublicPosts returns a keyset-paginated slice of public post summaries.
func (s *Store) ListPublicPosts(ctx context.Context, query application.ListPublicPostsQuery) (domain.FeedPage, error) {
	var cursorSeq pgtype.Int8
	var cursorID pgtype.UUID

	if query.Cursor != nil && *query.Cursor != "" {
		seq, id, err := DecodeCursor(*query.Cursor)
		if err != nil {
			return domain.FeedPage{}, err
		}
		uid, err := mustUUID(id)
		if err != nil {
			return domain.FeedPage{}, domain.ErrInvalidCursor
		}
		cursorSeq = pgtype.Int8{Int64: seq, Valid: true}
		cursorID = uid
	}

	var categoryParam pgtype.Text
	if query.Category != nil {
		categoryParam = pgtype.Text{String: string(*query.Category), Valid: true}
	}

	var kindParam pgtype.Text
	if query.Kind != nil {
		kindParam = pgtype.Text{String: string(*query.Kind), Valid: true}
	}

	var ufParam pgtype.Text
	if query.UF != nil {
		ufParam = pgtype.Text{String: *query.UF, Valid: true}
	}

	var municipalityParam pgtype.Int4
	if query.MunicipalityIBGE != nil {
		municipalityParam = pgtype.Int4{Int32: int32(*query.MunicipalityIBGE), Valid: true}
	}

	var fuelParam pgtype.Text
	if query.FuelProduct != nil {
		fuelParam = pgtype.Text{String: *query.FuelProduct, Valid: true}
	}

	var stationParam pgtype.UUID
	if query.StationID != nil {
		uid, err := mustUUID(*query.StationID)
		if err != nil {
			return domain.FeedPage{}, domain.ErrInvalidStationID
		}
		stationParam = uid
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	rows, err := s.queries.ListPublicPosts(ctx, forumsql.ListPublicPostsParams{
		Category:         categoryParam,
		Kind:             kindParam,
		Uf:               ufParam,
		MunicipalityIbge: municipalityParam,
		FuelProduct:      fuelParam,
		StationID:        stationParam,
		CursorSeq:        cursorSeq,
		CursorID:         cursorID,
		PageLimit:        int32(limit),
	})
	if err != nil {
		return domain.FeedPage{}, fmt.Errorf("forum: list public posts: %w", err)
	}

	items := make([]domain.PostSummary, 0, len(rows))
	for _, r := range rows {
		items = append(items, mapSummaryRowToDomain(r))
	}

	var nextCursor *string
	if len(rows) == limit {
		last := rows[len(rows)-1]
		c := EncodeCursor(last.FirstPublicationSeq, uuidString(last.ID))
		nextCursor = &c
	}

	snapshot, err := s.GetFeedSnapshot(ctx)
	if err != nil {
		return domain.FeedPage{}, err
	}

	return domain.FeedPage{
		Items:      items,
		NextCursor: nextCursor,
		Snapshot:   snapshot,
		FetchedAt:  time.Now().UTC(),
	}, nil
}

// CreateEditorialPost inserts an editorial post in draft or specified initial state.
func (s *Store) CreateEditorialPost(ctx context.Context, p *domain.Post) error {
	return s.insertEditorialPost(ctx, s.queries, p)
}

// CreateDraftTx inserts an editorial post inside an existing transaction.
func (s *Store) CreateDraftTx(ctx context.Context, tx pgx.Tx, p *domain.Post) error {
	return s.insertEditorialPost(ctx, s.queries.WithTx(tx), p)
}

func (s *Store) insertEditorialPost(ctx context.Context, q *forumsql.Queries, p *domain.Post) error {
	if err := p.Validate(); err != nil {
		return err
	}

	postUUID, err := mustUUID(p.ID)
	if err != nil {
		return fmt.Errorf("forum: invalid post id: %w", err)
	}

	var sourceUUID pgtype.UUID
	if p.SourceID != "" {
		sourceUUID, err = mustUUID(p.SourceID)
		if err != nil {
			return fmt.Errorf("forum: invalid source id: %w", err)
		}
	}

	var authorUUID pgtype.UUID
	if p.AuthorAccountID != "" {
		authorUUID, err = mustUUID(p.AuthorAccountID)
		if err != nil {
			return fmt.Errorf("forum: invalid author id: %w", err)
		}
	}

	var publicAlias pgtype.Text
	if p.PublicAlias != nil {
		publicAlias = pgtype.Text{String: *p.PublicAlias, Valid: true}
	}

	var sourcePubAt pgtype.Timestamptz
	if p.SourcePublishedAt != nil {
		sourcePubAt = pgtype.Timestamptz{Time: *p.SourcePublishedAt, Valid: true}
	}

	var sourceUpdAt pgtype.Timestamptz
	if p.SourceUpdatedAt != nil {
		sourceUpdAt = pgtype.Timestamptz{Time: *p.SourceUpdatedAt, Valid: true}
	}

	var firstPubAt pgtype.Timestamptz
	if p.FirstPublishedAt != nil {
		firstPubAt = pgtype.Timestamptz{Time: *p.FirstPublishedAt, Valid: true}
	}

	var correction pgtype.Text
	if p.Correction != nil {
		correction = pgtype.Text{String: *p.Correction, Valid: true}
	}

	var regionUf pgtype.Text
	var regionIBGE pgtype.Int4
	if p.Region != nil {
		if p.Region.UF != "" {
			regionUf = pgtype.Text{String: p.Region.UF, Valid: true}
		}
		if p.Region.MunicipalityIBGE != nil {
			regionIBGE = pgtype.Int4{Int32: int32(*p.Region.MunicipalityIBGE), Valid: true}
		}
	}

	var fuelProduct pgtype.Text
	if p.FuelProduct != nil {
		fuelProduct = pgtype.Text{String: *p.FuelProduct, Valid: true}
	}

	var stationUUID pgtype.UUID
	if p.StationID != nil && *p.StationID != "" {
		stationUUID, err = mustUUID(*p.StationID)
		if err != nil {
			return fmt.Errorf("forum: invalid station id: %w", err)
		}
	}

	var rightsSnapshot pgtype.Text
	if p.RightsSnapshot != nil {
		rightsSnapshot = pgtype.Text{String: *p.RightsSnapshot, Valid: true}
	}

	revision := p.Revision
	if revision < 1 {
		revision = 1
	}

	localCreated := p.LocalCreatedAt
	if localCreated.IsZero() {
		localCreated = time.Now().UTC()
	}

	localUpdated := p.LocalUpdatedAt
	if localUpdated.IsZero() {
		localUpdated = localCreated
	}

	return q.InsertEditorialPost(ctx, forumsql.InsertEditorialPostParams{
		ID:                     postUUID,
		Kind:                   string(p.Kind),
		Category:               string(p.Category),
		Title:                  p.Title,
		Excerpt:                p.Excerpt,
		BodyText:               p.BodyText,
		SourceID:               sourceUUID,
		CanonicalUrl:           p.CanonicalURL,
		CanonicalUrlHash:       "",
		ExternalID:             "",
		AuthorAccountID:        authorUUID,
		PublicAlias:            publicAlias,
		SourcePublishedAt:      sourcePubAt,
		SourceUpdatedAt:        sourceUpdAt,
		LocalCreatedAt:         pgtype.Timestamptz{Time: localCreated, Valid: true},
		LocalUpdatedAt:         pgtype.Timestamptz{Time: localUpdated, Valid: true},
		FirstPublishedAt:       firstPubAt,
		FirstPublicationSeq:    p.FirstPublicationSeq,
		PublicationState:       string(p.PublicationState),
		Visibility:             string(p.Visibility),
		Revision:               int32(revision),
		CommentsEnabled:        p.CommentsEnabled,
		Correction:             correction,
		RegionUf:               regionUf,
		RegionMunicipalityIbge: regionIBGE,
		FuelProduct:            fuelProduct,
		StationID:              stationUUID,
		AuthorOperationID:      pgtype.UUID{},
		BodySha256:             "",
		RightsSnapshot:         rightsSnapshot,
		BulletinKey:            pgtype.Text{},
		SupersedesPostID:       pgtype.UUID{},
	})
}

// PublishEditorialPost assigns the next monotonic publication sequence and sets state to PUBLISHED.
func (s *Store) PublishEditorialPost(ctx context.Context, id string, publishedAt time.Time) (int64, error) {
	postUUID, err := mustUUID(id)
	if err != nil {
		return 0, domain.ErrPostNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}

	seq, err := txQueries.BumpFeedRevision(ctx, pgtype.Timestamptz{Time: publishedAt, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("forum: bump feed revision: %w", err)
	}

	rows, err := txQueries.PublishEditorialPost(ctx, forumsql.PublishEditorialPostParams{
		ID:                  postUUID,
		FirstPublicationSeq: seq,
		PublishedAt:         pgtype.Timestamptz{Time: publishedAt, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("forum: publish post: %w", err)
	}
	if rows == 0 {
		return 0, domain.ErrPostNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("forum: commit publication: %w", err)
	}

	return seq, nil
}

// UpdateEditorialPost modifies post contents under optimistic revision locking.
func (s *Store) UpdateEditorialPost(ctx context.Context, p *domain.Post, expectedRevision int) error {
	if err := p.Validate(); err != nil {
		return err
	}

	postUUID, err := mustUUID(p.ID)
	if err != nil {
		return domain.ErrPostNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	var correction pgtype.Text
	if p.Correction != nil {
		correction = pgtype.Text{String: *p.Correction, Valid: true}
	}

	var regionUf pgtype.Text
	var regionIBGE pgtype.Int4
	if p.Region != nil {
		if p.Region.UF != "" {
			regionUf = pgtype.Text{String: p.Region.UF, Valid: true}
		}
		if p.Region.MunicipalityIBGE != nil {
			regionIBGE = pgtype.Int4{Int32: int32(*p.Region.MunicipalityIBGE), Valid: true}
		}
	}

	var fuelProduct pgtype.Text
	if p.FuelProduct != nil {
		fuelProduct = pgtype.Text{String: *p.FuelProduct, Valid: true}
	}

	var stationUUID pgtype.UUID
	if p.StationID != nil && *p.StationID != "" {
		stationUUID, err = mustUUID(*p.StationID)
		if err != nil {
			return fmt.Errorf("forum: invalid station id: %w", err)
		}
	}

	now := time.Now().UTC()

	rows, err := txQueries.UpdateEditorialPost(ctx, forumsql.UpdateEditorialPostParams{
		ID:                     postUUID,
		Title:                  p.Title,
		Excerpt:                p.Excerpt,
		BodyText:               p.BodyText,
		Category:               string(p.Category),
		FuelProduct:            fuelProduct,
		RegionUf:               regionUf,
		RegionMunicipalityIbge: regionIBGE,
		StationID:              stationUUID,
		CommentsEnabled:        p.CommentsEnabled,
		Correction:             correction,
		UpdatedAt:              pgtype.Timestamptz{Time: now, Valid: true},
		ExpectedRevision:       int32(expectedRevision),
	})
	if err != nil {
		return fmt.Errorf("forum: update post: %w", err)
	}
	if rows == 0 {
		return domain.ErrRevisionMismatch
	}

	// Bump feed revision if post is published
	if p.PublicationState == domain.StatePublished {
		if _, err := txQueries.BumpFeedRevision(ctx, pgtype.Timestamptz{Time: now, Valid: true}); err != nil {
			return fmt.Errorf("forum: bump feed revision: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// SetPostVisibility changes the public visibility of a post.
func (s *Store) SetPostVisibility(ctx context.Context, id string, visibility domain.Visibility) error {
	postUUID, err := mustUUID(id)
	if err != nil {
		return domain.ErrPostNotFound
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("forum: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	now := time.Now().UTC()

	rows, err := txQueries.SetPostVisibility(ctx, forumsql.SetPostVisibilityParams{
		ID:         postUUID,
		Visibility: string(visibility),
		UpdatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("forum: set post visibility: %w", err)
	}
	if rows == 0 {
		return domain.ErrPostNotFound
	}

	if _, err := txQueries.BumpFeedRevision(ctx, pgtype.Timestamptz{Time: now, Valid: true}); err != nil {
		return fmt.Errorf("forum: bump feed revision: %w", err)
	}

	return tx.Commit(ctx)
}

// ChangeRecord represents an audit change to a forum entity.
type ChangeRecord struct {
	ID            string
	TargetType    string
	TargetID      string
	Revision      int
	Action        string
	ActorKind     string
	ActorAccount  string
	AdminAuditRef string
	PolicyVersion int
	ReasonCode    string
	PriorState    string
	NewState      string
	CreatedAt     time.Time
}

// RecordChange appends an audit change record.
func (s *Store) RecordChange(ctx context.Context, rec ChangeRecord) error {
	changeUUID, err := mustUUID(rec.ID)
	if err != nil {
		return fmt.Errorf("forum: invalid change id: %w", err)
	}

	targetUUID, err := mustUUID(rec.TargetID)
	if err != nil {
		return fmt.Errorf("forum: invalid target id: %w", err)
	}

	var actorUUID pgtype.UUID
	if rec.ActorAccount != "" {
		actorUUID, err = mustUUID(rec.ActorAccount)
		if err != nil {
			return fmt.Errorf("forum: invalid actor account id: %w", err)
		}
	}

	createdAt := rec.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	policyVer := rec.PolicyVersion
	if policyVer < 1 {
		policyVer = 1
	}

	return s.queries.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             changeUUID,
		TargetType:     rec.TargetType,
		TargetID:       targetUUID,
		Revision:       int32(rec.Revision),
		Action:         rec.Action,
		ActorKind:      rec.ActorKind,
		ActorAccountID: actorUUID,
		AdminAuditRef:  rec.AdminAuditRef,
		PolicyVersion:  int32(policyVer),
		ReasonCode:     rec.ReasonCode,
		PriorState:     rec.PriorState,
		NewState:       rec.NewState,
		CreatedAt:      pgtype.Timestamptz{Time: createdAt, Valid: true},
	})
}

// EncodeCursor builds an opaque pagination cursor token.
func EncodeCursor(seq int64, id string) string {
	raw := fmt.Sprintf("%d:%s", seq, id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor decodes and validates an opaque pagination cursor token.
func DecodeCursor(token string) (int64, string, error) {
	if strings.TrimSpace(token) == "" {
		return 0, "", domain.ErrInvalidCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, "", domain.ErrInvalidCursor
	}
	parts := strings.SplitN(string(b), ":", 2)
	if len(parts) != 2 {
		return 0, "", domain.ErrInvalidCursor
	}
	seq, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || seq <= 0 {
		return 0, "", domain.ErrInvalidCursor
	}
	id := parts[1]
	if _, err := mustUUID(id); err != nil {
		return 0, "", domain.ErrInvalidCursor
	}
	return seq, id, nil
}

func mustUUID(text string) (pgtype.UUID, error) {
	if text == "" {
		return pgtype.UUID{}, errors.New("empty uuid")
	}
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("malformed uuid")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func mapPostRowToDomain(r forumsql.GetPostByIDRow) *domain.Post {
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

	var rightsSnapshot *string
	if r.RightsSnapshot.Valid {
		cp := r.RightsSnapshot.String
		rightsSnapshot = &cp
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

	var srcAttr *domain.SourceAttribution
	if r.SourceName.Valid && r.SourceOriginUrl.Valid {
		srcAttr = &domain.SourceAttribution{
			Name:        r.SourceName.String,
			URL:         r.SourceOriginUrl.String,
			Attribution: r.SourceAttribution.String,
		}
	}

	var srcPubAt *time.Time
	if r.SourcePublishedAt.Valid {
		t := r.SourcePublishedAt.Time.UTC()
		srcPubAt = &t
	}

	var srcUpdAt *time.Time
	if r.SourceUpdatedAt.Valid {
		t := r.SourceUpdatedAt.Time.UTC()
		srcUpdAt = &t
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
		SourceID:            uuidString(r.SourceID),
		SourceAttribution:   srcAttr,
		CanonicalURL:        r.CanonicalUrl,
		AuthorAccountID:     uuidString(r.AuthorAccountID),
		PublicAlias:         publicAlias,
		SourcePublishedAt:   srcPubAt,
		SourceUpdatedAt:     srcUpdAt,
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
		RightsSnapshot:      rightsSnapshot,
	}
}

func mapSummaryRowToDomain(r forumsql.ListPublicPostsRow) domain.PostSummary {
	var excerpt *string
	if r.Excerpt != "" {
		cp := r.Excerpt
		excerpt = &cp
	}

	var publicAlias *string
	if r.PublicAlias.Valid {
		cp := r.PublicAlias.String
		publicAlias = &cp
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

	var srcAttr *domain.SourceAttribution
	if r.SourceName.Valid && r.SourceOriginUrl.Valid {
		srcAttr = &domain.SourceAttribution{
			Name:        r.SourceName.String,
			URL:         r.SourceOriginUrl.String,
			Attribution: r.SourceAttribution.String,
		}
	}

	var srcPubAt *time.Time
	if r.SourcePublishedAt.Valid {
		t := r.SourcePublishedAt.Time.UTC()
		srcPubAt = &t
	}

	firstPub := time.Time{}
	if r.FirstPublishedAt.Valid {
		firstPub = r.FirstPublishedAt.Time.UTC()
	}

	return domain.PostSummary{
		ID:                uuidString(r.ID),
		Kind:              domain.PostKind(r.Kind),
		Category:          domain.Category(r.Category),
		Title:             r.Title,
		Excerpt:           excerpt,
		Source:            srcAttr,
		PublicAlias:       publicAlias,
		SourcePublishedAt: srcPubAt,
		FirstPublishedAt:  firstPub,
		UpdatedAt:         r.LocalUpdatedAt.Time.UTC(),
		Revision:          int(r.Revision),
		Region:            region,
		FuelProduct:       fuelProduct,
		StationID:         stationID,
	}
}

// GetPostTx retrieves a post by ID inside an active transaction.
func (s *Store) GetPostTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error) {
	postUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrPostNotFound
	}
	row, err := s.queries.WithTx(tx).GetPostByID(ctx, postUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPostNotFound
		}
		return nil, fmt.Errorf("forum: get post by id: %w", err)
	}
	return mapPostRowToDomain(row), nil
}

// GetPostForUpdateTx retrieves and locks a post by ID inside an active transaction.
func (s *Store) GetPostForUpdateTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error) {
	postUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrPostNotFound
	}
	row, err := s.queries.WithTx(tx).GetPostByIDForUpdate(ctx, postUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPostNotFound
		}
		return nil, fmt.Errorf("forum: get post by id for update: %w", err)
	}
	return mapPostForUpdateRowToDomain(row), nil
}

// UpdateEditorialTx modifies post contents under optimistic revision locking inside an active transaction.
func (s *Store) UpdateEditorialTx(ctx context.Context, tx pgx.Tx, p *domain.Post, expectedRevision int) error {
	if err := p.Validate(); err != nil {
		return err
	}
	postUUID, err := mustUUID(p.ID)
	if err != nil {
		return domain.ErrPostNotFound
	}

	var correction pgtype.Text
	if p.Correction != nil {
		correction = pgtype.Text{String: *p.Correction, Valid: true}
	}

	var regionUf pgtype.Text
	var regionIBGE pgtype.Int4
	if p.Region != nil {
		if p.Region.UF != "" {
			regionUf = pgtype.Text{String: p.Region.UF, Valid: true}
		}
		if p.Region.MunicipalityIBGE != nil {
			regionIBGE = pgtype.Int4{Int32: int32(*p.Region.MunicipalityIBGE), Valid: true}
		}
	}

	var fuelProduct pgtype.Text
	if p.FuelProduct != nil {
		fuelProduct = pgtype.Text{String: *p.FuelProduct, Valid: true}
	}

	var stationUUID pgtype.UUID
	if p.StationID != nil && *p.StationID != "" {
		stationUUID, err = mustUUID(*p.StationID)
		if err != nil {
			return fmt.Errorf("forum: invalid station id: %w", err)
		}
	}

	now := time.Now().UTC()
	rows, err := s.queries.WithTx(tx).UpdateEditorialPost(ctx, forumsql.UpdateEditorialPostParams{
		ID:                     postUUID,
		Title:                  p.Title,
		Excerpt:                p.Excerpt,
		BodyText:               p.BodyText,
		Category:               string(p.Category),
		FuelProduct:            fuelProduct,
		RegionUf:               regionUf,
		RegionMunicipalityIbge: regionIBGE,
		StationID:              stationUUID,
		CommentsEnabled:        p.CommentsEnabled,
		Correction:             correction,
		UpdatedAt:              pgtype.Timestamptz{Time: now, Valid: true},
		ExpectedRevision:       int32(expectedRevision),
	})
	if err != nil {
		return fmt.Errorf("forum: update post: %w", err)
	}
	if rows == 0 {
		return domain.ErrRevisionMismatch
	}
	return nil
}

// PublishEditorialTx transitions post to PUBLISHED state inside an active transaction.
func (s *Store) PublishEditorialTx(ctx context.Context, tx pgx.Tx, id string, publishedAt time.Time, seq int64) error {
	postUUID, err := mustUUID(id)
	if err != nil {
		return domain.ErrPostNotFound
	}
	rows, err := s.queries.WithTx(tx).PublishEditorialPost(ctx, forumsql.PublishEditorialPostParams{
		ID:                  postUUID,
		FirstPublicationSeq: seq,
		PublishedAt:         pgtype.Timestamptz{Time: publishedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("forum: publish post: %w", err)
	}
	if rows == 0 {
		return domain.ErrPostNotFound
	}
	return nil
}

// SetPostVisibilityTx updates post visibility inside an active transaction.
func (s *Store) SetPostVisibilityTx(ctx context.Context, tx pgx.Tx, id string, visibility domain.Visibility, updatedAt time.Time) error {
	postUUID, err := mustUUID(id)
	if err != nil {
		return domain.ErrPostNotFound
	}
	rows, err := s.queries.WithTx(tx).SetPostVisibility(ctx, forumsql.SetPostVisibilityParams{
		ID:         postUUID,
		Visibility: string(visibility),
		UpdatedAt:  pgtype.Timestamptz{Time: updatedAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("forum: set post visibility: %w", err)
	}
	if rows == 0 {
		return domain.ErrPostNotFound
	}
	return nil
}

// RejectPostTx transitions a post to REJECTED inside an ongoing transaction.
func (s *Store) RejectPostTx(ctx context.Context, tx pgx.Tx, id string, expectedRevision int, now time.Time) error {
	postUUID, err := mustUUID(id)
	if err != nil {
		return domain.ErrPostNotFound
	}
	rows, err := s.queries.WithTx(tx).RejectPost(ctx, forumsql.RejectPostParams{
		ID:               postUUID,
		ExpectedRevision: int32(expectedRevision),
		UpdatedAt:        pgtype.Timestamptz{Time: now.UTC(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("forum: reject post tx: %w", err)
	}
	if rows == 0 {
		return domain.ErrRevisionMismatch
	}
	return nil
}

// BumpFeedRevisionTx increments the feed state revision inside an active transaction.
func (s *Store) BumpFeedRevisionTx(ctx context.Context, tx pgx.Tx, now time.Time) (int64, error) {
	row, err := s.queries.WithTx(tx).BumpFeedRevision(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return 0, fmt.Errorf("forum: bump feed revision: %w", err)
	}
	return row, nil
}

// RecordForumChangeTx appends an audit change record to forum_changes inside an active transaction.
func (s *Store) RecordForumChangeTx(ctx context.Context, tx pgx.Tx, rec domain.ChangeRecord) error {
	targetUUID, err := mustUUID(rec.TargetID)
	if err != nil {
		return fmt.Errorf("forum: invalid target id: %w", err)
	}

	var actorUUID pgtype.UUID
	if rec.ActorAccountID != nil && *rec.ActorAccountID != "" {
		actorUUID, err = mustUUID(*rec.ActorAccountID)
		if err != nil {
			return fmt.Errorf("forum: invalid actor account id: %w", err)
		}
	}

	createdAt := rec.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	policyVer := rec.PolicyVersion
	if policyVer < 1 {
		policyVer = 1
	}

	adminAuditRef := ""
	if rec.AdminAuditRef != nil {
		adminAuditRef = *rec.AdminAuditRef
	}

	priorState := ""
	if rec.PriorState != nil {
		priorState = *rec.PriorState
	}

	q := s.queries
	if tx != nil {
		q = s.queries.WithTx(tx)
	}

	return q.RecordForumChange(ctx, forumsql.RecordForumChangeParams{
		ID:             newUUIDBytes(),
		TargetType:     rec.TargetType,
		TargetID:       targetUUID,
		Revision:       int32(rec.Revision),
		Action:         rec.Action,
		ActorKind:      rec.ActorKind,
		ActorAccountID: actorUUID,
		AdminAuditRef:  adminAuditRef,
		PolicyVersion:  int32(policyVer),
		ReasonCode:     rec.ReasonCode,
		PriorState:     priorState,
		NewState:       rec.NewState,
		CreatedAt:      pgtype.Timestamptz{Time: createdAt, Valid: true},
	})
}

// GetSourceByID retrieves source metadata by UUID.
func (s *Store) GetSourceByID(ctx context.Context, id string) (*domain.Source, error) {
	sourceUUID, err := mustUUID(id)
	if err != nil {
		return nil, domain.ErrMissingSource
	}
	row, err := s.queries.GetSourceByID(ctx, sourceUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrMissingSource
		}
		return nil, fmt.Errorf("forum: get source by id: %w", err)
	}
	return mapSourceRowToDomain(row), nil
}

// ListAllSourcesWithMetrics returns all registered publication sources with their latest run/checkpoint metrics.
func (s *Store) ListAllSourcesWithMetrics(ctx context.Context) ([]application.SourceWithMetrics, error) {
	rows, err := s.queries.ListAllSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: list all sources: %w", err)
	}

	result := make([]application.SourceWithMetrics, len(rows))
	for i, r := range rows {
		var checkedAt *time.Time
		if r.CheckedAt.Valid {
			t := r.CheckedAt.Time.UTC()
			checkedAt = &t
		}
		var reviewExpiresAt *time.Time
		if r.ReviewExpiresAt.Valid {
			t := r.ReviewExpiresAt.Time.UTC()
			reviewExpiresAt = &t
		}
		var fetchURL string
		if r.FetchUrl.Valid {
			fetchURL = r.FetchUrl.String
		}
		var fetchFormat string
		if r.FetchFormat.Valid {
			fetchFormat = r.FetchFormat.String
		}

		result[i] = application.SourceWithMetrics{
			ID:                 uuidString(r.ID),
			Code:               r.Code,
			Name:               r.Name,
			Kind:               domain.SourceKind(r.Kind),
			OriginURL:          r.OriginUrl,
			FetchURL:           fetchURL,
			FetchFormat:        fetchFormat,
			CollectionEnabled:  r.CollectionEnabled,
			PublicationEnabled: r.PublicationEnabled,
			RightsStatus:       domain.RightsStatus(r.RightsStatus),
			TitleAllowed:       r.TitleAllowed,
			ExcerptAllowed:     r.ExcerptAllowed,
			ImageAllowed:       r.ImageAllowed,
			TermsURL:           r.TermsUrl,
			TermsSHA256:        r.TermsSha256,
			CheckedAt:          checkedAt,
			ReviewExpiresAt:    reviewExpiresAt,
			Attribution:        r.Attribution,
		}

		if r.CheckpointEtag.Valid {
			etag := r.CheckpointEtag.String
			result[i].CheckpointETag = &etag
		}
		if r.CheckpointLastModified.Valid {
			lm := r.CheckpointLastModified.String
			result[i].CheckpointLastModified = &lm
		}
		if r.CheckpointLastStatus.Valid {
			st := r.CheckpointLastStatus.String
			result[i].CheckpointLastStatus = &st
		}
		if r.CheckpointFailures.Valid {
			result[i].CheckpointFailures = int(r.CheckpointFailures.Int32)
		}
		if r.CheckpointLastSuccessAt.Valid {
			t := r.CheckpointLastSuccessAt.Time.UTC()
			result[i].CheckpointLastSuccessAt = &t
		}
	}
	return result, nil
}

// AdminListPosts returns a paginated list of posts for administration.
func (s *Store) AdminListPosts(ctx context.Context, filter application.AdminListFilter) ([]domain.Post, *string, error) {
	var pubStateParam pgtype.Text
	if filter.PublicationState != nil && *filter.PublicationState != "" {
		pubStateParam = pgtype.Text{String: *filter.PublicationState, Valid: true}
	}

	var catParam pgtype.Text
	if filter.Category != nil && *filter.Category != "" {
		catParam = pgtype.Text{String: *filter.Category, Valid: true}
	}

	var kindParam pgtype.Text
	if filter.Kind != nil && *filter.Kind != "" {
		kindParam = pgtype.Text{String: *filter.Kind, Valid: true}
	}

	var cursorCreatedAt pgtype.Timestamptz
	var cursorID pgtype.UUID
	if filter.CursorCreatedAt != nil && filter.CursorID != nil && *filter.CursorID != "" {
		uid, err := mustUUID(*filter.CursorID)
		if err != nil {
			return nil, nil, domain.ErrInvalidCursor
		}
		cursorCreatedAt = pgtype.Timestamptz{Time: *filter.CursorCreatedAt, Valid: true}
		cursorID = uid
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	} else if limit > 50 {
		limit = 50
	}

	rows, err := s.queries.AdminListPosts(ctx, forumsql.AdminListPostsParams{
		PublicationState: pubStateParam,
		Category:         catParam,
		Kind:             kindParam,
		CursorCreatedAt:  cursorCreatedAt,
		CursorID:         cursorID,
		PageLimit:        int32(limit),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("forum: admin list posts: %w", err)
	}

	posts := make([]domain.Post, 0, len(rows))
	for _, r := range rows {
		posts = append(posts, mapAdminListRowToDomain(r))
	}

	var nextCursor *string
	if len(rows) == limit {
		last := rows[len(rows)-1]
		c := EncodeAdminCursor(last.LocalCreatedAt.Time, uuidString(last.ID))
		nextCursor = &c
	}

	return posts, nextCursor, nil
}

// EncodeAdminCursor serializes an admin pagination cursor.
func EncodeAdminCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%s:%s", createdAt.UTC().Format(time.RFC3339Nano), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeAdminCursor parses an admin pagination cursor.
func DecodeAdminCursor(token string) (time.Time, string, error) {
	if strings.TrimSpace(token) == "" {
		return time.Time{}, "", domain.ErrInvalidCursor
	}
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return time.Time{}, "", domain.ErrInvalidCursor
	}
	rawStr := string(b)
	idx := strings.LastIndex(rawStr, ":")
	if idx <= 0 || idx >= len(rawStr)-1 {
		return time.Time{}, "", domain.ErrInvalidCursor
	}
	timePart := rawStr[:idx]
	idPart := rawStr[idx+1:]
	t, err := time.Parse(time.RFC3339Nano, timePart)
	if err != nil {
		return time.Time{}, "", domain.ErrInvalidCursor
	}
	if _, err := mustUUID(idPart); err != nil {
		return time.Time{}, "", domain.ErrInvalidCursor
	}
	return t, idPart, nil
}

func newUUIDBytes() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122
	return pgtype.UUID{Bytes: b, Valid: true}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func mapSourceRowToDomain(r forumsql.ForumSource) *domain.Source {
	var checkedAt *time.Time
	if r.CheckedAt.Valid {
		t := r.CheckedAt.Time.UTC()
		checkedAt = &t
	}
	var reviewExpiresAt *time.Time
	if r.ReviewExpiresAt.Valid {
		t := r.ReviewExpiresAt.Time.UTC()
		reviewExpiresAt = &t
	}
	var fetchURL string
	if r.FetchUrl.Valid {
		fetchURL = r.FetchUrl.String
	}
	var fetchFormat string
	if r.FetchFormat.Valid {
		fetchFormat = r.FetchFormat.String
	}
	return &domain.Source{
		ID:                 uuidString(r.ID),
		Code:               r.Code,
		Name:               r.Name,
		Kind:               domain.SourceKind(r.Kind),
		OriginURL:          r.OriginUrl,
		FetchURL:           fetchURL,
		FetchFormat:        fetchFormat,
		CollectionEnabled:  r.CollectionEnabled,
		PublicationEnabled: r.PublicationEnabled,
		RightsStatus:       domain.RightsStatus(r.RightsStatus),
		TitleAllowed:       r.TitleAllowed,
		ExcerptAllowed:     r.ExcerptAllowed,
		ImageAllowed:       r.ImageAllowed,
		TermsURL:           r.TermsUrl,
		TermsSHA256:        r.TermsSha256,
		CheckedAt:          checkedAt,
		ReviewExpiresAt:    reviewExpiresAt,
		Attribution:        r.Attribution,
	}
}

func mapAdminListRowToDomain(r forumsql.AdminListPostsRow) domain.Post {
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
	var srcAttr *domain.SourceAttribution
	if r.SourceName.Valid && r.SourceOriginUrl.Valid {
		srcAttr = &domain.SourceAttribution{
			Name:        r.SourceName.String,
			URL:         r.SourceOriginUrl.String,
			Attribution: r.SourceAttribution.String,
		}
	}
	var srcPubAt *time.Time
	if r.SourcePublishedAt.Valid {
		t := r.SourcePublishedAt.Time.UTC()
		srcPubAt = &t
	}
	var firstPubAt *time.Time
	if r.FirstPublishedAt.Valid {
		t := r.FirstPublishedAt.Time.UTC()
		firstPubAt = &t
	}
	return domain.Post{
		ID:                  uuidString(r.ID),
		Kind:                domain.PostKind(r.Kind),
		Category:            domain.Category(r.Category),
		Title:               r.Title,
		Excerpt:             r.Excerpt,
		BodyText:            "", // List projection omits body text
		SourceID:            uuidString(r.SourceID),
		SourceAttribution:   srcAttr,
		CanonicalURL:        r.CanonicalUrl,
		AuthorAccountID:     uuidString(r.AuthorAccountID),
		PublicAlias:         publicAlias,
		SourcePublishedAt:   srcPubAt,
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

func mapPostForUpdateRowToDomain(r forumsql.GetPostByIDForUpdateRow) *domain.Post {
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
	var rightsSnapshot *string
	if r.RightsSnapshot.Valid {
		cp := r.RightsSnapshot.String
		rightsSnapshot = &cp
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
	var srcPubAt *time.Time
	if r.SourcePublishedAt.Valid {
		t := r.SourcePublishedAt.Time.UTC()
		srcPubAt = &t
	}
	var srcUpdAt *time.Time
	if r.SourceUpdatedAt.Valid {
		t := r.SourceUpdatedAt.Time.UTC()
		srcUpdAt = &t
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
		SourceID:            uuidString(r.SourceID),
		CanonicalURL:        r.CanonicalUrl,
		ExternalID:          r.ExternalID,
		AuthorAccountID:     uuidString(r.AuthorAccountID),
		PublicAlias:         publicAlias,
		SourcePublishedAt:   srcPubAt,
		SourceUpdatedAt:     srcUpdAt,
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
		RightsSnapshot:      rightsSnapshot,
	}
}

// ListEnabledSources returns all sources with collection_enabled = true.
func (s *Store) ListEnabledSources(ctx context.Context) ([]domain.Source, error) {
	rows, err := s.queries.ListEnabledSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("forum: list enabled sources: %w", err)
	}
	sources := make([]domain.Source, len(rows))
	for i, r := range rows {
		sources[i] = *mapSourceRowToDomain(r)
	}
	return sources, nil
}

// GetSourceCheckpoint retrieves the conditional GET and execution state for a source.
func (s *Store) GetSourceCheckpoint(ctx context.Context, sourceID string) (application.SourceCheckpoint, error) {
	uid, err := mustUUID(sourceID)
	if err != nil {
		return application.SourceCheckpoint{}, domain.ErrMissingSource
	}
	row, err := s.queries.GetSourceCheckpoint(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.SourceCheckpoint{
				SourceID: sourceID,
			}, nil
		}
		return application.SourceCheckpoint{}, fmt.Errorf("forum: get source checkpoint: %w", err)
	}
	var lastRunID *string
	if row.LastRunID.Valid {
		idStr := uuidString(row.LastRunID)
		lastRunID = &idStr
	}
	var lastSuccessAt *time.Time
	if row.LastSuccessAt.Valid {
		t := row.LastSuccessAt.Time.UTC()
		lastSuccessAt = &t
	}
	return application.SourceCheckpoint{
		SourceID:            uuidString(row.SourceID),
		ETag:                row.Etag,
		LastModified:        row.LastModified,
		LastRunID:           lastRunID,
		LastSuccessAt:       lastSuccessAt,
		LastStatus:          row.LastStatus,
		ConsecutiveFailures: int(row.ConsecutiveFailures),
		UpdatedAt:           row.UpdatedAt.Time.UTC(),
	}, nil
}

// UpsertSourceCheckpoint updates or inserts source checkpoint information.
func (s *Store) UpsertSourceCheckpoint(ctx context.Context, cp application.SourceCheckpoint) error {
	uid, err := mustUUID(cp.SourceID)
	if err != nil {
		return domain.ErrMissingSource
	}
	var lastRunUUID pgtype.UUID
	if cp.LastRunID != nil && *cp.LastRunID != "" {
		lastRunUUID, _ = mustUUID(*cp.LastRunID)
	}
	var lastSuccessAt pgtype.Timestamptz
	if cp.LastSuccessAt != nil {
		lastSuccessAt = pgtype.Timestamptz{Time: cp.LastSuccessAt.UTC(), Valid: true}
	}
	updatedAt := cp.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	params := forumsql.UpsertSourceCheckpointParams{
		SourceID:            uid,
		Etag:                cp.ETag,
		LastModified:        cp.LastModified,
		LastRunID:           lastRunUUID,
		LastSuccessAt:       lastSuccessAt,
		LastStatus:          cp.LastStatus,
		ConsecutiveFailures: int32(cp.ConsecutiveFailures),
		UpdatedAt:           pgtype.Timestamptz{Time: updatedAt.UTC(), Valid: true},
	}
	if err := s.queries.UpsertSourceCheckpoint(ctx, params); err != nil {
		return fmt.Errorf("forum: upsert source checkpoint: %w", err)
	}
	return nil
}

// RecordSourceRun inserts an audit record for a source ingestion attempt.
func (s *Store) RecordSourceRun(ctx context.Context, run application.SourceRun) error {
	runUUID, err := mustUUID(run.ID)
	if err != nil {
		return fmt.Errorf("forum: invalid run id: %w", err)
	}
	srcUUID, err := mustUUID(run.SourceID)
	if err != nil {
		return domain.ErrMissingSource
	}
	var completedAt pgtype.Timestamptz
	if run.CompletedAt != nil {
		completedAt = pgtype.Timestamptz{Time: run.CompletedAt.UTC(), Valid: true}
	}
	params := forumsql.InsertSourceRunParams{
		ID:              runUUID,
		SourceID:        srcUUID,
		Status:          run.Status,
		Etag:            run.ETag,
		LastModified:    run.LastModified,
		ItemsDiscovered: int32(run.ItemsDiscovered),
		ItemsIngested:   int32(run.ItemsIngested),
		ErrorMessage:    run.ErrorMessage,
		StartedAt:       pgtype.Timestamptz{Time: run.StartedAt.UTC(), Valid: true},
		CompletedAt:     completedAt,
	}
	if err := s.queries.InsertSourceRun(ctx, params); err != nil {
		return fmt.Errorf("forum: record source run: %w", err)
	}
	return nil
}

// HasPostByExternalID checks whether a post already exists for this source and external ID.
func (s *Store) HasPostByExternalID(ctx context.Context, sourceID, externalID string) (bool, error) {
	if externalID == "" {
		return false, nil
	}
	srcUUID, err := mustUUID(sourceID)
	if err != nil {
		return false, domain.ErrMissingSource
	}
	exists, err := s.queries.HasPostByExternalID(ctx, forumsql.HasPostByExternalIDParams{
		SourceID:   srcUUID,
		ExternalID: externalID,
	})
	if err != nil {
		return false, fmt.Errorf("forum: check post by external id: %w", err)
	}
	return exists, nil
}

// HasPostByUrlHash checks whether a post already exists for this source and URL hash.
func (s *Store) HasPostByUrlHash(ctx context.Context, sourceID, urlHash string) (bool, error) {
	if urlHash == "" {
		return false, nil
	}
	srcUUID, err := mustUUID(sourceID)
	if err != nil {
		return false, domain.ErrMissingSource
	}
	exists, err := s.queries.HasPostByUrlHash(ctx, forumsql.HasPostByUrlHashParams{
		SourceID:         srcUUID,
		CanonicalUrlHash: urlHash,
	})
	if err != nil {
		return false, fmt.Errorf("forum: check post by url hash: %w", err)
	}
	return exists, nil
}

// InsertNewsCandidatePost persists an ingested news candidate post as DRAFT.
func (s *Store) InsertNewsCandidatePost(ctx context.Context, post *domain.Post) error {
	if post.PublicationState == domain.StatePublished {
		return fmt.Errorf("forum: candidate news cannot be directly published")
	}
	if post.FirstPublicationSeq != 0 || post.FirstPublishedAt != nil {
		return fmt.Errorf("forum: candidate news cannot have publication sequence or timestamp")
	}
	if post.SourcePublishedAt == nil {
		return fmt.Errorf("forum: candidate news requires source publication timestamp")
	}
	postUUID, err := mustUUID(post.ID)
	if err != nil {
		return fmt.Errorf("forum: invalid candidate post id: %w", err)
	}
	srcUUID, err := mustUUID(post.SourceID)
	if err != nil {
		return domain.ErrMissingSource
	}

	var canonicalUrlHash string
	if post.CanonicalURL != "" {
		canonicalUrlHash = sha256Hex(post.CanonicalURL)
	}

	params := forumsql.InsertEditorialPostParams{
		ID:                     postUUID,
		Kind:                   string(domain.KindNews),
		Category:               string(post.Category),
		Title:                  post.Title,
		Excerpt:                post.Excerpt,
		BodyText:               post.BodyText,
		SourceID:               srcUUID,
		CanonicalUrl:           post.CanonicalURL,
		CanonicalUrlHash:       canonicalUrlHash,
		ExternalID:             post.ExternalID,
		AuthorAccountID:        pgtype.UUID{},
		PublicAlias:            pgtype.Text{},
		SourcePublishedAt:      pgtype.Timestamptz{Time: post.SourcePublishedAt.UTC(), Valid: true},
		SourceUpdatedAt:        pgtype.Timestamptz{},
		LocalCreatedAt:         pgtype.Timestamptz{Time: post.LocalCreatedAt.UTC(), Valid: true},
		LocalUpdatedAt:         pgtype.Timestamptz{Time: post.LocalUpdatedAt.UTC(), Valid: true},
		FirstPublishedAt:       pgtype.Timestamptz{},
		FirstPublicationSeq:    0,
		PublicationState:       string(domain.StateDraft),
		Visibility:             string(domain.VisibilityVisible),
		Revision:               1,
		CommentsEnabled:        post.CommentsEnabled,
		Correction:             pgtype.Text{},
		RegionUf:               pgtype.Text{},
		RegionMunicipalityIbge: pgtype.Int4{},
		FuelProduct:            pgtype.Text{},
		StationID:              pgtype.UUID{},
		AuthorOperationID:      pgtype.UUID{},
		BodySha256:             sha256Hex(post.BodyText),
		RightsSnapshot:         pgtype.Text{},
		BulletinKey:            pgtype.Text{},
		SupersedesPostID:       pgtype.UUID{},
	}
	if err := s.queries.InsertEditorialPost(ctx, params); err != nil {
		return fmt.Errorf("forum: insert candidate post: %w", err)
	}
	return nil
}
