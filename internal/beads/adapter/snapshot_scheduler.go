package adapter

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/watcher"
)

// Freshness timings of a SnapshotBackend.
const (
	// manifestDebounce coalesces the manifest events of one bd command. bd
	// replaces the manifest (temp file plus rename) at least once per
	// command, reads included.
	manifestDebounce = 500 * time.Millisecond
	// manifestMaxWait bounds how long the debounce can hold back a manifest
	// event (watcher.Config.MaxWait). bd rewrites the manifest throughout
	// each command, so agents writing about every 0.8s never leave the
	// debounce a 500ms pause; without a bound the watcher would stay silent,
	// and the snapshot stale, until they stop. While events keep coming,
	// the watcher reports at least this often, and the hash gate drops the
	// reports that find no change.
	//
	// It equals minRebuildGap. A report leads to a rebuild no sooner than
	// pacing allows (at least minRebuildGap after the last build), so
	// reporting more often could not rebuild more often or take more of
	// the lock; reporting less often would leave the snapshot staler than
	// pacing makes it. A report costs one read of the manifest.
	manifestMaxWait = minRebuildGap
	// minRebuildGap is the least idle time between the end of a build and
	// the start of the next one. A build that took longer than half of it
	// is followed by twice its duration instead, which keeps perles' share
	// of the embedded database's lock at or below a third under sustained
	// writes.
	minRebuildGap = 2 * time.Second
	// firstFailureBackoff and maxFailureBackoff bound the extra idle time
	// that automatic rebuilds wait after consecutive failed builds: 5s,
	// 10s, 20s, ... up to 2min.
	firstFailureBackoff = 5 * time.Second
	maxFailureBackoff   = 2 * time.Minute
)

// manifestWatcher publishes watcher.DBChanged on its broker, debounced,
// whenever the manifest is written or replaced, and at least every
// manifestMaxWait while it keeps being rewritten, until it is stopped.
// *watcher.Watcher is one.
type manifestWatcher interface {
	Broker() *pubsub.Broker[watcher.WatcherEvent]
	Stop() error
}

// scheduler is the part of a SnapshotBackend that decides when to rebuild
// its snapshot. Three triggers ask for a rebuild, all through the hash gate
// of requestRebuild: the manifest watcher (T1), perles' own writes (T2) and
// FlushCaches (T3). A single scheduler goroutine (runScheduler) then runs
// the rebuilds, one at a time, paced and backing off after failures. It
// loops while the snapshot is dirty, so a change that lands during a build
// gets exactly one follow-up.
//
// The scheduler goroutine and the watcher start only after the first
// snapshot is installed, so a backend that is never queried (the daemon)
// runs neither.
type scheduler struct {
	// buildMu is held by every build from its start to the end of its
	// bookkeeping. The lazy first build and the scheduler goroutine's
	// rebuilds therefore never overlap, and installs run one at a time, in
	// build order: install relies on this to prune the comment overlay. No
	// trigger takes it, so none waits on a build.
	buildMu sync.Mutex

	// mu (schedMu) guards the state below. It is never held across bd, a
	// build, a file read or a publish, so taking it never blocks for long.
	mu sync.Mutex
	// dirty is set by a trigger whose manifest hash no snapshot covers, and
	// by a build that finds the manifest changed after its install. A build
	// clears it when it starts.
	dirty bool
	// running is set from the start of a build to the end of its
	// bookkeeping.
	running bool
	// inFlightH0 is the manifest hash the running build read before its
	// export (empty when idle, or before the build has read it). The
	// export covers that state, so events carrying it need no follow-up:
	// perles' own bd children rewrite the manifest with identical bytes
	// while they wait on the database lock.
	inFlightH0 string
	// installedHash is the installed snapshot's manifest hash: the hash the
	// gate drops besides inFlightH0. A failed build clears it, so that every
	// trigger asks for a retry, FlushCaches included (T3 marks dirty when
	// the last build failed).
	installedHash string
	// lastEnd and lastDuration time the last build, failed or not, for
	// pacing.
	lastEnd      time.Time
	lastDuration time.Duration
	// lastErr is the last build's error, nil after a success (the
	// proposal's lastFailed is lastErr != nil). Queries return it while the
	// lazy first build is deferred (see buildFirst).
	lastErr error
	// consecutiveFailures counts the failed builds since the last success
	// or FlushCaches, for the failure backoff.
	consecutiveFailures int
	// retryIfFailed is set by a manual request (FlushCaches) made after a
	// failed build while a retry is running whose in-flight hash covers it.
	// The gate drops the request, since the running build covers it; but if
	// that build fails too, finishBuild replays the request: it resets the
	// backoff and marks the snapshot dirty, so a manual refresh after a
	// failure always gets its retry. The end of every build clears it.
	retryIfFailed bool

	// wake, with room for one signal, tells the scheduler goroutine to look
	// at the state again: the snapshot became dirty, or the backoff it
	// waits out was reset.
	wake chan struct{}

	// newWatcher creates and starts the manifest watcher (T1). It is
	// b.watchManifest; tests replace it to fire manifest events by hand.
	newWatcher func() (manifestWatcher, error)

	// loopMu orders starting the scheduler goroutine and the watcher with
	// stopping them in Close.
	loopMu  sync.Mutex
	started bool
	stopped bool
	// watch is the manifest watcher, nil when it is not running.
	watch manifestWatcher
	// loops tracks the scheduler goroutine and the goroutine that forwards
	// the watcher's events, so Close can wait for both.
	loops sync.WaitGroup
}

