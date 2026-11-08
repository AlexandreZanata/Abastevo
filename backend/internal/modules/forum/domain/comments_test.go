package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateCommentBody(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:    "valid short comment",
			input:   "Ótima notícia!",
			want:    "Ótima notícia!",
			wantErr: nil,
		},
		{
			name:    "valid comment with unicode and emojis",
			input:   "Preço do etanol caiu em SP hoje! ⛽🚗",
			want:    "Preço do etanol caiu em SP hoje! ⛽🚗",
			wantErr: nil,
		},
		{
			name:    "valid comment with leading/trailing whitespace trimmed",
			input:   "   Texto com espaços ao redor   ",
			want:    "Texto com espaços ao redor",
			wantErr: nil,
		},
		{
			name:    "valid boundary 1 code point",
			input:   "A",
			want:    "A",
			wantErr: nil,
		},
		{
			name:    "valid boundary 280 code points",
			input:   strings.Repeat("a", 280),
			want:    strings.Repeat("a", 280),
			wantErr: nil,
		},
		{
			name:    "valid boundary 280 multi-byte unicode code points",
			input:   strings.Repeat("ç", 280),
			want:    strings.Repeat("ç", 280),
			wantErr: nil,
		},
		{
			name:    "empty comment",
			input:   "",
			wantErr: ErrCommentBodyEmpty,
		},
		{
			name:    "whitespace only comment",
			input:   "   \n\t  ",
			wantErr: ErrCommentBodyEmpty,
		},
		{
			name:    "exceeds 280 code points",
			input:   strings.Repeat("a", 281),
			wantErr: ErrCommentBodyTooLong,
		},
		{
			name:    "exceeds 280 multi-byte unicode code points",
			input:   strings.Repeat("ç", 281),
			wantErr: ErrCommentBodyTooLong,
		},
		{
			name:    "contains NUL byte",
			input:   "Comentário com\x00byte nulo",
			wantErr: ErrInvalidUTF8,
		},
		{
			name:    "invalid utf-8 bytes",
			input:   "Comentário com bytes \xff\xfe inválidos",
			wantErr: ErrInvalidUTF8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateCommentBody(tt.input)
			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestComment_IsPubliclyVisible(t *testing.T) {
	c := &Comment{
		PublicationState: StatePublished,
		Visibility:       VisibilityVisible,
	}
	if !c.IsPubliclyVisible() {
		t.Errorf("expected published+visible comment to be publicly visible")
	}

	c.PublicationState = StatePendingReview
	if c.IsPubliclyVisible() {
		t.Errorf("expected pending_review comment not to be publicly visible")
	}

	c.PublicationState = StatePublished
	c.Visibility = VisibilityHidden
	if c.IsPubliclyVisible() {
		t.Errorf("expected hidden comment not to be publicly visible")
	}

	c.PublicationState = StateRemoved
	if c.IsPubliclyVisible() {
		t.Errorf("expected removed comment not to be publicly visible")
	}
}

func TestComment_VerdictCodes(t *testing.T) {
	verdictMap := map[error]string{
		ErrCommentBodyEmpty:         "comment-body-empty",
		ErrCommentBodyTooLong:       "comment-body-too-long",
		ErrInvalidCommentDepth:      "invalid-comment-depth",
		ErrCrossPostParent:          "cross-post-parent",
		ErrParentCommentNotFound:    "parent-comment-not-found",
		ErrParentCommentNotVisible:  "parent-comment-not-visible",
		ErrPostCommentsDisabled:     "post-comments-disabled",
		ErrPostNotCommentable:       "post-not-commentable",
		ErrConflictingOperation:     "conflicting-operation",
		ErrCommentLiveQuotaExceeded: "comment-live-quota-exceeded",
		ErrCommentNotFound:          "comment-not-found",
		ErrCommentAuthorMismatch:    "comment-author-mismatch",
		ErrCommentAlreadyRemoved:    "comment-already-removed",
	}

	for err, wantCode := range verdictMap {
		if got := VerdictCode(err); got != wantCode {
			t.Errorf("VerdictCode(%v) = %q, want %q", err, got, wantCode)
		}
	}
}
