// Package cache provides multi-tier caching abstractions including in-memory LRU,
// distributed Redis caching, and singleflight request coalescing.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"url-shortener/internal/storage"
)

// CacheSource indicates where the URL was retrieved from.
// This is surfaced in HTTP response headers and the frontend educational inspector.
type CacheSource string

const (
	// SourceLocalLRU indicates the key was served from the in-process MemoryLRU (< 0.1ms).
	SourceLocalLRU CacheSource = "LOCAL_LRU"
	// SourceRedis indicates the key was served from the distributed Redis cluster (< 2ms).
	SourceRedis CacheSource = "REDIS"
	// SourceDatabase indicates the single leader goroutine that executed the query against PostgreSQL.
	SourceDatabase CacheSource = "DATABASE"
	// SourceCoalesced indicates requests that waited on singleflight and received the shared result,
	// saving the database from duplicate queries.
	SourceCoalesced CacheSource = "SINGLEFLIGHT_COALESCED"
)

var (
	// ErrURLNotFound indicates that the requested URL does not exist in any layer.
	ErrURLNotFound = errors.New("url not found")
	// ErrURLExpired indicates that the URL mapping has passed its expiration time.
	ErrURLExpired = errors.New("url expired")
)

// DBFallbackFunc defines the signature for querying PostgreSQL on cache misses.
type DBFallbackFunc func(ctx context.Context, shortCode string) (*storage.URL, error)

// TwoTierCache combines process-local LRU, distributed Redis, and singleflight request
// coalescing to eliminate cache stampedes when hot keys expire under load.
type TwoTierCache struct {
	local      *MemoryLRU
	redis      *redis.Client
	group      singleflight.Group
	dbFallback DBFallbackFunc
	localTTL   time.Duration
	redisTTL   time.Duration
}

// NewTwoTierCache constructs a configured TwoTierCache instance.
func NewTwoTierCache(
	rdb *redis.Client,
	dbFallback DBFallbackFunc,
	localCapacity int,
	localTTL time.Duration,
	redisTTL time.Duration,
) *TwoTierCache {
	if localTTL <= 0 {
		localTTL = 5 * time.Second
	}
	if redisTTL <= 0 {
		redisTTL = 24 * time.Hour
	}

	return &TwoTierCache{
		local:      NewMemoryLRU(localCapacity),
		redis:      rdb,
		dbFallback: dbFallback,
		localTTL:   localTTL,
		redisTTL:   redisTTL,
	}
}

func redisKey(shortCode string) string {
	return "url_shortener:url:" + shortCode
}

// Get fetches a URL through the two-tier hierarchy:
// 1. Check in-process LRU (Tier 1: < 0.1ms, zero network hops)
// 2. Check distributed Redis (Tier 2: < 2ms)
// 3. On cache miss: Coalesce concurrent requests via singleflight into ONE PostgreSQL query
func (c *TwoTierCache) Get(ctx context.Context, shortCode string) (*storage.URL, CacheSource, error) {
	// --- Tier 1: Check Local In-Memory LRU ---
	if val, ok := c.local.Get(shortCode); ok {
		var u storage.URL
		if err := json.Unmarshal([]byte(val), &u); err == nil {
			if u.ExpiresAt != nil && time.Now().After(*u.ExpiresAt) {
				c.local.Delete(shortCode)
				return nil, SourceLocalLRU, ErrURLExpired
			}
			return &u, SourceLocalLRU, nil
		}
	}

	// --- Tier 2: Check Distributed Redis ---
	if c.redis != nil {
		rawJSON, err := c.redis.Get(ctx, redisKey(shortCode)).Result()
		if err == nil {
			var u storage.URL
			if jsonErr := json.Unmarshal([]byte(rawJSON), &u); jsonErr == nil {
				if u.ExpiresAt != nil && time.Now().After(*u.ExpiresAt) {
					_ = c.redis.Del(ctx, redisKey(shortCode)).Err()
					return nil, SourceRedis, ErrURLExpired
				}
				c.local.Set(shortCode, rawJSON, c.localTTL)
				return &u, SourceRedis, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			log.Printf("[WARN] Redis read error for %s: %v", shortCode, err)
		}
	}

	// --- Cache Miss: Go Singleflight Coalescing ---
	result, err, shared := c.group.Do(shortCode, func() (interface{}, error) {
		if c.dbFallback == nil {
			return nil, ErrURLNotFound
		}

		u, dbErr := c.dbFallback(ctx, shortCode)
		if dbErr != nil {
			if errors.Is(dbErr, storage.ErrNotFound) {
				return nil, ErrURLNotFound
			}
			return nil, fmt.Errorf("singleflight db fallback: %w", dbErr)
		}

		if u.ExpiresAt != nil && time.Now().After(*u.ExpiresAt) {
			return nil, ErrURLExpired
		}

		data, marshalErr := json.Marshal(u)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal url for cache: %w", marshalErr)
		}

		// Re-populate Tier-1 Local LRU
		c.local.Set(shortCode, string(data), c.localTTL)

		// Re-populate Tier-2 Redis Cache
		if c.redis != nil {
			ttl := c.redisTTL
			if u.ExpiresAt != nil {
				remaining := time.Until(*u.ExpiresAt)
				if remaining < ttl {
					ttl = remaining
				}
			}
			if setErr := c.redis.Set(ctx, redisKey(shortCode), data, ttl).Err(); setErr != nil {
				log.Printf("[WARN] Redis set error for %s: %v", shortCode, setErr)
			}
		}

		return u, nil
	})

	if err != nil {
		return nil, SourceDatabase, err
	}

	urlObj, ok := result.(*storage.URL)
	if !ok {
		return nil, SourceDatabase, errors.New("invalid result type from singleflight")
	}

	if shared {
		return urlObj, SourceCoalesced, nil
	}

	return urlObj, SourceDatabase, nil
}

// Set saves the URL into both Tier-1 (Local LRU) and Tier-2 (Redis).
func (c *TwoTierCache) Set(ctx context.Context, u *storage.URL) error {
	data, err := json.Marshal(u)
	if err != nil {
		return fmt.Errorf("twotier: marshal url: %w", err)
	}

	c.local.Set(u.ShortCode, string(data), c.localTTL)

	if c.redis != nil {
		ttl := c.redisTTL
		if u.ExpiresAt != nil {
			remaining := time.Until(*u.ExpiresAt)
			if remaining < ttl {
				ttl = remaining
			}
		}
		if err := c.redis.Set(ctx, redisKey(u.ShortCode), data, ttl).Err(); err != nil {
			return fmt.Errorf("twotier: set redis: %w", err)
		}
	}

	return nil
}

// Invalidate removes a key from both cache tiers.
func (c *TwoTierCache) Invalidate(ctx context.Context, shortCode string) {
	c.local.Delete(shortCode)
	if c.redis != nil {
		_ = c.redis.Del(ctx, redisKey(shortCode)).Err()
	}
}
