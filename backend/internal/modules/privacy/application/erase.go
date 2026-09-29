package application

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// EraseStore is the owned persistence port for erasure: deletion
// intents, the replay ledger, archive purges and guarded outcome
// writes.
type EraseStore interface {
	RequestDeletion(ctx context.Context, r domain.Request, reason string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (id string, replayed bool, err error)
	Get(ctx context.Context, id string) (domain.Request, error)
	CompleteDeletionRequest(ctx context.Context, id string, at time.Time) error
	FailExport(ctx context.Context, id string, at time.Time) error
	RecordLedger(ctx context.Context, e domain.LedgerEntry) (string, bool, error)
	ListLedger(ctx context.Context, contributorID string) ([]domain.LedgerEntry, error)
	MarkLedgerReplayed(ctx context.Context, id string, at time.Time) error
	PurgeOwnerArchives(ctx context.Context, contributorID string) (int64, error)
}

// IdentityErasure revokes one owner's writes and unlinks identity.
type IdentityErasure interface {
	DeleteContributor(ctx context.Context, contributorID string) (deleted bool, keysRevoked int64, err error)
}

// CommunityErasure unlinks one owner's community footprint and
// recomputes affected price keys.
type CommunityErasure interface {
	EraseContributor(ctx context.Context, ref, anon string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (unlinkedObs, unlinkedVotes int64, keys int, err error)
}

// EvidenceErasure purges one owner's evidence footprint.
type EvidenceErasure interface {
	EraseContributor(ctx context.Context, ref string, deleteKey func(ctx context.Context, key, namespace string) error) (purgedSessions, purgedObjects int64, err error)
}

// TrustErasure removes one owner's trust footprint.
type TrustErasure interface {
	EraseContributor(ctx context.Context, ref, anon string) (int64, error)
}

// ErasePorts wires erasure across the owned modules. Every scope port
// converges on re-run so crashes and restore replays resume safely.
type ErasePorts struct {
	Clock        func() time.Time
	NewID        func() (string, error)
	Attribution  func(ctx context.Context, contributorID string) (string, error)
	Store        EraseStore
	Identity     IdentityErasure
	Community    CommunityErasure
	Evidence     EvidenceErasure
	Trust        TrustErasure
	DeleteObject func(ctx context.Context, key, namespace string) error
	EnqueueJob   func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// RequestDeletionResult is the safe acknowledgment for an erasure
// intent: the request identity and whether the call converged.
type RequestDeletionResult struct {
	RequestID string
	Replayed  bool
}

// RequestDeletion validates and persists one erasure intent with its
// durable erasure job. Execution happens in the worker; this call only
// records intent. Same contributor/key/body converges; divergent
// payloads conflict (B-BR-005).
func RequestDeletion(ctx context.Context, p ErasePorts, contributorID string, dto EraseDTO) (RequestDeletionResult, error) {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" {
		return RequestDeletionResult{}, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" || strings.TrimSpace(dto.Reason) == "" {
		return RequestDeletionResult{}, domain.ErrInvalidRequest
	}
	token, err := p.Attribution(ctx, contributorID)
	if err != nil {
		return RequestDeletionResult{}, err
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return RequestDeletionResult{}, err
	}
	req, err := domain.NewRequest(domain.RequestParams{
		ID: id, ContributorID: contributorID, ContributorRef: token,
		ClientSubmissionID: dto.ClientSubmissionID, Type: domain.TypeDeletion,
		RequestedAt: now,
	})
	if err != nil {
		return RequestDeletionResult{}, err
	}
	storedID, replayed, err := p.Store.RequestDeletion(ctx, req, strings.TrimSpace(dto.Reason),
		func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		})
	if err != nil {
		return RequestDeletionResult{}, err
	}
	return RequestDeletionResult{RequestID: storedID, Replayed: replayed}, nil
}

// EraseDTO carries one erasure intent: the reviewed reason plus a
// client operation identity for safe retries.
type EraseDTO struct {
	ClientSubmissionID string
	Reason             string
}

// ErasureReport is the auditable receipt: the deletion request plus
// per-scope counts.
type ErasureReport struct {
	RequestID       string
	Replayed        bool
	IdentityRevoked bool
	KeysRevoked     int64
	ObsUnlinked     int64
	VotesUnlinked   int64
	KeysRecomputed  int
	SessionsPurged  int64
	ObjectsPurged   int64
	TrustUnlinked   int64
	ArchivesPurged  int64
}

// Erase revokes one owner's writes and purges/unlinks its data scope
// by scope (ADR-008, B-BR-016): identity first so no new writes land
// mid-erasure, then community, evidence, trust and export archives.
// Each scope records its ledger row after success; a failure marks the
// request FAILED for audited retry, and completed scopes converge on
// re-run. Price facts stay for history with anonymized references;
// private payload and links go.
func Erase(ctx context.Context, p ErasePorts, contributorID string, dto EraseDTO) (ErasureReport, error) {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" {
		return ErasureReport{}, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" {
		return ErasureReport{}, domain.ErrInvalidRequest
	}
	if strings.TrimSpace(dto.Reason) == "" {
		return ErasureReport{}, domain.ErrInvalidRequest
	}
	token, err := p.Attribution(ctx, contributorID)
	if err != nil {
		return ErasureReport{}, err
	}
	if strings.TrimSpace(token) == "" {
		return ErasureReport{}, domain.ErrNotFound
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return ErasureReport{}, err
	}
	req, err := domain.NewRequest(domain.RequestParams{
		ID: id, ContributorID: contributorID, ContributorRef: token,
		ClientSubmissionID: dto.ClientSubmissionID, Type: domain.TypeDeletion,
		RequestedAt: now,
	})
	if err != nil {
		return ErasureReport{}, err
	}
	storedID, replayed, err := p.Store.RequestDeletion(ctx, req, strings.TrimSpace(dto.Reason),
		func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		})
	if err != nil {
		return ErasureReport{}, err
	}
	if replayed {
		existing, err := p.Store.Get(ctx, storedID)
		if err != nil {
			return ErasureReport{}, err
		}
		if existing.Status != domain.StatusRequested {
			return ErasureReport{RequestID: storedID, Replayed: true}, nil
		}
	}
	anon := "erased-" + strings.ReplaceAll(storedID, "-", "")
	report, err := runScopes(ctx, p, contributorID, token, anon, strings.TrimSpace(dto.Reason), now)
	if err != nil {
		_ = p.Store.FailExport(ctx, storedID, now)
		return ErasureReport{}, err
	}
	current, err := p.Store.Get(ctx, storedID)
	if err != nil {
		return ErasureReport{}, err
	}
	completed, _, err := domain.CompleteDeletion(current, now)
	if err != nil {
		return ErasureReport{}, err
	}
	if err := p.Store.CompleteDeletionRequest(ctx, completed.ID, now); err != nil {
		return ErasureReport{}, err
	}
	report.RequestID = storedID
	return report, nil
}

