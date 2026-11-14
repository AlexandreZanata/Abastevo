package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// Claim is one fenced hold on a job. Lease is the fencing token; every write
// under the claim succeeds only while the same lease still holds the job.
type Claim struct {
	private
	ExportID    string
	Requester   Requester
	Kind        string
	Parameters  []byte
	AsOf        time.Time
	Limits      domain.Limits
	Retention   time.Duration
	Attempts    int
	MaxAttempts int
	Lease       string
	// Provisional is the object recorded by an earlier attempt, if any.
	Provisional *domain.Object
}

// Retirement is what cleanup must do for one job.
type Retirement struct {
	private
	// Object is the stored object still to delete, or nil.
	Object *domain.Object
	// Pending reports a READY job whose expiry has not been reached.
	Pending bool
}

// WorkerStore persists worker transitions. Claim returns ErrBusy while
// another live lease or claim holds the job and ErrSettled for a READY or
// terminal job. Provisional, Publish, Release and Fail are fenced by the
// claim's lease and return ErrBusy when it no longer holds the job.
type WorkerStore interface {
	Claim(context.Context, kernel.OrgScope, string, string, time.Duration) (Claim, error)
	// CheckRequester re-evaluates the recorded requester's rpt.export.run and
	// the kind's declared read permission now; ErrRightsLost when either is
	// gone.
	CheckRequester(context.Context, kernel.OrgScope, Claim, permissions.Permission) error
	Provisional(context.Context, kernel.OrgScope, Claim, domain.Object) error
	Publish(context.Context, kernel.OrgScope, Claim, domain.Object) error
	Release(context.Context, kernel.OrgScope, Claim) error
	Fail(context.Context, kernel.OrgScope, Claim, domain.FailureCode) error
	Retire(context.Context, kernel.OrgScope, string) (Retirement, error)
	MarkDeleted(context.Context, kernel.OrgScope, string, domain.Object) error
}

// DefaultLeaseTTL outlives the time ceiling plus two bounded transfers.
const DefaultLeaseTTL = 5 * time.Minute

// errRetry marks a transient failure the queue retries.
var errRetry = errors.New("orgexport: transient export failure")

// Worker produces and publishes exports (B-BR-EMP-228/229) and retires them
// (B-BR-EMP-230). Build returns nil for every settled outcome and an error
// only when the queue should retry.
type Worker struct {
	Store    WorkerStore
	Formats  Registry
	Objects  Objects
	NewID    func() (string, error)
	LeaseTTL time.Duration
}

func (w Worker) ready() bool {
	return !unavailable(w.Store) && !unavailable(w.Objects) && w.NewID != nil
}

func (w Worker) leaseTTL() time.Duration {
	if w.LeaseTTL > 0 {
		return w.LeaseTTL
	}
	return DefaultLeaseTTL
}

