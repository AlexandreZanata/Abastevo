//go:build integration

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Pinned MinIO image for the local S3-compatible service.
const testMinioImage = "minio/minio:RELEASE.2025-04-22T22-12-26Z"
const testMCImage = "bitnamilegacy/minio-client:latest"

const (
	testMinioUser = "testadmin"
	testMinioPass = "testadmin123456"
	testBucket    = "sec03c"
)

// startMinIO boots a disposable loopback MinIO with one bucket and
// returns its endpoint. Callers stop it via cleanup. Small synthetic
// payloads only; the anonymous volume dies with the container.
func startMinIO(t *testing.T, bucket string) (endpoint string, cleanup func()) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable for the S3-compatible matrix")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::9000",
		"-e", "MINIO_ROOT_USER="+testMinioUser,
		"-e", "MINIO_ROOT_PASSWORD="+testMinioPass,
		testMinioImage, "server", "/data").CombinedOutput()
	if err != nil {
		cancel()
		t.Fatalf("start minio: %v (%s)", err, out)
	}
	id := strings.TrimSpace(string(out))
	cleanup = func() {
		defer cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = exec.CommandContext(stopCtx, "docker", "stop", id).Run()
	}
	t.Cleanup(func() { cleanup() })
	portOut, err := exec.CommandContext(ctx, "docker", "port", id, "9000").CombinedOutput()
	if err != nil {
		t.Fatalf("minio port: %v (%s)", err, portOut)
	}
	port := strings.TrimSpace(string(portOut))
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	endpoint = "http://127.0.0.1:" + port
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/")
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	mk := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "host",
		"--entrypoint", "sh", testMCImage, "-c",
		fmt.Sprintf("mc alias set local %s %s %s && mc mb local/%s",
			endpoint, testMinioUser, testMinioPass, bucket))
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("make bucket: %v (%s)", err, out)
	}
	return endpoint, func() {}
}

func testCreds() Credentials {
	return Credentials{AccessKeyID: testMinioUser, SecretAccessKey: testMinioPass}
}

