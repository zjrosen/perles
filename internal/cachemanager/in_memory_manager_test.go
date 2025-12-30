package cachemanager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewInMemoryCacheManager(t *testing.T) {
	require.NotPanics(t, func() {
		NewInMemoryCacheManager[string, string]("test", DefaultExpiration, DefaultCleanupInterval)
	})
}

type ExampleStruct struct {
	ID   int
	Name string
}

func TestNewInMemoryCacheManager_GetExistingValue_StructType(t *testing.T) {
	cache := NewInMemoryCacheManager[string, ExampleStruct]("food-cache", DefaultExpiration, DefaultCleanupInterval)
	example := ExampleStruct{
		Name: "apple",
	}
	cache.Set(context.Background(), "ex:1", example, DefaultExpiration)

	got, ok := cache.Get(context.Background(), "ex:1")
	require.True(t, ok)
	require.Equal(t, example, got)
}

func TestNewInMemoryCacheManager_GetExistingValue(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)
	cache.Set(context.Background(), "food", "apple", DefaultExpiration)

	got, ok := cache.Get(context.Background(), "food")
	require.True(t, ok)
	require.Equal(t, "apple", got)
}

func TestNewInMemoryCacheManager_GetWithNoExistingValue(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	got, ok := cache.Get(context.Background(), "food")
	require.False(t, ok)
	require.Empty(t, got)
}

func TestNewInMemoryCacheManager_GetWithExistingInvalidValueType(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	cache.cache.Set("food", 123, DefaultExpiration)

	got, ok := cache.Get(context.Background(), "food")
	require.False(t, ok)
	require.Empty(t, got)
}

func TestNewInMemoryCacheManager_GetMultipleWithNoKeysDoesNothing(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	got, ok := cache.GetMultiple(context.Background(), []string{})
	require.False(t, ok)
	require.Nil(t, got)
}

func TestNewInMemoryCacheManager_GetMultipleCacheHit(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	cache.cache.Set("food", "apple", DefaultExpiration)
	cache.cache.Set("drink", "juice", DefaultExpiration)

	got, ok := cache.GetMultiple(context.Background(), []string{"food", "drink", "missing"})
	require.True(t, ok)
	require.Equal(t, map[string]string{"food": "apple", "drink": "juice"}, got)
}

func TestNewInMemoryCacheManager_GetMultipleCacheMiss(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	got, ok := cache.GetMultiple(context.Background(), []string{"food", "drink", "missing"})
	require.False(t, ok)
	require.Nil(t, got)
}

func TestNewInMemoryCacheManager_GetMultipleWithExistingInvalidValueType(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	cache.cache.Set("food", "apple", DefaultExpiration)
	cache.cache.Set("drink", 123, DefaultExpiration)

	got, ok := cache.GetMultiple(context.Background(), []string{"food", "drink"})
	require.True(t, ok)
	require.Equal(t, map[string]string{"food": "apple"}, got)
}

func TestNewInMemoryCacheManager_GetWithRefresh_WithNoExistingValue(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	got, ok := cache.GetWithRefresh(context.Background(), "food", time.Minute*60)
	require.False(t, ok)
	require.Equal(t, "", got)
}

func TestNewInMemoryCacheManager_GetWithRefresh_WithExistingValue(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)
	cache.Set(context.Background(), "food", "apple", DefaultExpiration)

	got, ok := cache.GetWithRefresh(context.Background(), "food", time.Minute*60)
	require.True(t, ok)
	require.Equal(t, "apple", got)
}

func TestNewInMemoryCacheManager_DeleteWithNoKeysDoesNothing(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)

	err := cache.Delete(context.Background())
	require.NoError(t, err)
}

func TestNewInMemoryCacheManager_DeleteExistingValue(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)
	cache.Set(context.Background(), "food", "apple", DefaultExpiration)

	got, ok := cache.Get(context.Background(), "food")
	require.True(t, ok)
	require.Equal(t, "apple", got)

	err := cache.Delete(context.Background(), "food")
	require.NoError(t, err)

	got, ok = cache.Get(context.Background(), "food")
	require.False(t, ok)
	require.Equal(t, "", got)
}

func TestNewInMemoryCacheManager_Flush(t *testing.T) {
	cache := NewInMemoryCacheManager[string, string]("food-cache", DefaultExpiration, DefaultCleanupInterval)
	cache.Set(context.Background(), "food", "apple", DefaultExpiration)

	got, ok := cache.Get(context.Background(), "food")
	require.True(t, ok)
	require.Equal(t, "apple", got)

	err := cache.Flush(context.Background())
	require.NoError(t, err)

	got, ok = cache.Get(context.Background(), "food")
	require.False(t, ok)
	require.Equal(t, "", got)
}
