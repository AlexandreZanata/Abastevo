//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var referenceTestSecret = bytes.Repeat([]byte{77}, 32)

func referenceStore(x selfIntakeState) *syncpg.ReferenceStore {
	return &syncpg.ReferenceStore{Pool: x.f.Runtime, Authority: accesspg.NewStore(x.f.Runtime), Assets: x.assets, Secret: referenceTestSecret, NewEpoch: jobs.NewUUIDv4,
		Confirm: func(ctx context.Context, tx pgx.Tx, s syncapp.Source) (time.Time, error) {
			_, now, err := x.devices.CheckCurrentDeviceTx(ctx, tx, s.Scope, s.Actor, s.MembershipID, s.EnrollmentID)
			if err != nil {
				return time.Time{}, syncapp.SourceDenied(err)
			}
			return now, nil
		}}
}

func referencePull(t *testing.T, s *syncpg.ReferenceStore, d syncFixtureState, cursor *string) syncapp.ReferenceFeed {
	t.Helper()
	var opened *syncapp.ReferenceCursor
	if cursor != nil {
		c, err := syncapp.OpenReferenceCursor(referenceTestSecret, *cursor)
		if err != nil {
			t.Fatal("own cursor did not open", err)
		}
		opened = &c
	}
	feed, err := s.PullReference(t.Context(), d.source, opened)
	if err != nil {
		t.Fatal("reference pull failed", err)
	}
	return feed
}

// referenceShape proves the B-BR-EMP-339 delta contract: revisions strictly
// increasing, after from_revision and at most the head; at most one change per
// entity; tombstones carry no content.
func referenceShape(t *testing.T, feed syncapp.ReferenceFeed) map[string]syncapp.ReferenceChange {
	t.Helper()
	out := map[string]syncapp.ReferenceChange{}
	prior := int64(0)
	if feed.FromRevision != nil {
		prior = *feed.FromRevision
	}
	for _, change := range feed.Changes {
		if change.Revision <= prior || change.Revision > feed.Revision || change.EntityType != syncapp.ReferenceEntityAsset || out[change.EntityID].EntityID != "" {
			t.Fatalf("delta order or uniqueness broken %+v", feed)
		}
		if change.Op == syncapp.ReferenceDelete && change.Asset != nil || change.Op == syncapp.ReferenceUpsert && (change.Asset == nil || change.Asset.ID != change.EntityID) {
			t.Fatal("change content does not match its operation")
		}
		prior = change.Revision
		out[change.EntityID] = change
	}
	if feed.Cursor == "" || feed.ConfirmedAt.IsZero() {
		t.Fatal("feed lacks cursor or confirmation")
	}
	return out
}

