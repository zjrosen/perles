package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/watcher"
)

// schedStart is the fake clock's start time.
var schedStart = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

// fakeClock is a Clock whose time moves only when the test advances it.
// Its timers fire when Advance reaches them.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock(now time.Time) *fakeClock { return &fakeClock{now: now} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) WaitUntil(deadline time.Time) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if !deadline.After(c.now) {
		ch <- c.now
		return ch
	}
	c.timers = append(c.timers, fakeTimer{at: deadline, ch: ch})
	return ch
}

// Advance moves the time on by d and fires the timers due by then.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	pending := c.timers[:0]
	for _, timer := range c.timers {
		if timer.at.After(c.now) {
			pending = append(pending, timer)
			continue
		}
		timer.ch <- c.now
	}
	c.timers = pending
}

// hasTimer reports whether a timer is pending for at.
func (c *fakeClock) hasTimer(at time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.ContainsFunc(c.timers, func(timer fakeTimer) bool { return timer.at.Equal(at) })
}

// fakeWatcher is a manifestWatcher whose events the test publishes.
type fakeWatcher struct {
	broker  *pubsub.Broker[watcher.WatcherEvent]
	stopped atomic.Bool
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{broker: pubsub.NewBroker[watcher.WatcherEvent]()}
}

func (w *fakeWatcher) Broker() *pubsub.Broker[watcher.WatcherEvent] { return w.broker }

func (w *fakeWatcher) Stop() error {
	w.stopped.Store(true)
	w.broker.Close()
	return nil
}

// schedView is a copy of a backend's scheduler state.
type schedView struct {
	dirty, running            bool
	inFlightH0, installedHash string
	lastEnd                   time.Time
	lastDuration              time.Duration
	lastErr                   error
	consecutiveFailures       int
	retryIfFailed             bool
}

func schedState(b *SnapshotBackend) schedView {
	s := &b.sched
	s.mu.Lock()
	defer s.mu.Unlock()
	return schedView{
		dirty: s.dirty, running: s.running,
		inFlightH0: s.inFlightH0, installedHash: s.installedHash,
		lastEnd: s.lastEnd, lastDuration: s.lastDuration,
		lastErr: s.lastErr, consecutiveFailures: s.consecutiveFailures,
		retryIfFailed: s.retryIfFailed,
	}
}

func loopsStarted(b *SnapshotBackend) bool {
	b.sched.loopMu.Lock()
	defer b.sched.loopMu.Unlock()
	return b.sched.started
}

// manifestHashOf is hashManifest of a manifest holding content.
func manifestHashOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// replaceManifest replaces the manifest the way bd does: it writes a temp
// file in the same directory and renames it over the manifest, so the
// manifest gets a new inode.
func replaceManifest(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".tmp"
	require.NoError(t, os.WriteFile(tmp, []byte(content), 0o600))
	require.NoError(t, os.Rename(tmp, path))
}

// schedHarness is a SnapshotBackend over a temp workspace (manifest
// "manifest v1"), a fake clock, a fake watcher unless the real one is
// asked for, and a fake bd. Its exports serve generationExport, one
// generation per successful export, and `update` writes succeed.
type schedHarness struct {
	t      *testing.T
	b      *SnapshotBackend
	ws     testWorkspace
	clock  *fakeClock
	runner *fakeRunner
	watch  *fakeWatcher
	// changes is a subscription to b.Changes(); stopChanges ends it.
	changes     <-chan pubsub.Event[watcher.WatcherEvent]
	stopChanges context.CancelFunc

	// watcherStarts counts the backend's watcher starts (fake watcher
	// only). onWatcherStart, when set before the first build, runs at each
	// start before the watcher is returned, so before it watches.
	watcherStarts  atomic.Int32
	onWatcherStart func()

	mu sync.Mutex
	// exports holds the clock time at the start of each export.
	exports []time.Time
	// onExport, when set, runs at the start of each export, numbered from
	// 1, on the building goroutine; an error fails the export.
	onExport func(ctx context.Context, n int) error
	// onWrite, when set, runs for each `update`; an error fails it.
	onWrite func() error
	gen     int
}

func newSchedHarness(t *testing.T, realWatcher bool) *schedHarness {
	t.Helper()
	h := &schedHarness{t: t, ws: newTestWorkspace(t), clock: newFakeClock(schedStart), watch: newFakeWatcher()}
	h.runner = &fakeRunner{answer: h.answer}
	h.b = newTestBackendWith(t, h.ws, SnapshotBackendConfig{BDVersion: "1.3.2", Runner: h.runner.run, Clock: h.clock})
	if !realWatcher {
		h.b.sched.newWatcher = func() (manifestWatcher, error) {
			h.watcherStarts.Add(1)
			if h.onWatcherStart != nil {
				h.onWatcherStart()
			}
			return h.watch, nil
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.changes, h.stopChanges = h.b.Changes().Subscribe(ctx), cancel
	return h
}

func (h *schedHarness) answer(ctx context.Context, req infrabeads.BDRequest) (string, error) {
	switch bdOp(req) {
	case "export":
		h.mu.Lock()
		h.exports = append(h.exports, h.clock.Now())
		n, hook := len(h.exports), h.onExport
		h.mu.Unlock()
		if hook != nil {
			if err := hook(ctx, n); err != nil {
				return "", err
			}
		}
		h.mu.Lock()
		h.gen++
		gen := h.gen
		h.mu.Unlock()
		return generationExport(h.t, gen), nil
	case "blocked":
		return generationFlags, nil
	case "update":
		h.mu.Lock()
		hook := h.onWrite
		h.mu.Unlock()
		if hook != nil {
			return "", hook()
		}
		return "", nil
	}
	return "", fmt.Errorf("unexpected bd request %v", req.Args)
}

func (h *schedHarness) setExportHook(hook func(ctx context.Context, n int) error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onExport = hook
}

func (h *schedHarness) setWriteHook(hook func() error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onWrite = hook
}

func (h *schedHarness) exportCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.exports)
}

