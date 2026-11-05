package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/jackc/pgx/v5"
)

// AdminListFilter specifies filtering and pagination criteria for the administrative post list.
type AdminListFilter struct {
	PublicationState *string
	Category         *string
	Kind             *string
	CursorCreatedAt  *time.Time
	CursorID         *string
	Limit            int
}

// SourceWithMetrics embeds a Source along with its most recent checkpoint metrics.
type SourceWithMetrics struct {
	ID                      string              `json:"id"`
	Code                    string              `json:"code"`
	Name                    string              `json:"name"`
	Kind                    domain.SourceKind   `json:"kind"`
	OriginURL               string              `json:"origin_url"`
	FetchURL                string              `json:"fetch_url,omitempty"`
	FetchFormat             string              `json:"fetch_format,omitempty"`
	CollectionEnabled       bool                `json:"collection_enabled"`
	PublicationEnabled      bool                `json:"publication_enabled"`
	RightsStatus            domain.RightsStatus `json:"rights_status"`
	TitleAllowed            bool                `json:"title_allowed"`
	ExcerptAllowed          bool                `json:"excerpt_allowed"`
	ImageAllowed            bool                `json:"image_allowed"`
	TermsURL                string              `json:"terms_url"`
	TermsSHA256             string              `json:"terms_sha256"`
	CheckedAt               *time.Time          `json:"checked_at,omitempty"`
	ReviewExpiresAt         *time.Time          `json:"review_expires_at,omitempty"`
	Attribution             string              `json:"attribution"`
	CheckpointETag          *string             `json:"checkpoint_etag,omitempty"`
	CheckpointLastModified  *string             `json:"checkpoint_last_modified,omitempty"`
	CheckpointLastStatus    *string             `json:"checkpoint_last_status,omitempty"`
	CheckpointFailures      int                 `json:"checkpoint_failures"`
	CheckpointLastSuccessAt *time.Time          `json:"checkpoint_last_success_at,omitempty"`
}

// AdminStore defines the storage methods required by AdminService.
type AdminStore interface {
	AdminListPosts(ctx context.Context, filter AdminListFilter) ([]domain.Post, *string, error)
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
	GetSourceByID(ctx context.Context, id string) (*domain.Source, error)
	ListAllSourcesWithMetrics(ctx context.Context) ([]SourceWithMetrics, error)
	CreateDraftTx(ctx context.Context, tx pgx.Tx, post *domain.Post) error
	GetPostTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error)
	GetPostForUpdateTx(ctx context.Context, tx pgx.Tx, id string) (*domain.Post, error)
	UpdateEditorialTx(ctx context.Context, tx pgx.Tx, post *domain.Post, expectedRevision int) error
	PublishEditorialTx(ctx context.Context, tx pgx.Tx, id string, publishedAt time.Time, seq int64) error
	RejectPostTx(ctx context.Context, tx pgx.Tx, id string, expectedRevision int, now time.Time) error
	SetPostVisibilityTx(ctx context.Context, tx pgx.Tx, id string, visibility domain.Visibility, updatedAt time.Time) error
	BumpFeedRevisionTx(ctx context.Context, tx pgx.Tx, now time.Time) (int64, error)
	RecordForumChangeTx(ctx context.Context, tx pgx.Tx, rec domain.ChangeRecord) error
}

// MutationRunner executes an audited administrative mutation inside a transaction.
type MutationRunner interface {
	Mutate(context.Context, adminapp.Mutation, func(context.Context, pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error)) (adminapp.MutationResult, error)
}

// AdminService coordinates administrative editorial operations on forum publications.
type AdminService struct {
	Store  AdminStore
	Runner MutationRunner
	Config domain.Config
	Clock  func() time.Time
	NewID  func() (string, error)
}

