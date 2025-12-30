package cachemanager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
)

type wrappedInput struct {
	Id int
}

func TestReadThroughCache_Get_WithCacheDisabled(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		true,
	)

	examples, err := readThroughCache.Get(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID: 1,
		},
	}, examples)
}

func TestReadThroughCache_GetWithRefresh_WithCacheDisabled(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		true,
	)

	examples, err := readThroughCache.GetWithRefresh(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID: 1,
		},
	}, examples)
}

func TestReadThroughCache_Get_WithValueInCache(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().Get(mock.Anything, "key").Return([]*ExampleStruct{
		{
			ID:   1,
			Name: "Example",
		},
	}, true)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		false,
	)

	examples, err := readThroughCache.Get(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID:   1,
			Name: "Example",
		},
	}, examples)
}

func TestReadThroughCache_Get_EmptyCache(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().Get(mock.Anything, "key").Return([]*ExampleStruct{}, false)
	managerMock.EXPECT().Set(mock.Anything, "key", []*ExampleStruct{
		{
			ID: 1,
		},
	}, mock.Anything).Return()

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		false,
	)

	examples, err := readThroughCache.Get(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID: 1,
		},
	}, examples)
}

func TestReadThroughCache_Get_DatabaseError(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().Get(mock.Anything, "key").Return([]*ExampleStruct{}, false)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return nil, errors.New("failed to get data")
		},
		false,
	)

	_, err := readThroughCache.Get(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.Error(t, err)
}

func TestReadThroughCache_GetWithRefresh_WithValueInCache(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().GetWithRefresh(mock.Anything, "key", mock.Anything).Return([]*ExampleStruct{
		{
			ID:   1,
			Name: "Example",
		},
	}, true)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		false,
	)

	examples, err := readThroughCache.GetWithRefresh(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID:   1,
			Name: "Example",
		},
	}, examples)
}

func TestReadThroughCache_GetWithRefresh_EmptyCache(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().GetWithRefresh(mock.Anything, "key", mock.Anything).Return([]*ExampleStruct{}, false)
	managerMock.EXPECT().Set(mock.Anything, "key", []*ExampleStruct{
		{
			ID: 1,
		},
	}, mock.Anything).Return()

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return []*ExampleStruct{
				{
					ID: input.Id,
				},
			}, nil
		},
		false,
	)

	examples, err := readThroughCache.GetWithRefresh(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.NoError(t, err)
	require.Equal(t, []*ExampleStruct{
		{
			ID: 1,
		},
	}, examples)
}

func TestReadThroughCache_GetWithRefresh_DatabaseError(t *testing.T) {
	managerMock := mocks.NewMockCacheManager[string, []*ExampleStruct](t)
	managerMock.EXPECT().GetWithRefresh(mock.Anything, "key", mock.Anything).Return([]*ExampleStruct{}, false)

	readThroughCache := NewReadThroughCache[string, []*ExampleStruct, wrappedInput](
		managerMock,
		func(ctx context.Context, input wrappedInput) ([]*ExampleStruct, error) {
			return nil, errors.New("failed to get data")
		},
		false,
	)

	_, err := readThroughCache.GetWithRefresh(
		context.Background(),
		"key",
		wrappedInput{
			Id: 1,
		},
		time.Minute)
	require.Error(t, err)
}
