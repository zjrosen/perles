package cachemanager

import (
	"context"
	"time"
)

type ReadThroughCache[K comparable, V any, I any] struct {
	cache           CacheManager[K, V]
	fn              func(ctx context.Context, input I) (V, error)
	shouldSkipCache bool
}

func NewReadThroughCache[K comparable, V any, I any](
	cache CacheManager[K, V],
	fn func(ctx context.Context, input I) (V, error),
	shouldSkipCache bool,
) *ReadThroughCache[K, V, I] {
	return &ReadThroughCache[K, V, I]{
		cache:           cache,
		fn:              fn,
		shouldSkipCache: shouldSkipCache,
	}
}

func (r *ReadThroughCache[K, V, I]) Get(ctx context.Context, key K, input I, ttl time.Duration) (V, error) {
	if r.shouldSkipCache {
		return r.fn(ctx, input)
	}

	if value, ok := r.cache.Get(ctx, key); ok {
		return value, nil
	}

	value, err := r.fn(ctx, input)
	if err != nil {
		return value, err
	}

	r.cache.Set(ctx, key, value, ttl)

	return value, nil
}

func (r *ReadThroughCache[K, V, I]) GetWithRefresh(ctx context.Context, key K, input I, ttl time.Duration) (V, error) {
	if r.shouldSkipCache {
		return r.fn(ctx, input)
	}

	if value, ok := r.cache.GetWithRefresh(ctx, key, ttl); ok {
		return value, nil
	}

	value, err := r.fn(ctx, input)
	if err != nil {
		return value, err
	}

	r.cache.Set(ctx, key, value, ttl)

	return value, nil
}
