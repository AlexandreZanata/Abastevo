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
	// CleanupMargin reserves retry/queue time; it is not a capacity claim.
	CleanupMargin         = 2 * time.Hour
	SweepTimeout          = 45 * time.Second
	NamespaceSweepTimeout = 20 * time.Second
	CopyDisposalTimeout   = 5 * time.Second
	OriginalCleanupBatch  = 32
	// CopyRetention is the single deadline for every app-owned copy:
	// first receipt + 24 h. Retries, processing, copying, disputes
	// and downloads never extend it.
	CopyRetention = 24 * time.Hour
	HashRetention = 90 * 24 * time.Hour
	// DefaultSweepBatch bounds one sweep category per run: retention work
	// is steady-state and resumable, so small batches converge without
	// long transactions.
	DefaultSweepBatch = 100
	FinalCleanupBatch = 32
)

// SweepStore is the retention inventory port: batched selection plus
// convergent markers. State flips stay conditional in SQL; markers are
// set-if-unset so repeated runs converge. IntakeHealth reports the
// bounded intake posture for monitoring (counts/bytes/timestamp
// only, never content).
type FinalCleanupReport struct {
	Checked  int
	Purged   int
	Skipped  int
	Failures int
}

type SweepStore interface {
	ReconcileFinalOutputs(context.Context, time.Time, int, func(context.Context, string) error) (FinalCleanupReport, error)
	DeleteFinalCopy(context.Context, string, string, time.Time, func(context.Context, string) error) (bool, error)
	CanDeleteQuarantine(context.Context, string) (bool, error)
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
	PurgeObjectsBefore(ctx context.Context, cutoff time.Time) (int64, error)
	IntakeHealth(ctx context.Context, since, now time.Time) (IntakeHealth, error)
}

// SweepJobs is retained for existing internal harnesses. A queued job cannot
// extend retention; actual writer leases guard cleanup. Sweep no longer uses
// the job-state lookup.
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
// is counted. Storage failures allow other cleanup to proceed and fail
// the completed pass for retry; query outages report unknown health. Health carries the
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
	HealthKnown        bool
	OverdueKnown       bool
	Health             IntakeHealth
	// Overdue is the post-pass failing-policy signal (SEC05-B):
	// copies still past deadline after this run's bounded work.
	// Nonzero means enforcement is behind; alarms name the namespace and
	// worst lag. The job handler refuses successful acknowledgement.
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
	batch = min(batch, DefaultSweepBatch)
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

// Sweep runs two independently bounded disposal slices. Expiry closes new
// source admission; one oldest-first scan serves originals. A failed original
// slice does not consume the finals slice. Partial effects survive failures.
func Sweep(ctx context.Context, deps SweepDeps) (SweepReport, error) {
	var rep SweepReport
	if deps.Batch < 1 || deps.Store == nil || deps.Storage == nil {
		return rep, errors.New("evidence: sweep requires a positive batch, store and storage")
	}
	ctx, cancel := context.WithTimeout(ctx, SweepTimeout)
	defer cancel()
	now := time.Now()
	if deps.Clock != nil {
		now = deps.Clock()
	}
	sourceCtx, stopSource := context.WithTimeout(ctx, NamespaceSweepTimeout)
	failures := sweepOriginals(sourceCtx, deps, now, &rep)
	stopSource()
	finalCtx, stopFinal := context.WithTimeout(ctx, NamespaceSweepTimeout)
	failures = errors.Join(failures, sweepFinals(finalCtx, deps, now, &rep))
	stopFinal()
	purged, err := deps.Store.PurgeObjectsBefore(ctx, now.Add(-HashRetention))
	rep.ObjectsPurged = purged
	failures = errors.Join(failures, err)
	// UNKNOWN is explicit; failed observations cannot become healthy zero.
	health, healthErr := deps.Store.IntakeHealth(ctx, dayStart(now), now)
	if healthErr != nil {
		failures = errors.Join(failures, healthErr)
	} else {
		rep.Health = health
		rep.HealthKnown = true
	}
	overdue, auditErr := AuditOverdue(ctx, deps.Store, now, deps.Batch)
	if auditErr != nil {
		failures = errors.Join(failures, auditErr)
	} else {
		rep.Overdue = overdue
		rep.OverdueKnown = true
	}
	if rep.StorageErrors > 0 {
		failures = errors.Join(failures, errors.New("evidence: incomplete storage cleanup"))
	}
	return rep, errors.Join(failures, ctx.Err())
}

