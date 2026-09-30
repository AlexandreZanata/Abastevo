// Package mail owns the local deterministic mail adapter (P13-T02C, A02).
// Delivery records code issuances in memory for tests and local runs; it
// performs no network I/O, logs neither addresses nor codes, and exposes
// the outbox only to tests in the same process. SMTP delivery lands with
// deployment configuration, never silently.
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

// Outbox is the memory MailSender. Not safe for production delivery.
type Outbox struct {
	mu    sync.Mutex
	clock domain.Clock
	sent  []Sent
}

// NewOutbox returns an empty outbox wired to a clock.
func NewOutbox(clock domain.Clock) *Outbox {
	return &Outbox{clock: clock}
}

// SendCode implements domain.MailSender by recording the issuance.
func (o *Outbox) SendCode(_ context.Context, address, code string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sent = append(o.sent, Sent{Address: address, Code: code, At: o.clock.NowUnix()})
	return nil
}

// All returns a copy of the recorded issuances for tests.
func (o *Outbox) All() []Sent {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Sent(nil), o.sent...)
}
