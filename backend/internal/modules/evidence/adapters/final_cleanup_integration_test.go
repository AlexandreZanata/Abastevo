//go:build integration

package adapters

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

type finalStorageFixture struct {
	endpoint, bucket string
	cred             storage.Credentials
}

func (f finalStorageFixture) input(key string) storage.PresignInput {
	ns := "f/"
	if strings.HasPrefix(key, "q/") {
		ns = "q/"
	}
	return storage.PresignInput{Endpoint: f.endpoint, Bucket: f.bucket, Key: key, Namespace: ns, ContentType: domain.AllowedMIME, MaxBytes: domain.MaxUploadBytes, TTL: storage.DefaultURLTTL}
}
func (f finalStorageFixture) put(ctx context.Context, key string, body []byte) error {
	signed, err := storage.PresignPUT(f.input(key), f.cred)
	if err != nil {
		return err
	}
	return storage.Put(ctx, nil, signed.URL, domain.AllowedMIME, body)
}
func (f finalStorageFixture) download(ctx context.Context, key string, limit int64) ([]byte, error) {
	signed, err := storage.PresignGET(f.input(key), f.cred)
	if err != nil {
		return nil, err
	}
	return storage.Get(ctx, nil, signed.URL, limit)
}
func (f finalStorageFixture) exists(ctx context.Context, key string) (bool, error) {
	signed, err := storage.PresignHEAD(f.input(key), f.cred)
	if err != nil {
		return false, err
	}
	return storage.Exists(ctx, nil, signed.URL)
}
func (f finalStorageFixture) delete(ctx context.Context, key string) error {
	return storage.DeleteConfirmed(ctx, nil, f.input(key), f.cred)
}
func finalObjectsFixture(t *testing.T) finalStorageFixture {
	t.Helper()
	endpoint := os.Getenv("ANPFUEL_TEST_EVIDENCE_S3_ENDPOINT")
	user, pass, bucket := "finalfixture", "final-fixture-only-20261011", "rva07-final"
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.User != nil {
			t.Fatal("only an explicit loopback S3 fixture is allowed")
		}
		user, pass, bucket = "localvalidation", "local-validation-only-20260930", "validation-private"
	} else {
		// Never start or stop containers on a remote Docker daemon.
		if host := os.Getenv("DOCKER_HOST"); host != "" && host != "unix:///var/run/docker.sock" {
			t.Fatal("local Docker required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		const minio = "bitnamilegacy/minio@sha256:451fe6858cb770cc9d0e77ba811ce287420f781c7c1b806a386f6896471a349c"
		const mc = "bitnamilegacy/minio-client@sha256:00dcc4e58ada0df45bb7d9ee435af98295f96c27c3c68292ce78ec700a87b511"
		var stderr bytes.Buffer
		start := exec.CommandContext(ctx, "docker", "--host", "unix:///var/run/docker.sock", "run", "-d", "--rm", "-p", "127.0.0.1::9000", "--memory", "512m", "--cpus", "2", "--tmpfs", "/data:rw,size=64m,mode=1777", "--entrypoint", "/opt/bitnami/minio/bin/minio", "-e", "MINIO_ROOT_USER="+user, "-e", "MINIO_ROOT_PASSWORD="+pass, minio, "server", "/data")
		start.Stderr = &stderr
		out, err := start.Output()
		if err != nil {
			t.Fatalf("start output fixture: %v %s", err, stderr.String())
		}
		id := strings.TrimSpace(string(out))
		if len(id) != 64 {
			t.Fatalf("invalid fixture container id %q", id)
		}
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
			defer stop()
			if out, err := exec.CommandContext(cleanup, "docker", "--host", "unix:///var/run/docker.sock", "stop", id).CombinedOutput(); err != nil {
				t.Errorf("stop owned output fixture: %v %s", err, out)
			}
		})
		ports, err := exec.CommandContext(ctx, "docker", "--host", "unix:///var/run/docker.sock", "port", id, "9000").Output()
		if err != nil {
			t.Fatal(err)
		}
		endpoint = "http://" + strings.TrimSpace(string(ports))
		client := &http.Client{Timeout: time.Second}
		for {
			resp, err := client.Get(endpoint + "/minio/health/ready")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					break
				}
			}
			select {
			case <-ctx.Done():
				t.Fatal("S3 fixture not ready")
			case <-time.After(100 * time.Millisecond):
			}
		}
		mk := exec.CommandContext(ctx, "docker", "--host", "unix:///var/run/docker.sock", "run", "--rm", "--network", "host", "--entrypoint", "sh", mc, "-c", `mc alias set local "$1" "$2" "$3" && mc mb "local/$4"`, "sh", endpoint, user, pass, bucket)
		if out, err := mk.CombinedOutput(); err != nil {
			t.Fatalf("make private fixture bucket: %v %s", err, out)
		}
	}
	return finalStorageFixture{endpoint: endpoint, bucket: bucket, cred: storage.Credentials{AccessKeyID: user, SecretAccessKey: pass}}
}