// exportStart is the clock time at which export n (from 1) started.
func (h *schedHarness) exportStart(n int) time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exports[n-1]
}

func (h *schedHarness) state() schedView { return schedState(h.b) }

func (h *schedHarness) replaceManifest(content string) {
	h.t.Helper()
	replaceManifest(h.t, h.ws.manifest, content)
}

// query runs `type = task` and returns the generation it answered from.
func (h *schedHarness) query() int {
	h.t.Helper()
	issues, err := h.b.Services().QueryExecutor.Execute("type = task")
	require.NoError(h.t, err)
	gen, err := checkGeneration(issues)
	require.NoError(h.t, err)
	return gen
}

// waitTimer waits until the scheduler waits on a timer that fires at at.
func (h *schedHarness) waitTimer(at time.Time) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.clock.hasTimer(at) }, 5*time.Second, time.Millisecond,
		"the scheduler does not wait until %s", at)
}

// advanceTo moves the clock to at.
func (h *schedHarness) advanceTo(at time.Time) {
	h.clock.Advance(at.Sub(h.clock.Now()))
}

// waitBuilt waits until n exports have started and no build is running,
// its bookkeeping included.
func (h *schedHarness) waitBuilt(n int) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.exportCount() == n && !h.state().running }, 5*time.Second, time.Millisecond,
		"export %d did not finish", n)
}

// rebuildAfterPacing waits for the scheduler to wait out lastEnd + gap,
// checks that nothing builds a millisecond earlier, then lets the next
// build run and waits for it.
func (h *schedHarness) rebuildAfterPacing(gap time.Duration) {
	h.t.Helper()
	exports := h.exportCount()
	due := h.state().lastEnd.Add(gap)
	h.waitTimer(due)
	h.advanceTo(due.Add(-time.Millisecond))
	require.Equal(h.t, exports, h.exportCount(), "no build before %s", due)
	h.clock.Advance(time.Millisecond)
	h.waitBuilt(exports + 1)
	require.Equal(h.t, due, h.exportStart(exports+1))
}

// events drains the events published on Changes() so far.
func (h *schedHarness) events() []watcher.WatcherEvent {
	var events []watcher.WatcherEvent
	for {
		select {
		case event, ok := <-h.changes:
			if !ok {
				return events
			}
			events = append(events, event.Payload)
		default:
			return events
		}
	}
}

// blockExport makes export n block until release is closed (or the
// build's context ends), and returns a channel closed when it starts.
func (h *schedHarness) blockExport(n int, release <-chan struct{}) <-chan struct{} {
	started := make(chan struct{})
	h.setExportHook(func(ctx context.Context, i int) error {
		if i != n {
			return nil
		}
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	})
	return started
}

func dbChanged(n int) []watcher.WatcherEvent {
	return slices.Repeat([]watcher.WatcherEvent{{Type: watcher.DBChanged}}, n)
}

// releaseOnCleanup returns a channel that the test closes when it ends at
// the latest, so a failing assertion never leaves a build blocked.
func releaseOnCleanup(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	release := make(chan struct{})
	closeRelease := sync.OnceFunc(func() { close(release) })
	t.Cleanup(closeRelease)
	return release, closeRelease
}

// TestSnapshotScheduler_LazyInit: before the first query, no trigger runs bd
// or starts the watcher or the scheduler. The first query builds, then
// starts both, once.
func TestSnapshotScheduler_LazyInit(t *testing.T) {
	h := newSchedHarness(t, false)
	te := h.b.Services().TaskExecutor
	h.setWriteHook(func() error {
		h.replaceManifest("manifest after a write")
		return nil
	})

	require.NoError(t, h.b.FlushCaches(context.Background()))
	require.NoError(t, te.UpdateStatus("s-a", task.StatusClosed), "an own write before the first query")
	h.b.requestRebuild(false) // a manifest event
	require.Equal(t, []string{"update"}, h.runner.commands(), "no bd read before the first query")
	require.Zero(t, h.watcherStarts.Load(), "no watcher before the first snapshot")
	require.False(t, loopsStarted(h.b), "no scheduler before the first snapshot")
	require.False(t, h.state().dirty)
	require.Empty(t, h.events())

	require.Equal(t, 1, h.query())
	require.Equal(t, []string{"update", "export", "blocked"}, h.runner.commands())
	require.Equal(t, int32(1), h.watcherStarts.Load())
	require.True(t, loopsStarted(h.b))
	require.Equal(t, uint64(1), h.b.current.Load().coveredWriteSeq)

	for range 3 {
		require.Equal(t, 1, h.query())
	}
	require.NoError(t, h.b.CheckCompatibility())
	require.Equal(t, int32(1), h.watcherStarts.Load(), "the watcher starts once")
	require.Equal(t, 1, h.exportCount())
	require.Equal(t, dbChanged(1), h.events())
}

