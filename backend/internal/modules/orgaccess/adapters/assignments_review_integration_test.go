//go:build integration

package adapters_test

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// Storage never fabricates retroactive authority: a new period starts now
// (within the bounded skew) or later and is not already over.
func TestRoleAssignmentPeriodsCannotStartInThePastOrAlreadyEnd(t *testing.T) {
	f, a, _ := fixture(t)
	seed(t, f, a, 601)
	org := a.OrgID().String()
	now := time.Now().UTC()
	for _, tc := range []struct {
		name             string
		id               int
		role, kind, then string
		from             time.Time
		to               any
	}{
		{"backdated owner", 701, "OWNER", "ORG", org, now.Add(-time.Hour), nil},
		{"backdated admin", 702, "ADMIN", "ORG", org, now.Add(-10 * time.Minute), nil},
		{"already expired", 703, "ADMIN", "ORG", org, now.Add(-time.Minute), now.Add(-time.Second)},
	} {
		err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
			return insert(t, tx, a, tc.id, 601, tc.role, tc.kind, tc.then, tc.from, tc.to)
		})
		code(t, err, "23514")
	}
	for _, tc := range []struct {
		id         int
		role, kind string
		target     string
		from       time.Time
	}{{704, "DRIVER", "SELF", key(601), now.Add(-time.Minute)}, {705, "ADMIN", "ORG", org, now.Add(time.Hour)}} {
		if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
			return insert(t, tx, a, tc.id, 601, tc.role, tc.kind, tc.target, tc.from, nil)
		}); err != nil {
			t.Fatal("current or scheduled period refused", tc.role, err)
		}
	}
	var stored int
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1`, org).Scan(&stored); err != nil || stored != 2 {
		t.Fatal("refused periods persisted", stored, err)
	}
}
