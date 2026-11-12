//go:build integration

package adapters_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"testing"
)

func TestMembershipSchemaUsesForcedScopedOwnership(t *testing.T) {
	f := orgtest.New(t)
	var complete bool
	err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN (to_regclass('org_employee_identities'),to_regclass('org_memberships'))`).Scan(&complete)
	if err != nil || !complete {
		t.Fatal("private membership schema missing or not forced", err)
	}
}