// TestSnapshotScheduler_Pacing: the next build starts no earlier than
// lastEnd + max(2s, 2 × the last build's duration).
func TestSnapshotScheduler_Pacing(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		wantGap  time.Duration
	}{
		{"short build", 500 * time.Millisecond, 2 * time.Second},
		{"one-second build", time.Second, 2 * time.Second},
		{"long build", 1500 * time.Millisecond, 3 * time.Second},
		{"very long build", 4 * time.Second, 8 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSchedHarness(t, false)
			h.setExportHook(func(context.Context, int) error {
				h.clock.Advance(tt.duration) // the export takes tt.duration
				return nil
			})
			require.Equal(t, 1, h.query())
			st := h.state()
			require.Equal(t, schedStart.Add(tt.duration), st.lastEnd)
			require.Equal(t, tt.duration, st.lastDuration)

			h.replaceManifest("manifest v2")
			h.b.requestRebuild(false)
			h.rebuildAfterPacing(tt.wantGap)
			require.Equal(t, 2, h.query())

			// The second build took tt.duration too: the same gap follows it.
			h.replaceManifest("manifest v3")
			h.b.requestRebuild(false)
			h.rebuildAfterPacing(tt.wantGap)
			require.Equal(t, 3, h.query())
		})
	}
}

// TestSnapshotScheduler_RealChangeDuringRebuild: a manifest event with a
// real hash change (neither the in-flight h0 nor the installed hash) during
// a rebuild causes exactly one follow-up rebuild, however often it fires.
func TestSnapshotScheduler_RealChangeDuringRebuild(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	release, closeRelease := releaseOnCleanup(t)
	exporting := h.blockExport(2, release)

	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	h.clock.Advance(minRebuildGap)
	<-exporting
	st := h.state()
	require.True(t, st.running)
	require.False(t, st.dirty, "the build cleared dirty")
	require.Equal(t, manifestHashOf("manifest v2"), st.inFlightH0, "h0 is published before the export")

	h.replaceManifest("manifest v3") // a write lands mid-export
	for range 3 {
		h.b.requestRebuild(false)
		require.True(t, h.state().dirty)
	}
	closeRelease()
	h.waitBuilt(2)
	require.Equal(t, manifestHashOf("manifest v2"), h.b.current.Load().manifestHash)
	require.True(t, h.state().dirty, "one follow-up is pending")

	h.rebuildAfterPacing(minRebuildGap)
	require.Equal(t, manifestHashOf("manifest v3"), h.b.current.Load().manifestHash)
	require.False(t, h.state().dirty)
	h.b.requestRebuild(false) // the watcher reports v3 again
	require.False(t, h.state().dirty)
	h.clock.Advance(time.Hour)
	require.Equal(t, 3, h.exportCount(), "exactly one follow-up")
	require.Equal(t, 3, h.query())
	require.Equal(t, dbChanged(3), h.events())
}

// TestSnapshotScheduler_InFlightHashCausesNoFollowUp: during a rebuild,
// manifest events whose hash equals the in-flight h0 (perles' own bd
// children rewriting the same bytes while the export waits on the lock)
// cause no follow-up, however long the export blocks.
func TestSnapshotScheduler_InFlightHashCausesNoFollowUp(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	release, closeRelease := releaseOnCleanup(t)
	exporting := h.blockExport(2, release)

	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	h.clock.Advance(minRebuildGap)
	<-exporting
	st := h.state()
	require.Equal(t, manifestHashOf("manifest v1"), st.installedHash, "only the in-flight hash matches the events below")
	require.Equal(t, manifestHashOf("manifest v2"), st.inFlightH0)

	for range 5 {
		h.clock.Advance(time.Second) // the export blocks well past the debounce
		h.replaceManifest("manifest v2")
		h.b.requestRebuild(false)
		require.NoError(t, h.b.FlushCaches(context.Background()))
		require.False(t, h.state().dirty, "an event equal to the in-flight h0 needs no follow-up")
	}
	closeRelease()
	h.waitBuilt(2)
	require.False(t, h.state().dirty)
	h.clock.Advance(time.Hour)
	require.Equal(t, 2, h.exportCount(), "no follow-up rebuild")
	require.Equal(t, dbChanged(2), h.events())
}

// TestSnapshotScheduler_WriteDuringExportPickedUpByReHash: a write that
// lands during a rebuild's export, with no trigger at all, is picked up by
// the post-install re-hash.
func TestSnapshotScheduler_WriteDuringExportPickedUpByReHash(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	h.setExportHook(func(_ context.Context, n int) error {
		if n == 2 {
			h.replaceManifest("manifest v3") // no trigger follows
		}
		return nil
	})

	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(minRebuildGap)
	require.Equal(t, manifestHashOf("manifest v2"), h.b.current.Load().manifestHash)
	require.True(t, h.state().dirty, "the re-hash found the write")

	h.rebuildAfterPacing(minRebuildGap)
	require.Equal(t, manifestHashOf("manifest v3"), h.b.current.Load().manifestHash)
	require.False(t, h.state().dirty)
	require.Equal(t, 3, h.query())
}

// firstBuilds are the synchronous first builds: CheckCompatibility (the
// TUI) and the first query (the daemon).
var firstBuilds = map[string]func(*schedHarness) error{
	"CheckCompatibility": func(h *schedHarness) error { return h.b.CheckCompatibility() },
	"first query": func(h *schedHarness) error {
		_, err := h.b.Services().QueryExecutor.Execute("type = task")
		return err
	},
}

// TestSnapshotScheduler_FirstInstallReHash: a write during the export of
// the synchronous first build (CheckCompatibility or the first query) is
// picked up: the first install re-hashes and marks dirty, and the scheduler
// rebuilds after pacing with no further trigger.
func TestSnapshotScheduler_FirstInstallReHash(t *testing.T) {
	for name, build := range firstBuilds {
		t.Run(name, func(t *testing.T) {
			h := newSchedHarness(t, false)
			h.setExportHook(func(_ context.Context, n int) error {
				if n == 1 {
					h.replaceManifest("manifest v2")
				}
				return nil
			})

			require.NoError(t, build(h))
			require.Equal(t, manifestHashOf("manifest v1"), h.b.current.Load().manifestHash)
			require.True(t, h.state().dirty)
			require.Equal(t, int32(1), h.watcherStarts.Load())

			h.rebuildAfterPacing(minRebuildGap)
			require.Equal(t, manifestHashOf("manifest v2"), h.b.current.Load().manifestHash)
			require.False(t, h.state().dirty)
			require.Equal(t, 2, h.query())
			require.Equal(t, dbChanged(2), h.events())
		})
	}
}