// Changes returns the broker on which the backend publishes
// watcher.DBChanged after each snapshot install, and watcher.WatcherError
// after each failed build. Raw manifest events never reach it: bd rewrites
// the manifest on every command, and only installs mean the data changed.
// The app's changeNotifier subscribes to it instead of creating a file
// watcher (Services().DBPath is empty).
//
// The broker is created with the backend, so it is never nil, and it lives
// as long as the backend: Close closes it, which closes every
// subscription's channel. Subscribers only cancel their subscriptions.
func (b *SnapshotBackend) Changes() *pubsub.Broker[watcher.WatcherEvent] {
	return b.changes
}

// FlushCaches is the user-driven refresh (T3): manual refresh, after a
// create or delete, on mode switches, and after each DBChanged. A
// snapshot's BQL caches cannot be stale for the snapshot they belong to,
// so it flushes nothing. Instead it asks for a rebuild through the hash
// gate (see requestRebuild) and resets the failure backoff, so after a
// failed build it retries as soon as pacing allows. It never runs bd and
// never waits on a build, so it returns at once on the UI goroutine.
func (b *SnapshotBackend) FlushCaches(context.Context) error {
	b.requestRebuild(true)
	return nil
}

// requestRebuild is the hash gate that every trigger goes through: the
// manifest watcher (T1), perles' own writes (T2) and FlushCaches (T3, with
// manual set). It hashes the manifest and drops the request when the
// installed snapshot or the running build covers that hash. Otherwise it
// marks the snapshot dirty and wakes the scheduler goroutine, which
// rebuilds once pacing and the failure backoff allow. A manifest it cannot
// read counts as changed, so the build reports why.
//
// A manual request also resets the failure backoff; automatic ones wait it
// out. A manual request after a failed build that the running retry covers
// is replayed if that retry fails (see retryIfFailed). Before the first
// snapshot nothing else happens, since the first query builds one from
// scratch, and after Close nothing happens.
//
// It never runs bd and never waits on a build: it reads the manifest (a
// few hundred bytes) and takes schedMu, which no build holds across its bd
// reads. It is called on the caller's goroutine (the UI's, a writer's, or
// the watcher's forwarding goroutine).
func (b *SnapshotBackend) requestRebuild(manual bool) {
	if manual {
		b.sched.mu.Lock()
		b.sched.consecutiveFailures = 0
		b.sched.mu.Unlock()
	}
	if b.current.Load() == nil {
		return
	}
	hash, err := hashManifest(b.manifestPath)

	s := &b.sched
	s.mu.Lock()
	needed := err != nil || !s.covers(hash)
	if needed {
		s.dirty = true
	} else if manual && s.running && s.lastErr != nil {
		// Covered only by the running retry (a failure cleared the
		// installed hash): retry again if it fails too.
		s.retryIfFailed = true
	}
	s.mu.Unlock()
	if needed {
		b.wakeScheduler()
	}
}

// covers reports whether the running build or the installed snapshot
// covers the manifest state with this hash, which is never empty. The
// caller holds mu.
func (s *scheduler) covers(hash string) bool {
	return (s.running && hash == s.inFlightH0) || hash == s.installedHash
}

// nextBuildAt is when the next build may start: lastEnd plus the longest of
// minRebuildGap, twice lastDuration and the failure backoff. It is the zero
// time before the first build. The caller holds mu.
func (s *scheduler) nextBuildAt() time.Time {
	if s.lastEnd.IsZero() {
		return time.Time{}
	}
	return s.lastEnd.Add(max(minRebuildGap, 2*s.lastDuration, failureBackoff(s.consecutiveFailures)))
}

// failureBackoff is the backoff after n consecutive failed builds: none
// after a success, then min(5s × 2^(n−1), 2min).
func failureBackoff(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	backoff := firstFailureBackoff
	for i := 1; i < n && backoff < maxFailureBackoff; i++ {
		backoff *= 2
	}
	return min(backoff, maxFailureBackoff)
}

