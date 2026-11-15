//go:build integration

package adapters_test

import (
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/jackc/pgx/v5"
)

// Migration 217 guards (B-BR-EMP-294..298) on the ordinary runtime role:
// forced RLS, the §6.4 transition rows only, the exclusion of confirmed
// windows and of blocks, append-only history and immutable receipts.
func TestFleetReservationSchemaGuardsAndIsolation(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := reservationStore(t, fx)
	org := fx.scope.OrgID().String()
	asset, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	now := exactNow(t, fx)
	r, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, _, err := reserve(t, fx, s, fa.ReservationRequestInput{AssetID: asset, Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour), Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	block, err := blockAsset(t, fx, s, fa.BlockCreateInput{AssetID: asset, Reason: fleet.SafetyBlock, Start: ptrTime(now.Add(10 * time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	var operation, actor string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text,requested_by::text FROM org_asset_reservations WHERE id=$1`, r.ReservationID).Scan(&operation, &actor); err != nil {
		t.Fatal(err)
	}
	insert := func(state string, from, to time.Time) error {
		return allocationExec(t, fx, `INSERT INTO org_asset_reservations(org_id,id,asset_id,purpose,starts_at,ends_at,no_show_tolerance,state,revision,operation_id,requested_by,confirm_operation_id,confirmed_by)
VALUES($1,$2,$3,'Synthetic direct',$4,$5,3600,$6,1,$7,$8,CASE WHEN $6='CONFIRMED' THEN $7::uuid END,CASE WHEN $6='CONFIRMED' THEN $8::uuid END)`, org, onboardingID(t), asset, from, to, state, operation, actor)
	}
	// Insert guards.
	allocationWant(t, "retroactive window", insert("REQUESTED", now.Add(-time.Hour), now.Add(time.Hour)), "23514")
	allocationWant(t, "initial terminal state", insert("EXPIRED", now.Add(time.Hour), now.Add(2*time.Hour)), "23514")
	allocationWant(t, "initial FULFILLED", insert("FULFILLED", now.Add(time.Hour), now.Add(2*time.Hour)), "23514")
	if code, constraint := allocationCode(insert("CONFIRMED", now.Add(3*time.Hour+30*time.Minute), now.Add(5*time.Hour))); code != "23P01" || constraint != "org_asset_reservations_confirmed_no_overlap" {
		t.Fatal("confirmed overlap reached storage", code, constraint)
	}
	if code, constraint := allocationCode(insert("CONFIRMED", now.Add(9*time.Hour), now.Add(11*time.Hour))); code != "23P01" || constraint != "org_asset_reservations_block_overlap" {
		t.Fatal("confirmation over a block reached storage", code, constraint)
	}
	// Transition guards: only the §6.4 rows, each with its cause and clock.
	for label, c := range map[string]struct{ sql, code string }{
		"FULFILLED has no source":        {`UPDATE org_asset_reservations SET state='FULFILLED',revision=revision+1 WHERE id=$1`, "55000"},
		"expired before its start":       {`UPDATE org_asset_reservations SET state='EXPIRED',revision=revision+1,end_cause='TIME' WHERE id=$1`, "55000"},
		"window rewritten":               {`UPDATE org_asset_reservations SET ends_at=ends_at+interval '1 hour',revision=revision+1 WHERE id=$1`, "55000"},
		"revision not advanced":          {`UPDATE org_asset_reservations SET state='CANCELLED',end_cause='CANCELLED',end_operation_id=operation_id,ended_by=requested_by WHERE id=$1`, "55000"},
		"cancelled without its command":  {`UPDATE org_asset_reservations SET state='CANCELLED',revision=revision+1,end_cause='CANCELLED' WHERE id=$1`, "23514"},
		"left service while ACTIVE":      {`UPDATE org_asset_reservations SET state='CANCELLED',revision=revision+1,end_cause='ASSET_LEFT_SERVICE',source_revision=1 WHERE id=$1`, "23514"},
		"confirmed without its command":  {`UPDATE org_asset_reservations SET state='CONFIRMED',revision=revision+1 WHERE id=$1`, "55000"},
		"requested reopened from cancel": {`UPDATE org_asset_reservations SET state='REQUESTED',revision=revision+1 WHERE id=$1`, "55000"},
	} {
		allocationWant(t, label, allocationExec(t, fx, c.sql, r.ReservationID), c.code)
	}
	allocationWant(t, "confirmed reverted to requested", allocationExec(t, fx, `UPDATE org_asset_reservations SET state='REQUESTED',revision=revision+1,confirm_operation_id=NULL,confirmed_by=NULL,confirmed_at=NULL WHERE id=$1`, confirmed.ReservationID), "55000")
	allocationWant(t, "no-show before its deadline", allocationExec(t, fx, `UPDATE org_asset_reservations SET state='NO_SHOW',revision=revision+1,end_cause='TIME' WHERE id=$1`, confirmed.ReservationID), "55000")
	// Blocks: retroactive start, removal without its command or reason.
	allocationWant(t, "retroactive block", allocationExec(t, fx, `INSERT INTO org_asset_blocks(org_id,id,asset_id,reason_kind,valid_from,operation_id,blocked_by) VALUES($1,$2,$3,'SAFETY',$4,$5,$6)`, org, onboardingID(t), asset, now.Add(-time.Hour), operation, actor), "23514")
	allocationWant(t, "block removed without a reason", allocationExec(t, fx, `UPDATE org_asset_blocks SET valid_to=valid_from,remove_operation_id=operation_id,removed_by=blocked_by WHERE id=$1`, block.BlockID), "23514")
	allocationWant(t, "block reason rewritten", allocationExec(t, fx, `UPDATE org_asset_blocks SET reason_kind='DOCUMENT' WHERE id=$1`, block.BlockID), "55000")
	// Append-only history and immutable receipts.
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE ON org_asset_reservations,org_asset_blocks TO "+role+";GRANT UPDATE,DELETE ON org_asset_reservation_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "reservation delete", allocationExec(t, fx, `DELETE FROM org_asset_reservations WHERE org_id=$1`, org), "55000")
	allocationWant(t, "block delete", allocationExec(t, fx, `DELETE FROM org_asset_blocks WHERE org_id=$1`, org), "55000")
	allocationWant(t, "receipt update", allocationExec(t, fx, `UPDATE org_asset_reservation_commands SET kind='CANCEL' WHERE org_id=$1`, org), "55000")
	// Forced RLS: nothing is visible or writable without the organization
	// setting, and another organization's scope sees none of these rows.
	if err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, table := range []string{"org_asset_reservations", "org_asset_blocks", "org_asset_reservation_commands"} {
			var n int
			if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
				t.Fatal(table, "visible without organization setting", n, err)
			}
		}
		if tag, err := tx.Exec(t.Context(), `UPDATE org_asset_reservations SET revision=revision+1`); err != nil || tag.RowsAffected() != 0 {
			t.Fatal("unscoped update", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if code, _ := allocationCode(allocationExec(t, other, `UPDATE org_asset_reservations SET revision=revision+1 WHERE id=$1`, r.ReservationID)); code != "" {
		t.Fatal("another organization reached the row", code)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_reservations WHERE id=$1 AND revision=1`, r.ReservationID); n != 1 {
		t.Fatal("another organization changed the row")
	}
	allocationWant(t, "insert into another organization", allocationExec(t, other, `INSERT INTO org_asset_blocks(org_id,id,asset_id,reason_kind,valid_from,operation_id,blocked_by) VALUES($1,$2,$3,'SAFETY',clock_timestamp()+interval '1 hour',$4,$5)`, org, onboardingID(t), asset, operation, actor), "42501")
	// The deadline function is the earlier of start + tolerance and end.
	var deadline time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT org_asset_reservation_deadline($1,$2,3600)`, now, now.Add(30*time.Minute)).Scan(&deadline); err != nil || !deadline.Equal(now.Add(30*time.Minute)) {
		t.Fatal("deadline", deadline, err)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }
