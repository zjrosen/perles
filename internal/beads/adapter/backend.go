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
// Returns *task.UnsupportedWorkspaceError for a workspace perles cannot read
// (such as embedded Dolt) and *task.ServerDownError when the workspace's Dolt
// server is unreachable.
func NewBeadsBackend(dataDir, workDir string) (*BeadsBackend, error) {
	client, err := infrabeads.NewClient(dataDir, infrabeads.ResolveOptions{WorkDir: workDir})
	if err != nil {
		return nil, translateClientError(err)
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
			WatcherConfig:     doltWatcherConfig(),
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

// doltWatcherConfig watches bd's .beads/last-touched sentinel, which bd
// rewrites after commands that touch an issue, with a longer debounce to
// coalesce rapid bd commands (e.g., bulk creates/deletes in a loop). The
// watcher watches the parent of DBPath (.beads/dolt), i.e. the .beads dir.
func doltWatcherConfig() task.WatcherConfig {
	return task.WatcherConfig{
		RelevantFiles:    []string{"last-touched"},
		DebounceDuration: 500 * time.Millisecond,
	}
}

// translateClientError maps infrastructure connection errors to the
// task-layer errors the startup screens dispatch on.
func translateClientError(err error) error {
	if ew, ok := errors.AsType[*infrabeads.EmbeddedWorkspaceError](err); ok {
		// Embedded workspaces are not read yet: show the embedded
		// unsupported screen, whose empty Detail renders the default body.
		return &task.UnsupportedWorkspaceError{
			Kind:     task.WorkspaceEmbeddedDolt,
			BeadsDir: ew.BeadsDir,
		}
	}
	if uw, ok := errors.AsType[*infrabeads.UnsupportedWorkspaceError](err); ok {
		return &task.UnsupportedWorkspaceError{
			Kind:     workspaceKind(uw.Kind),
			BeadsDir: uw.BeadsDir,
			Detail:   uw.Detail,
		}
	}
	if su, ok := errors.AsType[*infrabeads.ServerUnreachableError](err); ok {
		return &task.ServerDownError{
			Host:        su.Conn.DisplayHost(),
			Port:        su.Conn.Port,
			Socket:      su.Conn.Socket,
			ManagedByBD: su.Conn.ManagedByBD(),
			Err:         su.Err,
		}
	}
	return fmt.Errorf("beads backend: %w", err)
}

// workspaceKind maps an infrastructure workspace kind to its task-layer kind.
func workspaceKind(kind infrabeads.WorkspaceKind) task.UnsupportedWorkspaceKind {
	switch kind {
	case infrabeads.WorkspaceEmbeddedDolt:
		return task.WorkspaceEmbeddedDolt
	case infrabeads.WorkspaceProxiedServer:
		return task.WorkspaceProxiedServer
	case infrabeads.WorkspaceHTTPBackend:
		return task.WorkspaceHTTPBackend
	case infrabeads.WorkspaceGraph:
		return task.WorkspaceGraph
	case infrabeads.WorkspaceLegacySQLite:
		return task.WorkspaceLegacySQLite
	default:
		return task.WorkspaceUnknownBackend
	}
}
