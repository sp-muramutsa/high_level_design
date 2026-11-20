package generator

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// IDAllocator defines the interface for generating unique 64-bit integer IDs.
type IDAllocator interface {
	NextID(ctx context.Context) (uint64, error)
}

// RedisRangeAllocator manages a local in-memory range of IDs claimed in chunks from Redis.
// This guarantees zero cross-server collisions with zero database lookups per URL generation.
type RedisRangeAllocator struct {
	client    *redis.Client
	key       string
	chunkSize uint64

	mu      sync.Mutex
	current uint64
	max     uint64
}

// NewRedisRangeAllocator creates a new allocator that claims blocks of chunkSize from Redis.
func NewRedisRangeAllocator(client *redis.Client, key string, chunkSize uint64) *RedisRangeAllocator {
	if chunkSize == 0 {
		chunkSize = 1000
	}
	if key == "" {
		key = "url_shortener:global_id_counter"
	}
	return &RedisRangeAllocator{
		client:    client,
		key:       key,
		chunkSize: chunkSize,
	}
}

// NextID returns the next globally unique 64-bit integer ID.
// Most calls execute in < 1 microsecond using in-memory local state.
// Only when the local chunk is exhausted does it perform an atomic Redis INCRBY.
func (a *RedisRangeAllocator) NextID(ctx context.Context) (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// If current local range is exhausted, allocate the next chunk from Redis
	if a.current >= a.max {
		newMax, err := a.client.IncrBy(ctx, a.key, int64(a.chunkSize)).Result()
		if err != nil {
			return 0, fmt.Errorf("failed to allocate ID range from Redis: %w", err)
		}

		a.max = uint64(newMax)
		a.current = a.max - a.chunkSize + 1
	}

	id := a.current
	a.current++
	return id, nil
}
