package application

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// P33-T01 — Adversarial containment campaign (frozen workload: 50-claim
// rival burst, 20-way same-key divergent burst; no live/provider/load
// campaign, no production data).
//
// These tests must PASS against the shipped containment: exactly one
// created claim per (account, key), divergent replays conflict (never a
// second divergent grant), rival accounts stay mutually invisible, and
// quota still bounds a single account. A failure here blocks G33.

func adversarialPorts(store *fakeClaimStore) ClaimPorts {
	var n atomic.Int64
	return ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			return fmt.Sprintf("adv-claim-%d", n.Add(1)), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
}

func TestRivalClaimBurstStaysPrivate(t *testing.T) {
	store := &fakeClaimStore{}
	ports := adversarialPorts(store)
	const accounts = 10
	const perAccount = 5
	var wg sync.WaitGroup
	errs := make(chan error, accounts*perAccount)
	for a := 0; a < accounts; a++ {
		for k := 0; k < perAccount; k++ {
			wg.Add(1)
			go func(a, k int) {
				defer wg.Done()
				account := fmt.Sprintf("rival-%02d", a)
				key := fmt.Sprintf("burst-key-%d", k)
				_, created, err := OpenClaim(context.Background(), ports, account, "station-1", "administrator", []string{"profile.edit"}, key)
				if err != nil {
					errs <- err
					return
				}
				if !created {
					errs <- errors.New("rival burst: distinct (account,key) must create")
				}
			}(a, k)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for a := 0; a < accounts; a++ {
		account := fmt.Sprintf("rival-%02d", a)
		mine, err := ListMine(context.Background(), store, account)
		if err != nil {
			t.Fatal(err)
		}
		if len(mine) != perAccount {
			t.Fatalf("account %s sees %d claims, want %d (leak or loss)", account, len(mine), perAccount)
		}
		for _, c := range mine {
			if owner, dup := seen[c.ID]; dup {
				t.Fatalf("claim %s visible to both %s and %s (cross-account leak)", c.ID, owner, account)
			}
			seen[c.ID] = account
		}
	}
	if len(seen) != accounts*perAccount {
		t.Fatalf("unique visible claims = %d, want %d", len(seen), accounts*perAccount)
	}
}

func TestSameKeyDivergentBurstConflicts(t *testing.T) {
	store := &fakeClaimStore{}
	ports := adversarialPorts(store)
	const workers = 20
	var created atomic.Int64
	var replayed atomic.Int64
	var conflicted atomic.Int64
	var busy atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scopes := []string{"profile.edit"}
			if i%2 == 1 {
				scopes = []string{"reply.official"}
			}
			_, wasCreated, err := OpenClaim(context.Background(), ports, "acc-victim", "station-1", "administrator", scopes, "same-key")
			if err != nil {
				switch {
				case errors.Is(err, ErrClaimConflict):
					conflicted.Add(1)
					return
				case errors.Is(err, ErrClaimBusy):
					// Retryable transient: same request is safe to
					// replay once the winner publishes.
					busy.Add(1)
					return
				}
				t.Errorf("unexpected error: %v", err)
				return
			}
			if wasCreated {
				created.Add(1)
			} else {
				replayed.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if created.Load() != 1 {
		t.Fatalf("created = %d, want exactly 1 (no divergent second grant)", created.Load())
	}
	if replayed.Load()+conflicted.Load()+busy.Load() != workers-1 {
		t.Fatalf("replayed=%d conflicted=%d busy=%d, want sum %d", replayed.Load(), conflicted.Load(), busy.Load(), workers-1)
	}
	if conflicted.Load() == 0 {
		t.Fatal("divergent scopes must conflict at least once (silent merge forbidden)")
	}
}
