package http

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters/stream"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

const streamPostID = "22222222-2222-4222-8222-000000000001"

type fakePublicPosts struct{ public map[string]bool }

func (f fakePublicPosts) GetPostByID(_ context.Context, id string) (*domain.Post, error) {
	if !f.public[id] {
		return nil, domain.ErrPostNotFound
	}
	return &domain.Post{ID: id}, nil
}

type streamClient struct {
	resp   *http.Response
	reader *bufio.Reader
}

func openStream(t *testing.T, base, postID string) *streamClient {
	t.Helper()
	resp, err := http.Get(base + "/v1/forum/posts/" + postID + "/stream")
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return &streamClient{resp: resp, reader: bufio.NewReader(resp.Body)}
}

// next returns the next event name and data, skipping retry and comments.
func (c *streamClient) next(t *testing.T) (string, string) {
	t.Helper()
	var name, data string
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "" && name != "":
			return name, data
		case strings.HasPrefix(line, ": "):
			return "comment", line
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data = strings.TrimPrefix(line, "data: ")
		}
	}
	t.Fatal("no event before deadline")
	return "", ""
}

func newStreamServer(t *testing.T, hub *stream.Hub, mutate func(*ConversationStream)) (*httptest.Server, chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	s := &ConversationStream{
		Hub:          hub,
		Posts:        fakePublicPosts{public: map[string]bool{streamPostID: true}},
		Done:         done,
		Heartbeat:    time.Hour,
		MaxLifetime:  time.Hour,
		WriteTimeout: time.Second,
	}
	if mutate != nil {
		mutate(s)
	}
	r := chi.NewRouter()
	s.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, done
}

func waitSubscribed(t *testing.T, hub *stream.Hub, n int) {
	t.Helper()
	for i := 0; i < 200 && hub.Count() < n; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.Count() < n {
		t.Fatalf("subscribers = %d; want %d", hub.Count(), n)
	}
}

func TestStreamDeliversEventsInOrderWithHeaders(t *testing.T) {
	hub := stream.NewHub(10, 10, 8)
	srv, _ := newStreamServer(t, hub, nil)
	c := openStream(t, srv.URL, streamPostID)
	if ct := c.resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	if c.resp.Header.Get("X-Accel-Buffering") != "no" || c.resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("missing proxy/cache headers: %v", c.resp.Header)
	}
	if name, data := c.next(t); name != "ready" || !strings.Contains(data, streamPostID) {
		t.Fatalf("first event = %s %s", name, data)
	}
	waitSubscribed(t, hub, 1)
	hub.Publish(streamPostID, stream.Event{Type: "comment", Data: []byte(`{"comment":{"id":"a"}}`)})
	hub.Publish(streamPostID, stream.Event{Type: "comment_reactions", Data: []byte(`{"comment_id":"a","like_count":1,"dislike_count":0}`)})
	if name, _ := c.next(t); name != "comment" {
		t.Fatalf("second event = %s", name)
	}
	if name, data := c.next(t); name != "comment_reactions" || !strings.Contains(data, `"like_count":1`) {
		t.Fatalf("third event = %s %s", name, data)
	}
}

func TestStreamHeartbeatLifetimeAndShutdown(t *testing.T) {
	hub := stream.NewHub(10, 10, 8)
	srv, done := newStreamServer(t, hub, func(s *ConversationStream) {
		s.Heartbeat = 30 * time.Millisecond
		s.MaxLifetime = 200 * time.Millisecond
	})
	c := openStream(t, srv.URL, streamPostID)
	c.next(t) // ready
	if name, line := c.next(t); name != "comment" || line != ": ping" {
		t.Fatalf("heartbeat = %s %q", name, line)
	}
	for {
		name, _ := c.next(t)
		if name == "reconnect" {
			break
		}
	}
	waitEmpty(t, hub)

	other := openStream(t, srv.URL, streamPostID)
	other.next(t)
	close(done)
	for {
		name, _ := other.next(t)
		if name == "reconnect" {
			break
		}
	}
	waitEmpty(t, hub)
}

