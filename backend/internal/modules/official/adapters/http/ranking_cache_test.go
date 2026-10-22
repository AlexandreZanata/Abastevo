package http

import (
	"fmt"
	"sync"
	"testing"
	"time"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

func cachedResult(items int) application.RankingResult {
	return application.RankingResult{Stations: make([]application.StationObservation, items)}
}

func TestRankingCacheLRUAndTTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cache := NewRankingCache(2, 10, time.Minute, func() time.Time { return now })
	cache.Put("a", cachedResult(1))
	cache.Put("b", cachedResult(1))
	if _, ok := cache.Get("a"); !ok {
		t.Fatal("first entry missing")
	}
	cache.Put("c", cachedResult(1))
	if _, ok := cache.Get("b"); ok {
		t.Fatal("LRU entry survived")
	}
	if cache.Len() != 2 {
		t.Fatalf("entries %d", cache.Len())
	}
	now = now.Add(2 * time.Minute)
	if _, ok := cache.Get("a"); ok {
		t.Fatal("expired entry served")
	}
	if cache.Len() != 1 {
		t.Fatalf("expired entry removal left %d entries", cache.Len())
	}
}

func TestRankingCacheBoundsItemsAndConcurrentAccess(t *testing.T) {
	cache := NewRankingCache(4, 2, time.Minute, time.Now)
	cache.Put("too-large", cachedResult(3))
	if cache.Len() != 0 {
		t.Fatal("oversized result cached")
	}
	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			cache.Put(fmt.Sprintf("k%d", index), cachedResult(1))
			cache.Get("k0")
		}(index)
	}
	wait.Wait()
	if cache.Len() > 4 {
		t.Fatalf("entry bound exceeded: %d", cache.Len())
	}
}

func TestRankingCacheZeroDisables(t *testing.T) {
	cache := NewRankingCache(0, 10, time.Minute, time.Now)
	cache.Put("a", cachedResult(1))
	if _, ok := cache.Get("a"); ok || cache.Len() != 0 {
		t.Fatal("disabled cache retained data")
	}
}
