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

// failKanbanClose makes closing the kanban controller return err until the
// test ends.
func failKanbanClose(t *testing.T, err error) {
	t.Helper()
	orig := closeKanban
	closeKanban = func(*kanban.Model) error { return err }
	t.Cleanup(func() { closeKanban = orig })
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
			if tc.kanbanErr != nil {
				failKanbanClose(t, tc.kanbanErr)
			}

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

			require.ErrorIs(t, model.watcherCtx.Err(), context.Canceled,
				"Close should cancel the change listener")
			if tc.notifier {
				require.Eventually(t, func() bool { return notifier.changes.SubscriberCount() == 0 },
					time.Second, 10*time.Millisecond, "Close should drop the backend subscription")
			} else {
				// A stopped watcher has closed its broker, so a new
				// subscription comes back closed.
				_, open := <-model.watcherHandle.Broker().Subscribe(context.Background())
				require.False(t, open, "Close should stop the file watcher")
			}
			require.Nil(t, model.tracingProvider, "Close should shut down the tracing provider")
		})
	}
}