// TestSnapshotScheduler_WriteWhileWatcherStarts: the first install starts
// the watcher before it re-hashes the manifest. A write that lands after
// the export but before the watcher watches (the fake watcher never reports
// it) is therefore caught by the re-hash, and the scheduler rebuilds after
// pacing with no further trigger.
func TestSnapshotScheduler_WriteWhileWatcherStarts(t *testing.T) {
	for name, build := range firstBuilds {
		t.Run(name, func(t *testing.T) {
			h := newSchedHarness(t, false)
			h.onWatcherStart = func() { h.replaceManifest("manifest v2") }

			require.NoError(t, build(h))
			require.Equal(t, 1, h.exportCount())
			require.Equal(t, manifestHashOf("manifest v1"), h.b.current.Load().manifestHash)
			require.True(t, h.state().dirty, "the re-hash after the watcher start finds the write")

			h.rebuildAfterPacing(minRebuildGap)
			require.Equal(t, manifestHashOf("manifest v2"), h.b.current.Load().manifestHash)
			require.False(t, h.state().dirty)
			require.Equal(t, 2, h.query())
			require.Equal(t, dbChanged(2), h.events())
			require.Equal(t, int32(1), h.watcherStarts.Load())
		})
	}
}

// TestSnapshotScheduler_UnchangedHashIsDropped: when idle, manifest events
// and FlushCaches after reads (identical bytes) cause no rebuild and no
// broker event.
func TestSnapshotScheduler_UnchangedHashIsDropped(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())

	for range 20 {
		h.replaceManifest("manifest v1") // a bd read rewrites the same bytes
		h.b.requestRebuild(false)
		require.NoError(t, h.b.FlushCaches(context.Background()))
	}
	require.False(t, h.state().dirty)
	h.clock.Advance(time.Hour)
	require.Equal(t, 1, h.exportCount())
	require.Empty(t, h.events())
}

// TestSnapshotScheduler_WatcherEventsGoThroughTheGate: the watcher's events
// reach the hash gate, and never the Changes broker themselves.
func TestSnapshotScheduler_WatcherEventsGoThroughTheGate(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())

	h.watch.broker.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.WatcherError, Error: errors.New("boom")})
	h.replaceManifest("manifest v2")
	h.watch.broker.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.DBChanged})
	require.Eventually(t, func() bool { return h.state().dirty }, 5*time.Second, time.Millisecond)
	require.Empty(t, h.events(), "raw watcher events are not republished")

	h.rebuildAfterPacing(minRebuildGap)
	require.Equal(t, dbChanged(1), h.events(), "only the install is")
}

// TestSnapshotScheduler_OwnWritesTrigger: a successful write made through
// perles asks for a rebuild (T2), which covers it. A failed write asks for
// nothing.
func TestSnapshotScheduler_OwnWritesTrigger(t *testing.T) {
	h := newSchedHarness(t, false)
	te := h.b.Services().TaskExecutor
	require.Equal(t, 1, h.query())
	h.setWriteHook(func() error {
		h.replaceManifest("manifest after write")
		return nil
	})

	require.NoError(t, te.UpdateStatus("s-a", task.StatusClosed))
	require.True(t, h.state().dirty)
	h.rebuildAfterPacing(minRebuildGap)
	require.Equal(t, uint64(1), h.b.current.Load().coveredWriteSeq)
	require.Equal(t, manifestHashOf("manifest after write"), h.b.current.Load().manifestHash)

	h.setWriteHook(func() error { return errors.New("bd update failed: boom") })
	require.ErrorContains(t, te.UpdateStatus("s-a", task.StatusOpen), "boom")
	require.False(t, h.state().dirty)
	h.clock.Advance(time.Hour)
	require.Equal(t, 2, h.exportCount())
}

// errLocked is a failing export's error.
var errLocked = errors.New("bd export failed: database is locked")

// failingExports makes every export from the next one fail with errLocked
// while fail is set.
func failingExports(h *schedHarness) *atomic.Bool {
	fail := &atomic.Bool{}
	fail.Store(true)
	h.setExportHook(func(context.Context, int) error {
		if fail.Load() {
			return errLocked
		}
		return nil
	})
	return fail
}

