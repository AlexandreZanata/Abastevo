//go:build integration

package adapters_test

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

func TestMeterReviewSchemaRequiresRealHeldSourceAndValidatedReferences(t *testing.T) {
	f := orgtest.New(t)
	var complete bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid=ANY(ARRAY['org_sync_review_items'::regclass,'org_meter_held_readings'::regclass,'org_sync_review_resolutions'::regclass])`).Scan(&complete); err != nil || !complete {
		t.Fatal("actual forced-RLS review source storage missing", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT convalidated AND condeferrable FROM pg_constraint WHERE conname='org_sync_ack_review_proof_fkey' AND conrelid='org_sync_acknowledgements'::regclass`).Scan(&complete); err != nil || !complete {
		t.Fatal("review acknowledgement proof reference is not validated", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT org_sync_review_source_valid('00000000-0000-0000-0000-000000000001'::uuid,'00000000-0000-0000-0000-000000000002'::uuid,'validation.unknown',1)`).Scan(&complete); err != nil || !complete {
		t.Fatal("unknown review owner became a real held source", err)
	}
}
