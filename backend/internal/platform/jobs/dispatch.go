package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Handler executes one claimed job. Version gates the payload envelope;
// unknown versions fail safely toward DEAD instead of running blind.
type Handler interface {
	Kind() string
	Version() int
	Handle(ctx context.Context, job Job) error
}

// Dispatcher claims jobs and runs their registered handlers. Unknown kinds
// and version mismatches fail toward DEAD with a reason; handler errors
// retry with backoff until the cap, then park. Cancellation stops between
// jobs; in-flight handlers observe the context.
type Dispatcher struct {
	Queue      *Queue
	Handlers   map[string]Handler
	WorkerID   string
	LeaseTTL   time.Duration
	RetryDelay time.Duration
}

func (d *Dispatcher) leaseTTL() time.Duration {
	if d.LeaseTTL > 0 {
		return d.LeaseTTL
	}
	return 5 * time.Minute
}

func (d *Dispatcher) retryDelay() time.Duration {
	if d.RetryDelay > 0 {
		return d.RetryDelay
	}
	return time.Minute
}

// RunOnce claims and handles a single job of any registered kind,
// reporting whether work was found.
func (d *Dispatcher) RunOnce(ctx context.Context) (bool, error) {
	job, err := d.Queue.Claim(ctx, "", d.WorkerID, d.leaseTTL())
	if err != nil {
		if errors.Is(err, ErrNoJob) {
			return false, nil
		}
		return false, err
	}
	handler, ok := d.Handlers[job.Kind]
	if !ok {
		if _, ferr := d.Queue.Fail(ctx, job.ID, job.LeaseToken, 0, "unknown kind: "+job.Kind); ferr != nil {
			return true, ferr
		}
		return true, nil
	}
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &envelope); err != nil || envelope.Version != handler.Version() {
		if _, ferr := d.Queue.Fail(ctx, job.ID, job.LeaseToken, 0,
			fmt.Sprintf("unsupported payload version for %s", job.Kind)); ferr != nil {
			return true, ferr
		}
		return true, nil
	}
	if err := handler.Handle(ctx, job); err != nil {
		if _, ferr := d.Queue.Fail(ctx, job.ID, job.LeaseToken, d.retryDelay(), err.Error()); ferr != nil {
			return true, ferr
		}
		return true, nil
	}
	if err := d.Queue.Complete(ctx, job.ID, job.LeaseToken); err != nil {
		return true, err
	}
	return true, nil
}

// Run drains the queue until the context cancels.
func (d *Dispatcher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		found, err := d.RunOnce(ctx)
		if err != nil {
			return err
		}
		if !found {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
}
