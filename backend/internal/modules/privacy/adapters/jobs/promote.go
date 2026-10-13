package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Promote handles one privacy-training-promote job: exactly one
// offered image through the fail-closed pipeline. Loaded bytes
// hash-bind to the payload hash before any proposal, so a
// substituted object under the same image reference refuses instead
// of promoting. Replays converge without a duplicate namespace
// write; refusals and outages error for retry/park per the queue
// convention. Minimized bytes land only through the nonce-fenced publisher.
type Promote struct {
	LoadSource func(ctx context.Context, imageID string) ([]byte, error)
	Propose    func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.PromotionDTO) (privacyapp.PromotionResult, []byte, error)
	Publish    func(ctx context.Context, result privacyapp.PromotionResult, minimized []byte) error
}

// Kind implements jobs.Handler.
func (Promote) Kind() string { return "privacy-training-promote" }

// Version implements jobs.Handler.
func (Promote) Version() int { return 1 }

// Handle parses the versioned envelope and promotes one image.
func (h Promote) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version         int    `json:"version"`
		ImageID         string `json:"image_id"`
		ContributorID   string `json:"contributor_id"`
		CapturedAt      string `json:"captured_at"`
		PerImageAllowed bool   `json:"per_image_allowed"`
		ConsentRevision uint64 `json:"consent_revision"`
		SourceSHA256    string `json:"source_sha256"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("promote: bad payload: %w", err)
	}
	if payload.Version != 1 ||
		strings.TrimSpace(payload.ImageID) == "" ||
		strings.TrimSpace(payload.ContributorID) == "" ||
		strings.TrimSpace(payload.SourceSHA256) == "" ||
		payload.ConsentRevision == 0 {
		return fmt.Errorf("promote: bad payload")
	}
	captured, err := time.Parse(time.RFC3339, payload.CapturedAt)
	if err != nil {
		return fmt.Errorf("promote: bad payload: %w", err)
	}
	if h.LoadSource == nil || h.Propose == nil || h.Publish == nil {
		return fmt.Errorf("promote: promotion pipeline not configured")
	}
	source, err := h.LoadSource(ctx, strings.TrimSpace(payload.ImageID))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(source)
	if hex.EncodeToString(sum[:]) != strings.ToLower(strings.TrimSpace(payload.SourceSHA256)) {
		return fmt.Errorf("promote: source bytes do not match payload hash")
	}
	result, minimized, err := h.Propose(ctx,
		privacyapp.Caller{ContributorID: strings.TrimSpace(payload.ContributorID)},
		privacyapp.PromotionDTO{
			ImageID: strings.TrimSpace(payload.ImageID), CapturedAt: captured, ConsentRevision: payload.ConsentRevision,
			PerImageAllowed: payload.PerImageAllowed, Source: source,
		})
	if err != nil {
		return err
	}
	// Queue revision cannot silently adopt a later grant. Identity/key come
	// exclusively from the committed inventory, including on replay.
	if result.ConsentRevision != payload.ConsentRevision || result.ContributorID != strings.TrimSpace(payload.ContributorID) || result.ImageID != strings.TrimSpace(payload.ImageID) || result.ObjectKey != privacyapp.TrainingObjectKey(result.ImageID, result.ConsentRevision, result.PromotedAt) {
		return fmt.Errorf("promote: output identity mismatch")
	}
	return h.Publish(ctx, result, minimized)
}
