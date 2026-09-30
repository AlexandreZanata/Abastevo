package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Retention bounds (policy, B-BR-M01/M04 as of P15-T04): every
// app-owned copy — quarantine originals and sanitized finals alike —
// dies 24 h after first receipt. Reviewed case extensions no longer
// extend photo bytes (their columns stay as audit history); the
// duplicate-signal hashes survive 90 days under the separate
// hash/privacy review, and sessions stay as the reservation ledger.
// Personal erasure belongs to the rights workflow (P07).
const (
	SessionHardCap = 24 * time.Hour
	// CopyRetention is the single deadline for every app-owned copy:
	// first receipt + 24 h. Retries, processing, copying, disputes
	// and downloads never extend it.
	CopyRetention = 24 * time.Hour
	HashRetention = 90 * 24 * time.Hour
	// DefaultSweepBatch bounds one sweep category per run: retention work
	// is steady-state and resumable, so small batches converge without
	// long transactions.
	DefaultSweepBatch = 100
)

// SweepStore is the retention inventory port: batched selection plus
// convergent markers. State flips stay conditional in SQL; markers are
// set-if-unset so repeated runs converge.
type SweepStore interface {
	ExpireIdleSessions(ctx context.Context, now time.Time, batch int) ([]domain.SessionRef, error)
	ListStuckVerifying(ctx context.Context, cutoff time.Time, batch int) ([]domain.SessionRef, error)
	ExpireStuckSession(ctx context.Context, id string, cutoff time.Time) error
	MarkQuarantineDeleted(ctx context.Context, id string) error
	QuarantineCandidates(ctx context.Context, oldCutoff time.Time, batch int) ([]domain.SessionRef, error)
	StaleFinalCandidates(ctx context.Context, youngCutoff time.Time, batch int) ([]domain.ObjectRef, error)
	OverdueFinals(ctx context.Context, cutoff time.Time, batch int) ([]domain.ObjectRef, error)
	MarkFinalDeleted(ctx context.Context, id string) error
	PurgeObjectsBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// SweepJobs reconciles in-flight validation: a session with a live
// verify job is untouchable, whatever its age.
type SweepJobs interface {
	HasLiveJob(ctx context.Context, sessionID string) (bool, error)
}

// SweepStorage deletes object bytes. Missing keys must succeed (the
// bytes are already gone); failures return errors for counting and
// retry on the next run.
type SweepStorage interface {
	DeleteQuarantine(ctx context.Context, key string) error
	DeleteFinal(ctx context.Context, key string) error
}

// SweepDeps wires one retention pass. Batch must be positive: no silent
// default hides a misconfigured schedule.
type SweepDeps struct {
	Clock   func() time.Time
	Batch   int
	Store   SweepStore
	Jobs    SweepJobs
	Storage SweepStorage
}

// SweepReport makes cleanup observable: every deleted or expired unit
// is counted, storage failures are counted rather than aborting the
// pass, and only database errors fail the run.
type SweepReport struct {
	ExpiredSessions    int
	SkippedActive      int
	QuarantinesDeleted int
	FinalsDeleted      int
	ObjectsPurged      int64
	StorageErrors      int
}

// FinalDue reports whether sanitized bytes must leave storage: past
// first receipt + 24 h. Reviewed extensions no longer move photo
// bytes (M01); their columns stay as audit history only. Pure
// policy, unit-tested.
func FinalDue(obj domain.ObjectRef, now time.Time) bool {
	return !now.Before(obj.ReceivedOrCreated().Add(CopyRetention))
}

// OverdueReport is the failing-policy signal (B-BR-M04): copies past
// their deadline whose bytes may still be present. Any count above
// zero fails the enforcement audit and trips the intake breaker.
type OverdueReport struct {
	Count  int
	Oldest time.Time
}

// AuditOverdue lists the overdue signal bounded oldest-first: the
// count plus the oldest first receipt, so alarms name the lag. Only
// database errors fail; an empty ledger is a healthy zero.
func AuditOverdue(ctx context.Context, store SweepStore, now time.Time, batch int) (OverdueReport, error) {
	var rep OverdueReport
	if batch < 1 {
		return rep, errors.New("evidence: audit batch must be positive")
	}
	rows, err := store.OverdueFinals(ctx, now.Add(-CopyRetention), batch)
	if err != nil {
		return rep, err
	}
	for _, obj := range rows {
		if rep.Count == 0 {
			rep.Oldest = obj.ReceivedOrCreated()
		}
		rep.Count++
	}
	return rep, nil
}

// Sweep runs one retention pass in dependency order: idle expiry first
// (DB truth advances even if storage is down), then stuck validation
// with live-job guard, terminal quarantine cleanup, stale sanitized
// bytes under policy, and finally the 90-day hash purge. Storage
// failures are best-effort with counts; database failures abort the run
// for retry, and every step converges on repeat.
func Sweep(ctx context.Context, deps SweepDeps) (SweepReport, error) {
	var rep SweepReport
	if deps.Batch < 1 {
		return rep, errors.New("evidence: sweep batch must be positive")
	}
	now := time.Now()
	if deps.Clock != nil {
		now = deps.Clock()
	}

	expired, err := deps.Store.ExpireIdleSessions(ctx, now, deps.Batch)
	if err != nil {
		return rep, err
	}
	for _, sess := range expired {
		rep.ExpiredSessions++
		if err := deps.Storage.DeleteQuarantine(ctx, sess.QuarantineKey); err != nil {
			rep.StorageErrors++
			continue
		}
		if err := deps.Store.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
			return rep, err
		}
		rep.QuarantinesDeleted++
	}

	hardCutoff := now.Add(-SessionHardCap)
	stuck, err := deps.Store.ListStuckVerifying(ctx, hardCutoff, deps.Batch)
	if err != nil {
		return rep, err
	}
	for _, sess := range stuck {
		live, err := deps.Jobs.HasLiveJob(ctx, sess.ID)
		if err != nil {
			return rep, err
		}
		if live {
			rep.SkippedActive++
			continue
		}
		if err := deps.Store.ExpireStuckSession(ctx, sess.ID, hardCutoff); err != nil {
			return rep, err
		}
		rep.ExpiredSessions++
		if err := deps.Storage.DeleteQuarantine(ctx, sess.QuarantineKey); err != nil {
			rep.StorageErrors++
			continue
		}
		if err := deps.Store.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
			return rep, err
		}
		rep.QuarantinesDeleted++
	}

	cands, err := deps.Store.QuarantineCandidates(ctx, now.Add(-SessionHardCap), deps.Batch)
	if err != nil {
		return rep, err
	}
	for _, sess := range cands {
		if err := deps.Storage.DeleteQuarantine(ctx, sess.QuarantineKey); err != nil {
			rep.StorageErrors++
			continue
		}
		if err := deps.Store.MarkQuarantineDeleted(ctx, sess.ID); err != nil {
			return rep, err
		}
		rep.QuarantinesDeleted++
	}

	finals, err := deps.Store.StaleFinalCandidates(ctx, now.Add(-CopyRetention), deps.Batch)
	if err != nil {
		return rep, err
	}
	for _, obj := range finals {
		if !FinalDue(obj, now) {
			continue
		}
		if err := deps.Storage.DeleteFinal(ctx, obj.FinalKey); err != nil {
			rep.StorageErrors++
			continue
		}
		if err := deps.Store.MarkFinalDeleted(ctx, obj.ID); err != nil {
			return rep, err
		}
		rep.FinalsDeleted++
	}

	purged, err := deps.Store.PurgeObjectsBefore(ctx, now.Add(-HashRetention))
	if err != nil {
		return rep, err
	}
	rep.ObjectsPurged = purged
	return rep, nil
}
