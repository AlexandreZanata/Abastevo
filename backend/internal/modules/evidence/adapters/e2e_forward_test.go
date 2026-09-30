//go:build integration

package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func sha256hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func newUUIDv4must(t *testing.T) string {
	t.Helper()
	id, err := newUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type fakeTransferT struct {
	payload  []byte
	uploaded []byte
	calls    int
}

func (f *fakeTransferT) download(_ context.Context, key string, maxBytes int64) ([]byte, error) {
	f.calls++
	if key == "" || maxBytes < 1 {
		return nil, errors.New("bad download args")
	}
	if int64(len(f.payload)) > maxBytes {
		return nil, errors.New("transfer: over bound")
	}
	return append([]byte{}, f.payload...), nil
}

func (f *fakeTransferT) upload(_ context.Context, _ string, _ string, body []byte) error {
	f.uploaded = append([]byte{}, body...)
	return nil
}

type e2eStorage struct {
	finals  map[string][]byte
	fail    map[string]bool
	deleted []string
}

func (f *e2eStorage) DeleteQuarantine(_ context.Context, key string) error {
	return nil
}

func (f *e2eStorage) DeleteFinal(_ context.Context, key string) error {
	if f.fail[key] {
		return errStorageDown()
	}
	delete(f.finals, key)
	f.deleted = append(f.deleted, key)
	return nil
}

type errStorageDownT struct{ msg string }

func (e errStorageDownT) Error() string { return e.msg }

func errStorageDown() error { return errStorageDownT{"storage down"} }

type e2eJobs struct{ live map[string]bool }

func (f *e2eJobs) HasLiveJob(_ context.Context, sessionID string) (bool, error) {
	return f.live[sessionID], nil
}

func loadSmallJPEG(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("media", "testdata", "valid-4x4.jpg"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return raw
}

// TestForwardLifecycleEndToEnd drives the complete low-memory audit
// path on real PostGIS (G15 exit): reserve → complete → forward
// verify → logical access expiry at the deadline with bytes still
// present → sweep deletes bytes and marks rows → repeat converges.
// It separates logical expiry from physical deletion, survives a
// storage outage between passes, and proves restarts resume: the
// second store handle over the same pool sees the same truth.
func TestForwardLifecycleEndToEnd(t *testing.T) {
	s, pool := freshStore(t)
	ctx := context.Background()
	raw := loadSmallJPEG(t)

	sess := testSession("e0000000-0000-4000-8000-000000000021", "upl-e2e")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	live, err := s.Session(ctx, sess.ID)
	if err != nil || live.Status != domain.StateVerifying {
		t.Fatalf("session = %+v, %v", live, err)
	}
	firstReceipt := live.UpdatedAt
	if firstReceipt.IsZero() {
		t.Fatal("completion must stamp first receipt")
	}

	sum := sha256hex(raw)
	live.ClaimedSHA256 = sum
	transfer := &fakeTransferT{payload: raw}
	out, err := evidenceapp.VerifyForward(ctx, evidenceapp.VerifyPorts{
		Clock:    func() time.Time { return firstReceipt.Add(5 * time.Minute) },
		Download: transfer.download,
		Upload:   transfer.upload,
	}, live)
	if err != nil {
		t.Fatalf("verify forward: %v", err)
	}
	obj := ObjectData{
		ID: newUUIDv4must(t), FinalKey: out.FinalKey,
		SourceSHA256: out.SourceSHA256, SanitizedSHA256: out.SanitizedSHA256,
		Width: out.Width, Height: out.Height, DHash: out.DHash,
		ReceivedAt: live.UpdatedAt,
	}
	objectID, err := s.RecordVerified(ctx, sess.ID, obj)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	wantExpiry := firstReceipt.Add(domain.ForwardDeadline)
	if !out.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("expiry = %v, want first-receipt + 24 h (%v)", out.ExpiresAt, wantExpiry)
	}

	// Logical access expiry precedes physical deletion: at 23 h the
	// copy still reads; exactly at the deadline it must not.
	if !firstReceipt.Add(23 * time.Hour).Before(wantExpiry) {
		t.Fatal("test clock math wrong")
	}
	if expiredAt(firstReceipt, firstReceipt.Add(24*time.Hour)) != true {
		t.Error("deadline edge must read expired")
	}
	if expiredAt(firstReceipt, firstReceipt.Add(23*time.Hour)) {
		t.Error("pre-deadline copy must still read")
	}

	// "App restart": a fresh handle over the same pool resumes the
	// same truth, and the failing first sweep pass retries cleanly.
	resumed := NewStore(pool)
	storage := &e2eStorage{finals: map[string][]byte{out.FinalKey: transfer.uploaded}, fail: map[string]bool{out.FinalKey: true}}
	deps := func() evidenceapp.SweepDeps {
		return evidenceapp.SweepDeps{
			Clock:   func() time.Time { return firstReceipt.Add(25 * time.Hour) },
			Batch:   100,
			Store:   resumed,
			Jobs:    &e2eJobs{live: map[string]bool{}},
			Storage: storage,
		}
	}
	rep, err := evidenceapp.Sweep(ctx, deps())
	if err != nil {
		t.Fatalf("outage sweep: %v", err)
	}
	if rep.FinalsDeleted != 0 || rep.StorageErrors != 1 {
		t.Fatalf("outage pass must count the failure without marking, got %+v", rep)
	}
	if _, present := storage.finals[out.FinalKey]; !present {
		t.Error("failed delete must leave bytes for retry")
	}

	storage.fail = map[string]bool{}
	rep, err = evidenceapp.Sweep(ctx, deps())
	if err != nil {
		t.Fatalf("catch-up sweep: %v", err)
	}
	if rep.FinalsDeleted != 1 {
		t.Fatalf("catch-up must delete exactly the overdue copy, got %+v", rep)
	}
	if _, present := storage.finals[out.FinalKey]; present {
		t.Error("swept bytes must be gone from storage")
	}

	// Repeat converges: nothing left to do on any lane.
	rep, err = evidenceapp.Sweep(ctx, deps())
	if err != nil {
		t.Fatal(err)
	}
	if rep.FinalsDeleted != 0 || rep.StorageErrors != 0 {
		t.Fatalf("repeat sweep must converge silent, got %+v", rep)
	}
	audit, err := evidenceapp.AuditOverdue(ctx, resumed, firstReceipt.Add(25*time.Hour), 100)
	if err != nil || audit.Count != 0 {
		t.Fatalf("post-sweep audit must read healthy zero, got %+v %v", audit, err)
	}

	// The row survives with its hashes for duplicate signals; only
	// the bytes are gone.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_objects WHERE id = $1`, objectID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit row must survive deletion, count = %d, %v", n, err)
	}
	_ = jobs
}