// wakeScheduler signals the scheduler goroutine without blocking. A signal
// already pending covers this one.
func (b *SnapshotBackend) wakeScheduler() {
	select {
	case b.sched.wake <- struct{}{}:
	default:
	}
}

// deferredBuildErr returns the last build's error while pacing and the
// failure backoff defer the next build after a failure, and nil otherwise.
func (b *SnapshotBackend) deferredBuildErr() error {
	s := &b.sched
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr == nil || !b.clock.Now().Before(s.nextBuildAt()) {
		return nil
	}
	return s.lastErr
}

// rebuild runs one build and installs its snapshot. Both the lazy first
// build and the scheduler goroutine build through it, holding buildMu
// throughout, so builds never overlap:
//
//  1. clear dirty and mark the build running;
//  2. build (see build), publishing the manifest hash it read before its
//     export as inFlightH0;
//  3. install the snapshot, retiring the old one;
//  4. record the outcome and publish it on the Changes broker (see
//     finishBuild), which re-hashes the manifest after a success.
//
// It returns the build's or the install's error.
func (b *SnapshotBackend) rebuild(ctx context.Context) error {
	s := &b.sched
	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	start := b.clock.Now()
	s.mu.Lock()
	s.dirty = false
	s.running = true
	s.mu.Unlock()

	var h0 string
	next, err := b.build(ctx, func(manifestHash string) {
		h0 = manifestHash
		s.mu.Lock()
		s.inFlightH0 = manifestHash
		s.mu.Unlock()
	})
	if err == nil {
		err = b.install(next)
	}
	b.finishBuild(h0, start, err)
	return err
}

// finishBuild records the outcome of a build that started at start and
// read the manifest hash h0 (empty when it failed before), and publishes
// it on the Changes broker.
//
// After a success it records h0 as the installed hash, resets the failure
// state and publishes DBChanged. The first install then starts the
// scheduler goroutine and the manifest watcher (startLoops). Only then does
// it re-hash the manifest: when the hash is no longer h0, a write landed
// during the export (or before the watcher was watching) and the snapshot
// is marked dirty, so the scheduler follows up with no further trigger. A
// write after the re-hash reaches the watcher, so none is lost.
//
// After a failure the current snapshot keeps serving. It clears the
// installed hash (every later trigger then asks for a retry), records the
// error for the backoff, logs a failed first build and publishes
// WatcherError. A manual request that the failed build covered is replayed
// (see retryIfFailed). A build that failed because Close cancelled it is not
// counted and publishes nothing.
func (b *SnapshotBackend) finishBuild(h0 string, start time.Time, err error) {
	s := &b.sched
	end := b.clock.Now()
	if err != nil {
		b.finishFailedBuild(start, end, err)
		return
	}

	s.mu.Lock()
	s.inFlightH0 = ""
	s.installedHash = h0
	s.lastEnd, s.lastDuration = end, end.Sub(start)
	s.lastErr = nil
	s.consecutiveFailures = 0
	s.retryIfFailed = false
	s.mu.Unlock()
	b.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.DBChanged})

	// Watch first, then re-hash: a write that lands before the watch is
	// registered is caught by the re-hash, one after it by the watcher.
	b.startLoops()

	h1, hashErr := hashManifest(b.manifestPath)
	s.mu.Lock()
	if hashErr != nil || h1 != h0 {
		s.dirty = true
	}
	dirty := s.dirty
	s.running = false
	s.mu.Unlock()
	if dirty {
		b.wakeScheduler()
	}
}

// finishFailedBuild is finishBuild after a build that failed with err.
func (b *SnapshotBackend) finishFailedBuild(start, end time.Time, err error) {
	s := &b.sched
	closing := b.closing()
	s.mu.Lock()
	s.running = false
	s.inFlightH0 = ""
	s.lastEnd, s.lastDuration = end, end.Sub(start)
	replay := s.retryIfFailed && !closing
	s.retryIfFailed = false
	if !closing {
		s.installedHash = ""
		s.lastErr = err
		s.consecutiveFailures++
		if replay {
			// The manual request, as if made right after this failure.
			s.consecutiveFailures = 0
			s.dirty = true
		}
	}
	failures := s.consecutiveFailures
	s.mu.Unlock()
	if closing {
		return
	}
	if b.current.Load() == nil {
		// The lazy first build (a query or CheckCompatibility); the
		// scheduler goroutine logs its own rebuilds' failures.
		log.Warn(log.CatBeads, "Building the first snapshot failed", "error", err, "consecutiveFailures", failures)
	}
	b.changes.Publish(pubsub.UpdatedEvent, watcher.WatcherEvent{Type: watcher.WatcherError, Error: err})
	if replay {
		b.wakeScheduler()
	}
}