func finalPhotoFixture(t *testing.T, s *Store, objects finalStorageFixture) (domain.Session, []byte) {
	t.Helper()
	raw := loadSmallJPEG(t)
	sess := testSession(newUUIDv4must(t), newUUIDv4must(t))
	sess.ClaimedSHA256 = sha256hex(raw)
	if _, _, err := s.ReserveSession(context.Background(), sess, openBudget()); err != nil {
		t.Fatal(err)
	}
	if err := objects.put(context.Background(), sess.QuarantineKey, raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{sess.QuarantineKey, "f/" + sess.ClaimedSHA256} {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := objects.delete(ctx, key); err != nil {
				t.Error(err)
			}
		})
	}
	var queued [][]byte
	if err := s.CompleteSession(context.Background(), sess.ID, enqueueStub(&queued)); err != nil {
		t.Fatal(err)
	}
	sess, err := s.Session(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	return sess, raw
}
func finalPorts(t *testing.T, s *Store, sess domain.Session, objects finalStorageFixture) app.VerifyPorts {
	t.Helper()
	return app.VerifyPorts{Download: objects.download, Upload: func(ctx context.Context, key, _ string, body []byte) error { return objects.put(ctx, key, body) }, BeforeUpload: func(ctx context.Context, key, hash string) (app.FinalWrite, error) {
		token, err := finalCleanupNonce()
		if err != nil {
			return app.FinalWrite{}, err
		}
		return s.BeginVerificationWrite(ctx, sess.ID, key, hash, uuidString(token))
	}}
}
func finalObject(t *testing.T, out app.Outcome) ObjectData {
	t.Helper()
	return ObjectData{ID: newUUIDv4must(t), FinalKey: out.FinalKey, SourceSHA256: out.SourceSHA256, SanitizedSHA256: out.SanitizedSHA256, Width: out.Width, Height: out.Height, DHash: out.DHash}
}

