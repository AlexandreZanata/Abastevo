package domain

import (
	"strings"
	"testing"
	"time"
)

func TestCategoryValidation(t *testing.T) {
	validCategories := []string{
		"FUELS",
		"ELECTRIC",
		"TRAFFIC_RULES",
		"VEHICLES_TECH",
		"ROADS_MOBILITY",
		"DRIVER_ECONOMY",
		"ABASTEVO_DATA",
	}

	for _, c := range validCategories {
		t.Run("valid_"+c, func(t *testing.T) {
			cat, err := ParseCategory(c)
			if err != nil {
				t.Fatalf("ParseCategory(%q) unexpected error: %v", c, err)
			}
			if string(cat) != c {
				t.Fatalf("ParseCategory(%q) = %q, want %q", c, cat, c)
			}
		})
	}

	invalidCategories := []string{
		"",
		"CRYPTO",
		"fuels",
		"OTHER",
		"SPORTS",
	}

	for _, c := range invalidCategories {
		t.Run("invalid_"+c, func(t *testing.T) {
			_, err := ParseCategory(c)
			if err != ErrInvalidCategory {
				t.Fatalf("ParseCategory(%q) error = %v, want ErrInvalidCategory", c, err)
			}
		})
	}
}

func TestPostKindValidation(t *testing.T) {
	validKinds := []string{
		"EDITORIAL",
		"NEWS",
		"TOPIC",
		"BULLETIN",
	}

	for _, k := range validKinds {
		t.Run("valid_"+k, func(t *testing.T) {
			kind, err := ParsePostKind(k)
			if err != nil {
				t.Fatalf("ParsePostKind(%q) unexpected error: %v", k, err)
			}
			if string(kind) != k {
				t.Fatalf("ParsePostKind(%q) = %q, want %q", k, kind, k)
			}
		})
	}

	invalidKinds := []string{
		"",
		"MEME",
		"editorial",
		"ARTICLE",
	}

	for _, k := range invalidKinds {
		t.Run("invalid_"+k, func(t *testing.T) {
			_, err := ParsePostKind(k)
			if err != ErrInvalidPostKind {
				t.Fatalf("ParsePostKind(%q) error = %v, want ErrInvalidPostKind", k, err)
			}
		})
	}
}

func TestPublicationStateTransitions(t *testing.T) {
	tests := []struct {
		from  PublicationState
		to    PublicationState
		valid bool
	}{
		{StateDraft, StatePendingReview, true},
		{StateDraft, StatePublished, true},
		{StateDraft, StateRejected, true},
		{StateDraft, StateRemoved, true},
		{StatePendingReview, StatePublished, true},
		{StatePendingReview, StateRejected, true},
		{StatePendingReview, StateDraft, true},
		{StatePendingReview, StateRemoved, true},
		{StatePublished, StateRemoved, true},
		{StatePublished, StateDraft, false}, // Published cannot become draft
		{StateRejected, StateDraft, true},
		{StateRejected, StateRemoved, true},
		{StateRejected, StatePublished, false}, // Must go through draft/review
		{StateRemoved, StateDraft, false},      // Terminal
		{StateRemoved, StatePublished, false},  // Terminal
		{StateRemoved, StatePendingReview, false},
		{StateRemoved, StateRejected, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			can := CanTransition(tc.from, tc.to)
			if can != tc.valid {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, can, tc.valid)
			}
		})
	}
}

