//go:build integration

package adapters_test

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestRemainingOwnershipPeriodsIncludesFutureWithoutGrantingCurrentAuthority(t *testing.T) {
	f, a, b := fixture(t)
	activate(t, f, a, 602, 702, "OWNER")
	seed(t, f, a, 601)
	activate(t, f, b, 603, 703, "OWNER")
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		if err := insert(t, tx, a, 701, 601, "OWNER", "ORG", a.OrgID().String(), now, future); err != nil {
			return err
		}
		if err := insert(t, tx, a, 704, 601, "OWNER", "ORG", a.OrgID().String(), future, nil); err != nil {
			return err
		}
		if err := insert(t, tx, a, 705, 601, "ADMIN", "ORG", a.OrgID().String(), now, future); err != nil {
			return err
		}
		if err := insert(t, tx, a, 706, 601, "ADMIN", "ORG", a.OrgID().String(), future, nil); err != nil {
			return err
		}
		if err := insert(t, tx, a, 707, 601, "DRIVER", "SELF", key(601), now, nil); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), key(601))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	store := adapters.NewStore(f.Runtime)
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		current, err := store.CandidatesTx(t.Context(), tx, a, key(601))
		if err != nil {
			return err
		}
		if len(current) != 3 {
			t.Fatal("scheduled OWNER became current authority", len(current))
		}
		remaining, err := store.RemainingOwnershipAssignmentsTx(t.Context(), tx, a, key(601))
		if err != nil {
			return err
		}
		if len(remaining) != 4 {
			t.Fatal("step-down omitted remaining owner period", len(remaining))
		}
		for i, row := range remaining {
			v, err := row.Snapshot(a)
			if err != nil {
				return err
			}
			if v.Role != []d.Role{d.Owner, d.Owner, d.Admin, d.Admin}[i] || v.ID != []string{key(701), key(704), key(705), key(706)}[i] {
				t.Fatal("remaining owners included unrelated role")
			}
		}
		foreign, err := store.RemainingOwnershipAssignmentsTx(t.Context(), tx, a, key(603))
		if err != nil {
			return err
		}
		if len(foreign) != 0 {
			t.Fatal("foreign owner exposed")
		}
		if _, err = store.RemainingOwnershipAssignmentsTx(t.Context(), tx, b, key(603)); err == nil {
			t.Fatal("foreign transaction scope accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := end(t, f, a, 704); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		rows, err := store.RemainingOwnershipAssignmentsTx(t.Context(), tx, a, key(601))
		if err != nil {
			return err
		}
		if len(rows) != 3 {
			t.Fatal("ended future owner retained for step-down", len(rows))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
