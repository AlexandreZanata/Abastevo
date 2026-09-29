//go:build integration

package adapters

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
)

const (
	eraseContributorID = "c1111111-0000-4000-8000-000000000001"
	eraseToken         = "tok-victim"
	eraseStationID     = "d2222222-0000-4000-8000-000000000001"
	eraseObservationID = "b3333333-0000-4000-8000-000000000001"
	eraseConfirmID     = "f4444444-0000-4000-8000-000000000001"
	eraseDisputeID     = "e5555555-0000-4000-8000-000000000001"
	eraseSessionID     = "a6666666-0000-4000-8000-000000000001"
	eraseObjectID      = "07abcdef-0000-4000-8000-000000000001"
	eraseTrustID       = "d7777777-0000-4000-8000-000000000001"
)

// seedErasureFixtures inserts one owner's footprint across every owned
// module with plain SQL: identity, one station observation with a
// confirmation and a dispute, one evidence session with a final object,
// and one trust verdict. It models the pre-erasure backup rows.
func seedErasureFixtures(t *testing.T, ctx context.Context, db pgx.Tx) {
	t.Helper()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("fixture: %v\n%s", err, sql)
		}
	}
	exec(`INSERT INTO identity_contributors (id, status, attribution_token)
		VALUES ('` + eraseContributorID + `', 'active', '` + eraseToken + `')`)
	exec(`INSERT INTO identity_keys (id, contributor_id, algorithm, public_jwk, fingerprint)
		VALUES ('88888888-0000-4000-8000-000000000001', '` + eraseContributorID + `', 'ecdsa-p256-sha512', '{}', 'fp-victim')`)
	exec(`INSERT INTO directory_stations (id, display_name) VALUES ('` + eraseStationID + `', 'Victim Station')`)
	exec(`INSERT INTO community_observations
		(id, contributor_ref, client_submission_id, station_id, fuel_product, unit,
		 amount_milli_brl, condition_kind, qualifier_key, received_at, policy_version)
		VALUES ('` + eraseObservationID + `', '` + eraseToken + `', 'obs-1', '` + eraseStationID + `',
		 'GASOLINE_REGULAR', 'L', 5890, 'STANDARD', 'STANDARD', now(), 'community-v1')`)
	exec(`INSERT INTO community_confirmations
		(id, observation_id, contributor_ref, client_submission_id, policy_version)
		VALUES ('` + eraseConfirmID + `', '` + eraseObservationID + `', '` + eraseToken + `', 'conf-1', 'community-v1')`)
	exec(`INSERT INTO community_disputes
		(id, target_observation_id, contributor_ref, client_submission_id, reason, policy_version)
		VALUES ('` + eraseDisputeID + `', '` + eraseObservationID + `', '` + eraseToken + `', 'disp-1', 'OTHER', 'community-v1')`)
	exec(`INSERT INTO evidence_sessions
		(id, contributor_ref, client_session_id, mime, declared_bytes, claimed_sha256,
		 quarantine_key, status, expires_at, policy_version)
		VALUES ('` + eraseSessionID + `', '` + eraseToken + `', 'sess-1', 'image/jpeg', 100,
		 'aa', 'q/erase-victim-1', 'READY', now() + interval '1 hour', 'evidence-v1')`)
	exec(`INSERT INTO evidence_objects
		(id, session_id, final_key, source_sha256, sanitized_sha256, width, height, dhash)
		VALUES ('` + eraseObjectID + `', '` + eraseSessionID + `', 'f/erase-victim-1', 'bb', 'cc', 4, 4, 42)`)
	exec(`INSERT INTO trust_decisions
		(id, contributor_ref, tier, reason, policy_version)
		VALUES ('` + eraseTrustID + `', '` + eraseToken + `', 'ESTABLISHED', 'review', 'trust-v1')`)
	exec(`INSERT INTO trust_current (contributor_ref, tier) VALUES ('` + eraseToken + `', 'ESTABLISHED')`)
}

type eraseHarness struct {
	privacy   *Store
	deleted   [][2]string
	jobs      []string
	newIDs    []string
	n         int
	clockTime time.Time
}

func (h *eraseHarness) newID() (string, error) {
	h.n++
	return fmt.Sprintf("d0000000-0000-4000-8000-0000000001%02d", h.n), nil
}

