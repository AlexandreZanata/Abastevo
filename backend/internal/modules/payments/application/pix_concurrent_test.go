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

// Distinct clocks force the divergent-quote interleave that once leaked a
// conflict: racers building different QuotedAt rendezvous on the
// build-then-store boundary and must still converge on one financial
// creation with busy losers, never a conflict.
func TestPixConcurrentStartsConvergeWithDistinctClocks(t *testing.T) {
	service, store, provider := newPixService(t)
	base := time.Now().UTC()
	var tick int64
	// Millisecond steps: the claim payload truncates timestamps to the
	// microsecond, so racers must differ above that precision to force
	// the divergent-quote interleave.
	service.Clock = func() time.Time { return base.Add(time.Duration(atomic.AddInt64(&tick, 1)) * time.Millisecond) }
	const rounds = 5
	totalSuccess := 0
	for round := 0; round < rounds; round++ {
		operation := fmt.Sprintf("44444444-4444-4444-8444-%012d", round)
		arrive := make(chan struct{}, 2)
		release := make(chan struct{})
		var once sync.Once
		releaseIt := func() { once.Do(func() { close(release) }) }
		store.beforeCreate = func() {
			arrive <- struct{}{}
			<-release
		}
		type outcome struct {
			order app.PixOrder
			err   error
		}
		outcomes := make(chan outcome, 2)
		for i := 0; i < 2; i++ {
			go func() {
				order, err := service.Start(context.Background(), app.Principal{AccountID: testAccount}, app.PixStartRequest{
					OperationID: operation, OfferID: testOffer,
					PayerName: "Synthetic Payer", PayerTaxID: "000.000.000-00",
				})
				outcomes <- outcome{order: order, err: err}
			}()
		}
		payments := map[string]bool{}
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
					if result.order.PaymentID == "" {
						t.Fatalf("round %d: missing payment", round)
					}
					payments[result.order.PaymentID] = true
					totalSuccess++
				} else if !errors.Is(result.err, domain.ErrBusy) {
					t.Fatalf("round %d: unexpected err = %v", round, result.err)
				}
				releaseIt()
			}
		}
		if len(payments) != 1 {
			t.Fatalf("round %d: payments = %v", round, payments)
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
