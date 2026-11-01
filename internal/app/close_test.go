package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mode/kanban"
	"github.com/zjrosen/perles/internal/task"
)

// stubKanbanClose makes closing the kanban controller return err until the
// test ends, and returns the number of times it has been closed.
func stubKanbanClose(t *testing.T, err error) (calls func() int) {
	t.Helper()
	n := 0
	orig := closeKanban
	closeKanban = func(*kanban.Model) error {
		n++
		return err
	}
	t.Cleanup(func() { closeKanban = orig })
	return func() int { return n }
}

func TestApp_Close_RunsEveryCleanupStep(t *testing.T) {
	kanbanErr := errors.New("kanban close failed")

	for _, tc := range []struct {
		name      string
		notifier  bool  // backend publishes Changes() instead of a file watcher
		kanbanErr error // nil: kanban closes cleanly
	}{
		{name: "change notifier", notifier: true},
		{name: "change notifier, kanban fails", notifier: true, kanbanErr: kanbanErr},
		{name: "file watcher"},
		{name: "file watcher, kanban fails", kanbanErr: kanbanErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kanbanCloses := stubKanbanClose(t, tc.kanbanErr)

			// Tracing is shut down after the watcher steps, so a released
			// provider shows Close got past them.
			cfg := config.Defaults()
			cfg.Orchestration.Tracing.Enabled = true
			cfg.Orchestration.Tracing.Exporter = "file"
			cfg.Orchestration.Tracing.FilePath = filepath.Join(t.TempDir(), "traces.jsonl")

			var backend task.Backend
			var notifier *notifyingBackend
			if tc.notifier {
				notifier = newNotifyingBackend(task.BackendServices{})
				backend = notifier
			} else {
				backend = &servicesBackend{services: task.BackendServices{
					DBPath:        filepath.Join(t.TempDir(), "last-touched"),
					WatcherConfig: task.WatcherConfig{RelevantFiles: []string{"last-touched"}},
				}}
			}

			model, err := NewWithConfig(AppConfig{Cfg: cfg, Backend: backend, WorkDir: t.TempDir()})
			require.NoError(t, err)
			// Closes the model if a check fails before the Close below; Close
			// is idempotent, and the test checks its error there.
			t.Cleanup(func() { _ = model.Close() })
			require.NotNil(t, model.watcherCtx)
			require.NotNil(t, model.tracingProvider)
			if tc.notifier {
				require.Equal(t, 1, notifier.changes.SubscriberCount())
			} else {
				require.NotNil(t, model.watcherHandle)
			}

			err = model.Close()
			if tc.kanbanErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.kanbanErr)
			}
			require.Equal(t, 1, kanbanCloses())

			require.ErrorIs(t, model.watcherCtx.Err(), context.Canceled,
				"Close should cancel the change listener")
			if tc.notifier {
				require.Eventually(t, func() bool { return notifier.changes.SubscriberCount() == 0 },
					time.Second, 10*time.Millisecond, "Close should drop the backend subscription")
			} else {
				// A stopped watcher has closed its broker, so a new
				// subscription comes back closed.
				select {
				case _, open := <-model.watcherHandle.Broker().Subscribe(t.Context()):
					require.False(t, open, "Close should stop the file watcher")
				default:
					t.Fatal("Close should stop the file watcher: its broker still accepts subscriptions")
				}
			}
			require.Nil(t, model.tracingProvider, "Close should shut down the tracing provider")
		})
	}
}

// TestApp_Close_Idempotent: a second Close, on the model or on a copy made
// before the first (Bubble Tea copies the model on every Update), runs no
// cleanup step again (the file watcher's Stop would panic) and returns the
// first call's error.
func TestApp_Close_Idempotent(t *testing.T) {
	kanbanErr := errors.New("kanban close failed")

	for _, tc := range []struct {
		name      string
		notifier  bool  // backend publishes Changes() instead of a file watcher
		kanbanErr error // nil: kanban closes cleanly
	}{
		{name: "change notifier", notifier: true},
		{name: "change notifier, kanban fails", notifier: true, kanbanErr: kanbanErr},
		{name: "file watcher"},
		{name: "file watcher, kanban fails", kanbanErr: kanbanErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kanbanCloses := stubKanbanClose(t, tc.kanbanErr)

			var backend task.Backend
			if tc.notifier {
				backend = newNotifyingBackend(task.BackendServices{})
			} else {
				backend = &servicesBackend{services: task.BackendServices{
					DBPath:        filepath.Join(t.TempDir(), "last-touched"),
					WatcherConfig: task.WatcherConfig{RelevantFiles: []string{"last-touched"}},
				}}
			}
			model, err := NewWithConfig(AppConfig{Cfg: config.Defaults(), Backend: backend, WorkDir: t.TempDir()})
			require.NoError(t, err)
			t.Cleanup(func() { _ = model.Close() })
			require.Equal(t, tc.notifier, model.watcherHandle == nil, "the app owns a file watcher unless the backend notifies")
			copied := model

			first := model.Close()
			if tc.kanbanErr == nil {
				require.NoError(t, first)
			} else {
				require.ErrorIs(t, first, tc.kanbanErr)
			}
			require.NotPanics(t, func() { require.Equal(t, first, model.Close(), "a second Close") })
			require.NotPanics(t, func() { require.Equal(t, first, copied.Close(), "Close on a copy") })
			require.Equal(t, 1, kanbanCloses(), "the cleanup ran once")
		})
	}
}

// TestApp_Close_ZeroModel: a Model not built by NewWithConfig has nothing to
// release, and its Close is idempotent too.
func TestApp_Close_ZeroModel(t *testing.T) {
	kanbanCloses := stubKanbanClose(t, nil)
	var model Model
	require.NoError(t, model.Close())
	require.NoError(t, model.Close())
	require.Equal(t, 1, kanbanCloses(), "the cleanup ran once")
}
