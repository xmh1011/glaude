package webfetch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_SetGet(t *testing.T) {
	c := NewCache()

	c.Set("https://example.com", "hello world")
	content, ok := c.Get("https://example.com")
	require.True(t, ok)
	assert.Equal(t, "hello world", content)
}

func TestCache_Miss(t *testing.T) {
	c := NewCache()

	_, ok := c.Get("https://not-cached.com")
	assert.False(t, ok)
}

func TestCache_Overwrite(t *testing.T) {
	c := NewCache()

	c.Set("https://example.com", "v1")
	c.Set("https://example.com", "v2")
	content, ok := c.Get("https://example.com")
	require.True(t, ok)
	assert.Equal(t, "v2", content)
}

func TestCache_Expiry(t *testing.T) {
	c := &Cache{
		items:    make(map[string]*entry),
		ttl:      1 * time.Millisecond,
		maxItems: 100,
	}

	c.Set("https://example.com", "short-lived")
	time.Sleep(5 * time.Millisecond)
	_, ok := c.Get("https://example.com")
	assert.False(t, ok, "expired entry should not be returned")
}

func TestCache_Eviction(t *testing.T) {
	c := &Cache{
		items:    make(map[string]*entry),
		ttl:      defaultTTL,
		maxItems: 2,
	}

	c.Set("a", "1")
	c.Set("b", "2")
	c.Set("c", "3") // should evict "a" (oldest)

	_, ok := c.Get("a")
	assert.False(t, ok, "oldest entry should be evicted")

	_, ok = c.Get("c")
	assert.True(t, ok, "newest entry should exist")
}

func TestCache_Clear(t *testing.T) {
	c := NewCache()

	c.Set("a", "1")
	c.Set("b", "2")
	assert.Equal(t, 2, c.Len())

	c.Clear()
	assert.Equal(t, 0, c.Len())
}
