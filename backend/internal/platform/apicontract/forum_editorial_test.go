package apicontract

import (
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestForumEditorialContractStructure(t *testing.T) {
	doc := loadSpec(t)

	// Check ForumCategory schema
	catSchema, ok := doc.Components.Schemas["ForumCategory"]
	if !ok {
		t.Fatal("ForumCategory schema missing from spec")
	}
	expectedCats := []string{
		"FUELS",
		"ELECTRIC",
		"TRAFFIC_RULES",
		"VEHICLES_TECH",
		"ROADS_MOBILITY",
		"DRIVER_ECONOMY",
		"ABASTEVO_DATA",
	}
	var actualCats []string
	for _, item := range catSchema.Value.Enum {
		if s, ok := item.(string); ok {
			actualCats = append(actualCats, s)
		}
	}
	for _, exp := range expectedCats {
		if !slices.Contains(actualCats, exp) {
			t.Errorf("ForumCategory missing enum value %q", exp)
		}
	}

	// Check ForumPostKind schema
	kindSchema, ok := doc.Components.Schemas["ForumPostKind"]
	if !ok {
		t.Fatal("ForumPostKind schema missing from spec")
	}
	expectedKinds := []string{
		"EDITORIAL",
		"NEWS",
		"TOPIC",
		"BULLETIN",
	}
	var actualKinds []string
	for _, item := range kindSchema.Value.Enum {
		if s, ok := item.(string); ok {
			actualKinds = append(actualKinds, s)
		}
	}
	for _, exp := range expectedKinds {
		if !slices.Contains(actualKinds, exp) {
			t.Errorf("ForumPostKind missing enum value %q", exp)
		}
	}

	// Check required schemas
	requiredSchemas := []string{
		"ForumFuelProduct",
		"ForumBulletinProvenance",
		"ForumBulletinRevision",
		"ForumSourceAttribution",
		"ForumPostRegion",
		"ForumPostSummary",
		"ForumPostDetail",
		"ForumCategoryItem",
		"ForumCategoriesResponse",
		"ForumFeedResponse",
		"ForumPostDetailResponse",
	}
	for _, name := range requiredSchemas {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("missing required schema %q", name)
		}
	}

	// Check required endpoints
	categoriesPath := doc.Paths.Find("/v1/forum/categories")
	if categoriesPath == nil || categoriesPath.Get == nil {
		t.Fatal("missing GET /v1/forum/categories endpoint")
	}
	if !slices.Contains(categoriesPath.Get.Tags, "forum") {
		t.Errorf("GET /v1/forum/categories missing 'forum' tag, got %v", categoriesPath.Get.Tags)
	}

	postsPath := doc.Paths.Find("/v1/forum/posts")
	if postsPath == nil || postsPath.Get == nil {
		t.Fatal("missing GET /v1/forum/posts endpoint")
	}
	if !slices.Contains(postsPath.Get.Tags, "forum") {
		t.Errorf("GET /v1/forum/posts missing 'forum' tag, got %v", postsPath.Get.Tags)
	}

	// Verify query params on /v1/forum/posts
	var paramNames []string
	for _, p := range postsPath.Get.Parameters {
		if p.Value != nil {
			paramNames = append(paramNames, p.Value.Name)
		}
	}
	for _, reqParam := range []string{"category", "kind", "uf", "municipality_ibge", "fuel_product", "station_id", "limit", "cursor"} {
		if !slices.Contains(paramNames, reqParam) {
			t.Errorf("GET /v1/forum/posts missing query param %q", reqParam)
		}
	}

	postDetailPath := doc.Paths.Find("/v1/forum/posts/{post_id}")
	if postDetailPath == nil || postDetailPath.Get == nil {
		t.Fatal("missing GET /v1/forum/posts/{post_id} endpoint")
	}
	if !slices.Contains(postDetailPath.Get.Tags, "forum") {
		t.Errorf("GET /v1/forum/posts/{post_id} missing 'forum' tag, got %v", postDetailPath.Get.Tags)
	}
	if postDetailPath.Get.Responses.Value("404") == nil {
		t.Error("GET /v1/forum/posts/{post_id} missing 404 response specification")
	}
}

