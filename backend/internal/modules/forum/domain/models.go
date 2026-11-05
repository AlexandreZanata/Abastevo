package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Category represents the primary forum taxonomy category.
type Category string

const (
	CategoryFuels         Category = "FUELS"
	CategoryElectric      Category = "ELECTRIC"
	CategoryTrafficRules  Category = "TRAFFIC_RULES"
	CategoryVehiclesTech  Category = "VEHICLES_TECH"
	CategoryRoadsMobility Category = "ROADS_MOBILITY"
	CategoryDriverEconomy Category = "DRIVER_ECONOMY"
	CategoryAbastevoData  Category = "ABASTEVO_DATA"
)

// ParseCategory validates and converts a string into a Category.
func ParseCategory(s string) (Category, error) {
	switch Category(s) {
	case CategoryFuels,
		CategoryElectric,
		CategoryTrafficRules,
		CategoryVehiclesTech,
		CategoryRoadsMobility,
		CategoryDriverEconomy,
		CategoryAbastevoData:
		return Category(s), nil
	default:
		return "", ErrInvalidCategory
	}
}

// PostKind represents the publication type.
type PostKind string

const (
	KindEditorial PostKind = "EDITORIAL"
	KindNews      PostKind = "NEWS"
	KindTopic     PostKind = "TOPIC"
	KindBulletin  PostKind = "BULLETIN"
)

// ParsePostKind validates and converts a string into a PostKind.
func ParsePostKind(s string) (PostKind, error) {
	switch PostKind(s) {
	case KindEditorial, KindNews, KindTopic, KindBulletin:
		return PostKind(s), nil
	default:
		return "", ErrInvalidPostKind
	}
}

// PublicationState represents the editorial and moderation lifecycle state.
type PublicationState string

const (
	StateDraft         PublicationState = "DRAFT"
	StatePendingReview PublicationState = "PENDING_REVIEW"
	StatePublished     PublicationState = "PUBLISHED"
	StateRejected      PublicationState = "REJECTED"
	StateRemoved       PublicationState = "REMOVED"
)

// ParsePublicationState validates and converts a string into a PublicationState.
func ParsePublicationState(s string) (PublicationState, error) {
	switch PublicationState(s) {
	case StateDraft, StatePendingReview, StatePublished, StateRejected, StateRemoved:
		return PublicationState(s), nil
	default:
		return "", ErrInvalidPublicationState
	}
}

// Visibility governs independent public visibility of a post.
type Visibility string

const (
	VisibilityVisible Visibility = "VISIBLE"
	VisibilityHidden  Visibility = "HIDDEN"
)

// RightsStatus represents the legal rights review status for external sources.
type RightsStatus string

const (
	RightsPending  RightsStatus = "PENDING"
	RightsApproved RightsStatus = "APPROVED"
	RightsDenied   RightsStatus = "DENIED"
)

// SourceKind represents the publisher origin classification.
type SourceKind string

const (
	SourceKindAbastevo   SourceKind = "ABASTEVO"
	SourceKindGovernment SourceKind = "GOVERNMENT"
	SourceKindPublisher  SourceKind = "PUBLISHER"
	SourceKindDataset    SourceKind = "DATASET"
)

// Source represents an approved or candidate publication source.
type Source struct {
	ID                 string
	Code               string
	Name               string
	Kind               SourceKind
	OriginURL          string
	FetchURL           string
	FetchFormat        string
	CollectionEnabled  bool
	PublicationEnabled bool
	RightsStatus       RightsStatus
	TitleAllowed       bool
	ExcerptAllowed     bool
	ImageAllowed       bool
	TermsURL           string
	TermsSHA256        string
	CheckedAt          *time.Time
	ReviewExpiresAt    *time.Time
	Attribution        string
}

// Region describes coarse subject-matter geography.
type Region struct {
	UF               string
	MunicipalityIBGE *int
}

// SourceAttribution contains public source attribution fields.
type SourceAttribution struct {
	Name        string
	URL         string
	LicenseURL  *string
	Attribution string
}

// CategoryItem represents taxonomy category presentation.
type CategoryItem struct {
	Code        Category
	Name        string
	Description string
}

// CurrentTaxonomyVersion is the current normative taxonomy version.
const CurrentTaxonomyVersion = 1

// DefaultCategories returns the normative 7 forum taxonomy categories.
func DefaultCategories() []CategoryItem {
	return []CategoryItem{
		{
			Code:        CategoryFuels,
			Name:        "Combustíveis",
			Description: "Preços, tributação, composição e qualidade de combustíveis.",
		},
		{
			Code:        CategoryElectric,
			Name:        "Eletromobilidade",
			Description: "Recarga elétrica, conectores, tarifas e infraestrutura.",
		},
		{
			Code:        CategoryTrafficRules,
			Name:        "Legislação de Trânsito",
			Description: "Normas, sinalização, fiscalização e regulamentação.",
		},
		{
			Code:        CategoryVehiclesTech,
			Name:        "Veículos e Tecnologia",
			Description: "Manutenção, consumo, eficiência energética e mecânica.",
		},
		{
			Code:        CategoryRoadsMobility,
			Name:        "Rodovias e Mobilidade",
			Description: "Condições de estradas, pedágios, rotas e tráfego.",
		},
		{
			Code:        CategoryDriverEconomy,
			Name:        "Economia do Motorista",
			Description: "Gestão de custos, cálculo de autonomia e dicas financeiras.",
		},
		{
			Code:        CategoryAbastevoData,
			Name:        "Dados Abastevo",
			Description: "Transparência, estatísticas de mercado e análises agregadas.",
		},
	}
}

