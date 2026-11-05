package apicontract

import (
	"slices"
	"strings"
	"testing"
)

func TestForumDiscussionContractStructure(t *testing.T) {
	doc := loadSpec(t)

	// 1. Verify required schemas exist
	requiredSchemas := []string{
		"ForumCreateCommentRequest",
		"ForumEditCommentRequest",
		"ForumRemoveCommentRequest",
		"ForumOwnerReadRequest",
		"ForumCommentReceipt",
		"ForumOwnerCommentResponse",
		"ForumCommentItem",
		"ForumCommentsResponse",
		"ForumRepliesResponse",
	}
	for _, name := range requiredSchemas {
		if _, ok := doc.Components.Schemas[name]; !ok {
			t.Errorf("missing required discussion schema %q", name)
		}
	}

	// 2. Verify POST /v1/forum/posts/{post_id}/comments
	postCommentsPath := doc.Paths.Find("/v1/forum/posts/{post_id}/comments")
	if postCommentsPath == nil {
		t.Fatal("missing /v1/forum/posts/{post_id}/comments path")
	}
	if postCommentsPath.Post == nil {
		t.Fatal("missing POST on /v1/forum/posts/{post_id}/comments")
	}
	if !slices.Contains(postCommentsPath.Post.Tags, "forum") {
		t.Errorf("POST /v1/forum/posts/{post_id}/comments missing 'forum' tag, got %v", postCommentsPath.Post.Tags)
	}
	if postCommentsPath.Post.Responses.Value("200") == nil {
		t.Error("POST /v1/forum/posts/{post_id}/comments missing 200 response")
	}
	if postCommentsPath.Post.Responses.Value("401") == nil {
		t.Error("POST /v1/forum/posts/{post_id}/comments missing 401 response")
	}
	if postCommentsPath.Post.Responses.Value("409") == nil {
		t.Error("POST /v1/forum/posts/{post_id}/comments missing 409 response")
	}
	if postCommentsPath.Post.Responses.Value("429") == nil {
		t.Error("POST /v1/forum/posts/{post_id}/comments missing 429 response")
	}

	// 3. Verify GET /v1/forum/posts/{post_id}/comments
	if postCommentsPath.Get == nil {
		t.Fatal("missing GET on /v1/forum/posts/{post_id}/comments")
	}
	if !slices.Contains(postCommentsPath.Get.Tags, "forum") {
		t.Errorf("GET /v1/forum/posts/{post_id}/comments missing 'forum' tag, got %v", postCommentsPath.Get.Tags)
	}

	// 4. Verify POST /v1/forum/comments/{comment_id}/edit
	editCommentPath := doc.Paths.Find("/v1/forum/comments/{comment_id}/edit")
	if editCommentPath == nil || editCommentPath.Post == nil {
		t.Fatal("missing POST /v1/forum/comments/{comment_id}/edit")
	}
	if !slices.Contains(editCommentPath.Post.Tags, "forum") {
		t.Errorf("POST /v1/forum/comments/{comment_id}/edit missing 'forum' tag, got %v", editCommentPath.Post.Tags)
	}
	if editCommentPath.Post.Responses.Value("404") == nil {
		t.Error("POST /v1/forum/comments/{comment_id}/edit missing 404 response (B-BR-FOR-04 IDOR)")
	}
	if editCommentPath.Post.Responses.Value("409") == nil {
		t.Error("POST /v1/forum/comments/{comment_id}/edit missing 409 response")
	}

	// 5. Verify POST /v1/forum/comments/{comment_id}/remove
	removeCommentPath := doc.Paths.Find("/v1/forum/comments/{comment_id}/remove")
	if removeCommentPath == nil || removeCommentPath.Post == nil {
		t.Fatal("missing POST /v1/forum/comments/{comment_id}/remove")
	}
	if !slices.Contains(removeCommentPath.Post.Tags, "forum") {
		t.Errorf("POST /v1/forum/comments/{comment_id}/remove missing 'forum' tag, got %v", removeCommentPath.Post.Tags)
	}

	// 6. Verify GET /v1/forum/comments/{comment_id}/replies
	repliesPath := doc.Paths.Find("/v1/forum/comments/{comment_id}/replies")
	if repliesPath == nil || repliesPath.Get == nil {
		t.Fatal("missing GET /v1/forum/comments/{comment_id}/replies")
	}
	if !slices.Contains(repliesPath.Get.Tags, "forum") {
		t.Errorf("GET /v1/forum/comments/{comment_id}/replies missing 'forum' tag, got %v", repliesPath.Get.Tags)
	}

	// 7. Verify POST /v1/forum/owner/read
	ownerReadPath := doc.Paths.Find("/v1/forum/owner/read")
	if ownerReadPath == nil || ownerReadPath.Post == nil {
		t.Fatal("missing POST /v1/forum/owner/read")
	}
	if !slices.Contains(ownerReadPath.Post.Tags, "forum") {
		t.Errorf("POST /v1/forum/owner/read missing 'forum' tag, got %v", ownerReadPath.Post.Tags)
	}
	if ownerReadPath.Post.Responses.Value("404") == nil {
		t.Error("POST /v1/forum/owner/read missing 404 response (B-BR-FOR-04 IDOR)")
	}
}

func TestForumDiscussionNegativeBounds(t *testing.T) {
	doc := loadSpec(t)

	// Negative 1: Empty body text rejected in ForumCreateCommentRequest
	t.Run("empty_body_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumCreateCommentRequest"].Value
		val := map[string]any{
			"family_id":    "11111111-1111-4111-8111-111111111111",
			"access_token": "token",
			"operation_id": "22222222-2222-4222-8222-222222222222",
			"body_text":    "",
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for empty comment body (minLength: 1), got nil")
		}
	})

	// Negative 2: Oversized body text (>280 chars) rejected in ForumCreateCommentRequest
	t.Run("oversized_body_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumCreateCommentRequest"].Value
		longBody := strings.Repeat("A", 281)
		val := map[string]any{
			"family_id":    "11111111-1111-4111-8111-111111111111",
			"access_token": "token",
			"operation_id": "22222222-2222-4222-8222-222222222222",
			"body_text":    longBody,
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for oversized comment body (>280), got nil")
		}
	})

	// Negative 3: Expected revision < 1 rejected in ForumEditCommentRequest
	t.Run("invalid_revision_rejected", func(t *testing.T) {
		schema := doc.Components.Schemas["ForumEditCommentRequest"].Value
		val := map[string]any{
			"family_id":         "11111111-1111-4111-8111-111111111111",
			"access_token":      "token",
			"operation_id":      "22222222-2222-4222-8222-222222222222",
			"expected_revision": 0,
			"body_text":         "Valid updated text",
		}
		if err := schema.VisitJSON(val); err == nil {
			t.Error("expected error for revision < 1 (minimum: 1), got nil")
		}
	})
}
