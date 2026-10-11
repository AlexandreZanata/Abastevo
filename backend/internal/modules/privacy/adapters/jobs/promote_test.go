package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// CS10C: the promotion job carries one offered image through the
// fail-closed pipeline. Payloads are identifier-only (references and
// hashes, never bytes); source bytes load through the reader port and
// minimized bytes leave through the writer port, so the queue never
// stores pixels. Loaded bytes hash-bind to the payload before any
// proposal, so a substituted object under the same image reference
// refuses instead of promoting.

func promoteJob(t *testing.T, payload map[string]any) jobs.Job {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return jobs.Job{Kind: "privacy-training-promote", Payload: raw}
}

func validPromotePayload() map[string]any {
	return map[string]any{
		"version": 1, "image_id": "img-1", "contributor_id": "c1",
		"captured_at": "2026-10-11T13:00:00Z", "per_image_allowed": true,
		"consent_revision": 1, "source_sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
	}
}

type fakePromoteFlow struct {
	source   map[string][]byte
	proposed []privacyapp.PromotionDTO
	replayed bool
	err      error
	written  map[string][]byte
}

func (f *fakePromoteFlow) propose(_ context.Context, caller privacyapp.Caller, dto privacyapp.PromotionDTO) (privacyapp.PromotionResult, []byte, error) {
	f.proposed = append(f.proposed, dto)
	if f.err != nil {
		return privacyapp.PromotionResult{}, nil, f.err
	}
	at := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
	sum := sha256.Sum256([]byte("minimized"))
	return privacyapp.PromotionResult{ImageID: dto.ImageID, ContributorID: caller.ContributorID, ConsentRevision: dto.ConsentRevision, DerivativeSHA256: hex.EncodeToString(sum[:]), Replayed: f.replayed, PromotedAt: at, ObjectKey: privacyapp.TrainingObjectKey(dto.ImageID, dto.ConsentRevision, at)}, []byte("minimized"), nil
}

func promoteHandler(f *fakePromoteFlow) Promote {
	return Promote{
		LoadSource: func(_ context.Context, image string) ([]byte, error) {
			raw, ok := f.source[image]
			if !ok {
				return nil, errors.New("quarantine miss")
			}
			return raw, nil
		},
		Propose: f.propose,
		Publish: func(_ context.Context, result privacyapp.PromotionResult, minimized []byte) error {
			f.written[result.ObjectKey] = minimized
			return nil
		},
	}
}

func TestPromoteHandleHappyPath(t *testing.T) {
	f := &fakePromoteFlow{source: map[string][]byte{"img-1": []byte("test")}, written: map[string][]byte{}}
	h := promoteHandler(f)
	if err := h.Handle(context.Background(), promoteJob(t, validPromotePayload())); err != nil {
		t.Fatalf("handle = %v", err)
	}
	if len(f.proposed) != 1 || len(f.written) != 1 {
		t.Errorf("proposed = %d, written keys = %v", len(f.proposed), f.written)
	}
	if got := f.proposed[0]; got.ImageID != "img-1" || !got.PerImageAllowed || string(got.Source) != "test" {
		t.Errorf("dto = %+v", got)
	}
	for key := range f.written {
		if !strings.HasPrefix(key, privacyapp.TrainingNamespace) {
			t.Errorf("output key escapes the training namespace: %q", key)
		}
	}
	// Replays delegate output repair using the original committed identity.
	f.replayed = true
	f.written = map[string][]byte{}
	if err := h.Handle(context.Background(), promoteJob(t, validPromotePayload())); err != nil {
		t.Fatalf("replay handle = %v", err)
	}
	if len(f.written) != 1 {
		t.Errorf("replay skipped repair: %v", f.written)
	}
}