func (w Worker) Build(ctx context.Context, scope kernel.OrgScope, exportID string) error {
	if !w.ready() {
		return ErrUnavailable
	}
	if scope.Validate() != nil || !canonicalID(exportID) {
		return nil // A malformed reference can never become claimable.
	}
	lease, err := w.NewID()
	if err != nil || !canonicalID(lease) {
		return ErrUnavailable
	}
	claim, err := w.Store.Claim(ctx, scope, exportID, lease, w.leaseTTL())
	switch {
	case errorIs(err, ErrSettled, ErrNotFound):
		return nil
	case err != nil:
		return err
	}
	if claim.Lease != lease || claim.ExportID != exportID || claim.Attempts < 1 || claim.Attempts > claim.MaxAttempts {
		return ErrUnavailable
	}
	format, ok := w.Formats.Lookup(claim.Kind)
	if !ok {
		return w.Store.Fail(ctx, scope, claim, domain.FormatUnavailable)
	}
	if err := w.Store.CheckRequester(ctx, scope, claim, format.ReadPermission()); err != nil {
		if errors.Is(err, ErrRightsLost) {
			return w.Store.Fail(ctx, scope, claim, domain.RightsLost)
		}
		return w.retry(ctx, scope, claim, err)
	}
	file, err := w.produce(ctx, scope, claim, format)
	if errors.Is(err, domain.ErrTooLarge) {
		return w.Store.Fail(ctx, scope, claim, domain.TooLarge)
	}
	if err != nil {
		return w.retry(ctx, scope, claim, err)
	}
	object, err := domain.ObjectFor(scope, exportID, format.MediaType(), file)
	if err != nil {
		return w.retry(ctx, scope, claim, err)
	}
	// A different object of an earlier attempt is deleted before another is
	// recorded, so a job never references two files.
	if previous := claim.Provisional; previous != nil && *previous != object {
		if err := w.Objects.Delete(ctx, scope, *previous); err != nil {
			return w.retry(ctx, scope, claim, err)
		}
	}
	if err := w.Store.Provisional(ctx, scope, claim, object); err != nil {
		if errors.Is(err, ErrBusy) {
			return err
		}
		return w.retry(ctx, scope, claim, err)
	}
	claim.Provisional = &object
	if err := w.Objects.Put(ctx, scope, object, file.Bytes); err != nil {
		// A stored copy that does not match is ours to remove: the key
		// belongs to this job, and a retry must be able to store it again.
		if errors.Is(err, domain.ErrIntegrity) {
			if derr := w.Objects.Delete(ctx, scope, object); derr != nil {
				return w.retry(ctx, scope, claim, derr)
			}
		}
		return w.retry(ctx, scope, claim, err)
	}
	if err := w.Store.Publish(ctx, scope, claim, object); err != nil {
		if errors.Is(err, ErrBusy) {
			return err
		}
		return w.retry(ctx, scope, claim, err)
	}
	return nil
}

// produce runs the format under the job's time ceiling. Only the ceiling's
// own deadline is a limit failure; worker shutdown stays retryable.
func (w Worker) produce(ctx context.Context, scope kernel.OrgScope, claim Claim, format Format) (domain.File, error) {
	sink, err := domain.NewSink(claim.Limits)
	if err != nil {
		return domain.File{}, err
	}
	limited, cancel := context.WithTimeout(ctx, claim.Limits.Duration)
	defer cancel()
	err = format.Produce(limited, ProduceInput{Scope: scope, ExportID: claim.ExportID, Requester: claim.Requester, Parameters: append([]byte(nil), claim.Parameters...), AsOf: claim.AsOf}, sink)
	switch {
	case ctx.Err() != nil:
		return domain.File{}, ctx.Err()
	case errors.Is(err, domain.ErrTooLarge), errors.Is(limited.Err(), context.DeadlineExceeded):
		return domain.File{}, domain.ErrTooLarge
	case err != nil:
		return domain.File{}, errRetry
	}
	return sink.Result()
}

// retry releases the lease for a later attempt; the last attempt settles the
// job FAILED instead. Release and Fail use a context that survives shutdown.
func (w Worker) retry(ctx context.Context, scope kernel.OrgScope, claim Claim, cause error) error {
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if claim.Attempts >= claim.MaxAttempts {
		if err := w.Store.Fail(settle, scope, claim, domain.AttemptsExhausted); err != nil {
			return err
		}
		return nil
	}
	if err := w.Store.Release(settle, scope, claim); err != nil {
		return err
	}
	if errorIs(cause, ErrUnavailable, ErrBusy, context.Canceled, context.DeadlineExceeded) {
		return cause
	}
	return errRetry
}

// Cleanup expires a READY job at its expiry and deletes the object of an
// EXPIRED or FAILED job, confirming absence before recording the deletion.
func (w Worker) Cleanup(ctx context.Context, scope kernel.OrgScope, exportID string) error {
	if !w.ready() {
		return ErrUnavailable
	}
	if scope.Validate() != nil || !canonicalID(exportID) {
		return nil
	}
	retirement, err := w.Store.Retire(ctx, scope, exportID)
	switch {
	case errors.Is(err, ErrNotFound):
		return nil
	case err != nil:
		return err
	case retirement.Pending:
		return ErrNotReady
	case retirement.Object == nil:
		return nil
	}
	object := *retirement.Object
	if object.ExportID != exportID || object.Validate(scope) != nil {
		return ErrUnavailable
	}
	if err := w.Objects.Delete(ctx, scope, object); err != nil {
		return ErrUnavailable
	}
	return w.Store.MarkDeleted(ctx, scope, exportID, object)
}
