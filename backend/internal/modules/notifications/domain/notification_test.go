package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	recipient = "11111111-1111-4111-8111-111111111111"
	actor     = "22222222-2222-4222-8222-222222222222"
	topic     = "33333333-3333-4333-8333-333333333333"
	comment   = "44444444-4444-4444-8444-444444444444"
	obs       = "55555555-5555-4555-8555-555555555555"
	station   = "66666666-6666-4666-8666-666666666666"
)

var now = time.Date(2026, 11, 28, 12, 30, 0, 123456789, time.UTC)

func TestForumReplyPayloadKeyAndExpiry(t *testing.T) {
	e, err := NewForumReply(recipient, actor, ForumReply{TopicID: topic, TopicTitle: " Preço do etanol ", CommentID: comment, AuthorName: "Motorista SP"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != KindForumReply || e.EventKey != "forum.reply:"+comment || e.AccountID != recipient {
		t.Fatalf("event identity %+v", e)
	}
	if want := `{"topic_id":"` + topic + `","topic_title":"Preço do etanol","comment_id":"` + comment + `","author_name":"Motorista SP"}`; string(e.Payload) != want {
		t.Fatalf("payload %s", e.Payload)
	}
	if !e.CreatedAt.Equal(now.Truncate(time.Microsecond)) || e.ExpiresAt.Sub(e.CreatedAt) != 30*24*time.Hour {
		t.Fatalf("expiry %v %v", e.CreatedAt, e.ExpiresAt)
	}
}

func TestForumReplyNeverNotifiesTheActor(t *testing.T) {
	_, err := NewForumReply(recipient, recipient, ForumReply{TopicID: topic, TopicTitle: "t", CommentID: comment}, now)
	if !errors.Is(err, ErrSelfNotification) {
		t.Fatalf("self reply: %v", err)
	}
}

func TestForumReplyEmptyAuthorNameStaysPresent(t *testing.T) {
	e, err := NewForumReply(recipient, actor, ForumReply{TopicID: topic, TopicTitle: "t", CommentID: comment}, now)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(e.Payload, &got); err != nil {
		t.Fatal(err)
	}
	if name, ok := got["author_name"]; !ok || name != "" {
		t.Fatalf("author_name must be present even when empty: %s", e.Payload)
	}
}

func TestForumReplyClipsDisplayTextToBounds(t *testing.T) {
	long := strings.Repeat("é", 300)
	e, err := NewForumReply(recipient, actor, ForumReply{TopicID: topic, TopicTitle: long, CommentID: comment, AuthorName: long + "\x00\n"}, now)
	if err != nil {
		t.Fatal(err)
	}
	var p ForumReply
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(p.TopicTitle) != MaxTopicTitle || utf8.RuneCountInString(p.AuthorName) != MaxAuthorName {
		t.Fatalf("clip %d %d", utf8.RuneCountInString(p.TopicTitle), utf8.RuneCountInString(p.AuthorName))
	}
	if len(e.Payload) > MaxPayloadBytes {
		t.Fatalf("payload %d bytes", len(e.Payload))
	}
}

func TestForumReplyRejectsMalformedFacts(t *testing.T) {
	for name, p := range map[string]ForumReply{
		"topic id":    {TopicID: "nope", TopicTitle: "t", CommentID: comment},
		"comment id":  {TopicID: topic, TopicTitle: "t", CommentID: "ABCDEF01-2345-4678-89AB-CDEF01234567"},
		"blank title": {TopicID: topic, TopicTitle: " \t ", CommentID: comment},
	} {
		if _, err := NewForumReply(recipient, actor, p, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := NewForumReply("", actor, ForumReply{TopicID: topic, TopicTitle: "t", CommentID: comment}, now); !errors.Is(err, ErrInvalid) {
		t.Errorf("recipient: %v", err)
	}
	if _, err := NewForumReply(recipient, actor, ForumReply{TopicID: topic, TopicTitle: "t", CommentID: comment}, time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero clock: %v", err)
	}
}

func TestContributionValidatedAndRejected(t *testing.T) {
	ok, err := NewContributionDecision(recipient, true, Contribution{ObservationID: obs, StationID: station, StationName: "Posto Central", Fuel: "ETHANOL"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Kind != KindContributionValidated || ok.EventKey != "contribution.validated:"+obs {
		t.Fatalf("validated %+v", ok)
	}
	if want := `{"observation_id":"` + obs + `","station_id":"` + station + `","station_name":"Posto Central","fuel":"ETHANOL"}`; string(ok.Payload) != want {
		t.Fatalf("validated payload %s", ok.Payload)
	}
	no, err := NewContributionDecision(recipient, false, Contribution{ObservationID: obs, StationID: station, StationName: "Posto Central", Fuel: "DIESEL_S10", Reason: "evidence-timeout"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if no.Kind != KindContributionRejected || no.EventKey != "contribution.rejected:"+obs {
		t.Fatalf("rejected %+v", no)
	}
	if !strings.Contains(string(no.Payload), `"reason":"evidence-timeout"`) {
		t.Fatalf("rejected payload %s", no.Payload)
	}
}

func TestContributionKeepsEmptyStationNamePresent(t *testing.T) {
	e, err := NewContributionDecision(recipient, true, Contribution{ObservationID: obs, StationID: station, Fuel: "CNG"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(e.Payload), `"station_name":""`) {
		t.Fatalf("station_name must be present: %s", e.Payload)
	}
}

func TestContributionRejectsMalformedFacts(t *testing.T) {
	base := Contribution{ObservationID: obs, StationID: station, StationName: "x", Fuel: "ETHANOL"}
	cases := map[string]struct {
		validated bool
		mutate    func(*Contribution)
	}{
		"observation id":      {true, func(c *Contribution) { c.ObservationID = "x" }},
		"station id":          {true, func(c *Contribution) { c.StationID = "" }},
		"lower-case fuel":     {true, func(c *Contribution) { c.Fuel = "ethanol" }},
		"long fuel":           {true, func(c *Contribution) { c.Fuel = strings.Repeat("A", 33) }},
		"reason on validated": {true, func(c *Contribution) { c.Reason = "evidence-timeout" }},
		"missing reason":      {false, func(c *Contribution) {}},
		"free-text reason":    {false, func(c *Contribution) { c.Reason = "Bad photo, sorry" }},
	}
	for name, tc := range cases {
		c := base
		tc.mutate(&c)
		if _, err := NewContributionDecision(recipient, tc.validated, c, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLimitBounds(t *testing.T) {
	if got, err := Limit(nil); err != nil || got != 30 {
		t.Fatalf("default %d %v", got, err)
	}
	for _, n := range []int{1, 50} {
		if got, err := Limit(&n); err != nil || got != n {
			t.Fatalf("%d: %d %v", n, got, err)
		}
	}
	for _, n := range []int{0, -1, 51} {
		if _, err := Limit(&n); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%d admitted", n)
		}
	}
}

func TestSelection(t *testing.T) {
	all, err := NewSelection(nil, true)
	if err != nil || !all.All {
		t.Fatalf("all %+v %v", all, err)
	}
	ids, err := NewSelection([]string{comment, comment, topic}, false)
	if err != nil || len(ids.IDs) != 2 || ids.All {
		t.Fatalf("ids %+v %v", ids, err)
	}
	fifty := make([]string, 50)
	for i := range fifty {
		fifty[i] = comment
	}
	if _, err := NewSelection(fifty, false); err != nil {
		t.Fatalf("50 ids refused: %v", err)
	}
	for name, tc := range map[string]struct {
		ids []string
		all bool
	}{
		"empty":       {nil, false},
		"both forms":  {[]string{comment}, true},
		"too many":    {append(fifty, comment), false},
		"not uuid":    {[]string{"1"}, false},
		"upper uuid":  {[]string{"ABCDEF01-2345-4678-89AB-CDEF01234567"}, false},
		"empty entry": {[]string{""}, false},
	} {
		if _, err := NewSelection(tc.ids, tc.all); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s admitted", name)
		}
	}
}

func TestClip(t *testing.T) {
	if got := Clip("  a\tb\x7f ", 10); got != "a b" {
		t.Fatalf("control chars %q", got)
	}
	if got := Clip("abc\xff", 10); got != "abc" {
		t.Fatalf("invalid utf8 %q", got)
	}
	if got := Clip("abcdef", 3); got != "abc" {
		t.Fatalf("clip %q", got)
	}
}