func TestForumEditorialSchemaNegativeAndBoundaryCases(t *testing.T) {
	doc := loadSpec(t)

	// Negative 1: Empty title rejected in ForumPostSummary
	t.Run("empty_title_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumPostSummary"].Value
		val := map[string]any{
			"id":                 "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
			"kind":               "EDITORIAL",
			"category":           "FUELS",
			"title":              "",
			"first_published_at": "2026-11-05T12:00:00Z",
			"updated_at":         "2026-11-05T12:00:00Z",
			"revision":           1,
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for empty title (minLength: 1), got nil")
		}
	})

	// Negative 2: Title exceeding 120 characters rejected
	t.Run("oversized_title_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumPostSummary"].Value
		longTitle := strings.Repeat("A", 121)
		val := map[string]any{
			"id":                 "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
			"kind":               "EDITORIAL",
			"category":           "FUELS",
			"title":              longTitle,
			"first_published_at": "2026-11-05T12:00:00Z",
			"updated_at":         "2026-11-05T12:00:00Z",
			"revision":           1,
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for title > 120 chars, got nil")
		}
	})

	// Negative 3: Excerpt exceeding 400 characters rejected
	t.Run("oversized_excerpt_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumPostSummary"].Value
		longExcerpt := strings.Repeat("E", 401)
		val := map[string]any{
			"id":                 "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
			"kind":               "EDITORIAL",
			"category":           "FUELS",
			"title":              "Valid Title",
			"excerpt":            longExcerpt,
			"first_published_at": "2026-11-05T12:00:00Z",
			"updated_at":         "2026-11-05T12:00:00Z",
			"revision":           1,
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for excerpt > 400 chars, got nil")
		}
	})

	// Negative 4: Body exceeding 4000 characters rejected in ForumPostDetail
	t.Run("oversized_body_text_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumPostDetail"].Value
		longBody := strings.Repeat("B", 4001)
		val := map[string]any{
			"id":                 "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
			"kind":               "EDITORIAL",
			"category":           "FUELS",
			"title":              "Valid Title",
			"body_text":          longBody,
			"first_published_at": "2026-11-05T12:00:00Z",
			"updated_at":         "2026-11-05T12:00:00Z",
			"revision":           1,
			"comments_enabled":   true,
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for body_text > 4000 chars, got nil")
		}
	})

	// Negative 5: Additional unexpected property rejected (additionalProperties: false)
	t.Run("additional_property_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumPostSummary"].Value
		val := map[string]any{
			"id":                 "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
			"kind":               "EDITORIAL",
			"category":           "FUELS",
			"title":              "Valid Title",
			"first_published_at": "2026-11-05T12:00:00Z",
			"updated_at":         "2026-11-05T12:00:00Z",
			"revision":           1,
			"author_account_id":  "e2a868f7-87ea-4bf0-8bc2-a1b7e411cbb0", // Forbidden leaked field!
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for additional property author_account_id, got nil")
		}
	})
}

func TestForumDiscoveryFilterParameters(t *testing.T) {
	doc := loadSpec(t)
	postsPath := doc.Paths.Find("/v1/forum/posts")
	if postsPath == nil || postsPath.Get == nil {
		t.Fatal("missing GET /v1/forum/posts endpoint")
	}

	params := map[string]*openapi3.Parameter{}
	for _, p := range postsPath.Get.Parameters {
		if p.Value != nil {
			params[p.Value.Name] = p.Value
		}
	}

	uf, ok := params["uf"]
	if !ok || uf.Schema == nil || uf.Schema.Value == nil {
		t.Fatal("missing uf filter schema")
	}
	if len(uf.Schema.Value.Enum) != 27 {
		t.Errorf("uf filter must enumerate 27 states, got %d", len(uf.Schema.Value.Enum))
	}
	if uf.Schema.Value.Pattern != "" {
		t.Errorf("uf filter must not use a free-text pattern")
	}

	ibge, ok := params["municipality_ibge"]
	if !ok || ibge.Schema == nil || ibge.Schema.Value == nil {
		t.Fatal("missing municipality_ibge filter schema")
	}
	if ibge.Schema.Value.Pattern != "^[0-9]{7}$" {
		t.Errorf("municipality_ibge pattern = %q", ibge.Schema.Value.Pattern)
	}

	fuel, ok := params["fuel_product"]
	if !ok || fuel.Schema == nil || fuel.Schema.Value == nil {
		t.Fatal("missing fuel_product filter schema")
	}
	if fuel.Schema.Ref != "#/components/schemas/ForumFuelProduct" {
		t.Errorf("fuel_product ref = %q", fuel.Schema.Ref)
	}

	station, ok := params["station_id"]
	if !ok || station.Schema == nil || station.Schema.Value == nil {
		t.Fatal("missing station_id filter schema")
	}
	if station.Schema.Value.Format != "uuid" {
		t.Errorf("station_id format = %q", station.Schema.Value.Format)
	}
}
