package domain_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// B-BR-EMP-320: the closed vocabulary keeps its fixed order and every
// reserved type is recognised but not supported; an unknown kind sorts last
// at an equal instant and never before the trip's own facts.
func TestLogbookVocabularyAndTimelineKinds(t *testing.T) {
	types := trip.LogbookTypes()
	if len(types) != 13 || types[0] != trip.LogbookNote || types[12] != trip.LogbookCancel {
		t.Fatal("logbook vocabulary order", types)
	}
	types[0] = "CHANGED"
	if trip.LogbookTypes()[0] != trip.LogbookNote {
		t.Fatal("the vocabulary is shared mutable state")
	}
	supported := 0
	for _, kind := range trip.LogbookTypes() {
		if kind.Supported() {
			supported++
		}
		if kind.RequiresText() != (kind == trip.LogbookNote) {
			t.Fatal("only NOTE requires a text", kind)
		}
	}
	if supported != 4 {
		t.Fatal("supported logbook types", supported)
	}
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	items := []trip.TimelineItem{{Kind: "UNKNOWN", ID: "a", At: at}, {Kind: trip.TimelineFinish, ID: "t", At: at}, {Kind: trip.TimelineStart, ID: "t", At: at}}
	trip.SortTimeline(items)
	if items[0].Kind != trip.TimelineStart || items[1].Kind != trip.TimelineFinish || items[2].Kind != "UNKNOWN" {
		t.Fatal("unknown timeline kind order", items)
	}
}

// Logbook values never print, serialize or log their private content.
func TestLogbookValuesAreRedacted(t *testing.T) {
	defer guard(t)
	s := scope(t, 1)
	in := startInput(t, s)
	started := must(trip.Start(s, in))
	text := "Painel apagado"
	at := *in.Declared
	at.OccurredAt = at.OccurredAt.Add(time.Hour)
	event := trip.LogbookInput{Reference: must(trip.NewLogbookRef(s, "5e000000-0000-4000-8000-000000000081")), Author: in.Author, Type: "NOTE", Text: &text, Declared: &at}
	entry := must(started.Log(s, event))
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, untimedLog))
	for _, v := range []any{event, entry, event.Reference} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			assertRedacted(t, fmt.Sprintf(format, v))
		}
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		assertRedacted(t, string(raw))
		logger.Info("logbook", "value", v)
	}
	assertRedacted(t, logs.String())
	if v := entry.Text(); v == nil || *v != text {
		t.Fatal("entry text", v)
	} else {
		*v = "changed"
		if *entry.Text() != text {
			t.Fatal("entry shares its text")
		}
	}
}