func waitEmpty(t *testing.T, hub *stream.Hub) {
	t.Helper()
	for i := 0; i < 200 && hub.Count() > 0; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if hub.Count() != 0 {
		t.Fatalf("stream slot not released: %d open", hub.Count())
	}
}

func TestStreamResyncWhenReaderFallsBehind(t *testing.T) {
	hub := stream.NewHub(10, 10, 1)
	srv, _ := newStreamServer(t, hub, nil)
	c := openStream(t, srv.URL, streamPostID)
	c.next(t)
	waitSubscribed(t, hub, 1)
	for i := 0; i < 50; i++ {
		hub.Publish(streamPostID, stream.Event{Type: "comment_reactions", Data: []byte(`{}`)})
	}
	for i := 0; i < 60; i++ {
		if name, _ := c.next(t); name == "resync" {
			waitEmpty(t, hub)
			return
		}
	}
	t.Fatal("overflowing stream never asked to resync")
}

func TestStreamRefusesHiddenPostsAndFullHub(t *testing.T) {
	hub := stream.NewHub(1, 1, 4)
	srv, _ := newStreamServer(t, hub, nil)
	resp, err := http.Get(srv.URL + "/v1/forum/posts/33333333-3333-4333-8333-000000000001/stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("hidden post status = %d; want 404", resp.StatusCode)
	}
	c := openStream(t, srv.URL, streamPostID)
	c.next(t)
	resp, err = http.Get(srv.URL + "/v1/forum/posts/" + streamPostID + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("full hub status = %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

type fakeCommentReader map[string]*domain.Comment

func (f fakeCommentReader) GetCommentByID(_ context.Context, id string) (*domain.Comment, error) {
	if c, ok := f[id]; ok {
		return c, nil
	}
	return nil, domain.ErrCommentNotFound
}

func TestStreamResolverPublishesOnlyPublicProjections(t *testing.T) {
	public := &domain.Comment{ID: "c1", PostID: streamPostID, AuthorAccountID: "11111111-1111-4111-8111-000000000009",
		PublicAlias: "Motorista", BodyText: "Oi", Sequence: 3, Revision: 2, LikeCount: 4,
		PublicationState: domain.StatePublished, Visibility: domain.VisibilityVisible, CreatedAt: time.Now()}
	hidden := &domain.Comment{ID: "c2", PublicationState: domain.StatePublished, Visibility: domain.VisibilityHidden}
	resolve := NewStreamResolver(fakeCommentReader{"c1": public, "c2": hidden})
	ctx := context.Background()

	ev, ok, err := resolve(ctx, stream.Notification{Type: "comment", Post: streamPostID, Comment: "c1"})
	if err != nil || !ok || ev.Type != "comment" {
		t.Fatalf("public comment event = %+v %v %v", ev, ok, err)
	}
	if strings.Contains(string(ev.Data), public.AuthorAccountID) || strings.Contains(string(ev.Data), "author") {
		t.Fatalf("event leaks the author account: %s", ev.Data)
	}
	if !strings.Contains(string(ev.Data), `"like_count":4`) {
		t.Fatalf("event lacks counts: %s", ev.Data)
	}
	for _, id := range []string{"c2", "missing"} {
		if _, ok, err := resolve(ctx, stream.Notification{Type: "comment", Post: streamPostID, Comment: id}); ok || err != nil {
			t.Fatalf("non-public comment %s published: ok=%v err=%v", id, ok, err)
		}
	}
	if ev, ok, _ := resolve(ctx, stream.Notification{Type: "post_reactions", Post: streamPostID, Likes: 7}); !ok || !strings.Contains(string(ev.Data), `"like_count":7`) {
		t.Fatalf("post reactions event = %+v", ev)
	}
	if _, ok, _ := resolve(ctx, stream.Notification{Type: "unknown", Post: streamPostID}); ok {
		t.Fatal("unknown notification type was published")
	}
}
