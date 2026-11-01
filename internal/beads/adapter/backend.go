package adapter

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/bql"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"
)

// Compile-time check: BeadsBackend implements task.Backend.
var _ task.Backend = (*BeadsBackend)(nil)

// BeadsBackend is a fully-wired beads backend for a server-mode workspace.
// It owns the database client, caches, and all executor adapters.
// Created via NewBeadsBackend; callers use Services() to get the task-layer interfaces.
type BeadsBackend struct {
	client        appbeads.DBClient
	services      task.BackendServices
	bqlCache      cachemanager.Flushable
	depGraphCache cachemanager.Flushable
}

// NewBeadsBackend creates the beads backend for the workspace at dataDir.
// The caller should defer Close() after checking the error.
//
// A server-mode workspace gets a *BeadsBackend: it opens the Dolt server
// connection and creates the caches and executor adapters.
//
// An embedded Dolt workspace has no server, so it gets a *SnapshotBackend,
// which reads it through bd. Constructing one runs no bd beyond the
// `bd context` that classified the workspace: CheckCompatibility (the TUI,
// before the UI starts) builds the first snapshot, and perles daemon, which
// never calls it, builds on its first query.
//
// Returns *task.UnsupportedWorkspaceError for a workspace perles cannot read
// (for an embedded workspace, one whose database or manifest cannot be
// resolved) and *task.ServerDownError when the workspace's Dolt server is
// unreachable.
func NewBeadsBackend(dataDir, workDir string) (task.Backend, error) {
	return newBeadsBackend(dataDir, workDir, backendDeps{})
}

// backendDeps are the bd seams of NewBeadsBackend. Zero fields run the bd on
// PATH; tests inject fakes so that they never run it.
type backendDeps struct {
	// bdContext runs `bd context --json` while the workspace is resolved
	// (infrabeads.ResolveOptions.BDContext).
	bdContext infrabeads.BDContextRunner
	// newBDRunner returns the runner of every bd command of an embedded
	// workspace's backend; nil means infrabeads.NewBDRunner.
	newBDRunner func(workDir, beadsDir string) infrabeads.BDRunner
}

func newBeadsBackend(dataDir, workDir string, deps backendDeps) (task.Backend, error) {
	client, err := infrabeads.NewClient(dataDir, infrabeads.ResolveOptions{WorkDir: workDir, BDContext: deps.bdContext})
	if err != nil {
		if ew, ok := errors.AsType[*infrabeads.EmbeddedWorkspaceError](err); ok {
			return newEmbeddedBackend(ew, workDir, deps)
		}
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

// newEmbeddedBackend returns the SnapshotBackend of the embedded workspace ew
// classified. It resolves the embedded database's manifest and runs no bd. A
// manifest that cannot be resolved is not a construction error but an
// unsupported workspace with the workspace cause, so the startup screen
// explains it.
func newEmbeddedBackend(ew *infrabeads.EmbeddedWorkspaceError, workDir string, deps backendDeps) (task.Backend, error) {
	manifest, err := infrabeads.EmbeddedManifestPath(ew.BeadsDir, ew.Database)
	if err != nil {
		return nil, embeddedStartupError(ew.BeadsDir, err)
	}
	newRunner := deps.newBDRunner
	if newRunner == nil {
		newRunner = infrabeads.NewBDRunner
	}
	log.Info(log.CatBeads, "Reading the embedded Dolt workspace through bd snapshots",
		"beadsDir", ew.BeadsDir, "database", ew.Database, "manifest", manifest,
		"bdVersion", ew.BDVersion, "bdContextErr", ew.BDContextErr)
	b, err := NewSnapshotBackend(SnapshotBackendConfig{
		BeadsDir:     ew.BeadsDir,
		ManifestPath: manifest,
		BDVersion:    ew.BDVersion,
		BDContextErr: ew.BDContextErr,
		Runner:       newRunner(workDir, ew.BeadsDir),
	})
	if err != nil {
		return nil, fmt.Errorf("beads backend: %w", err)
	}
	return b, nil
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
	if uw, ok := errors.AsType[*infrabeads.UnsupportedWorkspaceError](err); ok {
		if uw.Kind == infrabeads.WorkspaceEmbeddedDolt {
			// Defensive, unreachable today: NewClient reports an embedded
			// workspace as *infrabeads.EmbeddedWorkspaceError, which
			// newBeadsBackend routes to the snapshot backend first, and only
			// that backend's bd version gate makes an error of this kind.
			// Should NewClient refuse an embedded workspace itself, the
			// startup screen still gets a cause.
			return embeddedStartupError(uw.BeadsDir, err)
		}
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

// embeddedStartupError maps why perles' snapshot reader cannot start on the
// embedded workspace at beadsDir to the unsupported-workspace error the
// startup screen dispatches on. Its Detail is "<cause>: <text>"
// (task.EmbeddedDetail), and err stays reachable through errors.Is and
// errors.As:
//
//   - the embedded bd version gate's refusal (an
//     *infrabeads.UnsupportedWorkspaceError of kind WorkspaceEmbeddedDolt):
//     bd-version, with the gate's Detail, e.g. "embedded workspaces need
//     bd >= 1.0.5 (found 1.0.4)";
//   - a bd invocation that failed because bd is not on PATH: bd-not-found,
//     with bd's error ("bd export failed: bd CLI not found on PATH");
//   - any other failed bd invocation: bd-error, with bd's error, which leads
//     with perles' hint for a known failure class (e.g. "bd export failed:
//     beads database needs a schema migration: run `bd migrate schema` —
//     ...");
//   - anything else, such as a manifest that cannot be resolved: workspace,
//     with the error's text.
func embeddedStartupError(beadsDir string, err error) *task.UnsupportedWorkspaceError {
	cause, text := embeddedStartupCause(err)
	return &task.UnsupportedWorkspaceError{
		Kind:     task.WorkspaceEmbeddedDolt,
		BeadsDir: beadsDir,
		Detail:   task.EmbeddedDetail(cause, text),
		Err:      err,
	}
}

// embeddedStartupCause classifies err for embeddedStartupError.
func embeddedStartupCause(err error) (task.EmbeddedCause, string) {
	if uw, ok := errors.AsType[*infrabeads.UnsupportedWorkspaceError](err); ok && uw.Kind == infrabeads.WorkspaceEmbeddedDolt {
		return task.EmbeddedCauseBDVersion, uw.Detail
	}
	if infrabeads.IsBDError(err) {
		if errors.Is(err, exec.ErrNotFound) {
			return task.EmbeddedCauseBDNotFound, err.Error()
		}
		return task.EmbeddedCauseBDError, err.Error()
	}
	return task.EmbeddedCauseWorkspace, err.Error()
}

// workspaceKind maps an infrastructure workspace kind to its task-layer kind.
func workspaceKind(kind infrabeads.WorkspaceKind) task.UnsupportedWorkspaceKind {
	switch kind {
	case infrabeads.WorkspaceEmbeddedDolt:
		// Defensive and unreachable: translateClientError, the only caller,
		// maps that kind with embeddedStartupError first. Kept so that the
		// mapping is complete.
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
