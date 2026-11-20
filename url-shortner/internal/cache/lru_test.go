package cache

import (
	"testing"
	"time"
)

func TestMemoryLRUBasic(t *testing.T) {
	lru := NewMemoryLRU(3)

	lru.Set("a", "alpha", 1*time.Minute)
	lru.Set("b", "bravo", 1*time.Minute)
	lru.Set("c", "charlie", 1*time.Minute)

	if val, ok := lru.Get("a"); !ok || val != "alpha" {
		t.Fatalf("expected alpha, got %s (ok=%v)", val, ok)
	}

	// Insert 4th element, should evict least recently used ("b", since "a" was accessed above)
	lru.Set("d", "delta", 1*time.Minute)

	if _, ok := lru.Get("b"); ok {
		t.Fatalf("expected 'b' to be evicted, but it was found")
	}

	if val, ok := lru.Get("a"); !ok || val != "alpha" {
		t.Fatalf("expected 'a' to remain, got %s", val)
	}
	if val, ok := lru.Get("d"); !ok || val != "delta" {
		t.Fatalf("expected 'd' to remain, got %s", val)
	}
}

func TestMemoryLRUExpiration(t *testing.T) {
	lru := NewMemoryLRU(5)

	// Short TTL of 50ms
	lru.Set("temp", "val", 50*time.Millisecond)

	if val, ok := lru.Get("temp"); !ok || val != "val" {
		t.Fatalf("expected immediate get to succeed")
	}

	time.Sleep(70 * time.Millisecond)

	if _, ok := lru.Get("temp"); ok {
		t.Fatalf("expected 'temp' to expire, but it was found")
	}
}
