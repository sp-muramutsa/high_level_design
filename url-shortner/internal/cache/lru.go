// Package cache provides multi-tier caching abstractions including in-memory LRU,
// distributed Redis caching, and singleflight request coalescing.
package cache

import (
	"container/list"
	"sync"
	"time"
)

type lruItem struct {
	key       string
	value     string
	expiresAt time.Time
}

// MemoryLRU provides a thread-safe, in-process LRU cache with per-item TTL expiration.
// It serves as Tier-1 cache to absorb extreme traffic spikes (e.g., 50,000 req/s)
// without creating network hops to Redis.
type MemoryLRU struct {
	mu        sync.RWMutex
	capacity  int
	items     map[string]*list.Element
	evictList *list.List
}

// NewMemoryLRU creates an in-memory LRU cache with the specified capacity.
func NewMemoryLRU(capacity int) *MemoryLRU {
	if capacity <= 0 {
		capacity = 5000
	}
	return &MemoryLRU{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
	}
}

// Get retrieves a value from the local cache. If the item has expired,
// it is removed and (false) is returned.
func (c *MemoryLRU) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.items[key]
	if !exists {
		return "", false
	}

	item := elem.Value.(*lruItem)
	if time.Now().After(item.expiresAt) {
		c.removeElement(elem)
		return "", false
	}

	c.evictList.MoveToFront(elem)
	return item.value, true
}

// Set stores a key-value pair in the local cache with a time-to-live.
func (c *MemoryLRU) Set(key, value string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt := time.Now().Add(ttl)

	// If key exists, update value and expiration, move to front
	if elem, exists := c.items[key]; exists {
		c.evictList.MoveToFront(elem)
		item := elem.Value.(*lruItem)
		item.value = value
		item.expiresAt = expiresAt
		return
	}

	// Evict oldest item if at capacity
	if c.evictList.Len() >= c.capacity {
		c.evictOldest()
	}

	item := &lruItem{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	elem := c.evictList.PushFront(item)
	c.items[key] = elem
}

// Delete removes a key from the local cache.
func (c *MemoryLRU) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
	}
}

// Len returns the current count of cached items.
func (c *MemoryLRU) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *MemoryLRU) evictOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

func (c *MemoryLRU) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	item := elem.Value.(*lruItem)
	delete(c.items, item.key)
}
