package incident_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
)

// The 08-state-model table: every (state, action) pair, nothing else.
func TestIncidentTransitionTableIsExactlyTheStateModel(t *testing.T) {
	want := map[incident.State][]incident.Action{
		incident.Reported:     {incident.Acknowledge, incident.Treat, incident.Resolve, incident.Dismiss},
		incident.Acknowledged: {incident.Treat, incident.Resolve, incident.Dismiss},
		incident.InTreatment:  {incident.Resolve, incident.Dismiss},
	}
	for _, s := range incident.States() {
		for _, a := range incident.Actions() {
			allowed := false
			for _, w := range want[s] {
				allowed = allowed || w == a
			}
			if incident.Allowed(s, a) != allowed {
				t.Errorf("%s -> %s: allowed=%v", s, a, !allowed)
			}
		}
		if s.Terminal() != (s == incident.Resolved || s == incident.Dismissed) {
			t.Error("terminal states", s)
		}
	}
	targets := map[incident.Action]incident.State{incident.Acknowledge: incident.Acknowledged, incident.Treat: incident.InTreatment, incident.Resolve: incident.Resolved, incident.Dismiss: incident.Dismissed}
	for a, s := range targets {
		if a.Target() != s || a.NeedsNote() != (a != incident.Acknowledge) {
			t.Error("action target or note policy", a)
		}
	}
	if incident.Action("OPEN").Target() != "" || incident.Allowed(incident.Reported, "OPEN") {
		t.Error("unknown action admitted")
	}
}

func TestIncidentClosedVocabularies(t *testing.T) {
	for _, raw := range []string{"DAMAGE", "INCIDENT"} {
		if v, err := incident.ParseReportType(raw); err != nil || string(v) != raw {
			t.Error("report type", raw)
		}
	}
	for _, raw := range []string{"BREAKDOWN", "ACCIDENT", "THEFT", "FINE_NOTICE", "OTHER"} {
		if v, err := incident.ParseKind(raw); err != nil || string(v) != raw {
			t.Error("kind", raw)
		}
	}
	for _, raw := range []string{"REPORTED", "ACKNOWLEDGED", "IN_TREATMENT", "RESOLVED", "DISMISSED"} {
		if v, err := incident.ParseState(raw); err != nil || string(v) != raw {
			t.Error("state", raw)
		}
	}
	for _, raw := range []string{"ACKNOWLEDGE", "TREAT", "RESOLVE", "DISMISS"} {
		if v, err := incident.ParseAction(raw); err != nil || string(v) != raw {
			t.Error("action", raw)
		}
	}
	for _, raw := range []string{"", "damage", " DAMAGE", "FIRE"} {
		if _, err := incident.ParseReportType(raw); !errors.Is(err, incident.ErrInvalid) {
			t.Error("report type admitted", raw)
		}
	}
	for _, raw := range []string{"", "theft", "FLOOD"} {
		if _, err := incident.ParseKind(raw); !errors.Is(err, incident.ErrInvalid) {
			t.Error("kind admitted", raw)
		}
	}
	for _, raw := range []string{"", "OPEN", "CLOSED", "reported"} {
		if _, err := incident.ParseState(raw); !errors.Is(err, incident.ErrInvalid) {
			t.Error("state admitted", raw)
		}
		if _, err := incident.ParseAction(raw); !errors.Is(err, incident.ErrInvalid) {
			t.Error("action admitted", raw)
		}
	}
	if len(incident.Kinds()) != 5 || len(incident.States()) != 5 || len(incident.Actions()) != 4 {
		t.Error("vocabulary sizes")
	}
	k := incident.Kinds()
	k[0] = "MUTATED"
	if incident.Kinds()[0] != incident.Breakdown {
		t.Error("vocabulary is shared mutable state")
	}
}

// Report text never reaches a formatting, JSON or slog surface (BR-PRIV-008).
func TestIncidentReportTextIsRedacted(t *testing.T) {
	const secret = "PRIVATE-DESCRIPTION-7731"
	location := "PRIVATE-LOCATION-7731"
	note := "PRIVATE-NOTE-7731"
	r, err := incident.NewReport(incident.Draft{Type: incident.Damage, Description: secret, Location: &location})
	if err != nil {
		t.Fatal(err)
	}
	if r.Description != secret || r.Location == nil || *r.Location != location || &location == r.Location {
		t.Fatal("report text must be kept verbatim in an owned copy")
	}
	c := incident.Command{Action: incident.Resolve, ExpectedRevision: 1, Note: &note, Handler: []string{"9c000000-0000-4000-8000-000000005002"}}
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}))
	logger.Info("report", "value", r, "draft", incident.Draft{Description: secret, Location: &location}, "command", c)
	encoded, _ := json.Marshal([]any{r, c})
	for _, out := range []string{fmt.Sprint(r), fmt.Sprintf("%+v %#v %v", r, c, c), string(encoded), buffer.String()} {
		for _, private := range []string{secret, location, note} {
			if strings.Contains(out, private) {
				t.Fatalf("private report text leaked: %s", out)
			}
		}
	}
}

func TestIncidentTextProfileMatchesTheSQLProfile(t *testing.T) {
	for raw, want := range map[string]bool{
		"a": true, " a ": true, strings.Repeat("ã", 10): true, "\u200bx": true,
		"": false, " ": false, "\u3000": false, "\u00a0": false, "\u2028": false, "a\nb": false, "a\tb": false, "a\u0085": false, "a\x7f": false, "a\xff": false,
	} {
		if incident.ValidText(raw, 10) != want {
			t.Errorf("%q: want %v", raw, want)
		}
	}
	if incident.ValidText(strings.Repeat("a", 11), 10) || !incident.ValidText(strings.Repeat("ã", 10), 10) {
		t.Error("scalar bound")
	}
}