func sweepOriginals(ctx context.Context, deps SweepDeps, now time.Time, rep *SweepReport) error {
	batch := min(deps.Batch, OriginalCleanupBatch)
	expired, err := deps.Store.ExpireIdleSessions(ctx, now, batch)
	if err != nil {
		return err
	}
	rep.ExpiredSessions += len(expired)
	cutoff := now.Add(-SessionHardCap + CleanupMargin)
	stuck, err := deps.Store.ListStuckVerifying(ctx, cutoff, batch)
	if err != nil {
		return err
	}
	for _, sess := range stuck {
		// A queued/stalled validator is not byte authority. Expiry and guarded
		// final commit share the session row lock; an admitted PUT stays leased.
		if err := deps.Store.ExpireStuckSession(ctx, sess.ID, cutoff); err != nil {
			return err
		}
		rep.ExpiredSessions++
	}
	cands, err := deps.Store.QuarantineCandidates(ctx, cutoff, batch)
	if err != nil {
		return err
	}
	var failures error
	for _, sess := range cands {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		copyCtx, stop := context.WithTimeout(ctx, CopyDisposalTimeout)
		allowed, err := deps.Store.CanDeleteQuarantine(copyCtx, sess.ID)
		if err != nil {
			stop()
			return errors.Join(failures, err)
		}
		if !allowed {
			stop()
			rep.SkippedActive++
			continue
		}
		err = deps.Storage.DeleteQuarantine(copyCtx, sess.QuarantineKey)
		if err != nil {
			stop()
			rep.StorageErrors++
			failures = errors.Join(failures, err)
			continue
		}
		err = deps.Store.MarkQuarantineDeleted(copyCtx, sess.ID)
		stop()
		if err != nil {
			return errors.Join(failures, err)
		}
		rep.QuarantinesDeleted++
	}
	return errors.Join(failures, ctx.Err())
}

// FinalCleanupDue starts disposal before the hard deadline without moving
// it or changing the separate overdue audit's actual 24h boundary.
func FinalCleanupDue(obj domain.ObjectRef, now time.Time) bool {
	return !now.Before(obj.ReceivedOrCreated().Add(CopyRetention - CleanupMargin))
}

func sweepFinals(ctx context.Context, deps SweepDeps, now time.Time, rep *SweepReport) error {
	batch := min(deps.Batch, FinalCleanupBatch)
	cleanup, failures := deps.Store.ReconcileFinalOutputs(ctx, now, batch, deps.Storage.DeleteFinal)
	rep.FinalsDeleted += cleanup.Purged
	rep.SkippedActive += cleanup.Skipped
	rep.StorageErrors += cleanup.Failures
	remaining := batch - cleanup.Checked
	if remaining <= 0 {
		return errors.Join(failures, ctx.Err())
	}
	finals, err := deps.Store.StaleFinalCandidates(ctx, now.Add(-CopyRetention+CleanupMargin), remaining)
	if err != nil {
		return errors.Join(failures, err)
	}
	for _, obj := range finals {
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		if !FinalCleanupDue(obj, now) {
			continue
		}
		deleted, err := deps.Store.DeleteFinalCopy(ctx, obj.ID, obj.FinalKey, now, func(ctx context.Context, key string) error {
			copyCtx, stop := context.WithTimeout(ctx, CopyDisposalTimeout)
			defer stop()
			return deps.Storage.DeleteFinal(copyCtx, key)
		})
		if err != nil {
			rep.StorageErrors++
			failures = errors.Join(failures, err)
			continue
		}
		if !deleted {
			rep.SkippedActive++
			continue
		}
		rep.FinalsDeleted++
	}
	return errors.Join(failures, ctx.Err())
}