// UC-OFFL-08 on real PostGIS: a SELF driver's own assigned assets only,
// versioned deltas, a durable tombstone after reassignment, server-clock
// confirmation and an intake decision that never depends on the cache.
func TestReferenceFeedDeliversOwnAssetsDeltasAndTombstones(t *testing.T) {
	x := selfIntakeFixture(t)
	x.f.GrantReferenceFeed(t)
	store := referenceStore(x)
	d := x.driver(t)
	first, _ := x.assign(t, d.person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	other := x.driver(t)
	unit := x.unit
	foreign, _ := lifecycleAssetIn(t, x.legalFixtureState, x.assets, unit, fleet.Active)
	x.assign(t, other.person.employee, foreign, fleet.PrimaryAssignment, nil, nil)

	before := lifecycleDBNow(t, x.legalFixtureState)
	reset := referencePull(t, store, d, nil)
	after := lifecycleDBNow(t, x.legalFixtureState)
	changes := referenceShape(t, reset)
	if reset.Mode != syncapp.ReferenceReset || reset.Reason != syncapp.ReferenceNewCursor || reset.FromRevision != nil || reset.Revision != 1 || len(changes) != 1 {
		t.Fatalf("first pull was not a reset of the own asset %+v", reset)
	}
	asset := changes[x.installed.AssetID].Asset
	if asset == nil || len(asset.Meters) != 1 || asset.Meters[0].ID != x.installed.MeterID || asset.Meters[0].SegmentID != x.installed.SegmentID || asset.Catalog != syncapp.ReferenceCatalog || asset.Assignment.Kind != string(fleet.PrimaryAssignment) {
		t.Fatalf("own asset projection incomplete %+v", asset)
	}
	if _, leaked := changes[foreign]; leaked {
		t.Fatal("another driver's assigned asset reached the feed")
	}
	if reset.ConfirmedAt.Before(before) || reset.ConfirmedAt.After(after) {
		t.Fatal("confirmed_at is not the server clock", reset.ConfirmedAt, before, after)
	}
	steady := referencePull(t, store, d, &reset.Cursor)
	if referenceShape(t, steady); steady.Mode != syncapp.ReferenceDelta || *steady.FromRevision != 1 || steady.Revision != 1 || len(steady.Changes) != 0 || !steady.ConfirmedAt.After(reset.ConfirmedAt) {
		t.Fatalf("an unchanged feed was not an empty delta with a newer confirmation %+v", steady)
	}
	second, _ := lifecycleAssetIn(t, x.legalFixtureState, x.assets, unit, fleet.Active)
	x.assign(t, d.person.employee, second, fleet.AdditionalAssignment, nil, nil)
	added := referencePull(t, store, d, &steady.Cursor)
	if c := referenceShape(t, added); added.Mode != syncapp.ReferenceDelta || added.Revision != 2 || len(c) != 1 || c[second].Op != syncapp.ReferenceUpsert {
		t.Fatalf("a new assignment was not delivered as one upsert %+v", added)
	}
	// Reassignment: the cached asset leaves with a tombstone; intake still
	// decides from server history alone (held, as without any cache).
	x.end(t, x.installed.AssetID, first)
	removed := referencePull(t, store, d, &added.Cursor)
	if c := referenceShape(t, removed); removed.Revision != 3 || len(c) != 1 || c[x.installed.AssetID].Op != syncapp.ReferenceDelete {
		t.Fatalf("the unassignment was not tombstoned %+v", removed)
	}
	stale := x.operation(t, d, lifecycleDBNow(t, x.legalFixtureState))
	ack, err := x.process(t, d, stale)
	x.held(t, stale, ack, err, "PERMISSION_CHANGED")
	// An older cursor still receives the tombstone and the later upsert.
	replayed := referencePull(t, store, d, &reset.Cursor)
	c := referenceShape(t, replayed)
	if replayed.Mode != syncapp.ReferenceDelta || *replayed.FromRevision != 1 || replayed.Revision != 3 || len(c) != 2 || c[x.installed.AssetID].Op != syncapp.ReferenceDelete || c[second].Op != syncapp.ReferenceUpsert {
		t.Fatalf("an older cursor missed the tombstone %+v", replayed)
	}
	// Out of order: another epoch resets; a revision ahead of the head resets.
	opened, _ := syncapp.OpenReferenceCursor(referenceTestSecret, removed.Cursor)
	for reason, mutate := range map[string]func(*syncapp.ReferenceCursor){
		syncapp.ReferenceExpired: func(c *syncapp.ReferenceCursor) { c.Epoch = onboardingID(t) },
		syncapp.ReferenceAhead:   func(c *syncapp.ReferenceCursor) { c.Revision = 99 },
	} {
		moved := opened
		mutate(&moved)
		feed, err := store.PullReference(t.Context(), d.source, &moved)
		if err != nil || feed.Mode != syncapp.ReferenceReset || feed.Reason != reason || feed.FromRevision != nil || len(referenceShape(t, feed)) != 1 {
			t.Fatalf("%s did not reset to the complete current set %+v %v", reason, feed, err)
		}
	}
	var entries int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_reference_feed_entries WHERE org_id=$1 AND membership_id=$2`, x.scope.OrgID().String(), d.source.MembershipID).Scan(&entries); err != nil || entries != 3 {
		t.Fatal("feed log is not one entry per change", err, entries)
	}
}

// A member who no longer holds a covering grant gets RESET/FORBIDDEN with no
// entities, for a new and for an old cursor; nothing cached survives.
func TestReferenceFeedResetsWhenTheGrantIsRevoked(t *testing.T) {
	x := selfIntakeFixture(t)
	x.f.GrantReferenceFeed(t)
	store := referenceStore(x)
	d := x.driver(t)
	x.assign(t, d.person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	first := referencePull(t, store, d, nil)
	if len(first.Changes) != 1 {
		t.Fatal("fixture asset not delivered")
	}
	matrixAssign(t, x.deviceFixtureState, matrixPerson{member: d.person.member}, "", "", "NONE")
	for _, cursor := range []*string{&first.Cursor, nil} {
		feed := referencePull(t, store, d, cursor)
		if feed.Mode != syncapp.ReferenceReset || feed.Reason != syncapp.ReferenceForbidden || len(feed.Changes) != 0 {
			t.Fatalf("revoked grant kept cached reference data %+v", feed)
		}
	}
}

// Twelve concurrent pulls of one member serialize on the head: one revision,
// one entry, identical reset content.
func TestReferenceFeedConcurrentPullsKeepOneLog(t *testing.T) {
	x := selfIntakeFixture(t)
	x.f.GrantReferenceFeed(t)
	store := referenceStore(x)
	d := x.driver(t)
	x.assign(t, d.person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	feeds, errs := make([]syncapp.ReferenceFeed, 12), make([]error, 12)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range feeds {
		wg.Go(func() {
			<-start
			feeds[i], errs[i] = store.PullReference(t.Context(), d.source, nil)
		})
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil || feeds[i].Revision != 1 || len(feeds[i].Changes) != 1 {
			t.Fatal("concurrent pull diverged", err, feeds[i].Revision)
		}
	}
	var entries, revision int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_reference_feed_entries WHERE org_id=$1 AND membership_id=$2),(SELECT revision FROM org_reference_feed_heads WHERE org_id=$1 AND membership_id=$2)`, x.scope.OrgID().String(), d.source.MembershipID).Scan(&entries, &revision); err != nil || entries != 1 || revision != 1 {
		t.Fatal("concurrent pulls duplicated the log", err, entries, revision)
	}
}