// Post is the aggregate root for forum publications.
type Post struct {
	ID                  string
	Kind                PostKind
	Category            Category
	Title               string
	Excerpt             string
	BodyText            string
	SourceID            string
	Source              *Source
	SourceAttribution   *SourceAttribution
	CanonicalURL        string
	ExternalID          string
	AuthorAccountID     string
	PublicAlias         *string
	SourcePublishedAt   *time.Time
	SourceUpdatedAt     *time.Time
	FirstPublishedAt    *time.Time
	LocalCreatedAt      time.Time
	LocalUpdatedAt      time.Time
	FirstPublicationSeq int64
	PublicationState    PublicationState
	Visibility          Visibility
	Revision            int
	CommentsEnabled     bool
	Correction          *string
	Region              *Region
	FuelProduct         *string
	StationID           *string
	RightsSnapshot      *string
}

// CanTransition validates whether a publication state transition is allowed.
func CanTransition(from, to PublicationState) bool {
	if from == StateRemoved {
		return false // Terminal state
	}
	switch from {
	case StateDraft:
		return to == StatePendingReview || to == StatePublished || to == StateRejected || to == StateRemoved
	case StatePendingReview:
		return to == StatePublished || to == StateRejected || to == StateDraft || to == StateRemoved
	case StatePublished:
		return to == StateRemoved // Edits/hides update revision/visibility, not un-publishing to draft
	case StateRejected:
		return to == StateDraft || to == StateRemoved
	default:
		return false
	}
}

// Validate checks post business rules, UTF-8 scalars, and kind-specific constraints.
func (p *Post) Validate() error {
	trimmedTitle := strings.TrimSpace(p.Title)
	if trimmedTitle == "" {
		return ErrTitleEmpty
	}
	if !utf8.ValidString(p.Title) {
		return ErrInvalidUTF8
	}
	if utf8.RuneCountInString(trimmedTitle) > 120 {
		return ErrTitleTooLong
	}

	if p.Excerpt != "" {
		if !utf8.ValidString(p.Excerpt) {
			return ErrInvalidUTF8
		}
		if utf8.RuneCountInString(p.Excerpt) > 400 {
			return ErrExcerptTooLong
		}
	}

	if p.BodyText != "" {
		if !utf8.ValidString(p.BodyText) {
			return ErrInvalidUTF8
		}
		if utf8.RuneCountInString(p.BodyText) > 4000 {
			return ErrBodyTooLong
		}
	}

	if _, err := ParseCategory(string(p.Category)); err != nil {
		return err
	}
	if _, err := ParsePostKind(string(p.Kind)); err != nil {
		return err
	}

	// Kind-specific checks
	switch p.Kind {
	case KindEditorial:
		if strings.TrimSpace(p.SourceID) == "" && p.Source == nil {
			return ErrMissingSource
		}
	case KindNews:
		if strings.TrimSpace(p.SourceID) == "" && p.Source == nil {
			return ErrMissingSource
		}
		if p.SourcePublishedAt == nil {
			return ErrMissingSourceDate
		}
		if p.Source != nil && p.Source.RightsStatus != RightsApproved {
			return ErrMissingRights
		}
	case KindTopic:
		if strings.TrimSpace(p.SourceID) != "" {
			return ErrTopicCannotHaveSource
		}
		if strings.TrimSpace(p.AuthorAccountID) == "" && p.PublicationState != StateRemoved {
			return ErrMissingAuthor
		}
	}

	return nil
}

// IsPubliclyVisible returns true if the post is published and visible to anonymous readers.
func (p *Post) IsPubliclyVisible() bool {
	return p.PublicationState == StatePublished && p.Visibility == VisibilityVisible
}

// PostSummary is the public projection for feed listings.
type PostSummary struct {
	ID                string
	Kind              PostKind
	Category          Category
	Title             string
	Excerpt           *string
	Source            *SourceAttribution
	PublicAlias       *string
	SourcePublishedAt *time.Time
	FirstPublishedAt  time.Time
	UpdatedAt         time.Time
	Revision          int
	Region            *Region
	FuelProduct       *string
	StationID         *string
}

// PostDetail is the public projection for single post views.
type PostDetail struct {
	ID                string
	Kind              PostKind
	Category          Category
	Title             string
	Excerpt           *string
	BodyText          *string
	Source            *SourceAttribution
	PublicAlias       *string
	SourcePublishedAt *time.Time
	FirstPublishedAt  time.Time
	UpdatedAt         time.Time
	Revision          int
	CommentsEnabled   bool
	Correction        *string
	Region            *Region
	FuelProduct       *string
	StationID         *string
}

