package adapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/bql"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/task"
)

// Compile-time check: BeadsBackend implements task.Backend.
var _ task.Backend = (*BeadsBackend)(nil)

// BeadsBackend is a fully-wired beads backend.
// It owns the database client, caches, and all executor adapters.
// Created via NewBeadsBackend; callers use Services() to get the task-layer interfaces.
type BeadsBackend struct {
	client        appbeads.DBClient
	services      task.BackendServices
	bqlCache      cachemanager.Flushable
	depGraphCache cachemanager.Flushable
}

// NewBeadsBackend creates a fully-wired beads backend.
// It opens the database, creates caches and all executor adapters.
// The caller should defer Close() after checking the error.
//
// Returns *task.ErrServerDown if the backend is Dolt in server mode
// and the server is unreachable.
func NewBeadsBackend(dataDir, workDir string) (*BeadsBackend, error) {
	client, err := infrabeads.NewClient(dataDir)
	if err != nil {
		// Check if embedded mode (exclusive lock, can't share with perles)
		if _, ok := errors.AsType[*infrabeads.EmbeddedModeUnsupportedError](err); ok {
			return nil, &task.EmbeddedModeError{BeadsDir: dataDir}
		}
		// Check if this is a Dolt server that's unreachable
		if meta, metaErr := infrabeads.LoadMetadata(dataDir); metaErr == nil && meta.IsDoltServer() {
			return nil, &task.ServerDownError{
				Host: meta.GetDoltServerHost(),
				Port: meta.GetDoltServerPortWithDir(dataDir),
			}
		}
		return nil, fmt.Errorf("beads backend: %w", err)
	}

	// Create BQL caches (beads-typed for the BQL executor internals)
	bqlCache := cachemanager.NewInMemoryCacheManager[string, []beads.Issue](
		"bql-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)
	depGraphCache := cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph](
		"bql-dep-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)

	// Create task executor. Prefer direct DB comment reads, but fall back to
	// the bd CLI for backends where comments are not exposed as a table.
	beadsExec := infrabeads.NewBDExecutor(workDir, dataDir)
	taskExec := NewBeadsTaskExecutor(
		beadsExec,
		WithCommentReader(client),
		WithFallbackCommentReader(beadsExec),
	)

	// Create query executor (SQL-based via BQL engine)
	bqlExec := bql.NewExecutor(client.DB(), client.Dialect(), bqlCache, depGraphCache)
	queryExec := NewBeadsQueryExecutor(bqlExec)

	// Detect watcher config based on backend type (SQLite vs Dolt)
	watcherCfg := detectWatcherConfig(dataDir)

	return &BeadsBackend{
		client:        client,
		bqlCache:      bqlCache,
		depGraphCache: depGraphCache,
		services: task.BackendServices{
			TaskExecutor:      taskExec,
			QueryExecutor:     queryExec,
			QueryHelpers:      NewBeadsQueryHelpers(),
			SyntaxHighlighter: NewBeadsSyntaxHighlighter(),
			Capabilities:      beadsCapabilities(),
			WatcherConfig:     watcherCfg,
			DBPath:            client.DBPath(),
		},
	}, nil
}

// CheckCompatibility verifies the beads database version is new enough.
// Returns *task.ErrVersionIncompatible if the database needs upgrading.
func (b *BeadsBackend) CheckCompatibility() error {
	currentVersion, err := b.client.Version()
	if err != nil {
		return &task.VersionIncompatibleError{
			Current:  "unknown",
			Required: beads.MinBeadsVersion,
		}
	}
	if err := beads.CheckVersion(currentVersion); err != nil {
		return &task.VersionIncompatibleError{
			Current:  currentVersion,
			Required: beads.MinBeadsVersion,
		}
	}
	return nil
}

// Services returns the task-layer services produced by this backend.
func (b *BeadsBackend) Services() task.BackendServices {
	return b.services
}

// FlushCaches invalidates the BQL and dependency-graph caches so that
// subsequent queries hit the database instead of returning stale results.
func (b *BeadsBackend) FlushCaches(ctx context.Context) error {
	if err := b.bqlCache.Flush(ctx); err != nil {
		return fmt.Errorf("flushing BQL cache: %w", err)
	}
	if err := b.depGraphCache.Flush(ctx); err != nil {
		return fmt.Errorf("flushing dep graph cache: %w", err)
	}
	return nil
}

// Close releases all backend resources.
func (b *BeadsBackend) Close() error {
	return b.client.Close()
}

// beadsCapabilities returns the static capabilities for the beads backend.
func beadsCapabilities() task.BackendCapabilities {
	return task.BackendCapabilities{
		SupportsQuery:        true,
		QueryLanguageName:    "BQL",
		SupportsDependencies: true,
		SupportsTree:         true,
		SupportsComments:     true,
		SupportsLabels:       true,
		SupportsPriority:     true,
		SupportsDesignField:  true,
		SupportsNotesField:   true,
	}
}

// detectWatcherConfig returns file watcher config based on the backend type.
func detectWatcherConfig(dataDir string) task.WatcherConfig {
	meta, err := infrabeads.LoadMetadata(dataDir)
	if err == nil && meta.IsDoltServer() {
		// Dolt server mode: watch last-touched sentinel file with longer debounce
		// to coalesce rapid bd commands (e.g., bulk creates/deletes in a loop).
		return task.WatcherConfig{
			RelevantFiles:    []string{"last-touched"},
			DebounceDuration: 500 * time.Millisecond,
		}
	}

	// SQLite mode: watch database and WAL files
	return task.WatcherConfig{
		RelevantFiles:    []string{"beads.db", "beads.db-wal"},
		DebounceDuration: 100 * time.Millisecond,
	}
}