func TestFinalCleanupRealS3PausedWriterErasure(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	objects := finalObjectsFixture(t)
	sess, _ := finalPhotoFixture(t, s, objects)
	ports := finalPorts(t, s, sess, objects)
	entered, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(resume) })
	ports.Upload = func(ctx context.Context, key, _ string, body []byte) error {
		close(entered)
		select {
		case <-resume:
		case <-ctx.Done():
			return ctx.Err()
		}
		return objects.put(ctx, key, body)
	}
	type result struct {
		out app.Outcome
		err error
	}
	done := make(chan result, 1)
	go func() { out, err := app.Verify(ctx, ports, sess); done <- result{out, err} }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not pause")
	}
	deletes := func(ctx context.Context, key, _ string) error { return objects.delete(ctx, key) }
	if _, _, err := s.EraseContributor(ctx, sess.ContributorRef, deletes); err == nil {
		t.Fatal("live final writer falsely erased")
	}
	once.Do(func() { close(resume) })
	var got result
	select {
	case got = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not resume")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, finalObject(t, got.out), got.out.Write); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("late output became READY", err)
	}
	present, err := objects.exists(ctx, got.out.FinalKey)
	if err != nil || !present {
		t.Fatal("late provider effect must remain inventoried", present, err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, got.out.FinalKey)
	if err != nil || row.State != "WRITING" {
		t.Fatal(row, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second' WHERE final_key=$1`, got.out.FinalKey); err != nil {
		t.Fatal(err)
	}
	_, n, err := NewStore(pool).EraseContributor(ctx, sess.ContributorRef, deletes)
	if err != nil || n != 1 {
		t.Fatal("restart did not erase late output", n, err)
	}
	present, err = objects.exists(ctx, got.out.FinalKey)
	if err != nil || present {
		t.Fatal("late bytes survived", present, err)
	}
	row, err = evidence.New(pool).GetFinalOutputForUpdate(ctx, got.out.FinalKey)
	if err != nil || row.State != "PURGED" || !row.PurgedAt.Valid {
		t.Fatal(row, err)
	}
	a, b, err := s.EraseContributor(ctx, sess.ContributorRef, deletes)
	if err != nil || a != 0 || b != 0 {
		t.Fatal("repeat did work", a, b, err)
	}
}

func TestFinalCleanupRealS3LostReplyFailureRepairAndStaleCleaner(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	objects := finalObjectsFixture(t)
	sess, _ := finalPhotoFixture(t, s, objects)
	ports := finalPorts(t, s, sess, objects)
	ports.Upload = func(ctx context.Context, key, _ string, body []byte) error {
		if err := objects.put(ctx, key, body); err != nil {
			return err
		}
		return errors.New("synthetic lost PUT reply")
	}
	if _, err := app.Verify(ctx, ports, sess); err == nil {
		t.Fatal("lost reply accepted")
	}
	key := "f/" + sess.ClaimedSHA256
	rep, err := s.ReconcileFinalOutputs(ctx, time.Now(), 32, objects.delete)
	if err != nil || rep.Checked != 0 {
		t.Fatal("live writer bypassed", rep, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second' WHERE final_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	rep, err = s.ReconcileFinalOutputs(ctx, time.Now(), 32, func(context.Context, string) error { return errors.New("synthetic provider DELETE outage") })
	if err == nil || rep.Checked != 1 || rep.Purged != 0 || rep.Failures != 1 {
		t.Fatal(rep, err)
	}
	prior, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, key)
	if err != nil || prior.State != "PURGING" {
		t.Fatal(prior, err)
	}
	present, err := objects.exists(ctx, key)
	if err != nil || !present {
		t.Fatal(present, err)
	}
	rep, err = NewStore(pool).ReconcileFinalOutputs(ctx, time.Now(), 32, objects.delete)
	if err != nil || rep.Checked != 0 {
		t.Fatal("live cleaner bypassed", rep, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second' WHERE final_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	nonce, claimed, err := s.claimFinalCleanup(ctx, key, "", time.Now())
	if err != nil || !claimed || nonce == prior.LeaseNonce {
		t.Fatal(claimed, err)
	}
	calls := 0
	if err := s.disposeFinalClaim(ctx, key, prior.LeaseNonce, func(context.Context, string) error { calls++; return nil }); !errors.Is(err, domain.ErrBadTransition) || calls != 0 {
		t.Fatal("stale cleaner touched bytes", calls, err)
	}
	if err := s.finishFinalCleanup(ctx, key, prior.LeaseNonce); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("stale cleaner acknowledged", err)
	}
	if err := s.disposeFinalClaim(ctx, key, nonce, objects.delete); err != nil {
		t.Fatal(err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, key)
	if err != nil || row.State != "PENDING" || row.Attempts != 1 {
		t.Fatal(row, err)
	}
	present, err = objects.exists(ctx, key)
	if err != nil || present {
		t.Fatal("incomplete bytes survived repair", present, err)
	}
	out, err := app.Verify(ctx, finalPorts(t, s, sess, objects), sess)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, finalObject(t, out), out.Write); err != nil {
		t.Fatal(err)
	}
	// A duplicate request references existing bytes; it never PUTs again.
	other := testSession(newUUIDv4must(t), newUUIDv4must(t))
	other.ContributorRef = "foreign-owner"
	other.ClaimedSHA256 = sess.ClaimedSHA256
	if _, _, err := s.ReserveSession(ctx, other, openBudget()); err != nil {
		t.Fatal(err)
	}
	var queued [][]byte
	if err := s.CompleteSession(ctx, other.ID, enqueueStub(&queued)); err != nil {
		t.Fatal(err)
	}
	other.Status = domain.StateVerifying
	p := finalPorts(t, s, other, objects)
	p.Download = func(context.Context, string, int64) ([]byte, error) { return loadSmallJPEG(t), nil }
	p.Upload = func(context.Context, string, string, []byte) error {
		t.Fatal("duplicate overwrote first copy")
		return nil
	}
	duplicate, err := app.Verify(ctx, p, other)
	if err != nil || !duplicate.Write.Reused {
		t.Fatal(duplicate, err)
	}
	if _, err := s.RecordVerifiedGuarded(ctx, other.ID, finalObject(t, duplicate), duplicate.Write); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EraseContributor(ctx, other.ContributorRef, func(ctx context.Context, key, _ string) error { return objects.delete(ctx, key) }); err != nil {
		t.Fatal(err)
	}
	present, err = objects.exists(ctx, key)
	if err != nil || !present {
		t.Fatal("foreign erasure deleted original owner's bytes", present, err)
	}
}

func TestFinalCleanupBoundedBatchesAuditAndIndex(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	sess, _ := finalVerifyingFixture(t, s)
	// Large terminal history must not dominate the hot cleanup index.
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at,purged_at)
 SELECT 'f/terminal-'||n,$1,'','PURGED',now()-interval '1 day',now() FROM generate_series(1,10000) n`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO evidence_final_outputs(final_key,session_id,sanitized_sha256,state,deadline_at)
 SELECT 'f/pending-'||n,$1,'','PENDING',now()-interval '1 minute' FROM generate_series(1,65) n`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_sessions SET status='EXPIRED',quarantine_deleted_at=now() WHERE id=$1`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE evidence_final_outputs`); err != nil {
		t.Fatal(err)
	}
	overdue, err := s.OverdueFinals(ctx, time.Now().Add(-app.CopyRetention), 100)
	if err != nil || len(overdue) != 65 {
		t.Fatal("unconfirmed copy missing in audit", len(overdue), err)
	}
	// EXPLAIN the actual source query, not a handwritten similar statement.
	raw, err := os.ReadFile("../../../../db/queries/evidence/evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := strings.Split(strings.Split(string(raw), "-- name: NextFinalOutputCleanup :one\n")[1], ";\n")[0]
	query = strings.ReplaceAll(strings.ReplaceAll(query, "@at", "$1"), "@due_cutoff", "$2")
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) "+query, time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !strings.Contains(plan.String(), "evidence_final_output_cleanup_idx") || strings.Contains(plan.String(), "Seq Scan on evidence_final_outputs") {
		t.Fatal(plan.String())
	}
	for _, want := range []int{32, 32, 1, 0} {
		rep, err := NewStore(pool).ReconcileFinalOutputs(ctx, time.Now(), 100, func(ctx context.Context, _ string) error {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 5*time.Second {
				t.Fatal("copy has no 5s bound")
			}
			return nil
		})
		if err != nil || rep.Checked != want || rep.Purged != want || rep.Failures != 0 {
			t.Fatal(rep, err)
		}
	}
	overdue, err = s.OverdueFinals(ctx, time.Now().Add(-app.CopyRetention), 100)
	if err != nil || len(overdue) != 0 {
		t.Fatal(overdue, err)
	}
}

