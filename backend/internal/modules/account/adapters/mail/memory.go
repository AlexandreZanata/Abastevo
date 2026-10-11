// Package mail owns the local deterministic mail adapter (P13-T02C, A02).
// Delivery records code issuances in a bounded ring for tests and
// local runs; it performs no network I/O, logs neither addresses
// nor codes, and exposes the outbox only to tests in the same
// process. Production uses it only behind explicit local delivery
// configuration; SMTP delivery lands with deployment configuration,
// never silently.
package mail

import (
	"context"
	"sync"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// Sent is one recorded issuance. The code stays inside this struct; nothing
// here formats it into logs or errors.
type Sent struct {
	Address string
	Code    string
	At      int64
}

// DefaultMaxRetained bounds the ring so issuances never grow
// without a ceiling, even under a flood of distinct addresses.
const DefaultMaxRetained = 100

// Outbox is the bounded memory MailSender. Not safe for production
// delivery.
type Outbox struct {
	mu          sync.Mutex
	clock       domain.Clock
	sent        []Sent
	maxRetained int
}

// NewOutbox returns an empty outbox wired to a clock, retaining at
// most maxRetained recent issuances (oldest drops first).
// Non-positive caps clamp to DefaultMaxRetained.
func NewOutbox(clock domain.Clock, maxRetained int) *Outbox {
	if maxRetained <= 0 {
		maxRetained = DefaultMaxRetained
	}
	return &Outbox{clock: clock, maxRetained: maxRetained}
}

// SendCode implements domain.MailSender by recording the issuance,
// dropping the oldest past the ceiling.
func (o *Outbox) SendCode(_ context.Context, address, code string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, Sent{Address: address, Code: code, At: o.clock.NowUnix()})
	if overflow := len(o.sent) - o.maxRetained; overflow > 0 {
		o.sent = append([]Sent(nil), o.sent[overflow:]...)
	}
	return nil
}

// All returns a copy of the recorded issuances for tests.
func (o *Outbox) All() []Sent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Sent(nil), o.sent...)
}
