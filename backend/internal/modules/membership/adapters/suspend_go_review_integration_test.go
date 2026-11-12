//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// Platform suspension of a sole owner meets the same last-owner fence as
// deletion; the store reports it as a registered refusal, not a raw SQL error.
func TestSuspendingTheLastOwnerIsARegisteredRefusal(t *testing.T) {
	f, s, a, _ := fixture(t)
	accountGrants(t, f)
	m := bound(t, s, a, 501, 601, 401)
	policy(t, f)
	activate(t, f, s, a, m, "OWNER")
	err := accountpg.NewPGStore(f.Runtime).SuspendAccount(t.Context(), key(401), clock{}.NowUnix())
	if !errors.Is(err, accountdomain.ErrEnterpriseLastOwner) {
		t.Fatal("last-owner suspension refusal", err)
	}
	var active bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT status='active' FROM accounts WHERE id=$1`, key(401)).Scan(&active); err != nil || !active {
		t.Fatal("refused suspension changed the account", err)
	}
}