// TestSnapshotScheduler_FailureKeepsSnapshotAndBacksOff: a failed rebuild
// keeps the old snapshot, clears the recorded hash and publishes a
// WatcherError. Automatic triggers (watcher and own writes) then back off
// 5s, 10s, 20s, ... capped at 2min, and a success resets the failures.
func TestSnapshotScheduler_FailureKeepsSnapshotAndBacksOff(t *testing.T) {
	h := newSchedHarness(t, false)
	te := h.b.Services().TaskExecutor
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())
	fail := failingExports(h)

	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(minRebuildGap)
	st := h.state()
	require.ErrorIs(t, st.lastErr, errLocked)
	require.Equal(t, 1, st.consecutiveFailures)
	require.Empty(t, st.installedHash, "the recorded hash is cleared")
	require.Empty(t, st.inFlightH0)
	require.False(t, st.dirty)
	require.Equal(t, 1, h.query(), "the old snapshot keeps serving")
	require.Equal(t, []watcher.WatcherEvent{{Type: watcher.WatcherError, Error: errLocked}}, h.events())

	writes := 0
	for i, backoff := range []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 2 * time.Minute, 2 * time.Minute,
	} {
		if i%2 == 0 {
			// A manifest event with an unchanged hash: the recorded hash is
			// cleared, so it asks for a retry.
			h.b.requestRebuild(false)
		} else {
			writes++
			h.setWriteHook(func() error {
				h.replaceManifest(fmt.Sprintf("manifest after write %d", writes))
				return nil
			})
			require.NoError(t, te.UpdateStatus("s-a", task.StatusClosed))
		}
		require.True(t, h.state().dirty)
		h.rebuildAfterPacing(backoff)
		require.Equal(t, i+2, h.state().consecutiveFailures)
	}

	// A success after the backoff resets the failures and records the hash.
	fail.Store(false)
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(2 * time.Minute)
	st = h.state()
	require.NoError(t, st.lastErr)
	require.Zero(t, st.consecutiveFailures)
	require.Equal(t, manifestHashOf(fmt.Sprintf("manifest after write %d", writes)), st.installedHash)
	require.Equal(t, 2, h.query())

	h.replaceManifest("manifest v3")
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(minRebuildGap) // pacing only, no backoff
	require.Equal(t, 3, h.query())

	events := h.events()
	require.Len(t, events, 9)
	for _, event := range events[:7] {
		require.Equal(t, watcher.WatcherError, event.Type)
		require.ErrorIs(t, event.Error, errLocked)
	}
	require.Equal(t, dbChanged(2), events[7:])
}

// TestSnapshotScheduler_FlushCachesRetriesFailure: after a failed rebuild,
// FlushCaches marks dirty although the hash is unchanged, and resets the
// backoff: the retry runs as soon as pacing allows, also when an automatic
// trigger already waits out the backoff.
func TestSnapshotScheduler_FlushCachesRetriesFailure(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	fail := failingExports(h)
	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(minRebuildGap) // fails
	h.b.requestRebuild(false)
	h.rebuildAfterPacing(5 * time.Second) // fails
	require.Equal(t, 2, h.state().consecutiveFailures)

	// The watcher asks for a retry, which waits out the 10s backoff.
	h.b.requestRebuild(false)
	h.waitTimer(h.state().lastEnd.Add(10 * time.Second))

	// A manual refresh: same hash, but the last build failed.
	require.NoError(t, h.b.FlushCaches(context.Background()))
	st := h.state()
	require.True(t, st.dirty)
	require.Zero(t, st.consecutiveFailures, "FlushCaches resets the backoff")
	h.rebuildAfterPacing(minRebuildGap) // fails again, backoff restarts at 5s
	require.Equal(t, 1, h.state().consecutiveFailures)

	fail.Store(false)
	require.NoError(t, h.b.FlushCaches(context.Background()))
	h.rebuildAfterPacing(minRebuildGap)
	st = h.state()
	require.NoError(t, st.lastErr)
	require.Zero(t, st.consecutiveFailures)
	require.Equal(t, manifestHashOf("manifest v2"), st.installedHash)
	require.Equal(t, 2, h.query())
}

// TestSnapshotScheduler_ManualRefreshDuringFailingRetry: after a failed
// rebuild, a FlushCaches while the retry runs is dropped by the hash gate
// (the retry's in-flight hash covers it). If the retry fails too, the
// request is replayed: the backoff is reset and one more retry runs as soon
// as pacing allows. If the retry succeeds, nothing follows.
func TestSnapshotScheduler_ManualRefreshDuringFailingRetry(t *testing.T) {
	for _, retryFails := range []bool{true, false} {
		t.Run(fmt.Sprintf("retry fails %v", retryFails), func(t *testing.T) {
			h := newSchedHarness(t, false)
			require.Equal(t, 1, h.query())
			release, closeRelease := releaseOnCleanup(t)
			var failing atomic.Bool
			failing.Store(true)
			retrying := make(chan struct{})
			h.setExportHook(func(ctx context.Context, n int) error {
				if n == 3 {
					close(retrying)
					select {
					case <-release:
					case <-ctx.Done():
						return context.Cause(ctx)
					}
				}
				if failing.Load() {
					return errLocked
				}
				return nil
			})

			h.replaceManifest("manifest v2")
			h.b.requestRebuild(false)
			h.rebuildAfterPacing(minRebuildGap) // export 2 fails
			require.Equal(t, 1, h.state().consecutiveFailures)

			// The watcher asks for a retry, which runs after the 5s backoff
			// and blocks in its export.
			h.b.requestRebuild(false)
			due := h.state().lastEnd.Add(5 * time.Second)
			h.waitTimer(due)
			h.advanceTo(due)
			<-retrying
			require.Equal(t, manifestHashOf("manifest v2"), h.state().inFlightH0)

			// The manual refresh: the running retry covers its hash.
			require.NoError(t, h.b.FlushCaches(context.Background()))
			st := h.state()
			require.False(t, st.dirty, "the running retry covers the request")
			require.True(t, st.retryIfFailed)

			failing.Store(retryFails)
			closeRelease()
			h.waitBuilt(3)
			st = h.state()
			require.False(t, st.retryIfFailed, "the end of the build clears it")
			if !retryFails {
				require.NoError(t, st.lastErr)
				require.False(t, st.dirty, "the successful retry served the request")
				h.clock.Advance(time.Hour)
				require.Equal(t, 3, h.exportCount())
				require.Equal(t, 2, h.query())
				return
			}

			require.ErrorIs(t, st.lastErr, errLocked)
			require.True(t, st.dirty, "the failed retry replays the manual request")
			require.Zero(t, st.consecutiveFailures, "and resets the backoff")
			failing.Store(false)
			h.rebuildAfterPacing(minRebuildGap) // pacing only, no 5s backoff
			require.Equal(t, 4, h.exportCount())
			st = h.state()
			require.NoError(t, st.lastErr)
			require.Equal(t, manifestHashOf("manifest v2"), st.installedHash)
			require.Equal(t, 2, h.query())
		})
	}
}

