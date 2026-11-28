package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/notifications/domain"
)

const (
	owner   = "11111111-1111-4111-8111-111111111111"
	replier = "22222222-2222-4222-8222-222222222222"
	topicID = "33333333-3333-4333-8333-333333333333"
	comment = "44444444-4444-4444-8444-444444444444"
	obsID   = "55555555-5555-4555-8555-555555555555"
	station = "66666666-6666-4666-8666-666666666666"
)

var clock = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

type fakeInbox struct {
	rows      []Row
	unread    int64
	gotLimit  int
	gotBefore *domain.Position
	marked    int64
	err       error
}

func (f *fakeInbox) Page(_ context.Context, _ string, _ time.Time, before *domain.Position, limit int) ([]Row, int64, error) {
	f.gotLimit, f.gotBefore = limit, before
	if f.err != nil {
		return nil, 0, f.err
	}
	if len(f.rows) > limit {
		return f.rows[:limit], f.unread, nil
	}
	return f.rows, f.unread, nil
}

func (f *fakeInbox) MarkRead(_ context.Context, _ string, _ domain.Selection, _ time.Time) (int64, error) {
	return f.marked, f.err
}

func rows(n int) []Row {
	out := make([]Row, n)
	for i := range out {
		out[i] = Row{ID: comment, Kind: domain.KindForumReply, Payload: []byte(`{}`), CreatedAt: clock.Add(-time.Duration(i) * time.Minute)}
	}
	return out
}

func TestListFetchesOneExtraToCutTheNextPosition(t *testing.T) {
	inbox := &fakeInbox{rows: rows(3), unread: 7}
	page, err := Service{Inbox: inbox, Clock: func() time.Time { return clock }}.List(context.Background(), owner, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if inbox.gotLimit != 3 || len(page.Items) != 2 || page.Unread != 7 {
		t.Fatalf("page %+v limit %d", page, inbox.gotLimit)
	}
	if page.Next == nil || !page.Next.CreatedAt.Equal(clock.Add(-time.Minute)) || page.Items[0].CreatedAt != "2026-11-28T12:00:00Z" {
		t.Fatalf("next %+v items %+v", page.Next, page.Items)
	}
	last, err := Service{Inbox: &fakeInbox{rows: rows(2)}}.List(context.Background(), owner, page.Next, 2)
	if err != nil || last.Next != nil || len(last.Items) != 2 {
		t.Fatalf("last page %+v %v", last, err)
	}
}

func TestListRefusesBadInputAndMapsStoreFailures(t *testing.T) {
	s := Service{Inbox: &fakeInbox{}}
	for _, limit := range []int{0, 51} {
		if _, err := s.List(context.Background(), owner, nil, limit); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("limit %d admitted", limit)
		}
	}
	if _, err := s.List(context.Background(), "nope", nil, 1); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("bad owner admitted")
	}
	if _, err := (Service{Inbox: &fakeInbox{err: errors.New("db down")}}).List(context.Background(), owner, nil, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store failure: %v", err)
	}
	if _, err := (Service{}).List(context.Background(), owner, nil, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing store: %v", err)
	}
}

func TestMarkRead(t *testing.T) {
	s := Service{Inbox: &fakeInbox{marked: 2}}
	if n, err := s.MarkRead(context.Background(), owner, domain.Selection{IDs: []string{comment}}); err != nil || n != 2 {
		t.Fatalf("mark %d %v", n, err)
	}
	if _, err := s.MarkRead(context.Background(), owner, domain.Selection{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty selection admitted")
	}
	if _, err := (Service{Inbox: &fakeInbox{err: errors.New("x")}}).MarkRead(context.Background(), owner, domain.Selection{All: true}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store failure: %v", err)
	}
}

type fakeNotifier struct {
	events []domain.Event
	err    error
}

func (f *fakeNotifier) Notify(_ context.Context, _ pgx.Tx, e domain.Event) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, e)
	return nil
}

func TestForumReplyProducer(t *testing.T) {
	n := &fakeNotifier{}
	p := Producers{Notifier: n, Clock: func() time.Time { return clock }}
	facts := ForumReplyFacts{TopicID: topicID, TopicTitle: "Etanol", TopicAuthorAccountID: owner, CommentID: comment, CommentAuthorAccountID: replier}
	if err := p.ForumReply(context.Background(), nil, facts); err != nil || len(n.events) != 1 || n.events[0].AccountID != owner {
		t.Fatalf("reply %v %+v", err, n.events)
	}
	self := facts
	self.CommentAuthorAccountID = owner
	if err := p.ForumReply(context.Background(), nil, self); err != nil || len(n.events) != 1 {
		t.Fatalf("actor notified: %v %d", err, len(n.events))
	}
	editorial := facts
	editorial.TopicAuthorAccountID = ""
	if err := p.ForumReply(context.Background(), nil, editorial); err != nil || len(n.events) != 1 {
		t.Fatalf("authorless post notified: %v", err)
	}
	failing := Producers{Notifier: &fakeNotifier{err: errors.New("insert failed")}}
	if err := failing.ForumReply(context.Background(), nil, facts); err == nil {
		t.Fatal("notifier failure must roll the event back")
	}
}

func TestContributionProducer(t *testing.T) {
	n := &fakeNotifier{}
	accounts := map[string]string{"tok-bound": owner}
	resolverErr := errors.New("lookup failed")
	p := Producers{Notifier: n, Clock: func() time.Time { return clock }, AccountOfContributor: func(_ context.Context, _ pgx.Tx, ref string) (string, bool, error) {
		if ref == "tok-broken" {
			return "", false, resolverErr
		}
		account, ok := accounts[ref]
		return account, ok, nil
	}}
	facts := ContributionFacts{ContributorRef: "tok-bound", ObservationID: obsID, StationID: station, StationName: "Posto", Fuel: "ETHANOL", Validated: true}
	if err := p.ContributionDecided(context.Background(), nil, facts); err != nil || len(n.events) != 1 || n.events[0].Kind != domain.KindContributionValidated {
		t.Fatalf("validated %v %+v", err, n.events)
	}
	rejected := facts
	rejected.Validated, rejected.Reason = false, "evidence-timeout"
	if err := p.ContributionDecided(context.Background(), nil, rejected); err != nil || len(n.events) != 2 || n.events[1].Kind != domain.KindContributionRejected {
		t.Fatalf("rejected %v %+v", err, n.events)
	}
	unbound := facts
	unbound.ContributorRef = "tok-device-only"
	if err := p.ContributionDecided(context.Background(), nil, unbound); err != nil || len(n.events) != 2 {
		t.Fatalf("contributor without account notified: %v", err)
	}
	broken := facts
	broken.ContributorRef = "tok-broken"
	if err := p.ContributionDecided(context.Background(), nil, broken); !errors.Is(err, resolverErr) {
		t.Fatalf("resolver failure: %v", err)
	}
	if err := (Producers{Notifier: n}).ContributionDecided(context.Background(), nil, facts); err == nil {
		t.Fatal("missing resolver must fail loudly")
	}
}
