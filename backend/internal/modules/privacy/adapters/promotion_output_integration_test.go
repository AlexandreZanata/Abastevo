//go:build integration

package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Real private SigV4 transfers and SQL inventory; these synthetic bytes test
// delivery, never qualification of the content minimizer (collection is OFF).
func outputObjectsFixture(t *testing.T) *TrainingObjects {
	t.Helper()
	endpoint := os.Getenv("ANPFUEL_TEST_TRAINING_S3_ENDPOINT")
	user, pass, bucket := "outputfixture", "output-fixture-only-20261011", "rva05-output"
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
	return NewTrainingObjects(TrainingObjectConfig{Endpoint: endpoint, Bucket: bucket, Region: "auto", AccessKeyID: user, SecretAccessKey: pass})
}

type outputFault struct {
	*TrainingObjects
	mode            string
	calls           atomic.Int32
	entered, resume chan struct{}
}

func (f *outputFault) Put(ctx context.Context, key string, body []byte) error {
	n := f.calls.Add(1)
	if n == 1 {
		switch f.mode {
		case "before":
			return errors.New("synthetic pre-effect failure")
		case "lost":
			if err := f.TrainingObjects.Put(ctx, key, body); err != nil {
				return err
			}
			return errors.New("synthetic lost reply")
		case "corrupt":
			return f.TrainingObjects.Put(ctx, key, []byte("different synthetic bytes"))
		case "pause":
			close(f.entered)
			select {
			case <-f.resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return f.TrainingObjects.Put(ctx, key, body)
}
func nonceForOutput() (string, error) {
	// Test-local counter is used by callers which need several unique leases.
	n := outputNonce.Add(1)
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
}

var outputNonce atomic.Int64

func outputRow(t *testing.T, s *PromotionStore, suffix string, at time.Time) (app.PromotionResult, []byte) {
	t.Helper()
	body := []byte("synthetic derivative: " + suffix)
	d := mustDerivative(t, fmt.Sprintf("output-%d-%s", time.Now().UnixNano(), suffix), "c1", 1, at)
	sum := sha256.Sum256(body)
	d.DerivativeSHA256 = hex.EncodeToString(sum[:])
	if _, _, err := s.Put(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), d.ImageID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return app.PromotionResult{ContributorID: got.ContributorID, ImageID: got.ImageID, ConsentRevision: got.ConsentRevision, PromotedAt: got.PromotedAt, DerivativeSHA256: got.DerivativeSHA256, ObjectKey: got.ObjectKey, OutputState: got.OutputState}, body
}
func outputPorts(s *PromotionStore, o app.OutputObjects, clock func() time.Time) app.PublishPorts {
	return app.PublishPorts{Store: s, Objects: o, Terms: fakeConsentTerms{}, NewNonce: nonceForOutput, Clock: clock}
}
func outputState(t *testing.T, s *PromotionStore, r app.PromotionResult, want string, attempts int) {
	t.Helper()
	var state string
	var got int
	if err := s.pool.QueryRow(context.Background(), `SELECT output_state,output_attempts FROM privacy_training_derivatives WHERE image_id=$1 AND consent_revision=$2`, r.ImageID, r.ConsentRevision).Scan(&state, &got); err != nil {
		t.Fatal(err)
	}
	if state != want || got != attempts {
		t.Fatalf("state %s/%d, want %s/%d", state, got, want, attempts)
	}
}
func TestOutputRecoveryOnRealSQLAndPrivateS3(t *testing.T) {
	objects := outputObjectsFixture(t)
	for _, mode := range []string{"before", "lost", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			s := promotionPG(t)
			ctx := context.Background()
			at := time.Now().UTC().Truncate(time.Millisecond)
			r, body := outputRow(t, s, mode, at)
			t.Cleanup(func() { _ = objects.Delete(ctx, r.ObjectKey) })
			fault := &outputFault{TrainingObjects: objects, mode: mode}
			p := outputPorts(s, fault, func() time.Time { return at })
			if err := app.Publish(ctx, p, r, body); err == nil {
				t.Fatal("fault was acknowledged")
			}
			outputState(t, s, r, domain.OutputPending, 1)
			if exists, err := objects.Exists(ctx, r.ObjectKey); err != nil || exists {
				t.Fatal("failed output cleanup", exists, err)
			}
			if selected, err := s.ListActiveByOwner(ctx, "c1"); err != nil || len(selected) != 0 {
				t.Fatal("pending output selected", selected, err)
			}
			// Restarted inventory and later attempt retain the original day/key.
			restarted := NewPromotionStore(s.pool)
			got, err := restarted.Get(ctx, r.ImageID, 1)
			if err != nil || got.ObjectKey != r.ObjectKey || !got.PromotedAt.Equal(at) {
				t.Fatal("immutable locator changed", got, err)
			}
			p = outputPorts(restarted, fault, func() time.Time { return at.Add(25 * time.Hour) })
			r.Replayed = true
			if err := app.Publish(ctx, p, r, body); err != nil {
				t.Fatal("replay failed to repair", err)
			}
			outputState(t, s, r, domain.OutputReady, 2)
			if valid, err := objects.HasHash(ctx, r.ObjectKey, r.DerivativeSHA256); err != nil || !valid {
				t.Fatal("byte verification", valid, err)
			}
			if selected, err := s.ListActiveByOwner(ctx, "c1"); err != nil || len(selected) != 1 {
				t.Fatal("verified output not selected", selected, err)
			}
			if err := app.Publish(ctx, p, r, body); err != nil || fault.calls.Load() != 2 {
				t.Fatal("READY replay rewrote bytes", fault.calls.Load(), err)
			}
		})
	}
	t.Run("revoke-during-write", func(t *testing.T) {
		s := promotionPG(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r, body := outputRow(t, s, "paused", time.Now().Truncate(time.Millisecond))
		t.Cleanup(func() { _ = objects.Delete(context.Background(), r.ObjectKey) })
		fault := &outputFault{TrainingObjects: objects, mode: "pause", entered: make(chan struct{}), resume: make(chan struct{})}
		result := make(chan error, 1)
		go func() { result <- app.Publish(ctx, outputPorts(s, fault, time.Now), r, body) }()
		select {
		case <-fault.entered:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		c, err := NewConsentStore(s.pool).Get(ctx, "c1")
		if err != nil {
			t.Fatal(err)
		}
		revoked, _, err := domain.RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := NewConsentStore(s.pool).Put(ctx, revoked); err != nil {
			t.Fatal(err)
		}
		close(fault.resume)
		if err := <-result; err == nil {
			t.Fatal("revoked writer acknowledged")
		}
		outputState(t, s, r, domain.OutputPurged, 1)
		if exists, err := objects.Exists(ctx, r.ObjectKey); err != nil || exists {
			t.Fatal("late bytes survived", exists, err)
		}
		if selected, err := s.ListActiveByOwner(ctx, "c1"); err != nil || len(selected) != 0 {
			t.Fatal("revoked output selected", selected, err)
		}
	})
	t.Run("crash-stale-completion-and-cleanup", func(t *testing.T) {
		s := promotionPG(t)
		ctx := context.Background()
		at := time.Now().Truncate(time.Millisecond)
		r, body := outputRow(t, s, "crash", at)
		t.Cleanup(func() { _ = objects.Delete(ctx, r.ObjectKey) })
		old, _ := nonceForOutput()
		if err := s.BeginOutput(ctx, r, old, at, domain.TrainingTermsV1, "digest-v1"); err != nil {
			t.Fatal(err)
		}
		if err := objects.Put(ctx, r.ObjectKey, body); err != nil {
			t.Fatal(err)
		}
		// No ack: durable WRITING remains unselectable through process restart.
		if selected, err := s.ListActiveByOwner(ctx, "c1"); err != nil || len(selected) != 0 {
			t.Fatal(selected, err)
		}
		restarted := NewPromotionStore(s.pool)
		later := at.Add(2 * time.Minute)
		if err := app.Publish(ctx, outputPorts(restarted, objects, func() time.Time { return later }), r, body); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishOutput(ctx, r, old, true, later, domain.TrainingTermsV1, "digest-v1"); !errors.Is(err, domain.ErrBadState) {
			t.Fatal("stale completion", err)
		}
		if err := s.PrepareOutputCleanup(ctx, r, old, later); !errors.Is(err, domain.ErrBadState) {
			t.Fatal("stale cleanup", err)
		}
		if valid, err := objects.HasHash(ctx, r.ObjectKey, r.DerivativeSHA256); err != nil || !valid {
			t.Fatal("new output was removed", valid, err)
		}
		outputState(t, s, r, domain.OutputReady, 2)
	})
}

func TestOutputLeaseIdentityAndAttemptsOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	r, _ := outputRow(t, s, "admission", at)
	for _, mutate := range []func(*app.PromotionResult){func(r *app.PromotionResult) { r.ContributorID = "c2" }, func(r *app.PromotionResult) { r.DerivativeSHA256 = "forged" }, func(r *app.PromotionResult) { r.ObjectKey += "other" }} {
		bad := r
		mutate(&bad)
		n, _ := nonceForOutput()
		if err := s.BeginOutput(ctx, bad, n, at, domain.TrainingTermsV1, "digest-v1"); err == nil {
			t.Fatal("forged identity admitted")
		}
	}
	var wg sync.WaitGroup
	var wins atomic.Int32
	winner := make(chan string, 1)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, _ := nonceForOutput()
			err := s.BeginOutput(ctx, r, n, at, domain.TrainingTermsV1, "digest-v1")
			if err == nil {
				wins.Add(1)
				winner <- n
			} else if !errors.Is(err, domain.ErrBadState) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatal("lease winners", wins.Load())
	}
	n := <-winner
	if err := s.PrepareOutputCleanup(ctx, r, n, at); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishOutput(ctx, r, n, false, at, domain.TrainingTermsV1, "digest-v1"); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 5; i++ {
		n, _ := nonceForOutput()
		if err := s.BeginOutput(ctx, r, n, at, domain.TrainingTermsV1, "digest-v1"); err != nil {
			t.Fatal(err)
		}
		if err := s.PrepareOutputCleanup(ctx, r, n, at); err != nil {
			t.Fatal(err)
		}
		if err := s.FinishOutput(ctx, r, n, false, at, domain.TrainingTermsV1, "digest-v1"); err != nil {
			t.Fatal(err)
		}
	}
	n, _ = nonceForOutput()
	if err := s.BeginOutput(ctx, r, n, at, domain.TrainingTermsV1, "digest-v1"); !errors.Is(err, domain.ErrBadState) {
		t.Fatal("attempt budget bypass", err)
	}
	outputState(t, s, r, domain.OutputPending, 5)
}

func TestOutputUpgradeAppliesOnly000065(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dsn := freshPre057DB(t, testDSN(t))
	previous, target := fstest.MapFS{}, fstest.MapFS{}
	files, err := fs.Glob(dbmigrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if name > "000065_privacy_training_output.sql" {
			continue
		}
		data, err := fs.ReadFile(dbmigrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		target[name] = &fstest.MapFile{Data: data}
		if name < "000065_privacy_training_output.sql" {
			previous[name] = &fstest.MapFile{Data: data}
		}
	}
	if _, err := migrate.Apply(ctx, dsn, previous); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO privacy_training_derivatives(image_id,consent_revision,contributor_id,contributor_ref,source_sha256,derivative_sha256,captured_at,terms_version,terms_digest,state,promoted_at,revoked_at)
 VALUES ('old-active',1,'old-owner','old-ref','src','sha',now(),'training-terms-v1','digest-v1','ACTIVE','2026-10-11T01:30:00+03',NULL),
 ('old-revoked',1,'old-owner','old-ref','src','sha',now(),'training-terms-v1','digest-v1','REVOKED','2026-10-11T01:30:00+03',now())`); err != nil {
		t.Fatal(err)
	}
	applied, err := migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 1 || applied[0] != "000065" {
		t.Fatal(applied, err)
	}
	for _, tc := range []struct{ image, state string }{{"old-active", domain.OutputPending}, {"old-revoked", domain.OutputPurging}} {
		var key, state string
		var attempts int
		if err := conn.QueryRow(ctx, `SELECT object_key,output_state,output_attempts FROM privacy_training_derivatives WHERE image_id=$1`, tc.image).Scan(&key, &state, &attempts); err != nil {
			t.Fatal(err)
		}
		if state != tc.state || key != "training/v1/2026-10-10/"+tc.image+"-1.jpg" || attempts != 0 {
			t.Fatal("unsafe historical output backfill", key, state, attempts)
		}
	}
	applied, err = migrate.Apply(ctx, dsn, target)
	if err != nil || len(applied) != 0 {
		t.Fatal("non-idempotent upgrade", applied, err)
	}
}

type constantOutputSanitizer struct{}

func (constantOutputSanitizer) Sanitize(context.Context, []byte) ([]byte, domain.SanitizeVerdict, error) {
	return []byte("synthetic minimized bytes"), domain.VerdictPass, nil
}
func TestPromotionReplayKeepsCommittedDateAndQueuedRevisionOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	at := time.Now().Truncate(time.Millisecond)
	p := app.PromotePorts{ConsentPorts: consentAppPorts(NewConsentStore(s.pool)), Inventory: s, Sanitizer: constantOutputSanitizer{}}
	p.Clock = func() time.Time { return at }
	dto := app.PromotionDTO{ConsentRevision: 1, ImageID: "replay-date", CapturedAt: at, PerImageAllowed: true, Source: []byte("synthetic source")}
	first, _, err := app.Propose(ctx, p, app.Caller{ContributorID: "c1"}, dto)
	if err != nil {
		t.Fatal(err)
	}
	p.Clock = func() time.Time { return at.Add(48 * time.Hour) }
	second, body, err := app.Propose(ctx, p, app.Caller{ContributorID: "c1"}, dto)
	if err != nil || !second.Replayed || len(body) == 0 || first.ObjectKey != second.ObjectKey || !first.PromotedAt.Equal(second.PromotedAt) {
		t.Fatal("replay lost immutable output", first, second, err)
	}
	dto.ConsentRevision = 2
	if _, _, err := app.Propose(ctx, p, app.Caller{ContributorID: "c1"}, dto); !errors.Is(err, domain.ErrConsentRequired) {
		t.Fatal("queued revision mismatch accepted", err)
	}
}

func TestUnqualifiedContentCannotCreateInventoryOnSQL(t *testing.T) {
	s := promotionPG(t)
	ctx := context.Background()
	p := app.PromotePorts{ConsentPorts: consentAppPorts(NewConsentStore(s.pool)), Inventory: s, Sanitizer: unqualifiedSanitizer{}}
	_, body, err := app.Propose(ctx, p, app.Caller{ContributorID: "c1"}, app.PromotionDTO{ConsentRevision: 1, ImageID: "unqualified-image", CapturedAt: time.Now(), PerImageAllowed: true, Source: testJPEG(t, 32, 16)})
	if err == nil || len(body) != 0 {
		t.Fatal("unqualified image was proposed", len(body), err)
	}
	var rows int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM privacy_training_derivatives`).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("uncertain input persisted", rows, err)
	}
}

type unqualifiedSanitizer struct{}

func (unqualifiedSanitizer) Sanitize(ctx context.Context, source []byte) ([]byte, domain.SanitizeVerdict, error) {
	return SanitizeImage(ctx, source)
}
