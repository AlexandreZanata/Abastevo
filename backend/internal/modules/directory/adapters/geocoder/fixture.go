package geocoder

import (
	"context"
	"sync"
	"time"
)

// FixtureProvider serves deterministic candidates keyed by station ID for
// development and tests (D05 selects the live provider later). Unknown IDs
// miss with ErrNotFound. Calls counts invocations so tests prove caching;
// Delay and Err inject latency and failures for timeout coverage.
type FixtureProvider struct {
	Name_     string
	ByStation map[string]Candidate
	Delay     time.Duration
	Err       error

	mu    sync.Mutex
	Calls int
}

// Name identifies the provider in revision attribution.
func (f *FixtureProvider) Name() string {
	if f.Name_ != "" {
		return f.Name_
	}
	return "fixture"
}

// Geocode returns the canned candidate, honors context cancellation, and
// counts every invocation.
func (f *FixtureProvider) Geocode(ctx context.Context, q Query) (Candidate, error) {
	f.mu.Lock()
	f.Calls++
	f.mu.Unlock()
	if f.Err != nil {
		return Candidate{}, f.Err
	}
	if f.Delay > 0 {
		select {
		case <-ctx.Done():
			return Candidate{}, ctx.Err()
		case <-time.After(f.Delay):
		}
	}
	if c, ok := f.ByStation[q.StationID]; ok {
		return c, nil
	}
	return Candidate{}, ErrNotFound
}

// CallCount reports invocations so far.
func (f *FixtureProvider) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Calls
}
