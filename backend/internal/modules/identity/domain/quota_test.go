package domain

import (
	"strings"
	"testing"
	"time"
)

func TestQuotaPolicyLookup(t *testing.T) {
	p := DefaultQuotaPolicy()
	if p.Version != 1 {
		t.Errorf("version = %d", p.Version)
	}
	for _, op := range []string{OperationRegister, OperationWrite, OperationChallenge} {
		q, err := p.Lookup(op)
		if err != nil || q.Limit <= 0 || q.Window <= 0 {
			t.Errorf("%s = %+v, %v", op, q, err)
		}
	}
	if _, err := p.Lookup("nope"); err == nil {
		t.Error("unknown operation accepted")
	}
	empty := QuotaPolicy{Version: 2, Ops: map[string]OperationQuota{}}
	if _, err := empty.Lookup(OperationWrite); err == nil {
		t.Error("empty policy accepted")
	}
}

func TestHashIPSubject(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	a, err := HashIPSubject("k1", key, "192.0.2.10")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(a, "ip:k1:") {
		t.Errorf("subject = %q", a)
	}
	b, err := HashIPSubject("k1", key, "192.0.2.10")
	if err != nil || a != b {
		t.Errorf("unstable digest: %q vs %q, %v", a, b, err)
	}
	c, err := HashIPSubject("k2", key, "192.0.2.10")
	if err != nil || c == a {
		t.Errorf("rotation does not retire digests: %q", c)
	}
	d, err := HashIPSubject("k1", key, "192.0.2.11")
	if err != nil || d == a {
		t.Errorf("addresses collide: %q", d)
	}
	if strings.Contains(a, "192.0.2.10") {
		t.Errorf("raw IP leaks into subject: %q", a)
	}
	for _, tc := range []struct {
		keyID, ip string
		key       []byte
	}{
		{"", "192.0.2.10", key},
		{"k1", "192.0.2.10", nil},
		{"k1", "not-an-ip", key},
		{"k1", "", key},
	} {
		if _, err := HashIPSubject(tc.keyID, tc.key, tc.ip); err == nil {
			t.Errorf("bad input accepted: %+v", tc)
		}
	}
}

func TestWindowStart(t *testing.T) {
	now := time.Date(2026, 9, 28, 15, 4, 5, 0, time.UTC)
	if got := WindowStart(now, time.Hour); !got.Equal(time.Date(2026, 9, 28, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("window = %v", got)
	}
}