func presignPUT(t *testing.T, endpoint, key string, maxBytes int64) PresignedPUT {
	t.Helper()
	got, err := PresignPUT(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: key, Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: maxBytes, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// SEC03-C provider matrix (AUD-02): small synthetic payloads over the
// actual SigV4/HTTP protocol against a real local S3-compatible
// service — never a permissive fake. A returned JSON maximum alone
// is insufficient: transport accepts what verification must refuse.
func TestSEC03CTransportMatrixOnRealS3(t *testing.T) {
	endpoint, _ := startMinIO(t, testBucket)
	ctx := context.Background()

	// max accepted exactly; max+1 accepted by transport (proving the
	// JSON maximum alone caps nothing) but refused by the bounded
	// read that verification uses.
	maxBody := bytes.Repeat([]byte{0x7f}, int(domain.MaxUploadBytes))
	overBody := bytes.Repeat([]byte{0x7f}, int(domain.MaxUploadBytes)+1)
	putMax := presignPUT(t, endpoint, "q/max", domain.MaxUploadBytes)
	if err := Put(ctx, nil, putMax.URL, domain.AllowedMIME, maxBody); err != nil {
		t.Fatalf("max PUT: %v", err)
	}
	putOver := presignPUT(t, endpoint, "q/over", domain.MaxUploadBytes)
	if err := Put(ctx, nil, putOver.URL, domain.AllowedMIME, overBody); err != nil {
		t.Fatalf("transport must accept over-max (verify refuses later), got %v", err)
	}
	getOver, err := PresignGET(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/over", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: domain.MaxUploadBytes, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Get(ctx, nil, getOver.URL, domain.MaxUploadBytes); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("bounded read of max+1 = %v, want too-large", err)
	}

	// Declared/actual mismatch: transport stores; the claim bind at
	// verify time is the only defense (pinned in application tests).
	mismatch := presignPUT(t, endpoint, "q/mismatch", 1024)
	small := bytes.Repeat([]byte{0x7e}, 1024)
	if err := Put(ctx, nil, mismatch.URL, domain.AllowedMIME, small); err != nil {
		t.Fatalf("small PUT: %v", err)
	}

	// Wrong content type breaks the signature (type is bound).
	typed := presignPUT(t, endpoint, "q/typed", 1024)
	if err := Put(ctx, nil, typed.URL, "image/png", small); err == nil {
		t.Fatal("wrong content type accepted")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || status.Status != 403 {
			t.Fatalf("wrong type = %v, want 403", err)
		}
	}

	// Wrong key with a transplanted query string breaks the signature
	// (key is bound).
	keyed := presignPUT(t, endpoint, "q/one", 1024)
	if i := strings.Index(keyed.URL, "?"); i >= 0 {
		tampered := endpoint + "/" + testBucket + "/q/two" + keyed.URL[i:]
		if err := Put(ctx, nil, tampered, domain.AllowedMIME, small); err == nil {
			t.Fatal("transplanted key accepted")
		} else {
			var status *StatusError
			if !errors.As(err, &status) || status.Status != 403 {
				t.Fatalf("transplanted key = %v, want 403", err)
			}
		}
	} else {
		t.Fatal("presigned URL carries no query to transplant")
	}

	// Missing key reads 404; missing deletes converge nil.
	getMissing, err := PresignGET(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/absent", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: 1024, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Get(ctx, nil, getMissing.URL, 1024); err == nil {
		t.Fatal("missing key read accepted")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || status.Status != 404 {
			t.Fatalf("missing key = %v, want 404", err)
		}
	}
	delMissing, err := PresignDELETE(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/absent", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: 1024, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, nil, delMissing.URL); err != nil {
		t.Fatalf("missing delete must converge nil, got %v", err)
	}

	// Expired URL (backdated mint, real 5-minute TTL long past)
	// refuses at the provider edge.
	expired, err := PresignPUT(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/stale", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: 1024, TTL: DefaultURLTTL,
		Now: time.Now().Add(-time.Hour),
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	if err := Put(ctx, nil, expired.URL, domain.AllowedMIME, small); err == nil {
		t.Fatal("expired URL accepted")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || (status.Status != 403 && status.Status != 400) {
			t.Fatalf("expired URL = %v, want 4xx", err)
		}
	}

	// Interrupted transfer: declared length, failing body. The object
	// must stay absent (atomic PUT): the follow-up read 404s and the
	// session can never verify from it.
	interruptible := presignPUT(t, endpoint, "q/interrupted", 1<<20)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, interruptible.URL, &failingReader{err: errors.New("client gone")})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", domain.AllowedMIME)
	req.ContentLength = 1 << 20
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("interrupted PUT unexpectedly succeeded")
	}
	getInterrupted, err := PresignGET(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/interrupted", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: 1 << 20, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Get(ctx, nil, getInterrupted.URL, 1<<20); err == nil {
		t.Fatal("interrupted object readable")
	} else {
		var status *StatusError
		if !errors.As(err, &status) || status.Status != 404 {
			t.Fatalf("interrupted object = %v, want 404", err)
		}
	}

	// Repeated URL reuse overwrites the key: the second body wins the
	// read. Verify-time claim binding (not transport) decides which
	// bytes promote — pinned in application tests.
	reused := presignPUT(t, endpoint, "q/reused", 1024)
	first := bytes.Repeat([]byte{0x11}, 512)
	second := bytes.Repeat([]byte{0x22}, 512)
	if err := Put(ctx, nil, reused.URL, domain.AllowedMIME, first); err != nil {
		t.Fatal(err)
	}
	if err := Put(ctx, nil, reused.URL, domain.AllowedMIME, second); err != nil {
		t.Fatal(err)
	}
	getReused, err := PresignGET(PresignInput{
		Endpoint: endpoint, Bucket: testBucket, Key: "q/reused", Namespace: QuarantinePrefix,
		ContentType: domain.AllowedMIME, MaxBytes: 1024, TTL: DefaultURLTTL,
	}, testCreds())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, nil, getReused.URL, 1024)
	if err != nil || !bytes.Equal(got, second) {
		t.Fatalf("reuse read binds second body: len=%d err=%v", len(got), err)
	}

	// Concurrent distinct transfers all complete over the protocol.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("q/concurrent-%d", i)
			p := presignPUT(t, endpoint, key, 1<<20)
			body := bytes.Repeat([]byte{byte(i + 1)}, 1<<20)
			if err := Put(ctx, nil, p.URL, domain.AllowedMIME, body); err != nil {
				t.Errorf("concurrent %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
}

type failingReader struct{ err error }

func (r *failingReader) Read([]byte) (int, error) { return 0, r.err }