// runScopes executes every erasure scope in ledger order and records
// one ledger row per completed scope.
func runScopes(ctx context.Context, p ErasePorts, contributorID, token, anon, reason string, now time.Time) (ErasureReport, error) {
	var report ErasureReport
	ledger := func(scope string) error {
		id, err := p.NewID()
		if err != nil {
			return err
		}
		entry, err := domain.NewLedgerEntry(domain.LedgerParams{
			ID: id, ContributorID: contributorID, ContributorRef: token,
			Scope: scope, Reason: reason, OccurredAt: now,
		})
		if err != nil {
			return err
		}
		_, _, err = p.Store.RecordLedger(ctx, entry)
		return err
	}
	deleted, keys, err := p.Identity.DeleteContributor(ctx, contributorID)
	if err != nil {
		return report, err
	}
	report.IdentityRevoked, report.KeysRevoked = deleted, keys
	if err := ledger(domain.ScopeIdentity); err != nil {
		return report, err
	}
	obs, votes, nkeys, err := p.Community.EraseContributor(ctx, token, anon,
		func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		})
	if err != nil {
		return report, err
	}
	report.ObsUnlinked, report.VotesUnlinked, report.KeysRecomputed = obs, votes, nkeys
	if err := ledger(domain.ScopeCommunity); err != nil {
		return report, err
	}
	sess, objs, err := p.Evidence.EraseContributor(ctx, token, p.DeleteObject)
	if err != nil {
		return report, err
	}
	report.SessionsPurged, report.ObjectsPurged = sess, objs
	if err := ledger(domain.ScopeEvidence); err != nil {
		return report, err
	}
	unlinked, err := p.Trust.EraseContributor(ctx, token, anon)
	if err != nil {
		return report, err
	}
	report.TrustUnlinked = unlinked
	if err := ledger(domain.ScopeTrust); err != nil {
		return report, err
	}
	purged, err := p.Store.PurgeOwnerArchives(ctx, contributorID)
	if err != nil {
		return report, err
	}
	report.ArchivesPurged = purged
	if err := ledger(domain.ScopeExports); err != nil {
		return report, err
	}
	return report, nil
}