// TestSnapshotScheduler_FailedFirstBuildIsLogged: a failed lazy first
// build (CheckCompatibility or the first query) is logged, like the
// scheduler goroutine's failed rebuilds.
func TestSnapshotScheduler_FailedFirstBuildIsLogged(t *testing.T) {
	for name, build := range firstBuilds {
		t.Run(name, func(t *testing.T) {
			readLog := captureLog(t)
			h := newSchedHarness(t, false)
			failingExports(h)

			require.ErrorIs(t, build(h), errLocked)
			logged := readLog()
			require.Contains(t, logged, "Building the first snapshot failed")
			require.Contains(t, logged, errLocked.Error())
		})
	}
}

// TestSnapshotScheduler_FirstBuildBacksOff: a failed lazy first build is
// retried by a later query only once pacing and the failure backoff allow;
// until then queries return the last error without running bd. This is the
// daemon's path, which never calls CheckCompatibility. FlushCaches resets
// the backoff, and CheckCompatibility always builds.
func TestSnapshotScheduler_FirstBuildBacksOff(t *testing.T) {
	queryErr := func(h *schedHarness) error {
		_, err := h.b.Services().QueryExecutor.Execute("type = task")
		return err
	}

	t.Run("queries", func(t *testing.T) {
		h := newSchedHarness(t, false)
		fail := failingExports(h)

		require.ErrorIs(t, queryErr(h), errLocked)
		for _, backoff := range []time.Duration{5 * time.Second, 10 * time.Second} {
			exports := h.exportCount()
			for range 5 {
				require.ErrorIs(t, queryErr(h), errLocked)
			}
			h.clock.Advance(backoff - time.Millisecond)
			require.ErrorIs(t, queryErr(h), errLocked)
			require.Equal(t, exports, h.exportCount(), "no bd during the %s backoff", backoff)
			h.clock.Advance(time.Millisecond)
			require.ErrorIs(t, queryErr(h), errLocked)
			require.Equal(t, exports+1, h.exportCount(), "a retry after the %s backoff", backoff)
		}
		require.Equal(t, 3, h.state().consecutiveFailures)

		// FlushCaches resets the backoff (but builds nothing itself): the
		// next query retries once pacing allows.
		fail.Store(false)
		require.NoError(t, h.b.FlushCaches(context.Background()))
		require.Equal(t, 3, h.exportCount())
		h.clock.Advance(minRebuildGap - time.Millisecond)
		require.ErrorIs(t, queryErr(h), errLocked)
		h.clock.Advance(time.Millisecond)
		require.Equal(t, 1, h.query())
		require.Equal(t, 4, h.exportCount())
		require.Zero(t, h.state().consecutiveFailures)
		require.Equal(t, int32(1), h.watcherStarts.Load())
	})

	t.Run("CheckCompatibility", func(t *testing.T) {
		h := newSchedHarness(t, false)
		failingExports(h)
		for range 3 {
			require.ErrorIs(t, h.b.CheckCompatibility(), errLocked)
		}
		require.Equal(t, 3, h.exportCount(), "CheckCompatibility always builds")
		require.ErrorIs(t, queryErr(h), errLocked)
		require.Equal(t, 3, h.exportCount(), "a query waits out the backoff")
		require.Zero(t, h.watcherStarts.Load())
		require.False(t, loopsStarted(h.b))
	})
}

// TestSnapshotScheduler_TriggersNeverBlock: FlushCaches returns within 10ms,
// and an own write returns promptly, while a build is blocked in its export.
func TestSnapshotScheduler_TriggersNeverBlock(t *testing.T) {
	requireFastFlushes := func(t *testing.T, h *schedHarness) {
		t.Helper()
		before := h.runner.count("")
		for i := range 20 {
			if i%2 == 1 {
				h.replaceManifest(fmt.Sprintf("manifest change %d", i))
			}
			start := time.Now()
			require.NoError(t, h.b.FlushCaches(context.Background()))
			require.Less(t, time.Since(start), 10*time.Millisecond, "FlushCaches waited")
		}
		require.Equal(t, before, h.runner.count(""), "FlushCaches runs no bd")
	}

	t.Run("scheduled rebuild blocked", func(t *testing.T) {
		h := newSchedHarness(t, false)
		require.Equal(t, 1, h.query())
		release, closeRelease := releaseOnCleanup(t)
		exporting := h.blockExport(2, release)
		h.replaceManifest("manifest v2")
		h.b.requestRebuild(false)
		h.clock.Advance(minRebuildGap)
		<-exporting

		requireFastFlushes(t, h)
		require.True(t, h.state().dirty, "a change during the build is still recorded")

		done := make(chan error, 1)
		go func() { done <- h.b.Services().TaskExecutor.UpdateStatus("s-a", task.StatusClosed) }()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("a write waited on the rebuild")
		}
		closeRelease()
		h.waitBuilt(2)
	})

	t.Run("first build blocked", func(t *testing.T) {
		h := newSchedHarness(t, false)
		release, closeRelease := releaseOnCleanup(t)
		exporting := h.blockExport(1, release)
		queried := make(chan error, 1)
		go func() {
			_, err := h.b.Services().QueryExecutor.Execute("type = task")
			queried <- err
		}()
		<-exporting

		requireFastFlushes(t, h)
		closeRelease()
		require.NoError(t, <-queried)
	})
}

// goroutinesIn returns the number of goroutines with a frame in fn.
func goroutinesIn(fn string) int {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for g := range strings.SplitSeq(string(buf), "\n\n") {
		if strings.Contains(g, fn) {
			n++
		}
	}
	return n
}

