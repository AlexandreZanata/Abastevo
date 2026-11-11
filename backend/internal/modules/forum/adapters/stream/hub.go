// Package stream fans committed forum changes out to open conversations
// (B-BR-FOR-18). One LISTEN connection per API process feeds an in-memory
// hub; each open conversation holds a small buffered channel. A slow or
// overflowing reader is told to resync instead of growing a backlog.
package stream

import (
	"errors"
	"sync"
)

// Event is one server-sent event: its name and JSON payload.
type Event struct {
	Type string
	Data []byte
}

// ErrHubFull reports that the process holds its maximum of open streams.
var ErrHubFull = errors.New("forum stream: subscriber limit reached")

// Subscriber is one open conversation stream.
type Subscriber struct {
	postID string
	events chan Event
	resync chan struct{}
	once   sync.Once
}

// Events delivers the conversation's events in commit order.
func (s *Subscriber) Events() <-chan Event { return s.events }

// Resync is closed when events may have been lost; the client must reload.
func (s *Subscriber) Resync() <-chan struct{} { return s.resync }

func (s *Subscriber) markResync() { s.once.Do(func() { close(s.resync) }) }

// Hub routes events to the subscribers of each post.
type Hub struct {
	mu         sync.RWMutex
	posts      map[string]map[*Subscriber]struct{}
	total      int
	maxTotal   int
	maxPerPost int
	buffer     int
}

// NewHub bounds the open streams per process and per post and the buffered
// events per stream.
func NewHub(maxTotal, maxPerPost, buffer int) *Hub {
	return &Hub{
		posts:      make(map[string]map[*Subscriber]struct{}),
		maxTotal:   maxTotal,
		maxPerPost: maxPerPost,
		buffer:     buffer,
	}
}

// Subscribe opens a stream for postID or reports ErrHubFull.
func (h *Hub) Subscribe(postID string) (*Subscriber, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.posts[postID]
	if h.total >= h.maxTotal || len(subs) >= h.maxPerPost {
		return nil, ErrHubFull
	}
	if subs == nil {
		subs = make(map[*Subscriber]struct{})
		h.posts[postID] = subs
	}
	s := &Subscriber{postID: postID, events: make(chan Event, h.buffer), resync: make(chan struct{})}
	subs[s] = struct{}{}
	h.total++
	return s, nil
}

// Unsubscribe closes the stream's slot; calling it twice is harmless.
func (h *Hub) Unsubscribe(s *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.posts[s.postID]
	if _, ok := subs[s]; !ok {
		return
	}
	delete(subs, s)
	h.total--
	if len(subs) == 0 {
		delete(h.posts, s.postID)
	}
}

// HasSubscribers lets the listener skip work for conversations nobody reads.
func (h *Hub) HasSubscribers(postID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.posts[postID]) > 0
}

// Count returns the open streams of the process.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.total
}

// Publish never blocks: a subscriber whose buffer is full is told to resync.
func (h *Hub) Publish(postID string, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.posts[postID] {
		select {
		case s.events <- ev:
		default:
			s.markResync()
		}
	}
}

// ResyncAll asks every open stream to reload, used when the listener lost
// its connection and events may be missing.
func (h *Hub) ResyncAll() {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, subs := range h.posts {
		for s := range subs {
			s.markResync()
		}
	}
}
