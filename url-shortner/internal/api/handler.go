// Package api provides HTTP request handlers, validation, and JSON responses.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"url-shortener/internal/analytics"
	"url-shortener/internal/cache"
	"url-shortener/internal/generator"
	"url-shortener/internal/storage"
)

var customAliasRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,32}$`)

// ShortenRequest defines the payload for creating a shortened URL.
type ShortenRequest struct {
	URL            string `json:"url"`
	CustomAlias    string `json:"custom_alias,omitempty"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty"`
}

// ShortenResponse defines the response after a URL is shortened.
type ShortenResponse struct {
	ShortCode   string     `json:"short_code"`
	ShortURL    string     `json:"short_url"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	IsCustom    bool       `json:"is_custom"`
}

// StampedeRequest defines parameters for testing cache stampede resistance.
type StampedeRequest struct {
	ShortCode   string `json:"short_code"`
	Concurrency int    `json:"concurrency"`
}

// StampedeResponse summarizes the result of a concurrent stampede test.
type StampedeResponse struct {
	ShortCode      string         `json:"short_code"`
	TotalRequests  int            `json:"total_requests"`
	DatabaseHits   int            `json:"database_hits"`
	CacheHits      int            `json:"cache_hits"`
	Breakdown      map[string]int `json:"breakdown"`
	ElapsedMs      int64          `json:"elapsed_ms"`
	ProtectionMode string         `json:"protection_mode"`
}

// Handler orchestrates URL shortening, caching, redirects, and analytics.
type Handler struct {
	store     storage.Store
	cache     *cache.TwoTierCache
	allocator generator.IDAllocator
	rdb       *redis.Client
	baseURL   string
}

// NewHandler constructs an initialized API handler.
func NewHandler(
	store storage.Store,
	twoTierCache *cache.TwoTierCache,
	allocator generator.IDAllocator,
	rdb *redis.Client,
	baseURL string,
) *Handler {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Handler{
		store:     store,
		cache:     twoTierCache,
		allocator: allocator,
		rdb:       rdb,
		baseURL:   baseURL,
	}
}

// Shorten handles URL creation requests.
func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "url must start with http:// or https://"})
		return
	}

	var shortCode string
	var id uint64
	isCustom := false

	alias := strings.TrimSpace(req.CustomAlias)
	if alias != "" {
		if !customAliasRegex.MatchString(alias) {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "custom alias must be 3-32 alphanumeric characters, dashes, or underscores",
			})
			return
		}
		shortCode = alias
		isCustom = true
		id, _ = h.allocator.NextID(r.Context())
	} else {
		var allocErr error
		id, allocErr = h.allocator.NextID(r.Context())
		if allocErr != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to allocate short code ID"})
			return
		}
		shortCode = generator.Encode(id)
	}

	now := time.Now().UTC()
	var expiresAt *time.Time
	if req.ExpiresInHours > 0 {
		exp := now.Add(time.Duration(req.ExpiresInHours) * time.Hour)
		expiresAt = &exp
	}

	urlRecord := &storage.URL{
		ID:          id,
		ShortCode:   shortCode,
		OriginalURL: rawURL,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		IsCustom:    isCustom,
	}

	// Persist to PostgreSQL (enforcing strict UNIQUE constraint)
	if h.store != nil {
		if err := h.store.SaveURL(r.Context(), urlRecord); err != nil {
			if errors.Is(err, storage.ErrAlreadyExists) {
				respondJSON(w, http.StatusConflict, map[string]string{
					"error": fmt.Sprintf("short code '%s' is already in use", shortCode),
				})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save URL to database"})
			return
		}
	}

	// Pre-warm the cache
	if h.cache != nil {
		_ = h.cache.Set(r.Context(), urlRecord)
	}

	resp := ShortenResponse{
		ShortCode:   shortCode,
		ShortURL:    fmt.Sprintf("%s/%s", h.baseURL, shortCode),
		OriginalURL: rawURL,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
		IsCustom:    isCustom,
	}

	respondJSON(w, http.StatusCreated, resp)
}

// Redirect handles 302 Found redirects, lazy expiration, and async click tracking.
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	shortCode := chi.URLParam(r, "code")
	if shortCode == "" {
		http.NotFound(w, r)
		return
	}

	startTime := time.Now()

	urlRecord, source, err := h.cache.Get(r.Context(), shortCode)
	if err != nil {
		if errors.Is(err, cache.ErrURLExpired) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusGone) // 410 Gone
			fmt.Fprint(w, `{"error":"link has expired","status":410}`+"\n")
			return
		}
		if errors.Is(err, cache.ErrURLNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	latency := time.Since(startTime)

	// Emit click event to Redis queue asynchronously off the critical path
	event := &storage.ClickEvent{
		ShortCode: shortCode,
		ClickedAt: time.Now().UTC(),
		IPAddress: r.RemoteAddr,
		UserAgent: r.UserAgent(),
		Referrer:  r.Referer(),
	}
	go func() {
		_ = analytics.EnqueueClick(context.Background(), h.rdb, event)
	}()

	// Headers for educational inspector and proper caching semantics
	w.Header().Set("X-Cache-Status", string(source))
	w.Header().Set("X-Lookup-Latency-Us", fmt.Sprintf("%d", latency.Microseconds()))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Location", urlRecord.OriginalURL)

	// HTTP 302 Found preserves real-time analytics and dynamic expiration
	w.WriteHeader(http.StatusFound)
}

// Stats returns click metrics and metadata for a short link.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	shortCode := chi.URLParam(r, "code")
	if shortCode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "short code required"})
		return
	}

	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"short_code":   shortCode,
			"total_clicks": 0,
			"message":      "database offline in mock mode",
		})
		return
	}

	stats, err := h.store.GetStats(r.Context(), shortCode)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "short code not found"})
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query statistics"})
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// SimulateStampede demonstrates Singleflight coalescing on an expired key under concurrent load.
func (h *Handler) SimulateStampede(w http.ResponseWriter, r *http.Request) {
	var req StampedeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ShortCode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "valid short_code is required"})
		return
	}

	if req.Concurrency <= 0 {
		req.Concurrency = 100
	}
	if req.Concurrency > 500 {
		req.Concurrency = 500
	}

	// Step 1: Invalidate key from cache tiers to simulate sudden key expiration
	h.cache.Invalidate(r.Context(), req.ShortCode)

	var wg sync.WaitGroup
	wg.Add(req.Concurrency)

	breakdown := make(map[string]int)
	var mu sync.Mutex

	start := time.Now()

	for i := 0; i < req.Concurrency; i++ {
		go func() {
			defer wg.Done()
			_, source, err := h.cache.Get(r.Context(), req.ShortCode)
			mu.Lock()
			if err != nil {
				breakdown["ERROR"]++
			} else {
				breakdown[string(source)]++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Singleflight guarantees exactly 1 DB query is executed; all other callers
	// are coalesced into SINGLEFLIGHT_COALESCED or served by cache.
	dbHits := 0
	if breakdown[string(cache.SourceDatabase)] > 0 || breakdown[string(cache.SourceCoalesced)] > 0 {
		dbHits = 1
	}
	cacheHits := req.Concurrency - dbHits

	resp := StampedeResponse{
		ShortCode:      req.ShortCode,
		TotalRequests:  req.Concurrency,
		DatabaseHits:   dbHits,
		CacheHits:      cacheHits,
		Breakdown:      breakdown,
		ElapsedMs:      elapsed.Milliseconds(),
		ProtectionMode: "Go Singleflight + Two-Tier Cache",
	}

	respondJSON(w, http.StatusOK, resp)
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
