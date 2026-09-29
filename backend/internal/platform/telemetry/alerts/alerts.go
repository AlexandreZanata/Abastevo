package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Severities. Critical pages; warning notifies the daily channel.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
)

// DefaultCooldown suppresses refires of a continuously firing rule.
const DefaultCooldown = 15 * time.Minute

var (
	ErrBadRule     = errors.New("alerts: invalid rule")
	ErrBadNotifier = errors.New("alerts: invalid notifier")
)

// Rule is one evaluated condition. Check returns firing plus a stable,
// identifier-free detail (error codes and counts, never payload).
type Rule struct {
	Name     string
	Severity string
	Cooldown time.Duration
	Check    func(ctx context.Context) (firing bool, detail string, err error)
}

// Alert is the fired notification payload: fixed fields only, so no
// caller context can leak identifiers into the route.
type Alert struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	FiredAt  string `json:"fired_at"`
}

// Notifier delivers alerts to exactly one operator route.
type Notifier interface {
	Notify(ctx context.Context, alert Alert) error
}

// Evaluator runs rules with per-rule cooldown dedupe.
type Evaluator struct {
	Rules    []Rule
	Notifier Notifier
	Clock    func() time.Time
	mu       sync.Mutex
	lastFire map[string]time.Time
}

func (e *Evaluator) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

// Eval runs every rule: firing rules notify unless still cooling down;
// check errors fail the evaluation loudly (a blind rule never counts
// as green). Returned alerts are the newly notified ones.
func (e *Evaluator) Eval(ctx context.Context) ([]Alert, error) {
	if len(e.Rules) == 0 {
		return nil, fmt.Errorf("%w: no rules", ErrBadRule)
	}
	if e.Notifier == nil {
		return nil, fmt.Errorf("%w: no notifier", ErrBadNotifier)
	}
	now := e.now()
	var fired []Alert
	for _, rule := range e.Rules {
		if rule.Name == "" || rule.Check == nil {
			return nil, fmt.Errorf("%w: %q", ErrBadRule, rule.Name)
		}
		if rule.Severity != SeverityCritical && rule.Severity != SeverityWarning {
			return nil, fmt.Errorf("%w: severity of %q", ErrBadRule, rule.Name)
		}
		firing, detail, err := rule.Check(ctx)
		if err != nil {
			return nil, fmt.Errorf("alerts: check %q: %w", rule.Name, err)
		}
		if !firing {
			continue
		}
		cooldown := rule.Cooldown
		if cooldown <= 0 {
			cooldown = DefaultCooldown
		}
		e.mu.Lock()
		if e.lastFire == nil {
			e.lastFire = map[string]time.Time{}
		}
		last, ok := e.lastFire[rule.Name]
		if ok && now.Sub(last) < cooldown {
			e.mu.Unlock()
			continue
		}
		e.lastFire[rule.Name] = now
		e.mu.Unlock()
		alert := Alert{
			Name: rule.Name, Severity: rule.Severity,
			Detail: detail, FiredAt: now.UTC().Format(time.RFC3339),
		}
		if err := e.Notifier.Notify(ctx, alert); err != nil {
			return nil, fmt.Errorf("alerts: notify %q: %w", rule.Name, err)
		}
		fired = append(fired, alert)
	}
	return fired, nil
}

// LogNotifier writes alerts to the service log: the persistence-free
// route that always works, audited with the regular log retention.
type LogNotifier struct {
	Log func(msg string, args ...any)
}

// Notify implements Notifier.
func (l LogNotifier) Notify(_ context.Context, alert Alert) error {
	if l.Log == nil {
		return fmt.Errorf("%w: no log sink", ErrBadNotifier)
	}
	l.Log("alert firing", "name", alert.Name, "severity", alert.Severity,
		"detail", alert.Detail, "fired_at", alert.FiredAt)
	return nil
}

// WebhookNotifier POSTs alerts as JSON to one operator endpoint with a
// bounded timeout. Non-2xx responses fail loudly for retry.
type WebhookNotifier struct {
	URL     string
	Client  *http.Client
	Timeout time.Duration
}

// Notify implements Notifier.
func (w WebhookNotifier) Notify(ctx context.Context, alert Alert) error {
	if w.URL == "" {
		return fmt.Errorf("%w: empty webhook URL", ErrBadNotifier)
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	raw, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alerts: webhook status %d", resp.StatusCode)
	}
	return nil
}
