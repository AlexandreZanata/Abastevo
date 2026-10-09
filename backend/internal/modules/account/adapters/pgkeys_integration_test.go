//go:build integration

package adapters

import (
	"context"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// Key accounts run the real Postgres path: credential persistence,
// key-only login sessions and the parallel-signup race.
func TestPGKeyAccountRoundTrip(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	created, err := svc.CreateKeyAccount(ctx, "chaveana")
	if err != nil {
		t.Fatalf("CreateKeyAccount: %v", err)
	}
	if created.Account.Alias != "chaveana" {
		t.Fatalf("alias = %q", created.Account.Alias)
	}
	logged, err := svc.LoginWithKey(ctx, created.Key)
	if err != nil {
		t.Fatalf("LoginWithKey: %v", err)
	}
	if logged.Account.ID != created.Account.ID || logged.Session.AccessToken == "" {
		t.Fatalf("login must return the account session: %+v", logged)
	}
	wrong := created.Key[:31] + "q"
	if wrong == created.Key {
		wrong = created.Key[:31] + "w"
	}
	if _, err := svc.LoginWithKey(ctx, wrong); err != domain.ErrKeyInvalid {
		t.Fatalf("wrong key must fail closed, got %v", err)
	}
}

func TestPGParallelKeySignupAdmitsExactlyOne(t *testing.T) {
	svc, _, _ := freshService(t)
	ctx := context.Background()
	const racers = 8
	var wg sync.WaitGroup
	wins := make(chan string, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := svc.CreateKeyAccount(ctx, "corrida")
			if err != nil {
				return
			}
			wins <- got.Account.ID
		}()
	}
	wg.Wait()
	close(wins)
	ids := map[string]bool{}
	for id := range wins {
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Fatalf("parallel signup must converge on one account, got %d", len(ids))
	}
}
