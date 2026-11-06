package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// Handler exposes public read endpoints for forum publications and taxonomy.
type Handler struct {
	Service *application.Service
	Secrets []byte
}

// NewHandler constructs a public forum Handler.
func NewHandler(service *application.Service, secrets []byte) *Handler {
	return &Handler{
		Service: service,
		Secrets: secrets,
	}
}

// RegisterRoutes registers public forum read routes on a chi router.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/forum/categories", h.Categories)
	r.Get("/v1/forum/posts", h.Feed)
	r.Get("/v1/forum/posts/{post_id}", h.Detail)
}

// Categories handles anonymous taxonomy category discovery.
func (h Handler) Categories(w http.ResponseWriter, r *http.Request) {
	res, err := h.Service.GetCategories(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	items := make([]CategoryItemDTO, 0, len(res.Items))
	for _, item := range res.Items {
		items = append(items, CategoryItemDTO{
			Code:        string(item.Code),
			Name:        item.Name,
			Description: item.Description,
		})
	}
	body, _ := json.Marshal(CategoriesResponseDTO{
		Items:           items,
		TaxonomyVersion: res.TaxonomyVersion,
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

// Feed handles anonymous paginated reading of published forum summaries.
func (h Handler) Feed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	allowedFilters := map[string]bool{
		"category": true, "kind": true, "limit": true, "cursor": true,
		"uf": true, "municipality_ibge": true, "fuel_product": true,
		"station_id": true,
	}
	for key, values := range q {
		if !allowedFilters[key] {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "unknown query filter",
				[]httpapi.Detail{{Field: key, Code: "unknown"}})
			return
		}
		if len(values) > 1 {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "duplicate query filter",
				[]httpapi.Detail{{Field: key, Code: "duplicate"}})
			return
		}
	}

	limit := 20
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		n, err := strconv.Atoi(rawLimit)
		if err != nil || n < 1 || n > 50 {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-limit", "limit must be 1..50",
				[]httpapi.Detail{{Field: "limit", Code: "range"}})
			return
		}
		limit = n
	}

	var catFilter *domain.Category
	var catStr string
	if rawCat := strings.TrimSpace(q.Get("category")); rawCat != "" {
		cat, err := domain.ParseCategory(rawCat)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid category filter",
				[]httpapi.Detail{{Field: "category", Code: "invalid"}})
			return
		}
		catFilter = &cat
		catStr = string(cat)
	}

	var kindFilter *domain.PostKind
	var kindStr string
	if rawKind := strings.TrimSpace(q.Get("kind")); rawKind != "" {
		kind, err := domain.ParsePostKind(rawKind)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid kind filter",
				[]httpapi.Detail{{Field: "kind", Code: "invalid"}})
			return
		}
		kindFilter = &kind
		kindStr = string(kind)
	}

	var ufFilter *string
	var ufStr string
	if rawUF := strings.TrimSpace(q.Get("uf")); rawUF != "" {
		uf, err := domain.ParseFeedUF(rawUF)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid uf filter",
				[]httpapi.Detail{{Field: "uf", Code: "invalid"}})
			return
		}
		ufFilter = &uf
		ufStr = uf
	}

	var municipalityFilter *int
	var municipalityStr string
	if rawIBGE := strings.TrimSpace(q.Get("municipality_ibge")); rawIBGE != "" {
		ibge, err := domain.ParseFeedMunicipalityIBGE(rawIBGE)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid municipality_ibge filter",
				[]httpapi.Detail{{Field: "municipality_ibge", Code: "invalid"}})
			return
		}
		municipalityFilter = &ibge
		municipalityStr = rawIBGE
	}

	if err := domain.ValidateFeedRegion(ufStr, municipalityFilter); err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "municipality_ibge does not match uf",
			[]httpapi.Detail{{Field: "municipality_ibge", Code: "invalid"}})
		return
	}

	var fuelFilter *string
	var fuelStr string
	if rawFuel := strings.TrimSpace(q.Get("fuel_product")); rawFuel != "" {
		fuel, err := domain.ParseFeedFuelProduct(rawFuel)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid fuel_product filter",
				[]httpapi.Detail{{Field: "fuel_product", Code: "invalid"}})
			return
		}
		fuelFilter = &fuel
		fuelStr = fuel
	}

	var stationFilter *string
	var stationStr string
	if rawStation := strings.TrimSpace(q.Get("station_id")); rawStation != "" {
		station, err := domain.ParseFeedStationID(rawStation)
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid station_id filter",
				[]httpapi.Detail{{Field: "station_id", Code: "invalid"}})
			return
		}
		stationFilter = &station
		stationStr = station
	}

	fh := httpapi.FilterHash("forum_feed", catStr, kindStr, ufStr, municipalityStr, fuelStr, stationStr, strconv.Itoa(limit))

	var internalCursor *string
	if rawCursor := strings.TrimSpace(q.Get("cursor")); rawCursor != "" {
		if len(rawCursor) > 512 {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-cursor", "cursor too long",
				[]httpapi.Detail{{Field: "cursor", Code: "too-long"}})
			return
		}
		afterPos, err := httpapi.ParseCursor(h.Secrets, rawCursor, fh, time.Now())
		if err != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-cursor", "cursor invalid, expired or changed",
				[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
			return
		}
		internalCursor = &afterPos
	}

	page, err := h.Service.GetFeed(r.Context(), application.ListPublicPostsQuery{
		Category:         catFilter,
		Kind:             kindFilter,
		UF:               ufFilter,
		MunicipalityIBGE: municipalityFilter,
		FuelProduct:      fuelFilter,
		StationID:        stationFilter,
		Limit:            limit,
		Cursor:           internalCursor,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	var nextCursor *string
	if page.NextCursor != nil && *page.NextCursor != "" {
		sealed := httpapi.Seal(h.Secrets, fh, *page.NextCursor, time.Now())
		nextCursor = &sealed
	}

	items := make([]PostSummaryDTO, 0, len(page.Items))
	for _, it := range page.Items {
		items = append(items, mapSummaryToDTO(it))
	}

	body, _ := json.Marshal(FeedResponseDTO{
		Items:      items,
		NextCursor: nextCursor,
		Snapshot:   page.Snapshot,
		FetchedAt:  formatRFC3339(page.FetchedAt),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

// Detail handles anonymous reading of full content for a single published post.
func (h Handler) Detail(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimSpace(chi.URLParam(r, "post_id"))
	if !isUUID(postID) {
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-id", "invalid post identifier",
			[]httpapi.Detail{{Field: "post_id", Code: "invalid"}})
		return
	}

	detail, err := h.Service.GetPostDetail(r.Context(), strings.ToLower(postID))
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	body, _ := json.Marshal(PostDetailResponseDTO{
		Post: mapDetailToDTO(detail),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrForumDisabled):
		httpapi.WriteError(w, r, http.StatusNotFound, "forum.disabled", "forum reads are off", nil)
	case errors.Is(err, domain.ErrPostNotFound), errors.Is(err, domain.ErrNotPubliclyVisible):
		httpapi.WriteError(w, r, http.StatusNotFound, "forum.not-found", "post not found", nil)
	case errors.Is(err, domain.ErrInvalidCursor):
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-cursor", "cursor invalid, expired or changed", nil)
	case errors.Is(err, domain.ErrInvalidCategory), errors.Is(err, domain.ErrInvalidPostKind),
		errors.Is(err, domain.ErrInvalidRegion), errors.Is(err, domain.ErrInvalidFuelProduct),
		errors.Is(err, domain.ErrInvalidStationID):
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-filter", "invalid filter", nil)
	default:
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.unavailable", "service unavailable", nil)
	}
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if r != '-' {
				return false
			}
		case (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'):
		default:
			return false
		}
	}
	return true
}

// SourceAttributionDTO mirrors OpenAPI ForumSourceAttribution.
type SourceAttributionDTO struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	LicenseURL  *string `json:"license_url"`
	Attribution *string `json:"attribution"`
}

// RegionDTO mirrors OpenAPI ForumPostRegion.
type RegionDTO struct {
	UF               string `json:"uf"`
	MunicipalityIBGE *int   `json:"municipality_ibge"`
}

// PostSummaryDTO mirrors OpenAPI ForumPostSummary.
type PostSummaryDTO struct {
	ID                string                `json:"id"`
	Kind              string                `json:"kind"`
	Category          string                `json:"category"`
	Title             string                `json:"title"`
	Excerpt           *string               `json:"excerpt"`
	Source            *SourceAttributionDTO `json:"source"`
	PublicAlias       *string               `json:"public_alias"`
	SourcePublishedAt *string               `json:"source_published_at"`
	FirstPublishedAt  string                `json:"first_published_at"`
	UpdatedAt         string                `json:"updated_at"`
	Revision          int                   `json:"revision"`
	Region            *RegionDTO            `json:"region"`
	FuelProduct       *string               `json:"fuel_product"`
	StationID         *string               `json:"station_id"`
}

// BulletinRevisionDTO mirrors OpenAPI ForumBulletinRevision.
type BulletinRevisionDTO struct {
	RevisionID         string `json:"revision_id"`
	SurveyStart        string `json:"survey_start"`
	SurveyEnd          string `json:"survey_end"`
	PublishedAt        string `json:"published_at"`
	SourceURL          string `json:"source_url"`
	SourceChecksum     string `json:"source_checksum"`
	ParserVersion      string `json:"parser_version"`
	SumMilliBrl        int64  `json:"sum_milli_brl"`
	ObservationCount   int64  `json:"observation_count"`
	SampledStations    int64  `json:"sampled_stations"`
	SourceObservations int64  `json:"source_observations"`
	SourceStations     int64  `json:"source_stations"`
	MeanNumerator      int64  `json:"mean_numerator"`
	MeanDenominator    int64  `json:"mean_denominator"`
	Mean               string `json:"mean"`
}

// BulletinProvenanceDTO mirrors OpenAPI ForumBulletinProvenance.
type BulletinProvenanceDTO struct {
	MethodologyVersion string              `json:"methodology_version"`
	TemplateVersion    string              `json:"template_version"`
	Fuel               string              `json:"fuel"`
	Unit               string              `json:"unit"`
	Current            BulletinRevisionDTO `json:"current"`
	Previous           BulletinRevisionDTO `json:"previous"`
}

// PostDetailDTO mirrors OpenAPI ForumPostDetail.
type PostDetailDTO struct {
	ID                string                 `json:"id"`
	Kind              string                 `json:"kind"`
	Category          string                 `json:"category"`
	Title             string                 `json:"title"`
	Excerpt           *string                `json:"excerpt"`
	BodyText          *string                `json:"body_text"`
	Source            *SourceAttributionDTO  `json:"source"`
	PublicAlias       *string                `json:"public_alias"`
	SourcePublishedAt *string                `json:"source_published_at"`
	FirstPublishedAt  string                 `json:"first_published_at"`
	UpdatedAt         string                 `json:"updated_at"`
	Revision          int                    `json:"revision"`
	CommentsEnabled   bool                   `json:"comments_enabled"`
	Correction        *string                `json:"correction"`
	Region            *RegionDTO             `json:"region"`
	FuelProduct       *string                `json:"fuel_product"`
	StationID         *string                `json:"station_id"`
	Bulletin          *BulletinProvenanceDTO `json:"bulletin"`
	SupersedesPostID  *string                `json:"supersedes_post_id"`
}

// CategoryItemDTO mirrors OpenAPI ForumCategoryItem.
type CategoryItemDTO struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// CategoriesResponseDTO mirrors OpenAPI ForumCategoriesResponse.
type CategoriesResponseDTO struct {
	Items           []CategoryItemDTO `json:"items"`
	TaxonomyVersion int               `json:"taxonomy_version"`
}

// FeedResponseDTO mirrors OpenAPI ForumFeedResponse.
type FeedResponseDTO struct {
	Items      []PostSummaryDTO `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	Snapshot   string           `json:"snapshot"`
	FetchedAt  string           `json:"fetched_at"`
}

// PostDetailResponseDTO mirrors OpenAPI ForumPostDetailResponse.
type PostDetailResponseDTO struct {
	Post PostDetailDTO `json:"post"`
}

func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatRFC3339Ptr(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func mapSourceAttributionToDTO(sa *domain.SourceAttribution) *SourceAttributionDTO {
	if sa == nil {
		return nil
	}
	var attr *string
	if sa.Attribution != "" {
		cp := sa.Attribution
		attr = &cp
	}
	return &SourceAttributionDTO{
		Name:        sa.Name,
		URL:         sa.URL,
		LicenseURL:  sa.LicenseURL,
		Attribution: attr,
	}
}

func mapRegionToDTO(r *domain.Region) *RegionDTO {
	if r == nil {
		return nil
	}
	return &RegionDTO{
		UF:               r.UF,
		MunicipalityIBGE: r.MunicipalityIBGE,
	}
}

func mapSummaryToDTO(s domain.PostSummary) PostSummaryDTO {
	return PostSummaryDTO{
		ID:                s.ID,
		Kind:              string(s.Kind),
		Category:          string(s.Category),
		Title:             s.Title,
		Excerpt:           s.Excerpt,
		Source:            mapSourceAttributionToDTO(s.Source),
		PublicAlias:       s.PublicAlias,
		SourcePublishedAt: formatRFC3339Ptr(s.SourcePublishedAt),
		FirstPublishedAt:  formatRFC3339(s.FirstPublishedAt),
		UpdatedAt:         formatRFC3339(s.UpdatedAt),
		Revision:          s.Revision,
		Region:            mapRegionToDTO(s.Region),
		FuelProduct:       s.FuelProduct,
		StationID:         s.StationID,
	}
}

func mapDetailToDTO(d *domain.PostDetail) PostDetailDTO {
	return PostDetailDTO{
		ID:                d.ID,
		Kind:              string(d.Kind),
		Category:          string(d.Category),
		Title:             d.Title,
		Excerpt:           d.Excerpt,
		BodyText:          d.BodyText,
		Source:            mapSourceAttributionToDTO(d.Source),
		PublicAlias:       d.PublicAlias,
		SourcePublishedAt: formatRFC3339Ptr(d.SourcePublishedAt),
		FirstPublishedAt:  formatRFC3339(d.FirstPublishedAt),
		UpdatedAt:         formatRFC3339(d.UpdatedAt),
		Revision:          d.Revision,
		CommentsEnabled:   d.CommentsEnabled,
		Correction:        d.Correction,
		Region:            mapRegionToDTO(d.Region),
		FuelProduct:       d.FuelProduct,
		StationID:         d.StationID,
		Bulletin:          mapBulletinToDTO(d.Bulletin),
		SupersedesPostID:  d.SupersedesPostID,
	}
}

func mapBulletinToDTO(b *domain.BulletinProvenance) *BulletinProvenanceDTO {
	if b == nil {
		return nil
	}
	return &BulletinProvenanceDTO{
		MethodologyVersion: b.MethodologyVersion,
		TemplateVersion:    b.TemplateVersion,
		Fuel:               b.Fuel,
		Unit:               b.Unit,
		Current:            mapBulletinRevisionToDTO(b.Current),
		Previous:           mapBulletinRevisionToDTO(b.Previous),
	}
}

func mapBulletinRevisionToDTO(r domain.BulletinRevisionProvenance) BulletinRevisionDTO {
	return BulletinRevisionDTO{
		RevisionID:         r.RevisionID,
		SurveyStart:        r.SurveyStart,
		SurveyEnd:          r.SurveyEnd,
		PublishedAt:        r.PublishedAt,
		SourceURL:          r.SourceURL,
		SourceChecksum:     r.SourceChecksum,
		ParserVersion:      r.ParserVersion,
		SumMilliBrl:        r.SumMilliBrl,
		ObservationCount:   r.ObservationCount,
		SampledStations:    r.SampledStations,
		SourceObservations: r.SourceObservations,
		SourceStations:     r.SourceStations,
		MeanNumerator:      r.MeanNumerator,
		MeanDenominator:    r.MeanDenominator,
		Mean:               r.Mean,
	}
}
