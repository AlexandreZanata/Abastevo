//go:build integration

package adapters_test

import (
	"testing"

	m "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestCurrentAuthorityDoesNotCacheRevokedOrForeignMembership(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 601, 701, "OWNER")
	activate(t, f, a, 602, 702, "DRIVER")
	s := adapters.NewStore(f.Runtime)
	current, err := s.LoadAuthority(t.Context(), a, key(702))
	if err != nil || current.MembershipID != key(602) || len(current.Assignments) != 1 {
		t.Fatal("live authority missing", err)
	}
	if _, err := s.LoadAuthority(t.Context(), b, key(702)); err != app.ErrNotFound {
		t.Fatal("foreign authority leaked", err)
	}
	if _, _, err := s.End(t.Context(), a, key(702), 1); err != nil {
		t.Fatal(err)
	}
	current, err = s.LoadAuthority(t.Context(), a, key(702))
	if err != nil || len(current.Assignments) != 0 {
		t.Fatal("revoked role cached", err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(602))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	current, err = s.LoadAuthority(t.Context(), a, key(702))
	if err != nil || current.MembershipState != m.MembershipSuspended {
		t.Fatal("suspension cached", err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(602))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LoadAuthority(t.Context(), a, key(702)); err != app.ErrNotFound {
		t.Fatal("ended membership cached", err)
	}
}
