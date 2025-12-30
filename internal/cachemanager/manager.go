package cachemanager

import (
	"context"
	"time"
)

type CacheManager[K comparable, V any] interface {
	Get(ctx context.Context, key K) (V, bool)
	GetMultiple(ctx context.Context, keys []K) (map[K]V, bool)
	GetWithRefresh(ctx context.Context, key K, ttl time.Duration) (V, bool)
	Set(ctx context.Context, key K, value V, ttl time.Duration)
	Delete(ctx context.Context, keys ...K) error
	Flush(ctx context.Context) error
}
