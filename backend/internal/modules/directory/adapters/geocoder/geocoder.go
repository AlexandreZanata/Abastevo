package geocoder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// ErrNotFound reports a provider miss. It records an unknown revision,
// never an error to the caller beyond the miss itself.
var ErrNotFound = errors.New("geocoder: no candidate")

// ErrQuotaExceeded refuses provider calls beyond the configured budget.
var ErrQuotaExceeded = errors.New("geocoder: provider quota exceeded")

// Query identifies what to locate. Address carries the source components
// verbatim; nothing is normalized away before the provider sees it.
type Query struct {
	StationID    string
	Address      string
	Municipality string
	State        string
}

// Candidate is one provider answer. Ambiguous covers city-level, centroid
// and multi-match results: all of them record city-centroid at best.
type Candidate struct {
	PointWKT  string
	Ambiguous bool
	MatchInfo string
}

// Provider is the port live geocoders will implement once D05 selects one.
// Until then FixtureProvider serves deterministic answers.
type Provider interface {
	Name() string
	Geocode(ctx context.Context, q Query) (Candidate, error)
}

// Limiter enforces a global minimum interval plus a call quota per window
// across all callers. Zero MinInterval disables spacing; zero MaxCalls
// disables the quota. now is replaceable for deterministic tests.
type Limiter struct {
	MinInterval time.Duration
	MaxCalls    int
	Window      time.Duration
	now         func() time.Time

	mu   sync.Mutex
	last time.Time
	hits []time.Time
}

func (l *Limiter) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// Acquire reserves one provider call or fails without waiting. Callers must
// treat refusal as a normal outcome (skip, keep last reviewed position),
// never as a reason to bypass the limiter.
func (l *Limiter) Acquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock()
	if l.MinInterval > 0 && !l.last.IsZero() && now.Sub(l.last) < l.MinInterval {
		return ErrQuotaExceeded
	}
	if l.MaxCalls > 0 {
		cutoff := now.Add(-l.Window)
		kept := l.hits[:0]
		for _, h := range l.hits {
			if h.After(cutoff) {
				kept = append(kept, h)
			}
		}
		l.hits = kept
		if len(l.hits) >= l.MaxCalls {
			return ErrQuotaExceeded
		}
		l.hits = append(l.hits, now)
	}
	l.last = now
	return nil
}

// Cache memoizes recorded revisions by query hash for a TTL. Hits return
// the stored revision without new provider calls or rows.
type Cache struct {
	TTL time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	revision  domain.LocationRevision
	expiresAt time.Time
}

func (c *Cache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func cacheKey(q Query) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		q.StationID, q.Address, q.Municipality, q.State,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) get(q Query) (domain.LocationRevision, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cacheKey(q)]
	if !ok || !c.clock().Before(e.expiresAt) {
		return domain.LocationRevision{}, false
	}
	return e.revision, true
}

func (c *Cache) put(q Query, rev domain.LocationRevision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cacheEntry{}
	}
	c.entries[cacheKey(q)] = cacheEntry{revision: rev, expiresAt: c.clock().Add(c.TTL)}
}

// Service composes cache, throttle and provider over the directory store.
// Provider output never projects directly: misses record unknown, answers
// record city-centroid, and only an explicit ProjectLocation call (manual
// review) may set the current projection.
type Service struct {
	Provider        Provider
	Store           domain.Store
	Limiter         *Limiter
	Cache           *Cache
	ProviderTimeout time.Duration
}

func (s Service) timeout() time.Duration {
	if s.ProviderTimeout > 0 {
		return s.ProviderTimeout
	}
	return 30 * time.Second
}

// Resolve returns the recorded revision for a query, calling the provider
// at most once per cache TTL and quota window.
func (s Service) Resolve(ctx context.Context, q Query) (domain.LocationRevision, error) {
	if s.Cache != nil {
		if rev, ok := s.Cache.get(q); ok {
			return rev, nil
		}
	}
	if s.Limiter != nil {
		if err := s.Limiter.Acquire(); err != nil {
			return domain.LocationRevision{}, err
		}
	}
	pctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()
	cand, err := s.Provider.Geocode(pctx, q)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.record(ctx, q, domain.LocationRevision{
				StationID: q.StationID,
				Quality:   domain.QualityUnknown,
			})
		}
		return domain.LocationRevision{}, err
	}
	quality := domain.QualityCityCentroid
	point := cand.PointWKT
	if strings.TrimSpace(point) == "" {
		quality = domain.QualityUnknown
	}
	return s.record(ctx, q, domain.LocationRevision{
		StationID:       q.StationID,
		PointWKT:        point,
		Quality:         quality,
		Provider:        s.Provider.Name(),
		SourceReference: cand.MatchInfo,
	})
}

func (s Service) record(ctx context.Context, q Query, rev domain.LocationRevision) (domain.LocationRevision, error) {
	stored, err := s.Store.RecordLocation(ctx, rev)
	if err != nil {
		return domain.LocationRevision{}, err
	}
	if s.Cache != nil {
		s.Cache.put(q, stored)
	}
	return stored, nil
}