func (s *AdminService) now() time.Time {
	if s != nil && s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// CreateDraftInput contains payload data for creating an editorial draft post.
type CreateDraftInput struct {
	Kind                   string
	Category               string
	Title                  string
	Excerpt                string
	Body                   string
	SourceID               string
	CanonicalURL           string
	FuelProduct            *string
	RegionUF               *string
	RegionMunicipalityIBGE *int
	StationID              *string
}

// EditPostInput contains fields for editing an existing publication.
type EditPostInput struct {
	ID                     string
	Title                  string
	Excerpt                string
	Body                   string
	Category               string
	ExpectedRevision       int
	CommentsEnabled        bool
	Correction             *string
	FuelProduct            *string
	RegionUF               *string
	RegionMunicipalityIBGE *int
	StationID              *string
}

// AdminListInput defines criteria for admin list queries.
type AdminListInput struct {
	PublicationState *string
	Category         *string
	Kind             *string
	CursorCreatedAt  *time.Time
	CursorID         *string
	Limit            int
}

// CreateDraft validates input and saves a new publication in DRAFT state.
func (s *AdminService) CreateDraft(ctx context.Context, m adminapp.Mutation, in CreateDraftInput) (*domain.Post, error) {
	if s == nil || s.Store == nil || s.Runner == nil || s.NewID == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}

	kind, err := domain.ParsePostKind(in.Kind)
	if err != nil {
		return nil, err
	}
	category, err := domain.ParseCategory(in.Category)
	if err != nil {
		return nil, err
	}

	now := s.now()
	postID, err := s.NewID()
	if err != nil {
		return nil, fmt.Errorf("forum: generate post id: %w", err)
	}

	var region *domain.Region
	if in.RegionUF != nil && *in.RegionUF != "" {
		region = &domain.Region{
			UF:               *in.RegionUF,
			MunicipalityIBGE: in.RegionMunicipalityIBGE,
		}
	}

	post := &domain.Post{
		ID:                  postID,
		Kind:                kind,
		Category:            category,
		Title:               strings.TrimSpace(in.Title),
		Excerpt:             strings.TrimSpace(in.Excerpt),
		BodyText:            strings.TrimSpace(in.Body),
		SourceID:            in.SourceID,
		CanonicalURL:        strings.TrimSpace(in.CanonicalURL),
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 0,
		PublicationState:    domain.StateDraft,
		Visibility:          domain.VisibilityVisible,
		Revision:            1,
		CommentsEnabled:     true,
		Region:              region,
		FuelProduct:         in.FuelProduct,
		StationID:           in.StationID,
	}

	// For editorial and news, check source existence
	if post.Kind == domain.KindEditorial || post.Kind == domain.KindNews {
		if strings.TrimSpace(post.SourceID) == "" {
			return nil, domain.ErrMissingSource
		}
		src, err := s.Store.GetSourceByID(ctx, post.SourceID)
		if err != nil {
			return nil, fmt.Errorf("forum: source lookup: %w", err)
		}
		post.Source = src
	}

	if err := post.Validate(); err != nil {
		return nil, err
	}

	changeID, err := s.NewID()
	if err != nil {
		return nil, fmt.Errorf("forum: generate change id: %w", err)
	}

	res, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		if err := s.Store.CreateDraftTx(ctx, tx, post); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		changeRec := domain.ChangeRecord{
			TargetType:     "POST",
			TargetID:       post.ID,
			Revision:       1,
			Action:         "CREATE_DRAFT",
			ActorKind:      "ADMIN",
			ActorAccountID: nil,
			AdminAuditRef:  &m.Key,
			PolicyVersion:  1,
			ReasonCode:     "editorial_draft",
			PriorState:     nil,
			NewState:       string(domain.StateDraft),
			CreatedAt:      now,
		}
		// Pass changeID through context or let store handle UUID
		_ = changeID

		if err := s.Store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		// PRIVACY: Audit view strictly omits BodyText and Excerpt!
		auditJSON, err := json.Marshal(post.AuditView())
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		events := []adminapp.AuditEvent{
			{
				Action:     "admin.forum.post.create",
				TargetType: "forum_post",
				TargetID:   post.ID,
				Reason:     m.Reason,
				Before:     nil,
				After:      auditJSON,
			},
		}

		body, err := json.Marshal(post)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		return adminapp.MutationResult{Status: 201, Body: body}, events, nil
	})
	if err != nil {
		return nil, err
	}

	var created domain.Post
	if err := json.Unmarshal(res.Body, &created); err != nil {
		return post, nil
	}
	return &created, nil
}