// ReplayLedger reapplies one contributor's recorded removals after a
// restore (B-BR-016): every ledger scope re-runs against the restored
// rows, then stamps replayed_at. Replays converge: scopes with no
// matching rows report zero and still stamp, so a second replay is a
// safe no-op.
func ReplayLedger(ctx context.Context, p ErasePorts, contributorID string) (ErasureReport, error) {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" {
		return ErasureReport{}, ErrUnauthorized
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	entries, err := p.Store.ListLedger(ctx, contributorID)
	if err != nil {
		return ErasureReport{}, err
	}
	if len(entries) == 0 {
		return ErasureReport{}, domain.ErrNotFound
	}
	token := entries[0].ContributorRef
	anon := "erased-replay-" + strings.ReplaceAll(entries[0].ID, "-", "")
	var report ErasureReport
	scoped := map[string]bool{}
	for _, e := range entries {
		scoped[e.Scope] = true
	}
	run := func(scope string, fn func() error) error {
		if !scoped[scope] {
			return nil
		}
		if err := fn(); err != nil {
			return err
		}
		for _, e := range entries {
			if e.Scope == scope {
				_ = p.Store.MarkLedgerReplayed(ctx, e.ID, now)
			}
		}
		return nil
	}
	if err := run(domain.ScopeIdentity, func() error {
		deleted, keys, err := p.Identity.DeleteContributor(ctx, contributorID)
		report.IdentityRevoked, report.KeysRevoked = deleted, keys
		return err
	}); err != nil {
		return report, err
	}
	if err := run(domain.ScopeCommunity, func() error {
		obs, votes, nkeys, err := p.Community.EraseContributor(ctx, token, anon,
			func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
				return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
			})
		report.ObsUnlinked, report.VotesUnlinked, report.KeysRecomputed = obs, votes, nkeys
		return err
	}); err != nil {
		return report, err
	}
	if err := run(domain.ScopeEvidence, func() error {
		sess, objs, err := p.Evidence.EraseContributor(ctx, token, p.DeleteObject)
		report.SessionsPurged, report.ObjectsPurged = sess, objs
		return err
	}); err != nil {
		return report, err
	}
	if err := run(domain.ScopeTrust, func() error {
		unlinked, err := p.Trust.EraseContributor(ctx, token, anon)
		report.TrustUnlinked = unlinked
		return err
	}); err != nil {
		return report, err
	}
	if err := run(domain.ScopeExports, func() error {
		purged, err := p.Store.PurgeOwnerArchives(ctx, contributorID)
		report.ArchivesPurged = purged
		return err
	}); err != nil {
		return report, err
	}
	return report, nil
}
