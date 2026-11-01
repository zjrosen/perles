package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/watcher"
)

// servicesBackend is a noopBackend that returns the given services and counts
// FlushCaches calls. Update runs on the test goroutine, so flushes needs no lock.
type servicesBackend struct {
	noopBackend
	services task.BackendServices
	flushes  int
}

func (b *servicesBackend) Services() task.BackendServices { return b.services }

func (b *servicesBackend) FlushCaches(_ context.Context) error {
	b.flushes++
	return nil
}

// notifyingBackend publishes its own change events through Changes(), like the
// beads embedded-Dolt snapshot backend.
type notifyingBackend struct {
	servicesBackend
	changes *pubsub.Broker[watcher.WatcherEvent]
}

func (b *notifyingBackend) Changes() *pubsub.Broker[watcher.WatcherEvent] { return b.changes }

func newNotifyingBackend(services task.BackendServices) *notifyingBackend {
	return &notifyingBackend{
		servicesBackend: servicesBackend{services: services},
		changes:         pubsub.NewBroker[watcher.WatcherEvent](),
	}
}

// newChangeTestModel builds the app through NewWithConfig and closes it when
// the test ends.
func newChangeTestModel(t *testing.T, backend task.Backend) *Model {
	t.Helper()
	model, err := NewWithConfig(AppConfig{
		Cfg:     config.Defaults(),
		Backend: backend,
		WorkDir: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, model.Close()) })
	return &model
}

// receiveChange runs the listener's Listen command and returns the change
// event it delivers, failing the test if none arrives in time.
func receiveChange(t *testing.T, listen tea.Cmd) pubsub.Event[watcher.WatcherEvent] {
	t.Helper()
	require.NotNil(t, listen, "expected a listen command")
	msgs := make(chan tea.Msg, 1)
	go func() { msgs <- listen() }()
	select {
	case msg := <-msgs:
		event, ok := msg.(pubsub.Event[watcher.WatcherEvent])
		require.True(t, ok, "expected a change event, got %T", msg)
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a change event")
		return pubsub.Event[watcher.WatcherEvent]{}
	}
}

// runCmd runs cmd and, for a batch, each of its commands, returning the
// non-nil messages. Every command must return without blocking.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, c := range batch {
			msgs = append(msgs, runCmd(c)...)
		}
		return msgs
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func TestApp_ChangeNotifier_ListensToBackendBrokerWithoutFileWatcher(t *testing.T) {
	// A DBPath is set to show that the backend's broker replaces the file
	// watcher outright (the snapshot backend reports DBPath "").
	backend := newNotifyingBackend(task.BackendServices{
		DBPath:        filepath.Join(t.TempDir(), "last-touched"),
		WatcherConfig: task.WatcherConfig{RelevantFiles: []string{"last-touched"}},
	})

	model, err := NewWithConfig(AppConfig{
		Cfg:     config.Defaults(),
		Backend: backend,
		WorkDir: t.TempDir(),
	})
	require.NoError(t, err)

	require.Nil(t, model.watcherHandle, "no file watcher when the backend publishes its own changes")
	require.NotNil(t, model.watcherListener)
	require.Equal(t, 1, backend.changes.SubscriberCount(), "the app should subscribe to the backend's broker")

	// The listener delivers what the backend publishes.
	backend.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.DBChanged})
	event := receiveChange(t, model.watcherListener.Listen())
	require.Equal(t, watcher.DBChanged, event.Payload.Type)

	// Close cancels the subscription; the broker belongs to the backend.
	require.NoError(t, model.Close())
	require.Eventually(t, func() bool { return backend.changes.SubscriberCount() == 0 },
		time.Second, 10*time.Millisecond, "Close should cancel the subscription")
	require.Equal(t, 0, backend.flushes)
}

func TestApp_ChangeNotifier_DBChangedFlushesCachesAndReloadsMode(t *testing.T) {
	// Search mode re-runs its query on a reload, so the executor sees exactly
	// one Execute call for the one DBChanged.
	queries := mocks.NewMockQueryExecutor(t)
	queries.EXPECT().Execute("").Return(nil, nil).Once()
	backend := newNotifyingBackend(task.BackendServices{QueryExecutor: queries})

	model := newChangeTestModel(t, backend)
	model.currentMode = mode.ModeSearch

	backend.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.DBChanged})
	event := receiveChange(t, model.watcherListener.Listen())

	updated, cmd := model.Update(event)
	require.Equal(t, 1, backend.flushes, "DBChanged should flush the backend caches once")

	// Stop listening so the re-listen command in the batch returns at once,
	// then run the batch: the search reload executes the query.
	updated.(Model).watcherCancel()
	msgs := runCmd(cmd)
	require.Len(t, msgs, 1, "expected only the search reload result")
}

func TestApp_ChangeNotifier_WatcherErrorIsOnlyLogged(t *testing.T) {
	// The mock has no Execute expectation, so a reload would fail the test.
	queries := mocks.NewMockQueryExecutor(t)
	backend := newNotifyingBackend(task.BackendServices{QueryExecutor: queries})

	model := newChangeTestModel(t, backend)
	model.currentMode = mode.ModeSearch

	backend.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{
		Type:  watcher.WatcherError,
		Error: errors.New("snapshot rebuild failed"),
	})
	event := receiveChange(t, model.watcherListener.Listen())

	_, cmd := model.Update(event)
	require.Equal(t, 0, backend.flushes, "a WatcherError must not flush caches")

	// The only command is the re-listen: it delivers the next event.
	backend.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.DBChanged})
	next := receiveChange(t, cmd)
	require.Equal(t, watcher.DBChanged, next.Payload.Type)
}

func TestApp_NoChangeNotifier_KeepsFileWatcher(t *testing.T) {
	// A backend without Changes() (server-mode beads, beads_rust) keeps the
	// file watcher on its DBPath, e.g. .beads/last-touched.
	dbPath := filepath.Join(t.TempDir(), "last-touched")
	backend := &servicesBackend{services: task.BackendServices{
		DBPath: dbPath,
		WatcherConfig: task.WatcherConfig{
			RelevantFiles:    []string{"last-touched"},
			DebounceDuration: 10 * time.Millisecond,
		},
	}}

	model := newChangeTestModel(t, backend)
	require.NotNil(t, model.watcherHandle, "a backend without Changes() should get a file watcher")
	require.NotNil(t, model.watcherListener)

	require.NoError(t, os.WriteFile(dbPath, []byte("1"), 0o600))
	event := receiveChange(t, model.watcherListener.Listen())
	require.Equal(t, watcher.DBChanged, event.Payload.Type)
}