// EditPost updates an existing post under optimistic revision concurrency control.
func (s *AdminService) EditPost(ctx context.Context, m adminapp.Mutation, in EditPostInput) (*domain.Post, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}

	category, err := domain.ParseCategory(in.Category)
	if err != nil {
		return nil, err
	}

	now := s.now()

	res, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		current, err := s.Store.GetPostForUpdateTx(ctx, tx, in.ID)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}
		if current.Revision != in.ExpectedRevision {
			return adminapp.MutationResult{}, nil, domain.ErrRevisionMismatch
		}

		beforeAudit := current.AuditView()

		// Apply edits
		current.Title = strings.TrimSpace(in.Title)
		current.Excerpt = strings.TrimSpace(in.Excerpt)
		current.BodyText = strings.TrimSpace(in.Body)
		current.Category = category
		current.CommentsEnabled = in.CommentsEnabled
		current.Correction = in.Correction
		current.FuelProduct = in.FuelProduct
		current.StationID = in.StationID
		current.LocalUpdatedAt = now

		if in.RegionUF != nil && *in.RegionUF != "" {
			current.Region = &domain.Region{
				UF:               *in.RegionUF,
				MunicipalityIBGE: in.RegionMunicipalityIBGE,
			}
		} else {
			current.Region = nil
		}

		if err := current.Validate(); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		if err := s.Store.UpdateEditorialTx(ctx, tx, current, in.ExpectedRevision); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		// Bump feed revision if post was published
		if current.PublicationState == domain.StatePublished {
			if _, err := s.Store.BumpFeedRevisionTx(ctx, tx, now); err != nil {
				return adminapp.MutationResult{}, nil, err
			}
		}

		newRevision := in.ExpectedRevision + 1
		current.Revision = newRevision

		priorStateStr := fmt.Sprintf("rev-%d", in.ExpectedRevision)
		newStateStr := fmt.Sprintf("rev-%d", newRevision)

		changeRec := domain.ChangeRecord{
			TargetType:     "POST",
			TargetID:       current.ID,
			Revision:       newRevision,
			Action:         "EDIT",
			ActorKind:      "ADMIN",
			ActorAccountID: nil,
			AdminAuditRef:  &m.Key,
			PolicyVersion:  1,
			ReasonCode:     "editorial_edit",
			PriorState:     &priorStateStr,
			NewState:       newStateStr,
			CreatedAt:      now,
		}

		if err := s.Store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeJSON, _ := json.Marshal(beforeAudit)
		afterJSON, _ := json.Marshal(current.AuditView())

		events := []adminapp.AuditEvent{
			{
				Action:     "admin.forum.post.edit",
				TargetType: "forum_post",
				TargetID:   current.ID,
				Reason:     m.Reason,
				Before:     beforeJSON,
				After:      afterJSON,
			},
		}

		body, err := json.Marshal(current)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		return adminapp.MutationResult{Status: 200, Body: body}, events, nil
	})
	if err != nil {
		return nil, err
	}

	var updated domain.Post
	if err := json.Unmarshal(res.Body, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// PublishPost transitions an editorial post to PUBLISHED state with monotonic sequence.
func (s *AdminService) PublishPost(ctx context.Context, m adminapp.Mutation, id string) (*domain.Post, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}

	now := s.now()

	res, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		current, err := s.Store.GetPostForUpdateTx(ctx, tx, id)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		if !domain.CanTransition(current.PublicationState, domain.StatePublished) {
			return adminapp.MutationResult{}, nil, domain.ErrInvalidTransition
		}

		// Verify source rights for editorial and news
		if current.Kind == domain.KindEditorial || current.Kind == domain.KindNews {
			src, err := s.Store.GetSourceByID(ctx, current.SourceID)
			if err != nil {
				return adminapp.MutationResult{}, nil, domain.ErrMissingSource
			}
			if src.RightsStatus != domain.RightsApproved || !src.PublicationEnabled || (src.ReviewExpiresAt != nil && src.ReviewExpiresAt.Before(now)) {
				return adminapp.MutationResult{}, nil, domain.ErrMissingRights
			}
		}

		seq, err := s.Store.BumpFeedRevisionTx(ctx, tx, now)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		if err := s.Store.PublishEditorialTx(ctx, tx, id, now, seq); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeAudit := current.AuditView()

		current.PublicationState = domain.StatePublished
		current.FirstPublicationSeq = seq
		current.FirstPublishedAt = &now
		current.LocalUpdatedAt = now
		current.Revision = current.Revision + 1

		priorStateStr := string(beforeAudit.PublicationState)
		changeRec := domain.ChangeRecord{
			TargetType:     "POST",
			TargetID:       current.ID,
			Revision:       current.Revision,
			Action:         "PUBLISH",
			ActorKind:      "ADMIN",
			ActorAccountID: nil,
			AdminAuditRef:  &m.Key,
			PolicyVersion:  1,
			ReasonCode:     "editorial_publish",
			PriorState:     &priorStateStr,
			NewState:       string(domain.StatePublished),
			CreatedAt:      now,
		}

		if err := s.Store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeJSON, _ := json.Marshal(beforeAudit)
		afterJSON, _ := json.Marshal(current.AuditView())

		events := []adminapp.AuditEvent{
			{
				Action:     "admin.forum.post.publish",
				TargetType: "forum_post",
				TargetID:   current.ID,
				Reason:     m.Reason,
				Before:     beforeJSON,
				After:      afterJSON,
			},
		}

		body, err := json.Marshal(current)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		return adminapp.MutationResult{Status: 200, Body: body}, events, nil
	})
	if err != nil {
		return nil, err
	}

	var published domain.Post
	if err := json.Unmarshal(res.Body, &published); err != nil {
		return nil, err
	}
	return &published, nil
}

