package application_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// The card path shares the order double: distinct clocks must converge on
// one checkout session with busy losers, never a conflict.
func TestConcurrentStartsConvergeWithDistinctClocks(t *testing.T) {
	service, store, provider := newService(t)
	base := time.Now().UTC()
	var tick int64
	// Millisecond steps: claim payloads truncate timestamps to the
	// microsecond, so racers must differ above that precision to force
	// the divergent-quote interleave.
	service.Clock = func() time.Time { return base.Add(time.Duration(atomic.AddInt64(&tick, 1)) * time.Millisecond) }
	const rounds = 5
	totalSuccess := 0
	for round := 0; round < rounds; round++ {
		operation := fmt.Sprintf("55555555-5555-4555-8555-%012d", round)
		arrive := make(chan struct{}, 2)
		release := make(chan struct{})
		var once sync.Once
		releaseIt := func() { once.Do(func() { close(release) }) }
		store.beforeCreate = func() {
			arrive <- struct{}{}
			<-release
		}
		type outcome struct {
			checkout app.Checkout
			err      error
		}
		outcomes := make(chan outcome, 2)
		for i := 0; i < 2; i++ {
			go func() {
				checkout, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, app.StartRequest{
					OperationID: operation, OfferID: testOffer,
				})
				outcomes <- outcome{checkout: checkout, err: err}
			}()
		}
		sessions := map[string]bool{}
		completed, arrivals := 0, 0
		for completed < 2 {
			select {
			case <-arrive:
				arrivals++
				if arrivals == 2 {
					releaseIt()
				}
			case result := <-outcomes:
				completed++
				if result.err == nil {
					if result.checkout.SessionID == "" {
						t.Fatalf("round %d: missing session", round)
					}
					sessions[result.checkout.SessionID] = true
					totalSuccess++
				} else if !errors.Is(result.err, domain.ErrBusy) {
					t.Fatalf("round %d: unexpected err = %v", round, result.err)
				}
				releaseIt()
			}
		}
		if len(sessions) != 1 {
			t.Fatalf("round %d: sessions = %v", round, sessions)
		}
	}
	store.beforeCreate = nil
	if totalSuccess == 0 {
		t.Fatal("no successful start")
	}
	if provider.createCount != rounds {
		t.Fatalf("provider creates = %d want %d", provider.createCount, rounds)
	}
	if len(store.orders) != rounds {
		t.Fatalf("orders = %d want %d", len(store.orders), rounds)
	}
}
