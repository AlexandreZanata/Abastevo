//go:build integration

package application

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Local S3-compatible service for the verify loop. Small synthetic
// payloads only; the anonymous volume dies with the container.
const (
	protoMinioImage = "bitnamilegacy/minio@sha256:451fe6858cb770cc9d0e77ba811ce287420f781c7c1b806a386f6896471a349c"
	protoMCImage    = "bitnamilegacy/minio-client@sha256:00dcc4e58ada0df45bb7d9ee435af98295f96c27c3c68292ce78ec700a87b511"
	protoUser       = "testadmin"
	protoPass       = "testadmin123456"
	protoBucket     = "sec03c-loop"
)

func startProtoMinIO(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Fatal("docker unavailable for the verify-loop proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "run", "-d", "--rm",
		"-p", "127.0.0.1::9000",
		"--memory", "512m", "--cpus", "2",
		"--tmpfs", "/data:rw,size=64m,mode=1777",
		"--entrypoint", "/opt/bitnami/minio/bin/minio",
		"-e", "MINIO_ROOT_USER="+protoUser,
		"-e", "MINIO_ROOT_PASSWORD="+protoPass,
		protoMinioImage, "server", "/data").CombinedOutput()
	if err != nil {
		t.Fatalf("start minio: %v (%s)", err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = exec.CommandContext(stopCtx, "docker", "stop", id).Run()
	})
	portOut, err := exec.CommandContext(ctx, "docker", "port", id, "9000").CombinedOutput()
	if err != nil {
		t.Fatalf("minio port: %v (%s)", err, portOut)
	}
	port := strings.TrimSpace(string(portOut))
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	endpoint := "http://127.0.0.1:" + port
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/") //nolint:bodyclose
		if err == nil {
			resp.Body.Close()
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	mk := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "host",
		"--entrypoint", "sh", protoMCImage, "-c",
		fmt.Sprintf("mc alias set local %s %s %s && mc mb local/%s",
			endpoint, protoUser, protoPass, protoBucket))
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("make bucket: %v (%s)", err, out)
	}
	return endpoint
}

func protoCreds() storage.Credentials {
	return storage.Credentials{AccessKeyID: protoUser, SecretAccessKey: protoPass}
}

func protoPresignPUT(t *testing.T, endpoint, key, namespace string, maxBytes int64) storage.PresignedPUT {
	t.Helper()
	got, err := storage.PresignPUT(storage.PresignInput{
		Endpoint: endpoint, Bucket: protoBucket, Key: key, Namespace: namespace,
		ContentType: domain.AllowedMIME, MaxBytes: maxBytes, TTL: storage.DefaultURLTTL,
	}, protoCreds())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func protoPresignGET(t *testing.T, endpoint, key, namespace string) storage.PresignedPUT {
	t.Helper()
	got, err := storage.PresignGET(storage.PresignInput{
		Endpoint: endpoint, Bucket: protoBucket, Key: key, Namespace: namespace,
		ContentType: domain.AllowedMIME, MaxBytes: domain.MaxUploadBytes, TTL: storage.DefaultURLTTL,
	}, protoCreds())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// SEC03-C verify loop over the actual protocol (AUD-02): reserve a
// session, PUT bytes through the real presigned flow, then run the
// worker Verify with a real bounded Download and server-side Upload.
// Honest bytes promote READY; oversize/substituted bytes reject with
// stable reasons and upload nothing final.
func TestSEC03CVerifyLoopOnRealS3(t *testing.T) {
	endpoint := startProtoMinIO(t)
	ctx := context.Background()
	raw := loadFixture(t)

	// Honest loop: PUT the fixture, verify from the wire.
	qkey := "q/sec03c-honest"
	put := protoPresignPUT(t, endpoint, qkey, "q/", domain.MaxUploadBytes)
	if err := storage.Put(ctx, nil, put.URL, domain.AllowedMIME, raw); err != nil {
		t.Fatalf("honest PUT: %v", err)
	}
	sess := verifyingSession(t, raw)
	sess.QuarantineKey = qkey
	ports := VerifyPorts{
		Download: func(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
			get := protoPresignGET(t, endpoint, key, "q/")
			return storage.Get(ctx, nil, get.URL, maxBytes)
		},
		Upload: func(ctx context.Context, key, contentType string, body []byte) error {
			up := protoPresignPUT(t, endpoint, key, "f/", domain.MaxUploadBytes)
			return storage.Put(ctx, nil, up.URL, contentType, body)
		},
	}
	out, err := Verify(ctx, ports, sess)
	if err != nil {
		t.Fatalf("honest verify: %v", err)
	}
	if out.Session.Status != domain.StateReady {
		t.Fatalf("status = %q, want READY", out.Session.Status)
	}
	// The sanitized final is really retrievable over the protocol.
	getFinal := protoPresignGET(t, endpoint, out.FinalKey, "f/")
	final, err := storage.Get(ctx, nil, getFinal.URL, domain.MaxUploadBytes)
	if err != nil || len(final) == 0 {
		t.Fatalf("final retrievable: len=%d err=%v", len(final), err)
	}

	// Oversize loop: MaxBytes+1 on the wire rejects oversize with no
	// final upload (declared intent stays small: the bypass shape).
	big := make([]byte, domain.MaxUploadBytes+1)
	copy(big, raw)
	okey := "q/sec03c-oversize"
	oput := protoPresignPUT(t, endpoint, okey, "q/", domain.MaxUploadBytes)
	if err := storage.Put(ctx, nil, oput.URL, domain.AllowedMIME, big); err != nil {
		t.Fatalf("oversize PUT (transport accepts): %v", err)
	}
	osess := verifyingSession(t, raw)
	osess.QuarantineKey = okey
	if _, err := Verify(ctx, ports, osess); err == nil {
		t.Fatal("oversize verify accepted")
	} else if reasons := asRejection(t, err); reasons[0] != ReasonOversize {
		t.Fatalf("reasons = %v, want oversize", reasons)
	}

	// Substituted loop: different valid JPEG bytes disagreeing with
	// the claim reject checksum-mismatch with no final upload.
	// Re-encoding at another quality yields deterministically
	// different but decodable bytes.
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("fixture must decode: %v", err)
	}
	var reencoded bytes.Buffer
	if err := jpeg.Encode(&reencoded, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	sub := reencoded.Bytes()
	if bytes.Equal(sub, raw) {
		t.Fatal("re-encode must differ from the fixture")
	}
	skey := "q/sec03c-substituted"
	sput := protoPresignPUT(t, endpoint, skey, "q/", domain.MaxUploadBytes)
	if err := storage.Put(ctx, nil, sput.URL, domain.AllowedMIME, sub); err != nil {
		t.Fatalf("substituted PUT: %v", err)
	}
	ssess := verifyingSession(t, raw)
	ssess.QuarantineKey = skey
	if _, err := Verify(ctx, ports, ssess); err == nil {
		t.Fatal("substituted verify accepted")
	} else if reasons := asRejection(t, err); reasons[0] != ReasonChecksumMismatch {
		t.Fatalf("reasons = %v, want checksum-mismatch", reasons)
	}
}