// SetVisibility toggles the visibility of an existing post.
func (s *AdminService) SetVisibility(ctx context.Context, m adminapp.Mutation, id string, visibility domain.Visibility) (*domain.Post, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}

	now := s.now()

	res, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		current, err := s.Store.GetPostForUpdateTx(ctx, tx, id)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeAudit := current.AuditView()

		if err := s.Store.SetPostVisibilityTx(ctx, tx, id, visibility, now); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		if _, err := s.Store.BumpFeedRevisionTx(ctx, tx, now); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		current.Visibility = visibility
		current.LocalUpdatedAt = now
		current.Revision = current.Revision + 1

		priorStateStr := string(beforeAudit.Visibility)
		changeRec := domain.ChangeRecord{
			TargetType:     "POST",
			TargetID:       current.ID,
			Revision:       current.Revision,
			Action:         "SET_VISIBILITY",
			ActorKind:      "ADMIN",
			ActorAccountID: nil,
			AdminAuditRef:  &m.Key,
			PolicyVersion:  1,
			ReasonCode:     "editorial_visibility",
			PriorState:     &priorStateStr,
			NewState:       string(visibility),
			CreatedAt:      now,
		}

		if err := s.Store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeJSON, _ := json.Marshal(beforeAudit)
		afterJSON, _ := json.Marshal(current.AuditView())

		events := []adminapp.AuditEvent{
			{
				Action:     "admin.forum.post.visibility",
				TargetType: "forum_post",
				TargetID:   current.ID,
				Reason:     m.Reason,
				Before:     beforeJSON,
				After:      afterJSON,
			},
		}

		body, err := json.Marshal(current)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		return adminapp.MutationResult{Status: 200, Body: body}, events, nil
	})
	if err != nil {
		return nil, err
	}

	var updated domain.Post
	if err := json.Unmarshal(res.Body, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// GetPost retrieves post detail for administrative review/preview.
func (s *AdminService) GetPost(ctx context.Context, id string) (*domain.Post, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}
	return s.Store.GetPostByID(ctx, id)
}

