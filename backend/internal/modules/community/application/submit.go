package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
)

// Caller is the server-resolved writer: contributor identity plus the
// attribution token that owns observations.
type Caller struct {
	ContributorID string
	Fingerprint   string
	KeyID         string
	Token         string
}

var (
	ErrUnauthorized   = errors.New("community: authentication required")
	ErrQuotaDenied    = errors.New("community: quota exceeded")
	ErrConflict       = errors.New("community: same key, different body")
	ErrAccountBlocked = errors.New("community: account suspended or deleted")
)

// QuotaDeniedError carries the retry delay for 429 mapping.
type QuotaDeniedError struct {
	RetryAfter time.Duration
}

func (e *QuotaDeniedError) Error() string { return ErrQuotaDenied.Error() }

// Unwrap matches ErrQuotaDenied so handlers map by errors.Is.
func (e *QuotaDeniedError) Unwrap() error { return ErrQuotaDenied }

// Outcome is the executed or replayed safe result.
type Outcome struct {
	StatusCode int
	Body       []byte
	Replayed   bool
}

// IdempotencyKey scopes one operation for replay semantics.
type IdempotencyKey struct {
	ContributorID string
	Method        string
	Route         string
	Key           string
}

// Ports declares every collaborator with stdlib-shaped signatures except
// the opaque transaction handle threaded into EnqueueJob. Port
// implementations map their own errors onto the community sentinels
// (ErrConflict, QuotaDeniedError) so handlers never import other modules.
type Ports struct {
	Clock       func() time.Time
	NewID       func() (string, error)
	Attribution func(ctx context.Context, contributorID string) (string, error)
	CheckQuota  func(ctx context.Context, subject, operation string) (time.Duration, error)
	// CheckAccount refuses writes from contributors bound to a
	// suspended or deleted FREE account (P13-T04D). Unbound
	// contributors keep the anonymous baseline. Nil skips the check
	// (pre-account behavior); the composition root injects the
	// account-owned gate and maps refusals onto ErrAccountBlocked.
	CheckAccount                 func(ctx context.Context, contributorID string) error
	CheckPhotoCapture            func(context.Context, Caller, SubmitDTO) error
	DevelopmentPhotoPreviewUntil time.Time
	CheckDevelopmentStation      func(context.Context, string) error
	Idempotent                   func(ctx context.Context, key IdempotencyKey, body []byte, run func(ctx context.Context) (Outcome, error)) (Outcome, error)
	EnqueueJob                   func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
	Resolve                      func(ctx context.Context, cnpj, display string, address map[string]string) (string, error)
	Store                        Store
	// Locate measures the PostGIS distance from a precise station
	// point to a transient fix and reports the station site. Nil
	// skips the location intake entirely (pre-location behavior);
	// when location evidence is present but ports are missing, the
	// submit fails closed instead of storing unverified bands.
	Locate func(ctx context.Context, stationID string, lat, lon float64) (distanceM float64, site StationSite, err error)
	// LastSite reports the contributor's most recent observation
	// site for teleport review. Nil skips teleport checks.
	LastSite func(ctx context.Context, contributorRef string) (stationID string, receivedAt time.Time, found bool, err error)
	// StationDistance measures PostGIS distance between two
	// stations for teleport review. Nil skips teleport checks.
	StationDistance func(ctx context.Context, a, b string) (distanceM float64, err error)
}