// TestSnapshotScheduler_CloseStopsEverything: Close cancels a running bd
// read (with errSnapshotBackendClosed, not counted as a failure), stops the
// watcher and the scheduler goroutine, and closes the Changes broker,
// leaving no goroutine behind. It is idempotent.
func TestSnapshotScheduler_CloseStopsEverything(t *testing.T) {
	ours := []string{
		"adapter.(*SnapshotBackend).runScheduler",
		"adapter.(*SnapshotBackend).forwardManifestEvents",
		"internal/watcher.(*Watcher).loop",
		"github.com/fsnotify/fsnotify.",
	}
	requireNoLeak := func(t *testing.T) {
		t.Helper()
		for _, fn := range ours {
			require.Eventually(t, func() bool { return goroutinesIn(fn) == 0 }, 5*time.Second, 10*time.Millisecond,
				"a goroutine in %s outlives Close", fn)
		}
	}
	closeWithin := func(t *testing.T, b *SnapshotBackend) {
		t.Helper()
		closed := make(chan error, 1)
		go func() { closed <- b.Close() }()
		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not return")
		}
		require.NoError(t, b.Close(), "Close is idempotent")
	}

	t.Run("during a rebuild, real watcher", func(t *testing.T) {
		h := newSchedHarness(t, true)
		require.Equal(t, 1, h.query())
		for _, fn := range ours {
			require.Eventually(t, func() bool { return goroutinesIn(fn) > 0 }, 5*time.Second, time.Millisecond,
				"%s runs after the first install", fn)
		}
		var cause atomic.Value
		exporting := make(chan struct{})
		h.setExportHook(func(ctx context.Context, n int) error {
			if n == 2 {
				close(exporting)
				<-ctx.Done() // waits on the lock until killed
				cause.Store(context.Cause(ctx))
				return fmt.Errorf("bd export failed: %w", context.Cause(ctx))
			}
			return nil
		})
		h.replaceManifest("manifest v2")
		h.b.requestRebuild(false)
		h.clock.Advance(minRebuildGap)
		<-exporting

		closeWithin(t, h.b)
		require.ErrorIs(t, cause.Load().(error), errSnapshotBackendClosed)
		require.Zero(t, h.state().consecutiveFailures, "a build killed by Close is no failure")
		require.Equal(t, dbChanged(1), h.events(), "and publishes no WatcherError")
		_, open := <-h.changes
		require.False(t, open, "Close closes the Changes broker")
		h.stopChanges()
		requireNoLeak(t)
	})

	t.Run("waiting out a backoff", func(t *testing.T) {
		h := newSchedHarness(t, false)
		require.Equal(t, 1, h.query())
		failingExports(h)
		h.replaceManifest("manifest v2")
		h.b.requestRebuild(false)
		h.rebuildAfterPacing(minRebuildGap)
		h.b.requestRebuild(false)
		h.waitTimer(h.state().lastEnd.Add(5 * time.Second))

		closeWithin(t, h.b)
		require.True(t, h.watch.stopped.Load())
		h.clock.Advance(time.Hour)
		require.Equal(t, 2, h.exportCount(), "nothing builds after Close")
		h.stopChanges()
		requireNoLeak(t)
	})
}

// TestSnapshotScheduler_RealWatcherRenameReplace: the real watcher, on a
// temp manifest replaced by rename the way bd does (a new inode each time),
// with the real 500ms debounce. Identical bytes are dropped by the gate,
// including the ones the export itself writes while it blocks past the
// debounce; new bytes give exactly one rebuild.
func TestSnapshotScheduler_RealWatcherRenameReplace(t *testing.T) {
	h := newSchedHarness(t, true)
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())
	h.b.sched.loopMu.Lock()
	w := h.b.sched.watch
	h.b.sched.loopMu.Unlock()
	require.IsType(t, &watcher.Watcher{}, w)
	raw := w.Broker().Subscribe(t.Context())
	waitRaw := func() {
		t.Helper()
		select {
		case <-raw:
		case <-time.After(5 * time.Second):
			t.Fatal("the watcher did not report the replaced manifest")
		}
	}

	// A bd read elsewhere replaces the manifest with identical bytes.
	h.replaceManifest("manifest v1")
	waitRaw()
	require.Never(t, func() bool { return h.state().dirty }, 300*time.Millisecond, 5*time.Millisecond)
	require.Equal(t, 1, h.exportCount())
	require.Empty(t, h.events())

	// A write replaces it with new bytes. The rebuild's own bd children
	// replace it again with the same bytes while the export blocks past the
	// debounce.
	exportErr := make(chan error, 1)
	h.setExportHook(func(ctx context.Context, n int) error {
		if n != 2 {
			return nil
		}
		if err := os.WriteFile(h.ws.manifest+".tmp", []byte("manifest v2"), 0o600); err != nil {
			exportErr <- err
			return err
		}
		if err := os.Rename(h.ws.manifest+".tmp", h.ws.manifest); err != nil {
			exportErr <- err
			return err
		}
		select {
		case <-raw:
		case <-time.After(5 * time.Second):
			exportErr <- errors.New("the watcher did not report the export's own rewrite")
		case <-ctx.Done():
			return context.Cause(ctx)
		}
		time.Sleep(100 * time.Millisecond) // the gate sees it while the export runs
		return nil
	})
	h.replaceManifest("manifest v2")
	waitRaw()
	require.Eventually(t, func() bool { return h.state().dirty }, 5*time.Second, time.Millisecond)
	h.rebuildAfterPacing(minRebuildGap)
	select {
	case err := <-exportErr:
		t.Fatal(err)
	default:
	}
	require.Equal(t, manifestHashOf("manifest v2"), h.b.current.Load().manifestHash)
	require.Never(t, func() bool { return h.state().dirty }, 300*time.Millisecond, 5*time.Millisecond)
	h.clock.Advance(time.Hour)
	require.Equal(t, 2, h.exportCount(), "exactly one rebuild")
	require.Equal(t, dbChanged(1), h.events())
	require.Equal(t, 2, h.query())
}

