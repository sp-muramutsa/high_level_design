package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"url-shortener/internal/cache"
	"url-shortener/internal/storage"
)

// mockStore provides an in-memory thread-safe implementation of storage.Store for testing.
type mockStore struct {
	mu          sync.RWMutex
	urls        map[string]*storage.URL
	events      []*storage.ClickEvent
	dbHitCounts int64
	queryDelay  time.Duration
}

func newMockStore(delay time.Duration) *mockStore {
	return &mockStore{
		urls:       make(map[string]*storage.URL),
		queryDelay: delay,
	}
}

func (m *mockStore) SaveURL(ctx context.Context, u *storage.URL) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.urls[u.ShortCode]; exists {
		return storage.ErrAlreadyExists
	}
	m.urls[u.ShortCode] = u
	return nil
}

func (m *mockStore) GetURLByShortCode(ctx context.Context, shortCode string) (*storage.URL, error) {
	atomic.AddInt64(&m.dbHitCounts, 1)
	if m.queryDelay > 0 {
		time.Sleep(m.queryDelay)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.urls[shortCode]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) SaveClickEventsBatch(ctx context.Context, events []*storage.ClickEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	return nil
}

func (m *mockStore) GetStats(ctx context.Context, shortCode string) (*storage.Stats, error) {
	u, err := m.GetURLByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return &storage.Stats{
		ShortCode:   u.ShortCode,
		OriginalURL: u.OriginalURL,
		CreatedAt:   u.CreatedAt,
		TotalClicks: int64(len(m.events)),
	}, nil
}

func (m *mockStore) Close() {}

// mockAllocator returns incrementing IDs in memory
type mockAllocator struct {
	counter uint64
}

func (m *mockAllocator) NextID(ctx context.Context) (uint64, error) {
	val := atomic.AddUint64(&m.counter, 1)
	return val, nil
}

func setupTestRouter(delay time.Duration) (http.Handler, *mockStore) {
	store := newMockStore(delay)
	allocator := &mockAllocator{counter: 100000}
	twoTierCache := cache.NewTwoTierCache(nil, store.GetURLByShortCode, 100, 5*time.Second, 1*time.Hour)
	handler := NewHandler(store, twoTierCache, allocator, nil, "http://localhost:8080")
	return NewRouter(handler, "8080"), store
}

func TestShortenAndRedirect(t *testing.T) {
	router, store := setupTestRouter(0)

	// 1. Shorten a valid URL
	reqBody := `{"url": "https://deepmind.google/technologies/gemini/"}`
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp ShortenResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ShortCode == "" {
		t.Fatal("expected non-empty short code")
	}
	if resp.OriginalURL != "https://deepmind.google/technologies/gemini/" {
		t.Fatalf("unexpected original url: %s", resp.OriginalURL)
	}

	// 2. Perform 302 Found Redirect
	redirectReq := httptest.NewRequest(http.MethodGet, "/"+resp.ShortCode, nil)
	rw := httptest.NewRecorder()
	router.ServeHTTP(rw, redirectReq)

	if rw.Code != http.StatusFound {
		t.Fatalf("expected status 302 Found, got %d", rw.Code)
	}

	location := rw.Header().Get("Location")
	if location != "https://deepmind.google/technologies/gemini/" {
		t.Fatalf("expected Location %s, got %s", "https://deepmind.google/technologies/gemini/", location)
	}

	cacheStatus := rw.Header().Get("X-Cache-Status")
	if cacheStatus == "" {
		t.Fatal("expected X-Cache-Status header to be present")
	}

	_ = store
}

func TestCustomAliasAndConflict(t *testing.T) {
	router, _ := setupTestRouter(0)

	// Create with custom alias "custom-link"
	body := `{"url": "https://example.com", "custom_alias": "custom-link"}`
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	// Try creating duplicate with same alias -> should get 409 Conflict
	dupReq := httptest.NewRequest(http.MethodPost, "/api/shorten", bytes.NewBufferString(body))
	dupW := httptest.NewRecorder()
	router.ServeHTTP(dupW, dupReq)

	if dupW.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for duplicate alias, got %d", dupW.Code)
	}
}

func TestExpiredLinkRedirect(t *testing.T) {
	router, store := setupTestRouter(0)

	// Manually insert an expired link
	pastTime := time.Now().Add(-1 * time.Hour)
	store.SaveURL(context.Background(), &storage.URL{
		ID:          999,
		ShortCode:   "expired123",
		OriginalURL: "https://example.com/expired",
		CreatedAt:   time.Now().Add(-2 * time.Hour),
		ExpiresAt:   &pastTime,
	})

	req := httptest.NewRequest(http.MethodGet, "/expired123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone for expired link, got %d", w.Code)
	}
}

func TestSimulateStampedeEndpoint(t *testing.T) {
	// Setup with 30ms query latency so 50 concurrent goroutines overlap
	router, store := setupTestRouter(30 * time.Millisecond)

	// Insert a valid link into the database
	store.SaveURL(context.Background(), &storage.URL{
		ID:          123,
		ShortCode:   "superbowl",
		OriginalURL: "https://example.com/superbowl",
		CreatedAt:   time.Now(),
	})

	body := `{"short_code": "superbowl", "concurrency": 50}`
	req := httptest.NewRequest(http.MethodPost, "/api/simulate-stampede", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp StampedeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode stampede response: %v", err)
	}

	if resp.TotalRequests != 50 {
		t.Fatalf("expected 50 total requests, got %d", resp.TotalRequests)
	}
	if resp.DatabaseHits != 1 {
		t.Fatalf("expected exactly 1 DB hit under stampede, got %d (breakdown: %v)", resp.DatabaseHits, resp.Breakdown)
	}
	if resp.CacheHits != 49 {
		t.Fatalf("expected 49 cache hits, got %d", resp.CacheHits)
	}
}
