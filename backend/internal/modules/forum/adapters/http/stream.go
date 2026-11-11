package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters/stream"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// PublicPostReader reads a post only when anonymous readers may see it.
type PublicPostReader interface {
	GetPostByID(ctx context.Context, id string) (*domain.Post, error)
}

// CommentReader reads one comment in any state; callers filter publicity.
type CommentReader interface {
	GetCommentByID(ctx context.Context, id string) (*domain.Comment, error)
}

// ConversationStream serves GET /v1/forum/posts/{post_id}/stream as
// server-sent events (B-BR-FOR-18). It announces only committed public
// changes and never replaces the paginated reads.
type ConversationStream struct {
	Hub    *stream.Hub
	Posts  PublicPostReader
	Config *domain.Config
	// Done ends every open stream, for example when the server drains.
	Done <-chan struct{}
	// Heartbeat keeps proxies from closing an idle stream.
	Heartbeat time.Duration
	// MaxLifetime spreads reconnections; the client reconnects and reloads.
	MaxLifetime time.Duration
	// WriteTimeout cuts a client that stops reading.
	WriteTimeout time.Duration
}

// RegisterRoutes mounts the stream endpoint.
func (s *ConversationStream) RegisterRoutes(r chi.Router) {
	r.Get("/v1/forum/posts/{post_id}/stream", s.Stream)
}

// Stream holds the connection open and writes conversation events.
func (s *ConversationStream) Stream(w http.ResponseWriter, r *http.Request) {
	if s.Config != nil && !s.Config.ReadsAllowed() {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.service-unavailable", "forum discussion is disabled", nil)
		return
	}
	postID := chi.URLParam(r, "post_id")
	if !isUUID(postID) {
		w.Header().Set("Cache-Control", "no-store")
		httpapi.WriteError(w, r, http.StatusBadRequest, "forum.bad-request", "invalid post id", nil)
		return
	}
	if _, err := s.Posts.GetPostByID(r.Context(), postID); err != nil {
		w.Header().Set("Cache-Control", "no-store")
		if errors.Is(err, domain.ErrPostNotFound) {
			httpapi.WriteError(w, r, http.StatusNotFound, "forum.not-found", "resource not found", nil)
			return
		}
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.service-unavailable", "service unavailable", nil)
		return
	}
	sub, err := s.Hub.Subscribe(postID)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "30")
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "forum.stream-busy", "too many open conversation streams", nil)
		return
	}
	defer s.Hub.Unsubscribe(sub)

	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	// Reverse proxies (nginx) must pass events through unbuffered.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(chunk string) bool {
		if err := rc.SetWriteDeadline(time.Now().Add(s.WriteTimeout)); err != nil {
			return false
		}
		if _, err := io.WriteString(w, chunk); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	ready, _ := json.Marshal(map[string]string{"post_id": postID})
	if !write("retry: 3000\n" + eventFrame("ready", ready)) {
		return
	}

	heartbeat := time.NewTicker(s.Heartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(s.MaxLifetime)
	defer lifetime.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.Done:
			write(eventFrame("reconnect", []byte("{}")))
			return
		case <-sub.Resync():
			write(eventFrame("resync", []byte("{}")))
			return
		case ev := <-sub.Events():
			if !write(eventFrame(ev.Type, ev.Data)) {
				return
			}
			if ev.Type == "post_gone" || ev.Type == "resync" {
				return
			}
		case <-heartbeat.C:
			if !write(": ping\n\n") {
				return
			}
		case <-lifetime.C:
			write(eventFrame("reconnect", []byte("{}")))
			return
		}
	}
}

// eventFrame formats one SSE event; payloads are single-line JSON.
func eventFrame(name string, data []byte) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", name, strings.ReplaceAll(string(data), "\n", ""))
}

// StreamCommentDTO announces a newly public comment or reply.
type StreamCommentDTO struct {
	Comment CommentItemDTO `json:"comment"`
}

// StreamCountsDTO carries new public counts of a comment or the post.
type StreamCountsDTO struct {
	CommentID    string  `json:"comment_id,omitempty"`
	ParentID     *string `json:"parent_id,omitempty"`
	LikeCount    int64   `json:"like_count"`
	DislikeCount int64   `json:"dislike_count"`
}

// StreamGoneDTO announces a comment that left public reads.
type StreamGoneDTO struct {
	CommentID string  `json:"comment_id"`
	ParentID  *string `json:"parent_id,omitempty"`
}

// NewStreamResolver builds client events from database notifications. Only
// newly public comments need a read; the rest travel in the notification.
func NewStreamResolver(comments CommentReader) stream.Resolver {
	return func(ctx context.Context, n stream.Notification) (stream.Event, bool, error) {
		var payload any
		switch n.Type {
		case "comment":
			c, err := comments.GetCommentByID(ctx, n.Comment)
			if errors.Is(err, domain.ErrCommentNotFound) {
				return stream.Event{}, false, nil
			}
			if err != nil {
				return stream.Event{}, false, err
			}
			// Hidden or edited again before the read: its gone event follows.
			if c.PublicationState != domain.StatePublished || c.Visibility != domain.VisibilityVisible {
				return stream.Event{}, false, nil
			}
			payload = StreamCommentDTO{Comment: mapCommentItemDTO(*c)}
		case "comment_reactions":
			payload = StreamCountsDTO{CommentID: n.Comment, ParentID: n.Parent, LikeCount: n.Likes, DislikeCount: n.Dislikes}
		case "comment_gone":
			payload = StreamGoneDTO{CommentID: n.Comment, ParentID: n.Parent}
		case "post_reactions":
			payload = StreamCountsDTO{LikeCount: n.Likes, DislikeCount: n.Dislikes}
		case "post_gone":
			payload = struct{}{}
		default:
			return stream.Event{}, false, nil
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return stream.Event{}, false, err
		}
		return stream.Event{Type: n.Type, Data: data}, true, nil
	}
}