// Store is the owned persistence port.
type Store interface {
	Submit(ctx context.Context, obs domain.Observation, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (string, bool, error)
	Observation(ctx context.Context, id string) (domain.Observation, error)
	Decisions(ctx context.Context, id string) ([]domain.Decision, error)
	ListByContributor(ctx context.Context, ref string, limit int, after time.Time, afterID string, hasCursor bool) ([]domain.Observation, error)
}

// SubmitDTO carries a validated submission body. Amounts arrive parsed;
// handlers reject malformed JSON before this point. Location is
// optional: nil preserves pre-location behavior exactly.
type SubmitDTO struct {
	ClientSubmissionID string
	StationID          string
	Product            string
	Unit               string
	AmountMilli        int64
	RawText            string
	ConditionKind      string
	Qualifier          string
	EvidenceID         string
	PhotoCaptureID     string
	ClaimedCapturedAt  time.Time
	SupersedesID       string
	Location           *LocationEvidence
}

// SubmitResult is the safe acknowledgment: identity plus RECEIVED state,
// never a publication claim.
type SubmitResult struct {
	ObservationID string
	State         string
	ReceivedAt    time.Time
	Replayed      bool
}

// Submit validates, quotas, deduplicates and persists one observation.
func Submit(ctx context.Context, p Ports, caller Caller, method, route, key string, body []byte, dto SubmitDTO) (SubmitResult, error) {
	if caller.ContributorID == "" || caller.Token == "" {
		return SubmitResult{}, ErrUnauthorized
	}
	if strings.TrimSpace(key) == "" {
		return SubmitResult{}, fmt.Errorf("community: idempotency key required")
	}
	// The account gate runs before quota so dead accounts refuse
	// without burning quota windows.
	if p.CheckAccount != nil {
		if err := p.CheckAccount(ctx, caller.ContributorID); err != nil {
			return SubmitResult{}, err
		}
	}
	if _, err := p.CheckQuota(ctx, caller.Fingerprint, "write"); err != nil {
		return SubmitResult{}, err
	}
	outcome, err := p.Idempotent(ctx, IdempotencyKey{
		ContributorID: caller.ContributorID, Method: method, Route: route, Key: key,
	}, body, func(ctx context.Context) (Outcome, error) {
		return execute(ctx, p, caller, dto)
	})
	if err != nil {
		return SubmitResult{}, err
	}
	var result SubmitResult
	if err := json.Unmarshal(outcome.Body, &result); err != nil {
		return SubmitResult{}, err
	}
	result.Replayed = outcome.Replayed
	return result, nil
}

func execute(ctx context.Context, p Ports, caller Caller, dto SubmitDTO) (Outcome, error) {
	now := p.Clock()
	if dto.PhotoCaptureID != "" {
		if dto.EvidenceID == "" || dto.ClaimedCapturedAt.IsZero() || p.CheckPhotoCapture == nil {
			return Outcome{}, ErrPhotoCaptureIneligible
		}
		if err := p.CheckPhotoCapture(ctx, caller, dto); err != nil {
			return Outcome{}, err
		}
	}
	id, err := p.NewID()
	if err != nil {
		return Outcome{}, err
	}
	obs, _, err := domain.NewObservation(domain.Params{
		ID: id, ContributorRef: caller.Token,
		ClientSubmissionID: dto.ClientSubmissionID, StationID: dto.StationID,
		Product: dto.Product, Unit: dto.Unit, AmountMilli: dto.AmountMilli,
		RawText: dto.RawText, ConditionKind: dto.ConditionKind,
		Qualifier: dto.Qualifier, EvidenceID: dto.EvidenceID, PhotoCaptureID: dto.PhotoCaptureID,
		ClaimedCapturedAt: dto.ClaimedCapturedAt, SupersedesID: dto.SupersedesID,
		ReceivedAt: now, PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("community: invalid submission: %w", err)
	}
	obs, err = attachLocation(ctx, p, caller.Token, dto, obs, now)
	if err != nil {
		return Outcome{}, err
	}
	storedID, _, err := p.Store.Submit(ctx, obs, func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
		return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
	})
	if err != nil {
		return Outcome{}, err
	}
	return encodeResult(SubmitResult{ObservationID: storedID, State: domain.StateReceived, ReceivedAt: now})
}

func encodeResult(r SubmitResult) (Outcome, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{StatusCode: 201, Body: raw}, nil
}

// Status returns one owned observation with its decision timeline.
// Missing and foreign observations share one 404 without distinction.
func Status(ctx context.Context, p Ports, caller Caller, id string) (StatusResult, error) {
	if caller.Token == "" {
		return StatusResult{}, ErrUnauthorized
	}
	obs, err := p.Store.Observation(ctx, id)
	if err != nil {
		return StatusResult{}, ErrUnauthorized
	}
	if obs.ContributorRef != caller.Token {
		return StatusResult{}, ErrUnauthorized
	}
	decisions, err := p.Store.Decisions(ctx, id)
	if err != nil {
		return StatusResult{}, err
	}
	state := domain.StateReceived
	for _, d := range decisions {
		state = d.ToState
	}
	return StatusResult{Observation: obs, State: state, Decisions: decisions}, nil
}

// StatusResult carries the fact, its derived state and timeline.
type StatusResult struct {
	Observation domain.Observation
	State       string
	Decisions   []domain.Decision
}

// HistoryItem couples a fact with its derived validation state.
type HistoryItem struct {
	Observation domain.Observation
	State       string
}

// History lists one contributor's facts newest-first with keyset pagination.
func History(ctx context.Context, p Ports, caller Caller, limit int, after time.Time, afterID string, hasCursor bool) ([]HistoryItem, string, error) {
	if caller.Token == "" {
		return nil, "", ErrUnauthorized
	}
	if limit < 1 || limit > 100 {
		return nil, "", fmt.Errorf("community: limit")
	}
	rows, err := p.Store.ListByContributor(ctx, caller.Token, limit+1, after, afterID, hasCursor)
	if err != nil {
		return nil, "", err
	}
	items := make([]HistoryItem, 0, len(rows))
	for _, o := range rows {
		decisions, err := p.Store.Decisions(ctx, o.ID)
		if err != nil {
			return nil, "", err
		}
		state := domain.StateReceived
		for _, d := range decisions {
			state = d.ToState
		}
		items = append(items, HistoryItem{Observation: o, State: state})
	}
	if len(items) <= limit {
		return items, "", nil
	}
	items = items[:limit]
	last := items[len(items)-1].Observation
	// Nano precision round-trips the timestamptz keyset exactly; second
	// precision would strand rows created within the same second.
	return items, last.ReceivedAt.Format(time.RFC3339Nano) + ":" + last.ID, nil
}
