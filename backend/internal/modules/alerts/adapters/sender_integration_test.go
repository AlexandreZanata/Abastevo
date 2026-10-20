//go:build integration

package adapters

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
)

type fakeProvider struct {
	mu      sync.Mutex
	calls   int
	payload []application.PushPayload
	tokens  []string
	fail    error
	lost    bool
}

func (f *fakeProvider) Send(_ context.Context, token string, payload application.PushPayload) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.tokens = append(f.tokens, token)
	f.payload = append(f.payload, payload)
	if f.lost {
		return "", errors.New("provider: lost response")
	}
	if f.fail != nil {
		return "", f.fail
	}
	return "provider-msg-1", nil
}

func TestSenderInvalidTokenFaultsCrossAccount(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	fake := &fakeProvider{}
	store.Sender = fake
	ctx := context.Background()
	now := alertEvaluationTime()
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-time.Hour), true)
	if _, err := store.Evaluate(ctx, owner, ruleID, "", 100, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dispatch(ctx, now, 25); err != nil {
		t.Fatal(err)
	}
	var inboxID string
	if err := pool.QueryRow(ctx, "SELECT inbox_id FROM alert_inbox WHERE account_id=$1 AND status='dispatched' LIMIT 1", owner).Scan(&inboxID); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", strings.Repeat("t", 257), "bad\x00token"} {
		if _, err := store.SendInbox(ctx, owner, inboxID, bad); err == nil {
			t.Fatalf("invalid token admitted %q", bad)
		}
	}
	if fake.calls != 0 {
		t.Fatal("provider called for invalid token")
	}
	if err := store.EnrollToken(ctx, owner, "valid-token-1"); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []error{errors.New("provider: 429 rate limited"), errors.New("provider: 5xx unavailable"), errors.New("provider: timeout"), errors.New("provider: network unreachable")} {
		fake.fail = fault
		if _, err := store.SendInbox(ctx, owner, inboxID, "valid-token-1"); err == nil {
			t.Fatalf("fault hidden %v", fault)
		}
	}
	fake.fail = nil
	foreign := newAccount(t, pool)
	if err := store.EnrollToken(ctx, foreign, "shared-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnrollToken(ctx, owner, "shared-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SendInbox(ctx, foreign, inboxID, "shared-token"); err == nil {
		t.Fatal("cross-account token reuse admitted")
	}
	if fake.calls != 4 {
		t.Fatalf("provider calls %d", fake.calls)
	}
}

func TestSenderSwitchRevokeStaleLeakageIdempotent(t *testing.T) {
	pool := freshDB(t)
	owner := newAccount(t, pool)
	station := newStation(t, pool)
	store := NewStore(pool)
	fake := &fakeProvider{}
	store.Sender = fake
	ctx := context.Background()
	now := alertEvaluationTime()
	ruleID := newid(t)
	if _, err := store.Apply(ctx, owner, application.Batch{Commands: []application.Command{{OperationID: newid(t), RuleID: ruleID, Payload: rulePayload(t, station)}}}); err != nil {
		t.Fatal(err)
	}
	insertCommunityPrice(t, pool, station, "ETHANOL", "L", 6000, "STANDARD", now.Add(-time.Hour), true)
	if _, err := store.Evaluate(ctx, owner, ruleID, "", 100, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dispatch(ctx, now, 25); err != nil {
		t.Fatal(err)
	}
	var inboxID string
	if err := pool.QueryRow(ctx, "SELECT inbox_id FROM alert_inbox WHERE account_id=$1 AND status='dispatched' LIMIT 1", owner).Scan(&inboxID); err != nil {
		t.Fatal(err)
	}
	if err := store.EnrollToken(ctx, owner, "old-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnrollToken(ctx, owner, "new-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeToken(ctx, owner, "old-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SendInbox(ctx, owner, inboxID, "old-token"); err == nil {
		t.Fatal("revoked send admitted")
	}
	messageID, err := store.SendInbox(ctx, owner, inboxID, "new-token")
	if err != nil || messageID == "" {
		t.Fatalf("send %+v %v", messageID, err)
	}
	if _, err = store.SendInbox(ctx, owner, inboxID, "new-token"); err == nil {
		t.Fatal("duplicate send admitted")
	}
	for _, payload := range fake.payload {
		raw := payload.Title + payload.Body + payload.Rule + payload.Inbox
		for _, secret := range []string{"new-token", "synthetic", "contributor", "refresh", "access_token"} {
			if strings.Contains(raw, secret) {
				t.Fatalf("payload leaked %s", secret)
			}
		}
	}
	if err = store.EraseAll(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SendInbox(ctx, owner, inboxID, "new-token"); err == nil {
		t.Fatal("stale deep link admitted")
	}
}
