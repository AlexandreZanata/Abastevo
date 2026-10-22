package http

import (
	"container/list"
	"sync"
	"time"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

// RankingCache is a bounded in-process cache for assembled ranking pages.
// Keys include the resolved source revision and every filter, so a revision
// switch cannot serve an old page. Entries also expire by TTL; the resolver
// still runs on every request and an outage is never answered from here.
type RankingCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	order      *list.List
	maxEntries int
	maxItems   int
	ttl        time.Duration
	now        func() time.Time
	items      int
}

type rankingCacheEntry struct {
	key     string
	value   application.RankingResult
	expires time.Time
	items   int
}

// NewRankingCache bounds entries and total cached items. maxEntries <= 0
// disables caching (the documented uncached recovery).
func NewRankingCache(maxEntries, maxItems int, ttl time.Duration, now func() time.Time) *RankingCache {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	if maxItems <= 0 {
		maxItems = 10_000
	}
	return &RankingCache{
		entries:    map[string]*list.Element{},
		order:      list.New(),
		maxEntries: maxEntries,
		maxItems:   maxItems,
		ttl:        ttl,
		now:        now,
	}
}

func (c *RankingCache) Get(key string) (application.RankingResult, bool) {
	if c == nil || c.maxEntries <= 0 {
		return application.RankingResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.entries[key]
	if !ok {
		return application.RankingResult{}, false
	}
	entry := element.Value.(*rankingCacheEntry)
	if !c.now().Before(entry.expires) {
		c.remove(element)
		return application.RankingResult{}, false
	}
	c.order.MoveToFront(element)
	return entry.value, true
}

func (c *RankingCache) Put(key string, value application.RankingResult) {
	if c == nil || c.maxEntries <= 0 {
		return
	}
	items := len(value.Stations) + len(value.Areas)
	if items > c.maxItems {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element, ok := c.entries[key]; ok {
		c.remove(element)
	}
	element := c.order.PushFront(&rankingCacheEntry{
		key: key, value: value, expires: c.now().Add(c.ttl), items: items,
	})
	c.entries[key] = element
	c.items += items
	for (len(c.entries) > c.maxEntries || c.items > c.maxItems) && c.order.Len() > 0 {
		c.remove(c.order.Back())
	}
}

// Len reports the current number of retained entries.
func (c *RankingCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *RankingCache) remove(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*rankingCacheEntry)
	c.order.Remove(element)
	delete(c.entries, entry.key)
	c.items -= entry.items
}