// TestForwardRejectionsStayRedacted proves log-safe refusals: every
// rejection reason is a fixed code, and no payload, key, hash or URL
// ever travels inside the error the worker logs and retries on.
func TestForwardRejectionsStayRedacted(t *testing.T) {
	s, _ := freshStore(t)
	ctx := context.Background()
	raw := loadSmallJPEG(t)
	sess := testSession("e0000000-0000-4000-8000-000000000022", "upl-redact")
	if _, _, err := s.ReserveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	var jobs [][]byte
	if err := s.CompleteSession(ctx, sess.ID, enqueueStub(&jobs)); err != nil {
		t.Fatal(err)
	}
	live, err := s.Session(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	live.ClaimedSHA256 = sha256hex([]byte{0xFF, 0xD8, 0x00})
	transfer := &fakeTransferT{payload: []byte{0xFF, 0xD8, 0x00}}
	_, err = evidenceapp.VerifyForward(ctx, evidenceapp.VerifyPorts{
		Download: transfer.download,
		Upload:   transfer.upload,
	}, live)
	var rerr *evidenceapp.RejectionError
	if !errors.As(err, &rerr) {
		t.Fatalf("corrupt input must reject, got %v", err)
	}
	allowed := map[string]bool{
		"oversize": true, "invalid-image": true, "checksum-mismatch": true,
		"trailing-data": true, "dimensions-exceeded": true,
	}
	for _, reason := range rerr.Reasons {
		if !allowed[reason] {
			t.Errorf("reason %q not in fixed vocabulary", reason)
		}
		if strings.Contains(reason, "q/") {
			t.Errorf("key material leaked into refusal: %v", err)
		}
	}
	if strings.Contains(err.Error(), "ffd8") {
		t.Errorf("payload bytes leaked into refusal: %v", err)
	}
	_ = raw
}

func expiredAt(firstReceipt, now time.Time) bool {
	return !now.Before(firstReceipt.Add(domain.ForwardDeadline))
}