// TestSnapshotScheduler_RealWatcherSustainedRewrites: the real watcher, with
// the real 500ms debounce and 2s maximum wait, while the manifest is
// replaced every 50ms without a pause, as bd commands that keep coming
// rewrite it, so the debounce never fires. Each rewrite writes the
// manifest's current bytes. When a write changes them, the watcher still
// reports it within the maximum wait, mid-stream, and the scheduler
// rebuilds; twice in a row, while the rewrites go on.
func TestSnapshotScheduler_RealWatcherSustainedRewrites(t *testing.T) {
	const rewriteEvery = 50 * time.Millisecond
	h := newSchedHarness(t, true)
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())

	var content atomic.Value // the bytes every rewrite writes
	content.Store("manifest v1")
	type streamResult struct {
		longestPause time.Duration
		err          error
	}
	stream := make(chan streamResult, 1)
	stop, stopped := make(chan struct{}), make(chan struct{})
	// Waits for the rewrites to end, so none outlives the test's workspace.
	stopRewrites := sync.OnceFunc(func() {
		close(stop)
		<-stopped
	})
	t.Cleanup(stopRewrites)
	go func() {
		defer close(stopped)
		var longest time.Duration
		last := time.Now()
		ticker := time.NewTicker(rewriteEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				stream <- streamResult{longestPause: longest}
				return
			case <-ticker.C:
			}
			tmp := h.ws.manifest + ".tmp"
			if err := os.WriteFile(tmp, []byte(content.Load().(string)), 0o600); err != nil {
				stream <- streamResult{err: err}
				return
			}
			if err := os.Rename(tmp, h.ws.manifest); err != nil {
				stream <- streamResult{err: err}
				return
			}
			now := time.Now()
			longest = max(longest, now.Sub(last))
			last = now
		}
	}()

	for gen := 2; gen <= 3; gen++ {
		next := fmt.Sprintf("manifest v%d", gen)
		content.Store(next) // a write: the rewrites write the new bytes from now on
		require.Eventually(t, func() bool { return h.state().dirty }, manifestMaxWait+3*time.Second, time.Millisecond,
			"the watcher reports generation %d while the rewrites go on", gen)
		h.rebuildAfterPacing(minRebuildGap)
		require.Equal(t, manifestHashOf(next), h.b.current.Load().manifestHash)
		require.Equal(t, gen, h.query())
	}

	stopRewrites()
	result := <-stream
	require.NoError(t, result.err)
	require.Less(t, result.longestPause, manifestDebounce,
		"the rewrites never paused for the debounce, so only the maximum wait can have reported them")
	require.Equal(t, 3, h.exportCount())
	require.Equal(t, dbChanged(2), h.events())
}

// TestSnapshotScheduler_EachInstallPublishesOneDBChanged: every install, the
// first included, publishes exactly one DBChanged; dropped triggers publish
// nothing.
func TestSnapshotScheduler_EachInstallPublishesOneDBChanged(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	require.Equal(t, dbChanged(1), h.events())
	for gen := 2; gen <= 4; gen++ {
		content := fmt.Sprintf("manifest v%d", gen)
		h.replaceManifest(content)
		h.b.requestRebuild(false)
		h.b.requestRebuild(false)
		h.rebuildAfterPacing(minRebuildGap)
		h.b.requestRebuild(false) // dropped: the installed hash
		require.NoError(t, h.b.FlushCaches(context.Background()))
		require.Equal(t, gen, h.query())
		require.Equal(t, dbChanged(1), h.events(), "generation %d", gen)
	}
}

// TestSnapshotBackend_ChangesBroker: Changes() returns the same non-nil
// broker from construction on, and Close closes it.
func TestSnapshotBackend_ChangesBroker(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	changes := b.Changes()
	require.NotNil(t, changes, "the broker exists before any build")
	require.Same(t, changes, b.Changes())
	sub := changes.Subscribe(t.Context())

	require.NoError(t, b.CheckCompatibility())
	select {
	case event := <-sub:
		require.Equal(t, watcher.DBChanged, event.Payload.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("the first install published nothing")
	}
	require.NoError(t, b.Close())
	_, open := <-sub
	require.False(t, open, "Close closes the broker")
	require.Same(t, changes, b.Changes())
}

// TestSnapshotBackend_SnapshotCachesRunNoJanitor: a snapshot's BQL caches
// start no go-cache janitor goroutine, so rebuilds do not pile them up.
func TestSnapshotBackend_SnapshotCachesRunNoJanitor(t *testing.T) {
	const janitor = "go-cache.(*janitor).Run"
	b := newTestBackend(t, generationRunner(t).run)
	before := goroutinesIn(janitor)
	for range 20 {
		require.NoError(t, b.rebuild(b.ctx))
		_, err := b.Services().QueryExecutor.Execute("type = task")
		require.NoError(t, err)
	}
	require.LessOrEqual(t, goroutinesIn(janitor), before)
}

func TestFailureBackoff(t *testing.T) {
	want := []time.Duration{0, 5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 2 * time.Minute, 2 * time.Minute}
	for n, backoff := range want {
		require.Equal(t, backoff, failureBackoff(n), "after %d failures", n)
	}
	require.Equal(t, 2*time.Minute, failureBackoff(1000))
}