// FeedPage represents a paginated set of public summaries.
type FeedPage struct {
	Items      []PostSummary
	NextCursor *string
	Snapshot   string
	FetchedAt  time.Time
}

// ToSummary converts a post to a public summary, failing if not publicly visible.
func (p *Post) ToSummary() (*PostSummary, error) {
	if !p.IsPubliclyVisible() {
		return nil, ErrNotPubliclyVisible
	}

	var excerpt *string
	if p.Excerpt != "" {
		cp := p.Excerpt
		excerpt = &cp
	}

	srcAttr := p.SourceAttribution
	if srcAttr == nil && p.Source != nil {
		srcAttr = &SourceAttribution{
			Name:        p.Source.Name,
			URL:         p.Source.OriginURL,
			Attribution: p.Source.Attribution,
		}
	}

	firstPub := time.Time{}
	if p.FirstPublishedAt != nil {
		firstPub = *p.FirstPublishedAt
	}

	return &PostSummary{
		ID:                p.ID,
		Kind:              p.Kind,
		Category:          p.Category,
		Title:             p.Title,
		Excerpt:           excerpt,
		Source:            srcAttr,
		PublicAlias:       p.PublicAlias,
		SourcePublishedAt: p.SourcePublishedAt,
		FirstPublishedAt:  firstPub,
		UpdatedAt:         p.LocalUpdatedAt,
		Revision:          p.Revision,
		Region:            p.Region,
		FuelProduct:       p.FuelProduct,
		StationID:         p.StationID,
	}, nil
}

// ToDetail converts a post to a public detail view, failing if not publicly visible.
func (p *Post) ToDetail() (*PostDetail, error) {
	if !p.IsPubliclyVisible() {
		return nil, ErrNotPubliclyVisible
	}

	var excerpt *string
	if p.Excerpt != "" {
		cp := p.Excerpt
		excerpt = &cp
	}

	var body *string
	if p.BodyText != "" {
		cp := p.BodyText
		body = &cp
	}

	srcAttr := p.SourceAttribution
	if srcAttr == nil && p.Source != nil {
		srcAttr = &SourceAttribution{
			Name:        p.Source.Name,
			URL:         p.Source.OriginURL,
			Attribution: p.Source.Attribution,
		}
	}

	firstPub := time.Time{}
	if p.FirstPublishedAt != nil {
		firstPub = *p.FirstPublishedAt
	}

	return &PostDetail{
		ID:                p.ID,
		Kind:              p.Kind,
		Category:          p.Category,
		Title:             p.Title,
		Excerpt:           excerpt,
		BodyText:          body,
		Source:            srcAttr,
		PublicAlias:       p.PublicAlias,
		SourcePublishedAt: p.SourcePublishedAt,
		FirstPublishedAt:  firstPub,
		UpdatedAt:         p.LocalUpdatedAt,
		Revision:          p.Revision,
		CommentsEnabled:   p.CommentsEnabled,
		Correction:        p.Correction,
		Region:            p.Region,
		FuelProduct:       p.FuelProduct,
		StationID:         p.StationID,
	}, nil
}

// ChangeRecord represents an audited change record in forum_changes.
type ChangeRecord struct {
	TargetType     string
	TargetID       string
	Revision       int
	Action         string
	ActorKind      string
	ActorAccountID *string
	AdminAuditRef  *string
	PolicyVersion  int
	ReasonCode     string
	PriorState     *string
	NewState       string
	CreatedAt      time.Time
}

// PostAuditView is the privacy-safe, sanitized representation of a post for audit events.
// CRITICAL PRIVACY RULE: BodyText and Excerpt are strictly excluded to avoid leaking
// editorial draft text or PII into admin_audit logs.
type PostAuditView struct {
	ID                  string     `json:"id"`
	Kind                string     `json:"kind"`
	Category            string     `json:"category"`
	Title               string     `json:"title"`
	SourceID            string     `json:"source_id,omitempty"`
	AuthorAccountID     string     `json:"author_account_id,omitempty"`
	FirstPublicationSeq int64      `json:"first_publication_seq,omitempty"`
	PublicationState    string     `json:"publication_state"`
	Visibility          string     `json:"visibility"`
	Revision            int        `json:"revision"`
	FirstPublishedAt    *time.Time `json:"first_published_at,omitempty"`
	LocalUpdatedAt      time.Time  `json:"local_updated_at"`
}

// AuditView returns the privacy-sanitized view of a post for administrative audit logs.
func (p *Post) AuditView() PostAuditView {
	return PostAuditView{
		ID:                  p.ID,
		Kind:                string(p.Kind),
		Category:            string(p.Category),
		Title:               p.Title,
		SourceID:            p.SourceID,
		AuthorAccountID:     p.AuthorAccountID,
		FirstPublicationSeq: p.FirstPublicationSeq,
		PublicationState:    string(p.PublicationState),
		Visibility:          string(p.Visibility),
		Revision:            p.Revision,
		FirstPublishedAt:    p.FirstPublishedAt,
		LocalUpdatedAt:      p.LocalUpdatedAt,
	}
}
