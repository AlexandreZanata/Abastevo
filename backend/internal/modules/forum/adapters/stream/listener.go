package stream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// Channel is the PostgreSQL notification channel written by migration 000164.
const Channel = "forum_events"

// Notification is the payload of one committed public change.
type Notification struct {
	Type     string  `json:"type"`
	Post     string  `json:"post"`
	Comment  string  `json:"comment,omitempty"`
	Parent   *string `json:"parent,omitempty"`
	Likes    int64   `json:"likes"`
	Dislikes int64   `json:"dislikes"`
}

// Resolver turns a notification into the event clients receive, reading the
// public projection when needed. ok=false drops it (for example, a comment
// hidden again before it could be read).
type Resolver func(ctx context.Context, n Notification) (ev Event, ok bool, err error)

// Listener holds one LISTEN connection and feeds the hub.
type Listener struct {
	// Connect returns a dedicated connection, outside any pool.
	Connect func(ctx context.Context) (*pgx.Conn, error)
	Hub     *Hub
	Resolve Resolver
	// Report receives operational failures ("disconnected", "event_dropped").
	Report func(event string, err error)
	// MinBackoff and MaxBackoff bound reconnect waits.
	MinBackoff, MaxBackoff time.Duration
}

// Run listens until ctx ends, reconnecting with backoff. After any
// reconnection every open stream is told to resync, because notifications
// sent while disconnected are gone.
func (l *Listener) Run(ctx context.Context) {
	backoff := l.MinBackoff
	reconnecting := false
	for ctx.Err() == nil {
		err := l.session(ctx, reconnecting)
		if ctx.Err() != nil {
			return
		}
		reconnecting = true
		l.Hub.ResyncAll()
		l.report("disconnected", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, l.MaxBackoff)
	}
}

func (l *Listener) session(ctx context.Context, reconnecting bool) error {
	conn, err := l.Connect(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	if reconnecting {
		// Streams opened while we were away may have missed events.
		l.Hub.ResyncAll()
	}
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		l.dispatch(ctx, n.Payload)
	}
}

func (l *Listener) dispatch(ctx context.Context, payload string) {
	var n Notification
	if err := json.Unmarshal([]byte(payload), &n); err != nil || n.Post == "" {
		return
	}
	if !l.Hub.HasSubscribers(n.Post) {
		return
	}
	ev, ok, err := l.Resolve(ctx, n)
	if err != nil {
		// The reader cannot trust its view anymore; let it reload.
		l.report("event_dropped", err)
		l.Hub.Publish(n.Post, Event{Type: "resync", Data: []byte("{}")})
		return
	}
	if ok {
		l.Hub.Publish(n.Post, ev)
	}
}

func (l *Listener) report(event string, err error) {
	if l.Report != nil {
		l.Report(event, err)
	}
}
