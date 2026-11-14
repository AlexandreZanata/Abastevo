//go:build integration

package adapters_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"testing"
)

func TestMeterSchemaInstallsScopedImmutableEvidence(t *testing.T) {
	f := orgtest.New(t)
	var complete bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=4 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid=ANY(ARRAY['org_meters'::regclass,'org_meter_segments'::regclass,'org_meter_readings'::regclass,'org_meter_commands'::regclass])`).Scan(&complete); err != nil || !complete {
		t.Fatal("actual forced-RLS meter persistence missing", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='org_meter_readings'::regclass AND contype='f' AND confrelid IN('company_web_sessions'::regclass,'accounts'::regclass))`).Scan(&complete); err != nil || !complete {
		t.Fatal("permanent evidence depends on ephemeral personal proof", err)
	}
}