func TestFinalCleanupUpgradePreservesPendingWriter(t *testing.T) {
	before, target := fstest.MapFS{}, fstest.MapFS{}
	names, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name > "000069_evidence_final_cleanup_index.sql" {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: raw}
		if name < "000069_evidence_final_cleanup_index.sql" {
			before[name] = &fstest.MapFile{Data: raw}
		}
	}
	s, pool := freshStoreSchema(t, before)
	ctx := context.Background()
	sess, obj := finalVerifyingFixture(t, s)
	authority, err := s.BeginVerificationWrite(ctx, sess.ID, obj.FinalKey, obj.SanitizedSHA256, newUUIDv4must(t))
	if err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target)
	if err != nil || len(applied) != 1 || applied[0] != "000069" {
		t.Fatal(applied, err)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil || uuidString(row.LeaseNonce) != authority.Nonce || row.State != "WRITING" {
		t.Fatal(row, err)
	}
	if applied, err := migrate.Apply(ctx, pool.Config().ConnString(), target); err != nil || len(applied) != 0 {
		t.Fatal(applied, err)
	}
}

func TestFinalCleanupConcurrentClaimAndReceiptRollback(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	objects := finalObjectsFixture(t)
	sess, _ := finalPhotoFixture(t, s, objects)
	out, err := app.Verify(ctx, finalPorts(t, s, sess, objects), sess)
	if err != nil {
		t.Fatal(err)
	}
	obj := finalObject(t, out)
	if _, err := s.RecordVerifiedGuarded(ctx, sess.ID, obj, out.Write); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	tokens := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nonce, claimed, err := s.claimFinalCleanup(ctx, out.FinalKey, sess.ContributorRef, time.Now())
			if err != nil {
				t.Error(err)
			}
			if claimed {
				tokens <- uuidString(nonce)
			}
		}()
	}
	wg.Wait()
	close(tokens)
	count := 0
	for range tokens {
		count++
	}
	if count != 1 {
		t.Fatal("cleanup authorities", count)
	}
	row, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, out.FinalKey)
	if err != nil || row.State != "PURGING" {
		t.Fatal(row, err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_final_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='PURGED' THEN RAISE EXCEPTION 'synthetic receipt fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_final_receipt BEFORE UPDATE ON evidence_final_outputs FOR EACH ROW EXECUTE FUNCTION fail_final_receipt()`); err != nil {
		t.Fatal(err)
	}
	if err := s.disposeFinalClaim(ctx, out.FinalKey, row.LeaseNonce, objects.delete); err == nil {
		t.Fatal("receipt fault acknowledged")
	}
	present, err := objects.exists(ctx, out.FinalKey)
	if err != nil || present {
		t.Fatal("provider effect missing", present, err)
	}
	var deleted bool
	if err := pool.QueryRow(ctx, `SELECT final_deleted_at IS NOT NULL FROM evidence_objects WHERE id=$1`, obj.ID).Scan(&deleted); err != nil || deleted {
		t.Fatal("partial receipt committed", deleted, err)
	}
	kept, err := evidence.New(pool).GetFinalOutputForUpdate(ctx, out.FinalKey)
	if err != nil || kept.State != "PURGING" || kept.LeaseNonce != row.LeaseNonce {
		t.Fatal(kept, err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_final_receipt ON evidence_final_outputs`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence_final_outputs SET lease_until=now()-interval '1 second' WHERE final_key=$1`, out.FinalKey); err != nil {
		t.Fatal(err)
	}
	rep, err := NewStore(pool).ReconcileFinalOutputs(ctx, time.Now(), 32, objects.delete)
	if err != nil || rep.Checked != 1 || rep.Purged != 1 {
		t.Fatal(rep, err)
	}
	if err := pool.QueryRow(ctx, `SELECT final_deleted_at IS NOT NULL FROM evidence_objects WHERE id=$1`, obj.ID).Scan(&deleted); err != nil || !deleted {
		t.Fatal(deleted, err)
	}
	if err := s.finishFinalCleanup(ctx, out.FinalKey, row.LeaseNonce); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatal("old cleaner acknowledged after restart", err)
	}
}
