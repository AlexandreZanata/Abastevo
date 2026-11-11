package stream

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestHubRoutesEventsOnlyToThePost(t *testing.T) {
	h := NewHub(10, 10, 4)
	a, _ := h.Subscribe("post-a")
	b, _ := h.Subscribe("post-b")
	h.Publish("post-a", Event{Type: "comment", Data: []byte(`{"id":1}`)})
	select {
	case ev := <-a.Events():
		if ev.Type != "comment" {
			t.Fatalf("event = %+v", ev)
		}
	default:
		t.Fatal("subscriber of post-a got nothing")
	}
	select {
	case ev := <-b.Events():
		t.Fatalf("subscriber of post-b got %+v", ev)
	default:
	}
}

func TestHubLimitsAndUnsubscribe(t *testing.T) {
	h := NewHub(3, 2, 1)
	s1, _ := h.Subscribe("p")
	if _, err := h.Subscribe("p"); err != nil {
		t.Fatalf("second subscriber: %v", err)
	}
	if _, err := h.Subscribe("p"); !errors.Is(err, ErrHubFull) {
		t.Fatalf("per-post limit error = %v", err)
	}
	if _, err := h.Subscribe("q"); err != nil {
		t.Fatalf("other post: %v", err)
	}
	if _, err := h.Subscribe("r"); !errors.Is(err, ErrHubFull) {
		t.Fatalf("process limit error = %v", err)
	}
	h.Unsubscribe(s1)
	h.Unsubscribe(s1)
	if h.Count() != 2 {
		t.Fatalf("count = %d after double unsubscribe; want 2", h.Count())
	}
	if _, err := h.Subscribe("r"); err != nil {
		t.Fatalf("slot not released: %v", err)
	}
}

func TestHubSlowReaderIsToldToResyncWithoutBlocking(t *testing.T) {
	h := NewHub(10, 10, 2)
	slow, _ := h.Subscribe("p")
	for i := 0; i < 5; i++ {
		h.Publish("p", Event{Type: "comment_reactions"})
	}
	select {
	case <-slow.Resync():
	default:
		t.Fatal("overflowing reader was not told to resync")
	}
	if len(slow.Events()) != 2 {
		t.Fatalf("buffer holds %d events; want the first 2", len(slow.Events()))
	}
}

func TestHubResyncAllAndConcurrentUse(t *testing.T) {
	h := NewHub(1000, 1000, 8)
	var wg sync.WaitGroup
	subs := make(chan *Subscriber, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := h.Subscribe("p")
			if err != nil {
				t.Error(err)
				return
			}
			subs <- s
			h.Publish("p", Event{Type: "x"})
		}()
	}
	wg.Wait()
	close(subs)
	h.ResyncAll()
	for s := range subs {
		select {
		case <-s.Resync():
		default:
			t.Fatal("subscriber missed ResyncAll")
		}
		h.Unsubscribe(s)
	}
	if h.Count() != 0 || h.HasSubscribers("p") {
		t.Fatalf("hub not empty: %d", h.Count())
	}
}

func TestListenerDispatchSkipsUnwatchedPostsAndResyncsOnResolveError(t *testing.T) {
	h := NewHub(10, 10, 4)
	resolved := 0
	l := &Listener{Hub: h, Resolve: func(_ context.Context, n Notification) (Event, bool, error) {
		resolved++
		if n.Type == "broken" {
			return Event{}, false, errors.New("db down")
		}
		return Event{Type: n.Type, Data: []byte("{}")}, true, nil
	}}
	l.dispatch(context.Background(), `{"type":"comment","post":"nobody"}`)
	if resolved != 0 {
		t.Fatal("resolved an event nobody listens to")
	}
	s, _ := h.Subscribe("p")
	l.dispatch(context.Background(), `not json`)
	l.dispatch(context.Background(), `{"type":"comment","post":"p"}`)
	l.dispatch(context.Background(), `{"type":"broken","post":"p"}`)
	if got := (<-s.Events()).Type; got != "comment" {
		t.Fatalf("first event = %q", got)
	}
	if got := (<-s.Events()).Type; got != "resync" {
		t.Fatalf("resolve failure event = %q; want resync", got)
	}
}
