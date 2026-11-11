package http

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// SetReactionRequest is the signed body that sets the caller's reaction.
type SetReactionRequest struct {
	SessionEnvelope
	OperationID string `json:"operation_id"`
	Reaction    string `json:"reaction"`
}

// ReactionResponseDTO is the caller's reaction and the target's public counts.
type ReactionResponseDTO struct {
	TargetType   string `json:"target_type"`
	TargetID     string `json:"target_id"`
	Reaction     string `json:"reaction"`
	LikeCount    int64  `json:"like_count"`
	DislikeCount int64  `json:"dislike_count"`
	Replayed     bool   `json:"replayed"`
}

// OwnerReactionsRequest is the signed body of a private read of the caller's
// reactions in one conversation.
type OwnerReactionsRequest struct {
	SessionEnvelope
	PostID string `json:"post_id"`
}

// OwnCommentReactionDTO is one of the caller's comment reactions.
type OwnCommentReactionDTO struct {
	CommentID string `json:"comment_id"`
	Reaction  string `json:"reaction"`
}

// OwnerReactionsResponseDTO lists the caller's reactions in one conversation.
type OwnerReactionsResponseDTO struct {
	PostReaction string                  `json:"post_reaction"`
	Comments     []OwnCommentReactionDTO `json:"comments"`
}

type canonicalReactionPayload struct {
	Action      string `json:"action"`
	OperationID string `json:"operation_id"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
	Reaction    string `json:"reaction"`
}

// CanonicalReactionIntent serializes the stable logical intent of a reaction.
func CanonicalReactionIntent(operationID string, targetType domain.ReactionTargetType, targetID string, reaction domain.Reaction) ([]byte, error) {
	return json.Marshal(canonicalReactionPayload{
		Action:      "set_reaction",
		OperationID: strings.TrimSpace(operationID),
		TargetType:  string(targetType),
		TargetID:    strings.TrimSpace(targetID),
		Reaction:    reaction.Wire(),
	})
}

// RegisterReactionRoutes adds the reaction endpoints (B-BR-FOR-17).
func (h *DiscussionHandler) RegisterReactionRoutes(r chi.Router) {
	r.Post("/v1/forum/posts/{post_id}/reaction", h.ReactToPost)
	r.Post("/v1/forum/comments/{comment_id}/reaction", h.ReactToComment)
	r.Post("/v1/forum/owner/reactions", h.OwnerReactions)
}

// ReactToPost sets the caller's reaction on a published post.
func (h *DiscussionHandler) ReactToPost(w http.ResponseWriter, r *http.Request) {
	h.setReaction(w, r, domain.ReactionTargetPost, chi.URLParam(r, "post_id"))
}

// ReactToComment sets the caller's reaction on a published comment or reply.
func (h *DiscussionHandler) ReactToComment(w http.ResponseWriter, r *http.Request) {
	h.setReaction(w, r, domain.ReactionTargetComment, chi.URLParam(r, "comment_id"))
}

func (h *DiscussionHandler) setReaction(w http.ResponseWriter, r *http.Request, targetType domain.ReactionTargetType, targetID string) {
	if !h.writesAllowed() || h.Reactions == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}
	if !isUUID(targetID) {
		h.writeBadRequest(w, r, "invalid target id")
		return
	}

	var req SetReactionRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}
	if !isUUID(req.OperationID) {
		h.writeBadRequest(w, r, "invalid operation id")
		return
	}
	reaction, err := domain.ParseReaction(req.Reaction)
	if err != nil {
		h.writeBadRequest(w, r, "reaction must be LIKE, DISLIKE or NONE")
		return
	}
	intentBytes, err := CanonicalReactionIntent(req.OperationID, targetType, targetID, reaction)
	if err != nil {
		h.writeBadRequest(w, r, "serialization error")
		return
	}

	guard := h.makeTxGuard(principal)
	run := func(ctx context.Context) (ReactionResponseDTO, error) {
		res, err := h.Reactions.SetReaction(ctx, application.SetReactionCommand{
			TargetType: targetType,
			TargetID:   targetID,
			AccountID:  principal.AccountID,
			Reaction:   reaction,
			Guard:      guard,
		})
		if err != nil {
			return ReactionResponseDTO{}, err
		}
		return ReactionResponseDTO{
			TargetType:   string(res.TargetType),
			TargetID:     res.TargetID,
			Reaction:     res.Reaction.Wire(),
			LikeCount:    res.Counts.Likes,
			DislikeCount: res.Counts.Dislikes,
		}, nil
	}

	if h.Runner != nil {
		key := identitydomain.IdempotencyKey{
			ContributorID: principal.ContributorID,
			Method:        r.Method,
			Route:         r.URL.Path,
			Key:           req.OperationID,
		}
		outcome, err := h.Runner.Do(r.Context(), key, intentBytes, func(ctx context.Context, _ pgx.Tx) (int, []byte, error) {
			resp, err := run(ctx)
			if err != nil {
				return 0, nil, err
			}
			body, err := json.Marshal(resp)
			return http.StatusOK, body, err
		})
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		var resp ReactionResponseDTO
		if err := json.Unmarshal(outcome.Response, &resp); err == nil {
			resp.Replayed = outcome.Replayed
			body, _ := json.Marshal(resp)
			httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
			return
		}
		httpapi.WriteJSON(w, r, outcome.StatusCode, "no-store", outcome.Response)
		return
	}

	resp, err := run(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	body, _ := json.Marshal(resp)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

// OwnerReactions returns the caller's own reactions in one conversation.
// Signed like every private owner read; other accounts' reactions never leak.
func (h *DiscussionHandler) OwnerReactions(w http.ResponseWriter, r *http.Request) {
	if !h.readsAllowed() || h.Reactions == nil {
		h.writeError(w, r, domain.ErrForumDisabled)
		return
	}
	var req OwnerReactionsRequest
	if err := readJSONBody(r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	principal, status, code, msg := h.authenticatePrincipal(r, req.SessionEnvelope)
	if status != 0 {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, status, code, msg, nil)
		return
	}
	postID := strings.TrimSpace(req.PostID)
	if !isUUID(postID) {
		h.writeBadRequest(w, r, "invalid post id")
		return
	}
	own, err := h.Reactions.OwnReactions(r.Context(), principal.AccountID, postID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	resp := OwnerReactionsResponseDTO{PostReaction: own.Post.Wire(), Comments: make([]OwnCommentReactionDTO, 0, len(own.Comments))}
	for id, reaction := range own.Comments {
		resp.Comments = append(resp.Comments, OwnCommentReactionDTO{CommentID: id, Reaction: reaction.Wire()})
	}
	sort.Slice(resp.Comments, func(i, j int) bool { return resp.Comments[i].CommentID < resp.Comments[j].CommentID })
	body, _ := json.Marshal(resp)
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}