func TestPromoteHandleRejectsMalformedPayloads(t *testing.T) {
	h := Promote{}
	for name, payload := range map[string]map[string]any{
		"empty":        {},
		"bad version":  {"version": 2, "image_id": "img-1"},
		"no image":     {"version": 1, "contributor_id": "c1"},
		"no owner":     {"version": 1, "image_id": "img-1"},
		"bad time":     {"version": 1, "image_id": "img-1", "contributor_id": "c1", "captured_at": "yesterday"},
		"bad revision": {"version": 1, "image_id": "img-1", "contributor_id": "c1", "captured_at": "2026-10-11T13:00:00Z", "consent_revision": 0},
		"no hash":      {"version": 1, "image_id": "img-1", "contributor_id": "c1", "captured_at": "2026-10-11T13:00:00Z", "consent_revision": 1},
	} {
		if err := h.Handle(context.Background(), promoteJob(t, payload)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := h.Handle(context.Background(), jobs.Job{Kind: "privacy-training-promote", Payload: []byte("not json")}); err == nil {
		t.Error("non-JSON payload accepted")
	}
}

func TestPromoteHandleBindsSourceHash(t *testing.T) {
	f := &fakePromoteFlow{source: map[string][]byte{"img-1": []byte("substituted")}, written: map[string][]byte{}}
	h := promoteHandler(f)
	if err := h.Handle(context.Background(), promoteJob(t, validPromotePayload())); err == nil {
		t.Error("substituted source accepted")
	}
	if len(f.proposed) != 0 || len(f.written) != 0 {
		t.Errorf("substituted source proposed=%d written=%v", len(f.proposed), f.written)
	}
}

func TestPromoteHandleFailsClosed(t *testing.T) {
	// Consent withdrawn before the commit refuses without a write.
	f := &fakePromoteFlow{source: map[string][]byte{"img-1": []byte("test")}, written: map[string][]byte{}, err: privacydomain.ErrConsentRequired}
	if err := promoteHandler(f).Handle(context.Background(), promoteJob(t, validPromotePayload())); err == nil {
		t.Error("revoked proposal accepted")
	}
	if len(f.written) != 0 {
		t.Error("refusal wrote output")
	}
	// Source outages error for retry; nothing proposes, nothing writes.
	missing := &fakePromoteFlow{source: map[string][]byte{}, written: map[string][]byte{}}
	if err := promoteHandler(missing).Handle(context.Background(), promoteJob(t, validPromotePayload())); err == nil {
		t.Error("source outage accepted")
	}
	if len(missing.proposed) != 0 {
		t.Error("outage proposed without bytes")
	}
}

func TestPromotionReplayRepairsFailedOutput(t *testing.T) {
	f := &fakePromoteFlow{source: map[string][]byte{"img-1": []byte("test")}, written: map[string][]byte{}}
	h := promoteHandler(f)
	attempts := 0
	h.Publish = func(context.Context, privacyapp.PromotionResult, []byte) error {
		attempts++
		if attempts == 1 {
			return errors.New("storage unavailable")
		}
		return nil
	}
	job := promoteJob(t, validPromotePayload())
	if err := h.Handle(context.Background(), job); err == nil {
		t.Fatal("first write must fail")
	}
	f.replayed = true
	if err := h.Handle(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("inventory replay skipped missing output: attempts=%d", attempts)
	}
}

func TestPromotionRejectsMismatchedCommittedIdentityBeforeOutput(t *testing.T) {
	for name, mutate := range map[string]func(*privacyapp.PromotionResult){
		"revision": func(r *privacyapp.PromotionResult) { r.ConsentRevision++ },
		"owner":    func(r *privacyapp.PromotionResult) { r.ContributorID = "other" },
		"image":    func(r *privacyapp.PromotionResult) { r.ImageID = "other" },
		"key":      func(r *privacyapp.PromotionResult) { r.ObjectKey += "other" },
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakePromoteFlow{source: map[string][]byte{"img-1": []byte("test")}, written: map[string][]byte{}}
			h := promoteHandler(f)
			h.Propose = func(ctx context.Context, c privacyapp.Caller, d privacyapp.PromotionDTO) (privacyapp.PromotionResult, []byte, error) {
				r, b, e := f.propose(ctx, c, d)
				mutate(&r)
				return r, b, e
			}
			if err := h.Handle(context.Background(), promoteJob(t, validPromotePayload())); err == nil {
				t.Fatal("mismatched result accepted")
			}
			if len(f.written) != 0 {
				t.Fatal("mismatched result wrote bytes")
			}
		})
	}
}
