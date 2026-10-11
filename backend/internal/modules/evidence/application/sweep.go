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
// set-if-unset so repeated runs converge. IntakeHealth reports the
// bounded intake posture for monitoring (counts/bytes/timestamp
// only, never content).
type SweepStore interface {
	ExpireIdleSessions(ctx context.Context, now time.Time, batch int) ([]domain.SessionRef, error)
	ListStuckVerifying(ctx context.Context, cutoff time.Time, batch int) ([]domain.SessionRef, error)
	ExpireStuckSession(ctx context.Context, id string, cutoff time.Time) error
	MarkQuarantineDeleted(ctx context.Context, id string) error
	QuarantineCandidates(ctx context.Context, oldCutoff time.Time, batch int) ([]domain.SessionRef, error)
	StaleFinalCandidates(ctx context.Context, youngCutoff time.Time, batch int) ([]domain.ObjectRef, error)
	OverdueFinals(ctx context.Context, cutoff time.Time, batch int) ([]domain.ObjectRef, error)
	// OverdueOriginals lists quarantine originals past their deadline
	// with bytes possibly present (SEC05-A): the originals half of
	// the failing-policy signal. Bounded oldest-first.
	OverdueOriginals(ctx context.Context, cutoff time.Time, batch int) ([]domain.SessionRef, error)
	MarkFinalDeleted(ctx context.Context, id string) error
	PurgeObjectsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	IntakeHealth(ctx context.Context, since, now time.Time) (IntakeHealth, error)
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
// pass, and only database errors fail the run. Health carries the
// intake posture (SEC03-D): reserved/accepted/rejected declared
// buckets plus the oldest incomplete reservation — counts, bytes
// and one timestamp, never keys, hashes or payloads.
type SweepReport struct {
	ExpiredSessions    int
	SkippedActive      int
	QuarantinesDeleted int
	FinalsDeleted      int
	ObjectsPurged      int64
	StorageErrors      int
	Health             IntakeHealth
	// Overdue is the post-pass failing-policy signal (SEC05-B):
	// copies still past deadline after this run's bounded work.
	// Nonzero means the next hourly run must catch up; alarms name
	// the namespace and worst lag. Never fails the pass itself.
	Overdue OverdueReport
}

// IntakeHealth is the intake posture snapshot: live reserved buckets
// (in-flight declared bytes), today decided buckets (READY accepted
// vs REJECTED declared bytes) and the oldest incomplete reservation
// (zero when none in flight). Byte sums are declared-bucket
// accounting, not measured wire payloads.
type IntakeHealth struct {
	ReservedSessions int
	ReservedBytes    int64
	AcceptedToday    int
	AcceptedBytes    int64
	RejectedToday    int
	RejectedBytes    int64
	OldestIncomplete time.Time
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
// Finals and Originals are reported separately so alarms name which
// namespace lags; Oldest* name the worst lag per namespace.
type OverdueReport struct {
	Count          int
	Oldest         time.Time
	Originals      int
	OldestOriginal time.Time
}

// AuditOverdue lists the overdue signal bounded oldest-first: the
// count plus the oldest first receipt, so alarms name the lag. Only
// database errors fail; an empty ledger is a healthy zero.
// SEC05-A: quarantine originals audit alongside finals — an overdue
// original with a failed deletion no longer reports healthy zero.
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
	originals, err := store.OverdueOriginals(ctx, now.Add(-CopyRetention), batch)
	if err != nil {
		return rep, err
	}
	for _, sess := range originals {
		if rep.Originals == 0 {
			rep.OldestOriginal = sess.CreatedAt
		}
		rep.Originals++
	}
	return rep, nil
}

// Unhealthy reports whether the intake breaker must trip: any overdue
// copy, either namespace.
func (r OverdueReport) Unhealthy() bool {
	return r.Count > 0 || r.Originals > 0
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
	// Intake posture last: monitoring-only aggregates that never fail
	// the retention pass (a health-query error degrades to a zero
	// snapshot rather than blocking cleanup).
	if health, herr := deps.Store.IntakeHealth(ctx, dayStart(now), now); herr == nil {
		rep.Health = health
	}
	// Post-pass overdue signal (SEC05-B): bounded re-audit after this
	// run's work, so alarms and the next run's catch-up see the
	// remaining backlog. Errors degrade to a zero signal rather than
	// failing cleanup that already succeeded.
	if overdue, oerr := AuditOverdue(ctx, deps.Store, now, deps.Batch); oerr == nil {
		rep.Overdue = overdue
	}
	return rep, nil
}