func TestPostValidation(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	validSource := &Source{
		ID:           "source-1",
		Code:         "abastevo-editorial",
		Name:         "Abastevo Editorial",
		Kind:         SourceKindAbastevo,
		RightsStatus: RightsApproved,
		TitleAllowed: true,
	}

	validEditorial := func() *Post {
		return &Post{
			ID:                  "post-1",
			Kind:                KindEditorial,
			Category:            CategoryFuels,
			Title:               "Como funciona o ICMS sobre combustiveis",
			Excerpt:             "Entenda o regime monofasico e as aliquotas ad rem em vigor.",
			BodyText:            "O ICMS sobre combustiveis passou a ser cobrado por valor fixo por litro...",
			SourceID:            "source-1",
			Source:              validSource,
			FirstPublishedAt:    &now,
			LocalCreatedAt:      now,
			LocalUpdatedAt:      now,
			FirstPublicationSeq: 1,
			PublicationState:    StatePublished,
			Visibility:          VisibilityVisible,
			Revision:            1,
			CommentsEnabled:     true,
		}
	}

	t.Run("valid_editorial_passes", func(t *testing.T) {
		p := validEditorial()
		if err := p.Validate(); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})

	t.Run("empty_title_fails", func(t *testing.T) {
		p := validEditorial()
		p.Title = "   "
		if err := p.Validate(); err != ErrTitleEmpty {
			t.Fatalf("Validate() = %v, want ErrTitleEmpty", err)
		}
	})

	t.Run("invalid_utf8_title_fails", func(t *testing.T) {
		p := validEditorial()
		p.Title = "Invalido \xff\xfe"
		if err := p.Validate(); err != ErrInvalidUTF8 {
			t.Fatalf("Validate() = %v, want ErrInvalidUTF8", err)
		}
	})

	t.Run("oversized_title_fails", func(t *testing.T) {
		p := validEditorial()
		p.Title = strings.Repeat("A", 121)
		if err := p.Validate(); err != ErrTitleTooLong {
			t.Fatalf("Validate() = %v, want ErrTitleTooLong", err)
		}
	})

	t.Run("oversized_excerpt_fails", func(t *testing.T) {
		p := validEditorial()
		p.Excerpt = strings.Repeat("E", 401)
		if err := p.Validate(); err != ErrExcerptTooLong {
			t.Fatalf("Validate() = %v, want ErrExcerptTooLong", err)
		}
	})

	t.Run("oversized_body_fails", func(t *testing.T) {
		p := validEditorial()
		p.BodyText = strings.Repeat("B", 4001)
		if err := p.Validate(); err != ErrBodyTooLong {
			t.Fatalf("Validate() = %v, want ErrBodyTooLong", err)
		}
	})

	t.Run("editorial_missing_source_fails", func(t *testing.T) {
		p := validEditorial()
		p.SourceID = ""
		p.Source = nil
		if err := p.Validate(); err != ErrMissingSource {
			t.Fatalf("Validate() = %v, want ErrMissingSource", err)
		}
	})

	t.Run("news_missing_date_fails", func(t *testing.T) {
		p := validEditorial()
		p.Kind = KindNews
		p.SourcePublishedAt = nil
		if err := p.Validate(); err != ErrMissingSourceDate {
			t.Fatalf("Validate() = %v, want ErrMissingSourceDate", err)
		}
	})

	t.Run("news_unapproved_rights_fails", func(t *testing.T) {
		p := validEditorial()
		p.Kind = KindNews
		p.SourcePublishedAt = &now
		unapprovedSource := *validSource
		unapprovedSource.RightsStatus = RightsPending
		p.Source = &unapprovedSource
		if err := p.Validate(); err != ErrMissingRights {
			t.Fatalf("Validate() = %v, want ErrMissingRights", err)
		}
	})

	t.Run("topic_with_source_fails", func(t *testing.T) {
		p := validEditorial()
		p.Kind = KindTopic
		p.AuthorAccountID = "author-1"
		p.SourceID = "source-1"
		if err := p.Validate(); err != ErrTopicCannotHaveSource {
			t.Fatalf("Validate() = %v, want ErrTopicCannotHaveSource", err)
		}
	})

	t.Run("topic_missing_author_fails", func(t *testing.T) {
		p := validEditorial()
		p.Kind = KindTopic
		p.SourceID = ""
		p.Source = nil
		p.AuthorAccountID = ""
		if err := p.Validate(); err != ErrMissingAuthor {
			t.Fatalf("Validate() = %v, want ErrMissingAuthor", err)
		}
	})
}

func TestPublicProjections(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	p := &Post{
		ID:                  "post-1",
		Kind:                KindEditorial,
		Category:            CategoryFuels,
		Title:               "Guia de Precos",
		Excerpt:             "Resumo de precos",
		BodyText:            "Corpo completo do artigo",
		SourceID:            "src-1",
		Source:              &Source{Name: "Abastevo", RightsStatus: RightsApproved},
		FirstPublishedAt:    &now,
		LocalCreatedAt:      now,
		LocalUpdatedAt:      now,
		FirstPublicationSeq: 10,
		PublicationState:    StatePublished,
		Visibility:          VisibilityVisible,
		Revision:            1,
		CommentsEnabled:     true,
	}

	t.Run("visible_post_projects_summary_and_detail", func(t *testing.T) {
		if !p.IsPubliclyVisible() {
			t.Fatal("expected post to be publicly visible")
		}

		summary, err := p.ToSummary()
		if err != nil {
			t.Fatalf("ToSummary() unexpected error: %v", err)
		}
		if summary.ID != p.ID || summary.Title != p.Title {
			t.Errorf("summary mismatch: %+v", summary)
		}

		detail, err := p.ToDetail()
		if err != nil {
			t.Fatalf("ToDetail() unexpected error: %v", err)
		}
		if detail.ID != p.ID || detail.BodyText == nil || *detail.BodyText != p.BodyText {
			t.Errorf("detail mismatch: %+v", detail)
		}
	})

	t.Run("hidden_post_refuses_public_projection", func(t *testing.T) {
		hidden := *p
		hidden.Visibility = VisibilityHidden
		if hidden.IsPubliclyVisible() {
			t.Fatal("hidden post must not be publicly visible")
		}
		if _, err := hidden.ToSummary(); err != ErrNotPubliclyVisible {
			t.Fatalf("ToSummary() error = %v, want ErrNotPubliclyVisible", err)
		}
		if _, err := hidden.ToDetail(); err != ErrNotPubliclyVisible {
			t.Fatalf("ToDetail() error = %v, want ErrNotPubliclyVisible", err)
		}
	})

	t.Run("draft_post_refuses_public_projection", func(t *testing.T) {
		draft := *p
		draft.PublicationState = StateDraft
		if draft.IsPubliclyVisible() {
			t.Fatal("draft post must not be publicly visible")
		}
		if _, err := draft.ToSummary(); err != ErrNotPubliclyVisible {
			t.Fatalf("ToSummary() error = %v, want ErrNotPubliclyVisible", err)
		}
		if _, err := draft.ToDetail(); err != ErrNotPubliclyVisible {
			t.Fatalf("ToDetail() error = %v, want ErrNotPubliclyVisible", err)
		}
	})
}
