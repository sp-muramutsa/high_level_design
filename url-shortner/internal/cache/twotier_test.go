package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"url-shortener/internal/storage"
)

// TestCacheStampedePrevention simulates a "celebrity viral link" scenario:
// 100 concurrent goroutines simultaneously request an expired/uncached key.
// The mock database query has an artificial 50ms delay.
// Without singleflight, 100 queries would hammer the database.
// With singleflight, exactly 1 query executes against the database.
func TestCacheStampedePrevention(t *testing.T) {
	var dbQueryCount int64

	mockDBFallback := func(ctx context.Context, shortCode string) (*storage.URL, error) {
		// Increment the DB hit counter
		atomic.AddInt64(&dbQueryCount, 1)

		// Simulate slow DB query under load
		time.Sleep(50 * time.Millisecond)

		return &storage.URL{
			ID:          42,
			ShortCode:   shortCode,
			OriginalURL: "https://example.com/viral-superbowl-link",
			CreatedAt:   time.Now(),
		}, nil
	}

	// Create TwoTierCache with nil Redis (forcing DB fallback through singleflight)
	cache := NewTwoTierCache(nil, mockDBFallback, 1000, 5*time.Second, 1*time.Hour)

	const concurrentClients = 100
	var wg sync.WaitGroup
	wg.Add(concurrentClients)

	ctx := context.Background()
	results := make([]*storage.URL, concurrentClients)
	sources := make([]CacheSource, concurrentClients)
	errorsList := make([]error, concurrentClients)

	// Fire all 100 requests concurrently
	for i := 0; i < concurrentClients; i++ {
		idx := i
		go func() {
			defer wg.Done()
			u, src, err := cache.Get(ctx, "superbowl")
			results[idx] = u
			sources[idx] = src
			errorsList[idx] = err
		}()
	}

	wg.Wait()

	// 1. Verify every single client received the URL with zero errors
	for i := 0; i < concurrentClients; i++ {
		if errorsList[i] != nil {
			t.Fatalf("client %d got error: %v", i, errorsList[i])
		}
		if results[i] == nil || results[i].OriginalURL != "https://example.com/viral-superbowl-link" {
			t.Fatalf("client %d got invalid url: %v", i, results[i])
		}
		if sources[i] != SourceDatabase && sources[i] != SourceCoalesced {
			t.Fatalf("client %d expected SourceDatabase or SourceCoalesced, got %s", i, sources[i])
		}
	}

	// 2. CRUCIAL CHECK: Database must have been queried EXACTLY once!
	finalDBHits := atomic.LoadInt64(&dbQueryCount)
	if finalDBHits != 1 {
		t.Fatalf("CACHE STAMPEDE OCCURRED: database was hit %d times! Want exactly 1", finalDBHits)
	}

	t.Logf("SUCCESS: 100 concurrent requests resolved with exactly %d DB hit!", finalDBHits)

	// 3. Verify that subsequent requests are now served from Tier-1 Local LRU (0 DB hits)
	u, src, err := cache.Get(ctx, "superbowl")
	if err != nil {
		t.Fatalf("subsequent get failed: %v", err)
	}
	if src != SourceLocalLRU {
		t.Fatalf("expected SourceLocalLRU, got %s", src)
	}
	if u.OriginalURL != "https://example.com/viral-superbowl-link" {
		t.Fatalf("unexpected url: %s", u.OriginalURL)
	}
	if atomic.LoadInt64(&dbQueryCount) != 1 {
		t.Fatalf("DB was hit on subsequent request; cache was not populated")
	}
}
