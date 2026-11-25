package application_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// loggedValue renders v through the JSON slog handler without the record's
// own timestamp, so leak checks inspect only what the value itself prints.
func loggedValue(v any) string {
	var out bytes.Buffer
	omitTime := func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}
	slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{ReplaceAttr: omitTime})).Info("value", "v", v)
	return out.String()
}

// Leak checks inspect what a value prints through slog. The record's own
// timestamp is not part of the value, and its random digits made numeric
// leak assertions flaky: a time ending in ...1389 looked like a leaked speed.
func TestLoggedValueOmitsTheRecordTime(t *testing.T) {
	out := loggedValue(struct{ Visible string }{"shown"})
	if strings.Contains(out, `"time"`) || !strings.Contains(out, "shown") {
		t.Fatal("a logged value keeps the value and drops the record time:", out)
	}
}
