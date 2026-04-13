// Package webfetch implements a web content fetching tool that retrieves
// URLs and processes the content through an LLM.
package webfetch

import (
	"sync"
	"time"
)

const (
	defaultTTL      = 15 * time.Minute
	defaultMaxItems = 100
)

// entry is a single cached item with an expiration time.
type entry struct {
	content   string
	expiresAt time.Time
}

// Cache is a thread-safe in-memory cache with TTL-based expiration.
// Expired entries are lazily evicted on access. When the cache exceeds
// maxItems, the oldest entry is evicted on Set.
type Cache struct {
	mu       sync.RWMutex
	items    map[string]*entry
	ttl      time.Duration
	maxItems int
}

// NewCache creates a cache with default TTL (15m) and capacity (100).
func NewCache() *Cache {
	return &Cache{
		items:    make(map[string]*entry),
		ttl:      defaultTTL,
		maxItems: defaultMaxItems,
	}
}

// Get retrieves a cached item by key. Returns the content and true if
// found and not expired. Expired entries are lazily deleted.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return "", false
	}
	if time.Now().After(e.expiresAt) {
		// Expired — delete lazily
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return "", false
	}
	return e.content, true
}

// Set stores a key-value pair with the default TTL. If the cache is at
// capacity, the entry with the earliest expiration is evicted.
func (c *Cache) Set(key, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries first
	now := time.Now()
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}

	// If still at capacity, evict the oldest entry
	if len(c.items) >= c.maxItems {
		var oldestKey string
		var oldestTime time.Time
		for k, e := range c.items {
			if oldestKey == "" || e.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = e.expiresAt
			}
		}
		if oldestKey != "" {
			delete(c.items, oldestKey)
		}
	}

	c.items[key] = &entry{
		content:   content,
		expiresAt: now.Add(c.ttl),
	}
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]*entry)
	c.mu.Unlock()
}

// Len returns the number of items in the cache (including expired ones
// not yet evicted).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
