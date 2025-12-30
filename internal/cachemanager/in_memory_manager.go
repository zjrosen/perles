package cachemanager

import (
	"context"
	"time"

	gocache "github.com/patrickmn/go-cache"

	"github.com/zjrosen/perles/internal/log"
)

const DefaultExpiration = 10 * time.Minute
const DefaultCleanupInterval = 30 * time.Minute

// NewInMemoryCacheManager initializes the in-memory cache with a default cleanup interval
func NewInMemoryCacheManager[K ~string, V any](useCase string, defaultExpiration, cleanupInterval time.Duration) *InMemoryCacheManager[K, V] {
	return &InMemoryCacheManager[K, V]{
		useCase: useCase,
		cache:   gocache.New(defaultExpiration, cleanupInterval),
	}
}

// InMemoryCacheManager is the concrete implementation of the CacheManager interface
type InMemoryCacheManager[K ~string, V any] struct {
	useCase string
	cache   *gocache.Cache
}

// Get retrieves an item from the cache by its key
func (c *InMemoryCacheManager[K, V]) Get(ctx context.Context, key string) (V, bool) {
	var zeroValue V

	value, found := c.cache.Get(key)
	if !found {
		return zeroValue, false
	}

	// Type assertion check to ensure the type is correct
	v, ok := value.(V)
	if !ok {
		log.Error(log.CatCache, "wrong type assertion when getting value", "key", key)

		return zeroValue, false
	}

	log.Debug(log.CatCache, "cache hit", "key", key)

	return v, true
}

func (c *InMemoryCacheManager[K, V]) GetMultiple(ctx context.Context, keys []string) (map[string]V, bool) {
	if len(keys) == 0 {
		return nil, false
	}

	isEveryFieldNil := true
	values := make(map[string]V, len(keys))
	missingKeys := make([]string, 0)
	for _, key := range keys {
		value, found := c.cache.Get(key)
		if !found {
			missingKeys = append(missingKeys, key)
			continue
		}

		v, ok := value.(V)
		if !ok {
			log.Error(log.CatCache, "wrong type assertion when getting value", "key", key)
			missingKeys = append(missingKeys, key)
			continue
		}

		isEveryFieldNil = false
		values[key] = v
	}

	if isEveryFieldNil {
		return nil, false
	}
	if len(missingKeys) > 0 {
		log.Error(log.CatCache, "partial cache miss", "keys", keys)
	}

	return values, true
}

// GetWithRefresh retrieves an item from the cache if one is found we extend the ttl
// by putting the item back in the cache
func (c *InMemoryCacheManager[K, V]) GetWithRefresh(ctx context.Context, key string, ttl time.Duration) (V, bool) {
	value, found := c.Get(ctx, key)
	if !found {
		return value, found
	}

	c.Set(ctx, key, value, ttl)

	return value, found
}

// Set sets a value in the cache with a key and TTL
func (c *InMemoryCacheManager[K, V]) Set(ctx context.Context, key string, value V, ttl time.Duration) {
	c.cache.Set(key, value, ttl)
}

// Delete remove a value in the cache by its key
func (c *InMemoryCacheManager[K, V]) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	for _, key := range keys {
		c.cache.Delete(key)
	}

	return nil
}

// Delete remove a value in the cache by its key
func (c *InMemoryCacheManager[K, V]) Flush(ctx context.Context) error {
	c.cache.Flush()

	return nil
}