// closing reports whether Close has been called. Close cancels the
// backend's context with errSnapshotBackendClosed before anything else, and
// a bd read it kills fails with that cause rather than context.Canceled.
func (b *SnapshotBackend) closing() bool {
	return errors.Is(context.Cause(b.ctx), errSnapshotBackendClosed)
}

// runScheduler is the scheduler goroutine. While the snapshot is dirty, it
// waits until nextBuildAt, then rebuilds; a build that leaves the snapshot
// dirty (a change during its export) loops. When it is not dirty it waits
// for a wake. A wake while it waits on a deadline recomputes it, since
// FlushCaches may have reset the backoff. It returns when Close cancels the
// backend's context.
func (b *SnapshotBackend) runScheduler() {
	s := &b.sched
	for b.ctx.Err() == nil {
		s.mu.Lock()
		dirty, due := s.dirty, s.nextBuildAt()
		s.mu.Unlock()

		var timer <-chan time.Time
		if dirty {
			if !b.clock.Now().Before(due) {
				if err := b.rebuild(b.ctx); err != nil && !b.closing() {
					s.mu.Lock()
					failures := s.consecutiveFailures
					s.mu.Unlock()
					log.Warn(log.CatBeads, "Rebuilding the snapshot failed; queries use the previous snapshot",
						"error", err, "consecutiveFailures", failures)
				}
				continue
			}
			timer = b.clock.WaitUntil(due)
		}
		select {
		case <-timer:
		case <-s.wake:
		case <-b.ctx.Done():
		}
	}
}

// startLoops starts the scheduler goroutine and the manifest watcher, once,
// after the first snapshot is installed. After Close it does nothing. A
// watcher that fails to start is logged: perles' own writes and
// FlushCaches still trigger rebuilds.
func (b *SnapshotBackend) startLoops() {
	s := &b.sched
	s.loopMu.Lock()
	defer s.loopMu.Unlock()
	if s.started || s.stopped {
		return
	}
	s.started = true
	s.loops.Go(b.runScheduler)

	w, err := s.newWatcher()
	if err != nil {
		log.Warn(log.CatBeads, "Watching the embedded Dolt manifest failed; changes made outside perles need a manual refresh",
			"manifest", b.manifestPath, "error", err)
		return
	}
	s.watch = w
	// Subscribed here, not in the goroutine, so no event is missed. The
	// subscription ends when Close cancels b.ctx or stops the watcher.
	events := w.Broker().Subscribe(b.ctx)
	s.loops.Go(func() { b.forwardManifestEvents(events) })
}

// forwardManifestEvents passes the watcher's debounced manifest events to
// the hash gate (T1) until the subscription ends.
func (b *SnapshotBackend) forwardManifestEvents(events <-chan pubsub.Event[watcher.WatcherEvent]) {
	for event := range events {
		switch event.Payload.Type {
		case watcher.DBChanged:
			b.requestRebuild(false)
		case watcher.WatcherError:
			log.Warn(log.CatBeads, "Embedded Dolt manifest watcher error", "error", event.Payload.Error)
		}
	}
}

// stopLoops stops the manifest watcher and waits for the scheduler
// goroutine and the forwarding goroutine to return. Close calls it after
// cancelling the backend's context, which ends a running build's bd reads,
// so the wait is short. Loops are never started afterwards.
func (b *SnapshotBackend) stopLoops() error {
	s := &b.sched
	s.loopMu.Lock()
	s.stopped = true
	w := s.watch
	s.watch = nil
	s.loopMu.Unlock()

	var err error
	if w != nil {
		if stopErr := w.Stop(); stopErr != nil {
			err = fmt.Errorf("stopping the embedded Dolt manifest watcher: %w", stopErr)
		}
	}
	s.loops.Wait()
	return err
}

// watchManifest creates and starts the internal watcher on the manifest.
// It watches the noms directory for the manifest's name, so it sees bd's
// rename-replaced manifests (a new inode each time, reported as Create).
// Its maximum wait keeps it reporting under sustained writes (see
// manifestMaxWait).
func (b *SnapshotBackend) watchManifest() (manifestWatcher, error) {
	w, err := watcher.New(watcher.Config{
		DBPath:        b.manifestPath,
		RelevantFiles: []string{filepath.Base(b.manifestPath)},
		DebounceDur:   manifestDebounce,
		MaxWait:       manifestMaxWait,
	})
	if err != nil {
		return nil, err
	}
	if err := w.Start(); err != nil {
		if stopErr := w.Stop(); stopErr != nil {
			err = errors.Join(err, stopErr)
		}
		return nil, err
	}
	return w, nil
}