// ListPosts retrieves a keyset-paginated slice of posts for administrative management.
func (s *AdminService) ListPosts(ctx context.Context, in AdminListInput) ([]domain.Post, *string, error) {
	if s == nil || s.Store == nil {
		return nil, nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, nil, domain.ErrForumDisabled
	}

	filter := AdminListFilter{
		PublicationState: in.PublicationState,
		Category:         in.Category,
		Kind:             in.Kind,
		CursorCreatedAt:  in.CursorCreatedAt,
		CursorID:         in.CursorID,
		Limit:            in.Limit,
	}

	return s.Store.AdminListPosts(ctx, filter)
}

// ListSources returns all candidate and approved sources with their latest checkpoint metrics.
func (s *AdminService) ListSources(ctx context.Context) ([]SourceWithMetrics, error) {
	if s == nil || s.Store == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}
	return s.Store.ListAllSourcesWithMetrics(ctx)
}

// RejectPost transitions a candidate or draft publication to REJECTED state.
func (s *AdminService) RejectPost(ctx context.Context, m adminapp.Mutation, id string, expectedRevision int) (*domain.Post, error) {
	if s == nil || s.Store == nil || s.Runner == nil {
		return nil, domain.ErrForumDisabled
	}
	if !s.Config.AdminAllowed() {
		return nil, domain.ErrForumDisabled
	}

	now := s.now()

	res, err := s.Runner.Mutate(ctx, m, func(ctx context.Context, tx pgx.Tx) (adminapp.MutationResult, []adminapp.AuditEvent, error) {
		current, err := s.Store.GetPostForUpdateTx(ctx, tx, id)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		if expectedRevision > 0 && current.Revision != expectedRevision {
			return adminapp.MutationResult{}, nil, domain.ErrRevisionMismatch
		}

		if !domain.CanTransition(current.PublicationState, domain.StateRejected) {
			return adminapp.MutationResult{}, nil, domain.ErrInvalidTransition
		}

		if err := s.Store.RejectPostTx(ctx, tx, id, current.Revision, now); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeAudit := current.AuditView()

		current.PublicationState = domain.StateRejected
		current.Revision = current.Revision + 1
		current.LocalUpdatedAt = now

		priorStateStr := string(beforeAudit.PublicationState)
		changeRec := domain.ChangeRecord{
			TargetType:     "POST",
			TargetID:       current.ID,
			Revision:       current.Revision,
			Action:         "REJECT",
			ActorKind:      "ADMIN",
			ActorAccountID: nil,
			AdminAuditRef:  &m.Key,
			PolicyVersion:  1,
			ReasonCode:     "editorial_reject",
			PriorState:     &priorStateStr,
			NewState:       string(domain.StateRejected),
			CreatedAt:      now,
		}

		if err := s.Store.RecordForumChangeTx(ctx, tx, changeRec); err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		beforeJSON, _ := json.Marshal(beforeAudit)
		afterJSON, _ := json.Marshal(current.AuditView())

		events := []adminapp.AuditEvent{
			{
				Action:     "admin.forum.post.reject",
				TargetType: "forum_post",
				TargetID:   current.ID,
				Reason:     m.Reason,
				Before:     beforeJSON,
				After:      afterJSON,
			},
		}

		body, err := json.Marshal(current)
		if err != nil {
			return adminapp.MutationResult{}, nil, err
		}

		return adminapp.MutationResult{Status: 200, Body: body}, events, nil
	})
	if err != nil {
		return nil, err
	}

	var rejected domain.Post
	if err := json.Unmarshal(res.Body, &rejected); err != nil {
		return nil, err
	}
	return &rejected, nil
}

// Helper to check valid UUID
func ValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
