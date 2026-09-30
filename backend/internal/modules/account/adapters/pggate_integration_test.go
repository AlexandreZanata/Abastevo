//go:build integration

package adapters

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestPGBindingBlockedMatrix(t *testing.T) {
	f := freshLinkService(t)
	f.svc.KeyProver = newStubKeyProver("contrib-g")
	ctx := context.Background()

	if blocked, err := application.BindingBlocked(ctx, f.svc.Store, "ghost-1"); blocked || err != nil {
		t.Errorf("unbound must not block, got %v %v", blocked, err)
	}
	sess := bindSession(t, f, "bind-g@example.invalid", "482916")
	accID, err := f.svc.ValidateAccess(ctx, sess.FamilyID, sess.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BindContributor(ctx, sess.FamilyID, sess.AccessToken, "contrib-g", "proof-contrib-g"); err != nil {
		t.Fatal(err)
	}
	if blocked, err := application.BindingBlocked(ctx, f.svc.Store, "contrib-g"); blocked || err != nil {
		t.Errorf("bound-active must not block, got %v %v", blocked, err)
	}
	if err := f.svc.SuspendAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := application.BindingBlocked(ctx, f.svc.Store, "contrib-g"); !blocked || err != nil {
		t.Errorf("bound-suspended must block, got %v %v", blocked, err)
	}
	if err := f.svc.DeleteAccount(ctx, accID); err != nil {
		t.Fatal(err)
	}
	if blocked, err := application.BindingBlocked(ctx, f.svc.Store, "contrib-g"); blocked || err != nil {
		t.Errorf("deleted bindings read as unbound, got %v %v", blocked, err)
	}
}

func TestPGSuspendStormClosesSessions(t *testing.T) {
	f := freshLinkService(t)
	ctx := context.Background()
	// One independent login per racer pair so rotations never
	// cross-talk: every racer owns a live session of the same
	// account. Four logins stay inside the hourly issuance quota.
	codes := []string{"482916", "111111", "222222", "333333"}
	sessions := make([]application.Session, 4)
	var accID string
	for i := range sessions {
		if i > 0 {
			f.clock.advance(61 * time.Second)
		}
		auth, err := func() (application.AuthResult, error) {
			if err := f.svc.RequestCode(ctx, "storm-a@example.invalid"); err != nil {
				return application.AuthResult{}, err
			}
			return f.svc.ConsumeCode(ctx, "storm-a@example.invalid", codes[i%len(codes)])
		}()
		if err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
		sessions[i] = auth.Session
		accID = auth.Account.ID
	}

	release := make(chan struct{})
	var wg sync.WaitGroup
	type outcome struct {
		validate bool
		err      error
	}
	results := make([]outcome, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-release
			sess := sessions[i/2]
			if i%2 == 0 {
				_, err := f.svc.ValidateAccess(ctx, sess.FamilyID, sess.AccessToken)
				results[i] = outcome{validate: true, err: err}
				return
			}
			_, err := f.svc.Refresh(ctx, sess.FamilyID, sess.RefreshToken)
			results[i] = outcome{err: err}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-release
		if err := f.svc.SuspendAccount(ctx, accID); err != nil {
			t.Errorf("SuspendAccount: %v", err)
		}
	}()
	close(release)
	wg.Wait()

	// Every racer either ran before the suspension (success) or
	// failed closed. Refresh racers may additionally lose to the
	// suspension revocation itself (revoked); validate racers may
	// present a token their pair just rotated away (unknown): both
	// deny a stale proof, never admit one.
	for _, r := range results {
		switch {
		case r.err == nil:
		case errors.Is(r.err, domain.ErrAccountSuspended):
		case !r.validate && errors.Is(r.err, domain.ErrSessionRevoked):
		case r.validate && errors.Is(r.err, domain.ErrCodeUnknown):
		default:
			t.Errorf("storm outcome must stay closed, got %+v", r)
		}
	}
	// After the suspension commits, every session of the account —
	// original and rotated — refuses with the suspension verdict.
	for _, sess := range sessions {
		if _, err := f.svc.ValidateAccess(ctx, sess.FamilyID, sess.AccessToken); !errors.Is(err, domain.ErrAccountSuspended) {
			t.Errorf("post-storm access must be suspended, got %v", err)
		}
		if _, err := f.svc.Refresh(ctx, sess.FamilyID, sess.RefreshToken); !errors.Is(err, domain.ErrAccountSuspended) {
			t.Errorf("post-storm refresh must be suspended, got %v", err)
		}
		fam, found, err := f.svc.Store.Family(ctx, sess.FamilyID)
		if err != nil || !found || fam.RevokedAt == 0 {
			t.Errorf("family must be revoked, got %+v found=%v err=%v", fam, found, err)
		}
	}
}