// The feed log is forced-RLS, append-only and monotonic at the database.
func TestReferenceFeedSchemaIsAppendOnlyAndMonotonic(t *testing.T) {
	x := selfIntakeFixture(t)
	x.f.GrantReferenceFeed(t)
	store := referenceStore(x)
	d := x.driver(t)
	x.assign(t, d.person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	referencePull(t, store, d, nil)
	org, member := x.scope.OrgID().String(), d.source.MembershipID
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_reference_feed_entries,org_reference_feed_heads TO "+role); err != nil {
		t.Fatal(err)
	}
	code := func(err error, want string) {
		t.Helper()
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != want {
			t.Fatalf("feed guard: want %s, got %v", want, err)
		}
	}
	exec := func(query string, args ...any) error {
		return database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), query, args...); return err })
	}
	for _, verb := range []string{`UPDATE org_reference_feed_entries SET digest=digest`, `DELETE FROM org_reference_feed_entries`, `TRUNCATE org_reference_feed_entries`, `DELETE FROM org_reference_feed_heads`} {
		code(exec(verb), "55000")
	}
	code(exec(`UPDATE org_reference_feed_heads SET revision=0 WHERE org_id=$1 AND membership_id=$2`, org, member), "55000")
	code(exec(`INSERT INTO org_reference_feed_entries(org_id,membership_id,revision,entity_type,entity_id,op,digest) VALUES($1,$2,1,'asset',$3,'UPSERT',repeat('a',64))`, org, member, onboardingID(t)), "23514")
	code(database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE org_reference_feed_heads SET revision=2 WHERE org_id=$1 AND membership_id=$2`, org, member); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_reference_feed_entries(org_id,membership_id,revision,entity_type,entity_id,op,digest) VALUES($1,$2,2,'asset',$3,'DELETE','')`, org, member, onboardingID(t))
		return err
	}), "23514")
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_reference_feed_entries`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("feed log visible without an organization scope", err, visible)
	}
}

// Through the actual signed route and default runtime: a foreign, tampered or
// other-organization cursor is NOT_FOUND and reveals nothing (EDR-217).
func TestReferenceFeedSignedRouteRefusesForeignCursors(t *testing.T) {
	fx := meterIntakeHTTP(t)
	fx.f.GrantReferenceFeed(t)
	path := strings.Replace(syncweb.ReferencePath, "{org_id}", fx.scope.OrgID().String(), 1)
	post := func(cursor any, extra map[string]any) (int, map[string]any) {
		t.Helper()
		body := map[string]any{"schema_version": 1, "request_id": onboardingID(t), "family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken, "membership_id": fx.source.MembershipID, "enrollment_id": fx.source.EnrollmentID, "cursor": cursor}
		for k, v := range extra {
			body[k] = v
		}
		r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{})
		endpoint, _ := url.Parse(fx.server.URL + path)
		r.URL, r.RequestURI, r.Host = endpoint, "", deviceTestHost
		response, err := fx.server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if response.Header.Get("Cache-Control") != "no-store" || strings.Contains(string(raw), fx.person.auth.Session.AccessToken) {
			t.Fatal("reference response cacheable or leaking proof")
		}
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return response.StatusCode, out
	}
	status, first := post(nil, nil)
	if status != 200 || first["mode"] != syncapp.ReferenceReset || first["reason"] != syncapp.ReferenceNewCursor || len(first["changes"].([]any)) != 0 {
		t.Fatalf("signed first pull %d %v", status, first)
	}
	if status, again := post(first["cursor"], nil); status != 200 || again["mode"] != syncapp.ReferenceDelta {
		t.Fatalf("own cursor refused %d", status)
	}
	key := hmac.New(sha256.New, bytes.Repeat([]byte{93}, 32))
	key.Write([]byte("abastevo/reference-feed/v1"))
	runtimeKey := key.Sum(nil)
	opened, err := syncapp.OpenReferenceCursor(runtimeKey, first["cursor"].(string))
	if err != nil {
		t.Fatal("runtime cursor key differs from the documented derivation", err)
	}
	for name, mutate := range map[string]func(*syncapp.ReferenceCursor){
		"other-organization": func(c *syncapp.ReferenceCursor) { c.OrgID = onboardingID(t) },
		"other-member":       func(c *syncapp.ReferenceCursor) { c.MembershipID = onboardingID(t) },
		"other-enrollment":   func(c *syncapp.ReferenceCursor) { c.EnrollmentID = onboardingID(t) },
	} {
		foreign := opened
		mutate(&foreign)
		raw, err := syncapp.SignReferenceCursor(runtimeKey, foreign)
		if err != nil {
			t.Fatal(err)
		}
		if status, out := post(raw, nil); status != 404 || out["changes"] != nil {
			t.Fatalf("%s cursor accepted %d", name, status)
		}
	}
	tampered := first["cursor"].(string)
	tampered = tampered[:len(tampered)-1] + map[bool]string{true: "A", false: "B"}[!strings.HasSuffix(tampered, "A")]
	if status, _ := post(tampered, nil); status != 404 {
		t.Fatal("tampered cursor accepted", status)
	}
	if status, _ := post(nil, map[string]any{"since": 1}); status != 400 {
		t.Fatal("an unknown request field was accepted", status)
	}
}

// A populated head before 000224 (an ACTIVE asset with a meter assigned to the
// member) upgrades through an ordinary migrator: failure changes nothing,
// recovery applies once, replay is a no-op, the previous image stays
// compatible, and the populated member then pulls its asset.
func TestReferenceFeedPopulatedUpgradeFailureRecoveryAndPreviousImage(t *testing.T) {
	const name = "000224_organization_reference_feed.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	if _, ok := previous["000219_organization_state_intake.sql"]; !ok {
		t.Fatal("previous image lacks the 000219 head")
	}
	f := orgtest.NewWithMigrations(t, previous)
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx, _, _, installed, _, _ := lifecycleIntakeFixtureOn(t, syncFixtureOn(t, devicesFixtureOn(t, legalFixtureFor(t, f, onboarding, actor, flags))))
	assets := assignmentStore(t, fx.legalFixtureState)
	if _, _, err := assignmentWrite(t, fx.legalFixtureState, assets, assignmentCreate(t, fa.AssignmentCreateInput{AssetID: installed.AssetID, Kind: fleet.PrimaryAssignment, EmployeeIDs: []string{fx.person.employee}})); err != nil {
		t.Fatal("populated assignment", err)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		if err := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_memberships x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_assets x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_asset_assignments x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,id)FROM org_meters x))::text`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	migrator := f.RuntimeRole + "_reference_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+";GRANT REFERENCES ON org_memberships TO "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	query.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected reference migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT to_regclass('org_reference_feed_heads') IS NULL AND to_regclass('org_reference_feed_entries') IS NULL AND to_regprocedure('org_reference_feed_entry_guard()') IS NULL AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000224')`).Scan(&clean); err != nil || !clean || before != snapshot() {
		t.Fatal("failed reference migration changed the schema or data", err)
	}
	current := fstest.MapFS{}
	for path, data := range previous {
		current[path] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if got, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(got) != 1 || got[0] != "000224" {
		t.Fatal("ordinary reference migration recovery", got, err)
	}
	if got, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(got) != 0 || before != snapshot() {
		t.Fatal("reference migration no-op replay or rewrite", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT bool_and(relrowsecurity AND relforcerowsecurity AND relowner=(SELECT oid FROM pg_roles WHERE rolname=$1 AND NOT rolsuper AND NOT rolbypassrls)) FROM pg_class WHERE oid=ANY(ARRAY['org_reference_feed_heads'::regclass,'org_reference_feed_entries'::regclass])`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("feed tables not owned by the ordinary forced-RLS migrator", err)
	}
	if _, err := migrate.Apply(t.Context(), f.AdminDSN, migrations.Files); err != nil {
		t.Fatal("remaining embedded chain after the 224 rehearsal", err)
	}
	f.GrantReferenceFeed(t)
	x := selfIntakeState{syncFixtureState: fx, assets: assets, installed: installed}
	feed := referencePull(t, referenceStore(x), fx, nil)
	if feed.Mode != syncapp.ReferenceReset || len(feed.Changes) != 1 || feed.Changes[0].EntityID != installed.AssetID {
		t.Fatalf("the populated member did not pull its asset after the upgrade %+v", feed)
	}
}