func queryCount(t *testing.T, ctx context.Context, db pgx.Tx, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func queryText(t *testing.T, ctx context.Context, db pgx.Tx, sql string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(ctx, sql, args...).Scan(&s); err != nil {
		t.Fatalf("text: %v", err)
	}
	return s
}

func erasePorts(h *eraseHarness, s *Store, pool *pgxpool.Pool) privacyapp.ErasePorts {
	return privacyapp.ErasePorts{
		Clock: func() time.Time { return h.clockTime },
		NewID: h.newID,
		Attribution: func(context.Context, string) (string, error) {
			return eraseToken, nil
		},
		Store:     s,
		Identity:  identityadapters.NewRegistrar(pool),
		Community: communityadapters.NewStore(pool),
		Evidence:  evidenceadapters.NewStore(pool),
		Trust:     trustadapters.NewStore(pool),
		DeleteObject: func(_ context.Context, key, namespace string) error {
			h.deleted = append(h.deleted, [2]string{key, namespace})
			return nil
		},
		EnqueueJob: func(_ context.Context, _ pgx.Tx, kind string, _ []byte, _ string) error {
			h.jobs = append(h.jobs, kind)
			return nil
		},
	}
}

func TestEraseRemovesLinksMediaAndLocation(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)

	admin, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = admin.Rollback(ctx) }()
	seedErasureFixtures(t, ctx, admin)
	if err := admin.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// One READY export archive of the same owner: erasure purges bytes.
	expReq, err := privacydomain.NewRequest(privacydomain.RequestParams{
		ID: "e9999999-0000-4000-8000-000000000001", ContributorID: eraseContributorID,
		ContributorRef: eraseToken, ClientSubmissionID: "export-1",
		Type: privacydomain.TypeExport, RequestedAt: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	var exportJobs int
	if _, _, err := s.RequestExport(ctx, expReq, enqueueOK(&exportJobs)); err != nil {
		t.Fatal(err)
	}
	archive := []byte(`{"format":"privacy-export-v1"}`)
	if err := s.CompleteExport(ctx, expReq.ID, archive, ArchiveSHA256(archive), base); err != nil {
		t.Fatal(err)
	}

	h := &eraseHarness{privacy: s, clockTime: base.Add(time.Hour)}
	ports := erasePorts(h, s, pool)
	report, err := privacyapp.Erase(ctx, ports, eraseContributorID, privacyapp.EraseDTO{
		ClientSubmissionID: "erase-1", Reason: "owner request",
	})
	if err != nil {
		t.Fatalf("erase = %v", err)
	}
	if !report.IdentityRevoked || report.KeysRevoked != 1 {
		t.Errorf("identity = %+v", report)
	}
	if report.ObsUnlinked != 1 || report.VotesUnlinked != 2 || report.KeysRecomputed != 1 {
		t.Errorf("community = %+v", report)
	}
	if report.SessionsPurged != 1 || report.ObjectsPurged != 1 {
		t.Errorf("evidence = %+v", report)
	}
	if report.TrustUnlinked != 1 {
		t.Errorf("trust = %+v", report)
	}
	if report.ArchivesPurged != 1 {
		t.Errorf("exports = %+v", report)
	}
	if len(h.deleted) != 2 {
		t.Errorf("storage deletes = %v", h.deleted)
	}
	if len(h.jobs) == 0 {
		t.Error("no recompute/erasure jobs enqueued")
	}

	check, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = check.Rollback(ctx) }()
	// Identity revoked and unlinked.
	if got := queryText(t, ctx, check, `SELECT status FROM identity_contributors WHERE id = $1`, eraseContributorID); got != "deleted" {
		t.Errorf("contributor status = %q", got)
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM identity_contributors WHERE id = $1 AND attribution_token IS NOT NULL`, eraseContributorID); n != 0 {
		t.Error("attribution token survives")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM identity_keys WHERE contributor_id = $1 AND revoked_at IS NULL`, eraseContributorID); n != 0 {
		t.Error("live keys survive")
	}
	// Community links gone, facts stay.
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM community_observations WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("observation links survive")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM community_observations WHERE id = $1::uuid`, eraseObservationID); n != 1 {
		t.Error("price fact deleted instead of unlinked")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM community_confirmations WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("confirmation links survive")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM community_disputes WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("dispute links survive")
	}
	// Evidence purged.
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM evidence_sessions WHERE contributor_ref = $1 AND status != 'EXPIRED'`, eraseToken); n != 0 {
		t.Error("live sessions survive")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM evidence_objects o JOIN evidence_sessions se ON se.id = o.session_id WHERE se.contributor_ref = $1 AND o.final_deleted_at IS NULL`, eraseToken); n != 0 {
		t.Error("live objects survive")
	}
	// Trust reads NEW with no linked history.
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM trust_current WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("trust projection survives")
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM trust_decisions WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("trust links survive")
	}
	// Export bytes purged; the purged download reads as expired.
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM privacy_requests WHERE contributor_id = $1 AND archive IS NOT NULL`, eraseContributorID); n != 0 {
		t.Error("export bytes survive")
	}
	// Ledger holds one row per scope.
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM privacy_deletion_ledger WHERE contributor_id = $1`, eraseContributorID); n != 5 {
		t.Errorf("ledger rows = %d, want 5", n)
	}
	if err := check.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	// Repeated erasure converges: the completed request replays.
	again, err := privacyapp.Erase(ctx, ports, eraseContributorID, privacyapp.EraseDTO{
		ClientSubmissionID: "erase-1", Reason: "owner request",
	})
	if err != nil {
		t.Fatalf("repeat erase = %v", err)
	}
	if !again.Replayed {
		t.Errorf("completed erasure did not converge: %+v", again)
	}
}

func TestRestoreReappliesDeletion(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)

	seed, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = seed.Rollback(ctx) }()
	seedErasureFixtures(t, ctx, seed)
	if err := seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	h := &eraseHarness{privacy: s, clockTime: base}
	ports := erasePorts(h, s, pool)
	if _, err := privacyapp.Erase(ctx, ports, eraseContributorID, privacyapp.EraseDTO{
		ClientSubmissionID: "erase-1", Reason: "owner request",
	}); err != nil {
		t.Fatalf("erase = %v", err)
	}

	// A stale backup restores the pre-erasure bytes over the current
	// rows: links point at the owner again. Traffic stays closed until
	// the ledger replays: replay pending is observable through the
	// unstamped ledger.
	stale, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stale.Rollback(ctx) }()
	restored := func(sql string, args ...any) {
		t.Helper()
		if _, err := stale.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("stale restore: %v", err)
		}
	}
	restored(`UPDATE identity_contributors SET status = 'active', attribution_token = $2, deleted_at = NULL WHERE id = $1`, eraseContributorID, eraseToken)
	restored(`UPDATE identity_keys SET revoked_at = NULL WHERE contributor_id = $1`, eraseContributorID)
	restored(`UPDATE community_observations SET contributor_ref = $2 WHERE id = $1::uuid`, eraseObservationID, eraseToken)
	restored(`UPDATE community_confirmations SET contributor_ref = $2 WHERE id = $1::uuid`, eraseConfirmID, eraseToken)
	restored(`UPDATE community_disputes SET contributor_ref = $2 WHERE id = $1::uuid`, eraseDisputeID, eraseToken)
	restored(`UPDATE evidence_sessions SET status = 'READY', quarantine_deleted_at = NULL WHERE id = $1::uuid`, eraseSessionID)
	restored(`UPDATE evidence_objects SET final_deleted_at = NULL WHERE id = $1::uuid`, eraseObjectID)
	restored(`INSERT INTO trust_current (contributor_ref, tier) VALUES ($1, 'ESTABLISHED')`, eraseToken)
	restored(`UPDATE trust_decisions SET contributor_ref = $2 WHERE id = $1::uuid`, eraseTrustID, eraseToken)
	if err := stale.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	check, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = check.Rollback(ctx) }()
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM community_observations WHERE contributor_ref = $1`, eraseToken); n != 1 {
		t.Fatalf("restored links = %d, want 1", n)
	}
	if n := queryCount(t, ctx, check, `SELECT COUNT(*) FROM privacy_deletion_ledger WHERE contributor_id = $1 AND replayed_at IS NULL`, eraseContributorID); n != 5 {
		t.Fatalf("pending replay rows = %d, want 5", n)
	}
	if err := check.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	replayed, err := privacyapp.ReplayLedger(ctx, ports, eraseContributorID)
	if err != nil {
		t.Fatalf("replay = %v", err)
	}
	if replayed.ObsUnlinked != 1 || replayed.TrustUnlinked < 1 {
		t.Errorf("replay report = %+v", replayed)
	}
	verify, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = verify.Rollback(ctx) }()
	if n := queryCount(t, ctx, verify, `SELECT COUNT(*) FROM community_observations WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("restored observation links survive replay")
	}
	if n := queryCount(t, ctx, verify, `SELECT COUNT(*) FROM trust_current WHERE contributor_ref = $1`, eraseToken); n != 0 {
		t.Error("restored trust projection survives replay")
	}
	if n := queryCount(t, ctx, verify, `SELECT COUNT(*) FROM evidence_objects o JOIN evidence_sessions se ON se.id = o.session_id WHERE se.contributor_ref = $1 AND o.final_deleted_at IS NULL`, eraseToken); n != 0 {
		t.Error("restored objects survive replay")
	}
	if n := queryCount(t, ctx, verify, `SELECT COUNT(*) FROM privacy_deletion_ledger WHERE contributor_id = $1 AND replayed_at IS NULL`, eraseContributorID); n != 0 {
		t.Error("replay left unstamped rows")
	}
	// A second replay converges without work.
	if _, err := privacyapp.ReplayLedger(ctx, ports, eraseContributorID); err != nil {
		t.Errorf("second replay = %v", err)
	}
}

func TestNoRestoredDataServedBeforeReplay(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	base := time.Now().Truncate(time.Millisecond)

	seed, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = seed.Rollback(ctx) }()
	seedErasureFixtures(t, ctx, seed)
	if err := seed.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	expReq, err := privacydomain.NewRequest(privacydomain.RequestParams{
		ID: "e9999999-0000-4000-8000-000000000001", ContributorID: eraseContributorID,
		ContributorRef: eraseToken, ClientSubmissionID: "export-1",
		Type: privacydomain.TypeExport, RequestedAt: base,
	})
	if err != nil {
		t.Fatal(err)
	}
	var exportJobs int
	if _, _, err := s.RequestExport(ctx, expReq, enqueueOK(&exportJobs)); err != nil {
		t.Fatal(err)
	}
	archive := []byte(`{"format":"privacy-export-v1"}`)
	if err := s.CompleteExport(ctx, expReq.ID, archive, ArchiveSHA256(archive), base); err != nil {
		t.Fatal(err)
	}
	h := &eraseHarness{privacy: s, clockTime: base.Add(time.Hour)}
	ports := erasePorts(h, s, pool)
	if _, err := privacyapp.Erase(ctx, ports, eraseContributorID, privacyapp.EraseDTO{
		ClientSubmissionID: "erase-1", Reason: "owner request",
	}); err != nil {
		t.Fatalf("erase = %v", err)
	}
	// The purged READY export refuses downloads: nothing erased is
	// served, before or after any restore.
	dlPorts := privacyapp.Ports{
		Clock: func() time.Time { return base.Add(2 * time.Hour) },
		Store: s,
	}
	if _, err := privacyapp.Download(ctx, dlPorts, privacyapp.Caller{ContributorID: eraseContributorID}, expReq.ID); !isExpired(err) {
		t.Errorf("purged download = %v, want expiry", err)
	}
	// A stale restore brings the receipt row back without bytes; the
	// download still refuses until replay re-purges.
	stale, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stale.Rollback(ctx) }()
	if _, err := stale.Exec(ctx, `INSERT INTO privacy_requests
		(id, contributor_id, contributor_ref, client_submission_id, type, status,
		 requested_at, ready_at, expires_at, completed_at, policy_version)
		VALUES ('e9999999-0000-4000-8000-000000000001', $1, $2, 'export-1', 'EXPORT',
		 'READY', $3::timestamptz, $3::timestamptz, $3::timestamptz + interval '24 hours', $3::timestamptz, 'privacy-v1')
		ON CONFLICT (id) DO UPDATE SET archive = NULL`,
		eraseContributorID, eraseToken, base); err != nil {
		t.Fatalf("stale restore: %v", err)
	}
	if err := stale.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := privacyapp.Download(ctx, dlPorts, privacyapp.Caller{ContributorID: eraseContributorID}, expReq.ID); !isExpired(err) {
		t.Errorf("restored download = %v, want expiry before replay", err)
	}
}

func isExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), "expired")
}
