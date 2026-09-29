package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testEvaluator(notifier Notifier, rules ...Rule) *Evaluator {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	return &Evaluator{
		Rules: rules, Notifier: notifier,
		Clock: func() time.Time { return base },
	}
}

type captureNotifier struct {
	alerts []Alert
	fail   bool
}

func (c *captureNotifier) Notify(_ context.Context, alert Alert) error {
	if c.fail {
		return errors.New("route down")
	}
	c.alerts = append(c.alerts, alert)
	return nil
}

func okRule(name, severity string) Rule {
	return Rule{Name: name, Severity: severity, Cooldown: time.Minute,
		Check: func(context.Context) (bool, string, error) {
			return true, "code=db_unreachable", nil
		}}
}

func TestEvalFiresAndDedupes(t *testing.T) {
	n := &captureNotifier{}
	ev := testEvaluator(n, okRule("db_down", SeverityCritical))
	fired, err := ev.Eval(context.Background())
	if err != nil || len(fired) != 1 {
		t.Fatalf("eval = %+v, %v", fired, err)
	}
	if fired[0].Name != "db_down" || fired[0].Severity != SeverityCritical {
		t.Errorf("alert = %+v", fired[0])
	}
	// Immediate re-eval stays silent inside cooldown.
	fired, err = ev.Eval(context.Background())
	if err != nil || len(fired) != 0 || len(n.alerts) != 1 {
		t.Errorf("refire = %+v, %v (notified %d)", fired, err, len(n.alerts))
	}
}

func TestEvalSkipsQuietRules(t *testing.T) {
	n := &captureNotifier{}
	ev := testEvaluator(n, Rule{Name: "quiet", Severity: SeverityWarning,
		Check: func(context.Context) (bool, string, error) {
			return false, "", nil
		}})
	fired, err := ev.Eval(context.Background())
	if err != nil || len(fired) != 0 || len(n.alerts) != 0 {
		t.Errorf("quiet rule notified: %+v, %v", fired, err)
	}
}

func TestEvalFailsCheckErrorsLoudly(t *testing.T) {
	n := &captureNotifier{}
	ev := testEvaluator(n, Rule{Name: "blind", Severity: SeverityCritical,
		Check: func(context.Context) (bool, string, error) {
			return false, "", errors.New("db probe failed")
		}})
	if _, err := ev.Eval(context.Background()); err == nil {
		t.Error("blind rule counted as green")
	}
	if len(n.alerts) != 0 {
		t.Error("blind rule notified")
	}
}

func TestEvalRejectsBadRules(t *testing.T) {
	n := &captureNotifier{}
	if _, err := (&Evaluator{Notifier: n}).Eval(context.Background()); err == nil {
		t.Error("empty rules accepted")
	}
	ev := testEvaluator(n, Rule{Name: "", Severity: SeverityCritical,
		Check: func(context.Context) (bool, string, error) { return false, "", nil }})
	if _, err := ev.Eval(context.Background()); err == nil {
		t.Error("nameless rule accepted")
	}
	ev = testEvaluator(n, Rule{Name: "x", Severity: "page-everyone",
		Check: func(context.Context) (bool, string, error) { return false, "", nil }})
	if _, err := ev.Eval(context.Background()); err == nil {
		t.Error("unknown severity accepted")
	}
	if _, err := (&Evaluator{Rules: []Rule{okRule("x", SeverityCritical)}}).Eval(context.Background()); err == nil {
		t.Error("missing notifier accepted")
	}
}

func TestAlertPayloadIsFixedFields(t *testing.T) {
	n := &captureNotifier{}
	ev := testEvaluator(n, okRule("jobs_stuck", SeverityWarning))
	fired, err := ev.Eval(context.Background())
	if err != nil || len(fired) != 1 {
		t.Fatal(err)
	}
	raw, err := json.Marshal(fired[0])
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for field := range decoded {
		switch field {
		case "name", "severity", "detail", "fired_at":
		default:
			t.Errorf("payload leaks extra field %q", field)
		}
	}
	// Details carry codes and counts, never identifiers: a rule that
	// echoes request data surfaces here in review, and this test pins
	// the shape reviewers check.
	if strings.Contains(string(raw), "tok-") || strings.Contains(string(raw), "127.0.0.1") {
		t.Error("identifier leaked into alert payload")
	}
}

func TestWebhookNotifier(t *testing.T) {
	var got Alert
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = raw
		_ = json.Unmarshal(raw, &got)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	n := WebhookNotifier{URL: srv.URL}
	if err := n.Notify(context.Background(), Alert{Name: "backup_stale", Severity: SeverityCritical, Detail: "age_hours=30"}); err != nil {
		t.Fatalf("notify = %v", err)
	}
	if got.Name != "backup_stale" || !strings.Contains(string(body), "age_hours=30") {
		t.Errorf("delivered = %+v %s", got, body)
	}
	if err := (WebhookNotifier{}).Notify(context.Background(), Alert{Name: "x"}); err == nil {
		t.Error("empty webhook URL accepted")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if err := (WebhookNotifier{URL: bad.URL}).Notify(context.Background(), Alert{Name: "x"}); err == nil {
		t.Error("5xx webhook accepted as delivered")
	}
}

func TestLogNotifier(t *testing.T) {
	var logged []any
	n := LogNotifier{Log: func(_ string, args ...any) { logged = args }}
	if err := n.Notify(context.Background(), Alert{Name: "db_down", Severity: SeverityCritical, Detail: "code=unreachable"}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range logged {
		if s, ok := a.(string); ok && s == "db_down" {
			found = true
		}
	}
	if !found {
		t.Errorf("log = %v", logged)
	}
	if err := (LogNotifier{}).Notify(context.Background(), Alert{Name: "x"}); err == nil {
		t.Error("sinkless notifier accepted")
	}
}
