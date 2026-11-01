package adapter

// Opt-in contention benchmark: the SnapshotBackend's lock budget on a real
// embedded workspace (the proposal's E24 and its "Lock budget" acceptance
// criterion).
//
// An embedded Dolt database is guarded by an exclusive flock on its
// noms/LOCK, and bd waits for that lock with no time limit. Perles never
// holds the engine open: a snapshot build runs bounded --readonly bd reads,
// and builds are paced (minRebuildGap). The benchmark measures what that
// costs an agent writing to the workspace. In a 5,000-issue workspace it
// times 30 external `bd update <id> --priority=N` runs, one about every 0.8s,
// first with no perles (the baseline), then again while a SnapshotBackend
// built the way NewBeadsBackend builds it, and pre-built by
// CheckCompatibility, rebuilds under those writes.
//
// Perles rebuilds as it does for a user: the benchmark asks for no rebuild
// itself, so the manifest watcher (T1) and the post-install re-hash are the
// only triggers. bd rewrites the manifest throughout each update, so updates
// every 0.8s that take about 0.5s never leave the watcher's 500ms debounce a
// quiet gap; the watcher reports them through its maximum wait
// (manifestMaxWait, 2s), and paced rebuilds follow. (Before the maximum
// wait, perles-8q4, the watcher fell silent under these writes, 4 events in
// 29s and none in the last 12s, and the benchmark asked for a rebuild after
// each write itself.)
//
// It asserts:
//
//   - external bd update p95 <= baseline p95 + 2.0s, and max <= 10s;
//   - at least two rebuilds overlapped the writes (a build still running
//     when the writes end is waited for, and counts);
//   - between rebuilds, lsof shows no process the test process spawned,
//     other than the external writer's bd, with noms/LOCK open, no bd or
//     dolt process that it cannot trace to the external writer (an orphaned
//     perles bd, say), nor the test process itself with it open for writing
//     (see classify for its read-only manifest-watcher descriptor on macOS);
//   - at least one of those lsof checks followed an install that ended once
//     the writes had started, and, on macOS, where that descriptor is always
//     there, they listed some holder;
//   - first, as a positive control, lsof lists a child of the test process
//     holding noms/LOCK open, and classify counts it as perles'.
//
// It logs the latencies (p50/p95/max), each rebuild's duration and the idle
// gap before it, the time spent in each bd read, and the machine.
//
// It runs real bd writes in a throwaway workspace and takes a minute or two,
// so it is skipped unless PERLES_BD_CONTENTION=1 is set: `make test` and CI
// never run it. Run it with:
//
//	PERLES_BD_CONTENTION=1 go test ./internal/beads/adapter/ -run Contention -count=1 -v
//
// Once opted in, it skips, with a logged reason, only when:
//
//   - -short is set;
//   - bd is not on PATH;
//   - the installed bd is older than domain.MinEmbeddedBeadsVersion (1.0.5),
//     checked with the production gate (infrabeads.CheckEmbeddedBDVersion);
//   - bd refuses the workspace location as an "unsafe location".
//
// A `bd --version` it cannot parse fails the test, as does any other failure.
// The lsof checks alone are skipped, with a logged reason, when lsof is not on
// PATH.
//
// Safety: the workspace is a fresh os.MkdirTemp directory under the user's
// home, removed by t.Cleanup (the removal uses the path MkdirTemp returned,
// before any symlink resolution). Every BEADS_* and BD_* variable is cleared
// and BEADS_DIR is set to the workspace's .beads for the test, and every bd
// runs with BEADS_DIR set to it and its working directory in the workspace.
// Before bd init, bd context must not resolve any other beads directory.
// After it, and again before the import and before each round of writes, bd
// context must report exactly the workspace's own embedded beads dir, or the
// test stops. This repository's .beads is never touched.
//
// The workspace comes from the integration tests' helpers
// (snapshot_integration_test.go): integrationInitWorkspace, with the bd
// check of integrationBDVersion and the safety guard of requireOwnContext,
// then the workspace's backend and bd helpers and integrationBDEnv. The
// helpers below, the opt-in gate included, are this file's own and prefixed
// contention.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/watcher"
)

// contentionEnv opts in to the contention benchmark.
const contentionEnv = "PERLES_BD_CONTENTION"

// The benchmark's workload and lock budget.
const (
	// contentionIssues is the size of the workspace.
	contentionIssues = 5000
	// contentionUpdates is the number of external bd updates per round.
	contentionUpdates = 30
	// contentionCadence is the time from the start of one external bd
	// update to the start of the next; an update that takes longer is
	// followed at once.
	contentionCadence = 800 * time.Millisecond
	// contentionP95Slack is how much perles may add to the external
	// updates' p95 latency.
	contentionP95Slack = 2 * time.Second
	// contentionMaxLatency bounds every external update while perles
	// rebuilds.
	contentionMaxLatency = 10 * time.Second
	// contentionMinRebuilds is the least number of rebuilds that must land
	// during the second round for it to measure anything.
	contentionMinRebuilds = 2
	// contentionIdleWait bounds the wait, after an install, for the build's
	// bookkeeping to end before the lsof check.
	contentionIdleWait = 2 * time.Second
)

// contentionPriorities are the priorities the external updates set, in
// turn. The workspace's issues are imported at priority 2, so every update
// is a real write.
var contentionPriorities = [...]int{0, 1, 3, 4}

// TestSnapshotBackend_Contention_LockBudget is the benchmark described in
// the file comment.
func TestSnapshotBackend_Contention_LockBudget(t *testing.T) {
	w := integrationInitWorkspace(t, "perles-embed-bench-*", "cnt", contentionRequireBD(t))
	ids := contentionImport(t, w)
	writer := &contentionWriter{w: w, pids: map[int]bool{}}

	// Round 1, the baseline: no perles.
	w.requireOwnContext(t, true)
	baseline := writer.updates(t, ids[:contentionUpdates])

	// Round 2: as many writes again, to other issues, while perles rebuilds.
	tracker := &contentionTracker{run: w.run, durations: map[string][]time.Duration{}}
	b, _ := w.backend(t, tracker.runner)
	lockPath := filepath.Join(filepath.Dir(b.manifestPath), "LOCK")
	require.FileExists(t, lockPath)
	m := contentionStartMonitor(t, b, tracker, writer, lockPath)
	if m.lsof != "" {
		m.control(t)
	}
	require.NoError(t, b.CheckCompatibility(), "the pre-build")
	require.Len(t, integrationQuery(t, b, `id != ""`), contentionIssues, "the snapshot holds every issue")

	w.requireOwnContext(t, true)
	windowStart := time.Now()
	loaded := writer.updates(t, ids[contentionUpdates:2*contentionUpdates])
	windowEnd := time.Now()
	// A build still running now overlapped the writes too: let it install,
	// so that it counts.
	inFlight := m.awaitBuild(t)
	m.stop()

	m.mu.Lock()
	builds := slices.Clone(m.builds)
	failures := slices.Clone(m.failures)
	m.mu.Unlock()
	require.NotEmpty(t, builds, "the pre-build installed a snapshot")
	pre, rebuilds := builds[0], builds[1:]
	during := contentionOverlapping(rebuilds, windowStart, windowEnd)

	base, load := contentionSummarize(baseline), contentionSummarize(loaded)
	t.Logf("machine: %s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	t.Logf("baseline bd update (%d runs, no perles):      %s", len(baseline), base)
	t.Logf("bd update while perles rebuilds (%d runs):    %s", len(loaded), load)
	t.Logf("p95 budget: <= %s (baseline p95 + %s); max budget: <= %s",
		contentionSeconds(base.p95+contentionP95Slack), contentionSeconds(contentionP95Slack), contentionSeconds(contentionMaxLatency))
	t.Logf("pre-build (CheckCompatibility): %s; the writes started %s after it ended",
		contentionSeconds(pre.duration()), contentionSeconds(windowStart.Sub(pre.end)))
	if inFlight {
		t.Log("a build was running when the writes ended; the benchmark waited for it")
	}
	contentionLogRebuilds(t, pre, rebuilds, windowStart, windowEnd)
	contentionLogReads(t, tracker)

	require.Empty(t, failures, "no snapshot build failed")
	require.GreaterOrEqual(t, len(during), contentionMinRebuilds, "perles rebuilt while the writes ran")

	t.Run("lock budget", func(t *testing.T) {
		require.LessOrEqual(t, load.p95, base.p95+contentionP95Slack,
			"external bd update p95 %s exceeds the baseline p95 %s + %s", load.p95, base.p95, contentionP95Slack)
		require.LessOrEqual(t, load.max, contentionMaxLatency,
			"an external bd update took %s while perles rebuilt", load.max)
	})

	t.Run("no perles process holds noms/LOCK between rebuilds", func(t *testing.T) {
		if m.lsof == "" {
			t.Skipf("lsof is not available, so the noms/LOCK holders cannot be listed: %v", m.lsofErr)
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		// Only the checks after installs that ended once the writes had
		// started count: the pre-build's own check runs before any write.
		var checks, holders, writers int
		for _, c := range m.checks {
			if !c.installEnd.Before(windowStart) {
				checks++
				holders += c.holders
				writers += c.writers
			}
		}
		t.Logf("lsof checks between rebuilds: %d, %d of them after installs that ended once the writes had started "+
			"(discarded because perles started bd during the check: %d); noms/LOCK holders those listed: %d, "+
			"of which the external writer's bd processes: %d", len(m.checks), checks, m.discarded, holders, writers)
		for _, note := range m.notes {
			t.Log(note)
		}
		require.Empty(t, m.checkErrs, "every lsof check ran")
		require.Empty(t, m.violations)
		require.Positive(t, checks, "at least one check, after an install that ended once the writes had started, ran while perles ran no bd")
		if runtime.GOOS == "darwin" {
			// The manifest watcher's kqueue descriptor (see classify) is
			// there in every check, so an empty listing means lsof missed it.
			require.Positive(t, holders, "on macOS lsof lists noms/LOCK's holders during the writes")
		}
	})
}

// contentionRequireBD skips the benchmark unless it is opted in and not
// -short, then checks the installed bd (see integrationBDVersion) and
// returns its version.
func contentionRequireBD(t *testing.T) string {
	t.Helper()
	if os.Getenv(contentionEnv) != "1" {
		t.Skipf("set %s=1 to run the contention benchmark (it runs bd writes in a throwaway 5,000-issue workspace under your home directory)", contentionEnv)
	}
	if testing.Short() {
		t.Skip("-short: skipping the contention benchmark")
	}
	return integrationBDVersion(t)
}

// contentionImport imports contentionIssues issues into w with one bd
// import, shaped like the proposal's E24 workspace: labels on every issue,
// metadata on every fifth, a blocks edge on every tenth (500) and a comment
// on every fifth (1,000). All are open at priority 2. It returns their IDs,
// in order.
func contentionImport(t *testing.T, w *integrationWorkspace) []string {
	t.Helper()
	ids := make([]string, contentionIssues)
	var jsonl strings.Builder
	for i := range ids {
		ids[i] = fmt.Sprintf("%s-b%04d", w.prefix, i+1)
		record := map[string]any{
			"id":          ids[i],
			"title":       fmt.Sprintf("contention issue %d", i+1),
			"description": strings.Repeat("Contention benchmark issue body. ", 6),
			"issue_type":  "task",
			"priority":    2,
			"status":      "open",
			"labels":      []string{"bench", fmt.Sprintf("area-%d", i%10)},
		}
		if i%5 == 0 {
			record["metadata"] = map[string]any{"team": fmt.Sprintf("team-%d", i%3)}
		}
		if i%10 == 9 {
			record["dependencies"] = []map[string]any{{"issue_id": ids[i], "depends_on_id": ids[i-1], "type": "blocks"}}
		}
		if i%5 == 1 {
			record["comments"] = []map[string]any{{"author": "perles-bench", "text": "comment on " + ids[i]}}
		}
		line, err := json.Marshal(record)
		require.NoError(t, err)
		jsonl.Write(line)
		jsonl.WriteByte('\n')
	}

	w.requireOwnContext(t, true)
	start := time.Now()
	w.bdRequest(t, infrabeads.BDRequest{Args: []string{"import", "-"}, Stdin: jsonl.String(), Timeout: integrationTimeout})
	t.Logf("imported %d issues in %s", contentionIssues, contentionSeconds(time.Since(start)))
	return ids
}

// contentionWriter is the external writer: an agent's bd updates in the
// workspace, outside perles. It remembers their PIDs for the lsof check.
type contentionWriter struct {
	w *integrationWorkspace

	mu   sync.Mutex
	pids map[int]bool
}

// updates runs `bd update <id> --priority=N` for each of ids, one every
// contentionCadence, and returns how long each update took.
func (cw *contentionWriter) updates(t *testing.T, ids []string) []time.Duration {
	t.Helper()
	latencies := make([]time.Duration, 0, len(ids))
	next := time.Now()
	for i, id := range ids {
		if wait := time.Until(next); wait > 0 {
			time.Sleep(wait)
		}
		start := time.Now()
		next = start.Add(contentionCadence)
		cw.update(t, id, contentionPriorities[i%len(contentionPriorities)])
		latencies = append(latencies, time.Since(start))
	}
	return latencies
}

// update runs one external `bd update id --priority=priority` in the
// workspace.
func (cw *contentionWriter) update(t *testing.T, id string, priority int) {
	t.Helper()
	require.True(t, cw.w.verified, "SAFETY: bd context has not confirmed the workspace")
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "update", id, fmt.Sprintf("--priority=%d", priority)) //nolint:gosec // G204: fixed arguments
	cmd.Dir = cw.w.dir
	cmd.Env = integrationBDEnv(cw.w.beadsDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	// Start under the lock: an lsof check that lists this bd before its PID
	// is recorded waits for it in spawned.
	cw.mu.Lock()
	err := cmd.Start()
	if err == nil {
		cw.pids[cmd.Process.Pid] = true
	}
	cw.mu.Unlock()
	require.NoError(t, err, "starting bd update %s", id)
	require.NoError(t, cmd.Wait(), "bd update %s: %s", id, strings.TrimSpace(stderr.String()))
}

// spawned reports whether any of pids is one of the external writer's bd
// processes.
func (cw *contentionWriter) spawned(pids []int) bool {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return slices.ContainsFunc(pids, func(pid int) bool { return cw.pids[pid] })
}

// contentionTracker wraps the backend's bd runner: it tells when perles runs
// no bd, and times each bd read.
type contentionTracker struct {
	run infrabeads.BDRunner

	mu        sync.Mutex
	inFlight  int
	started   int
	durations map[string][]time.Duration // by bd command
}

func (c *contentionTracker) runner(ctx context.Context, req infrabeads.BDRequest) (string, error) {
	c.mu.Lock()
	c.inFlight++
	c.started++
	c.mu.Unlock()
	start := time.Now()
	out, err := c.run(ctx, req)
	elapsed := time.Since(start)
	c.mu.Lock()
	c.inFlight--
	c.durations[bdCommand(req)] = append(c.durations[bdCommand(req)], elapsed)
	c.mu.Unlock()
	return out, err
}

// idle returns how many bd commands perles has started, and whether none of
// them is running.
func (c *contentionTracker) idle() (started int, idle bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started, c.inFlight == 0
}

// contentionBuild is one snapshot build that was installed.
type contentionBuild struct{ start, end time.Time }

func (b contentionBuild) duration() time.Duration { return b.end.Sub(b.start) }

// contentionOverlapping returns the builds that ran during [start, end).
func contentionOverlapping(builds []contentionBuild, start, end time.Time) []contentionBuild {
	var during []contentionBuild
	for _, b := range builds {
		if b.start.Before(end) && b.end.After(start) {
			during = append(during, b)
		}
	}
	return during
}

// contentionCheck is one lsof check of noms/LOCK made while perles ran no
// bd.
type contentionCheck struct {
	// installEnd is when the build whose install the check followed ended.
	installEnd time.Time
	// holders counts the processes lsof listed with noms/LOCK open; writers
	// those of them that were the external writer's bd processes, or their
	// descendants.
	holders, writers int
}

// contentionMonitor watches the backend's Changes broker. For each install
// it records the build's start and end, from the scheduler's own
// bookkeeping, then checks with lsof that no process perles spawned holds
// noms/LOCK. It records every failed build.
type contentionMonitor struct {
	b        *SnapshotBackend
	tracker  *contentionTracker
	writer   *contentionWriter
	lockPath string
	// lsof is lsof's path, empty when it is not on PATH (lsofErr says why).
	lsof    string
	lsofErr error
	// bdCommands are the command names of bd and Dolt processes (see
	// contentionBDCommands).
	bdCommands map[string]bool

	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	builds   []contentionBuild
	failures []error
	// checks are the lsof checks made while perles ran no bd; discarded
	// counts those thrown away because perles started bd during them.
	checks     []contentionCheck
	discarded  int
	checkErrs  []error
	violations []string
	// notes describe the holders that break nothing, once each: processes
	// other than bd or Dolt that this test did not spawn, and the test
	// process's read-only descriptor.
	notes []string
}

// contentionStartMonitor subscribes to b's Changes broker and starts
// watching it until stop, which also runs at cleanup.
func contentionStartMonitor(t *testing.T, b *SnapshotBackend, tracker *contentionTracker, writer *contentionWriter, lockPath string) *contentionMonitor {
	t.Helper()
	m := &contentionMonitor{
		b: b, tracker: tracker, writer: writer, lockPath: lockPath,
		bdCommands: contentionBDCommands(t), done: make(chan struct{}),
	}
	m.lsof, m.lsofErr = exec.LookPath("lsof")
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	events := b.Changes().Subscribe(ctx)
	go func() {
		defer close(m.done)
		for event := range events {
			switch event.Payload.Type {
			case watcher.DBChanged:
				if build, ok := m.recordBuild(); ok && m.lsof != "" {
					m.checkLock(ctx, build)
				}
			case watcher.WatcherError:
				m.mu.Lock()
				m.failures = append(m.failures, event.Payload.Error)
				m.mu.Unlock()
			}
		}
	}()
	t.Cleanup(m.stop)
	return m
}

// contentionBDCommands returns the command names that lsof lists for bd and
// Dolt processes: bd and dolt, and the base names of the bd on PATH and of
// the file it resolves to.
func contentionBDCommands(t *testing.T) map[string]bool {
	t.Helper()
	path, err := exec.LookPath("bd")
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return map[string]bool{"bd": true, "dolt": true, filepath.Base(path): true, filepath.Base(resolved): true}
}

// stop ends the subscription and waits for the monitor to return.
func (m *contentionMonitor) stop() {
	m.cancel()
	<-m.done
}

// control is the lock check's positive control, run before perles starts:
// lsof must list a child of the test process holding noms/LOCK open, with
// its parent, command and access mode, and classify must count it as
// perles'. The child's stdin is a read-only descriptor of the test process,
// which lsof must list as well, and which classify must not count.
func (m *contentionMonitor) control(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	lock, err := os.Open(m.lockPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, lock.Close()) }()
	child := exec.CommandContext(ctx, "sleep", "60")
	child.Stdin = lock
	require.NoError(t, child.Start())
	defer func() {
		require.NoError(t, child.Process.Kill())
		_, killed := errors.AsType[*exec.ExitError](child.Wait())
		require.True(t, killed, "the control's sleep ends killed")
	}()

	holders, err := contentionLockHolders(ctx, m.lsof, m.lockPath)
	require.NoError(t, err)
	t.Logf("positive control: lsof lists noms/LOCK's holders %+v", holders)
	var sawChild, sawSelf bool
	for _, h := range holders {
		perles, why := m.classify(ctx, h)
		switch h.pid {
		case child.Process.Pid:
			sawChild = true
			require.Equal(t, os.Getpid(), h.ppid, "lsof lists the child's parent")
			require.Equal(t, "sleep", h.command, "lsof lists the child's command")
			require.Equal(t, []string{"r"}, h.access, "lsof lists the child's read-only descriptor")
			require.True(t, perles, "a holder the test process spawned holds noms/LOCK for perles: %s", why)
		case os.Getpid():
			sawSelf = true
			require.False(t, perles, "the test process's read-only descriptor locks nothing: %s", why)
		}
	}
	require.True(t, sawChild, "positive control: lsof lists the test process's child holding noms/LOCK")
	require.True(t, sawSelf, "positive control: lsof lists the test process's own descriptor")
}

// awaitBuild waits, up to integrationPickupTimeout in all, for the build
// running now, if any, to end, then for the monitor to record the last
// build's outcome: its install, or its failure. It reports whether a build
// was running.
func (m *contentionMonitor) awaitBuild(t *testing.T) (running bool) {
	t.Helper()
	s := &m.b.sched
	s.mu.Lock()
	running = s.running
	s.mu.Unlock()

	deadline := time.Now().Add(integrationPickupTimeout)
	var lastEnd time.Time
	var failed bool
	contentionPoll(t, deadline, "the build running when the writes ended finishes", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		lastEnd, failed = s.lastEnd, s.lastErr != nil
		return !s.running
	})
	contentionPoll(t, deadline, "the monitor records the last build", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		if failed {
			return len(m.failures) > 0
		}
		n := len(m.builds)
		return n > 0 && !m.builds[n-1].end.Before(lastEnd)
	})
	return running
}

// contentionPoll calls done every 10ms until it returns true, and fails the
// test if it has not by deadline.
func contentionPoll(t *testing.T, deadline time.Time, what string, done func() bool) {
	t.Helper()
	for !done() {
		require.True(t, time.Now().Before(deadline), "timed out waiting: %s", what)
		time.Sleep(10 * time.Millisecond)
	}
}

// recordBuild records the build that was just installed, as the scheduler
// timed it, and returns it, unless it was recorded already. Builds are paced
// seconds apart, so the scheduler's last build is still the installed one.
func (m *contentionMonitor) recordBuild() (contentionBuild, bool) {
	s := &m.b.sched
	s.mu.Lock()
	end, duration := s.lastEnd, s.lastDuration
	s.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := len(m.builds); n > 0 && m.builds[n-1].end.Equal(end) {
		return contentionBuild{}, false
	}
	build := contentionBuild{start: end.Add(-duration), end: end}
	m.builds = append(m.builds, build)
	return build, true
}

// checkLock lists the processes with noms/LOCK open once build, which was
// just installed, has finished its bookkeeping, while perles runs no bd. A
// check during which perles started bd is discarded.
func (m *contentionMonitor) checkLock(ctx context.Context, build contentionBuild) {
	before, ok := m.waitIdle(ctx)
	if !ok {
		m.discard()
		return
	}
	holders, err := contentionLockHolders(ctx, m.lsof, m.lockPath)
	if ctx.Err() != nil {
		return // stopped
	}
	if after, idle := m.tracker.idle(); after != before || !idle {
		m.discard()
		return
	}
	if err != nil {
		m.mu.Lock()
		m.checkErrs = append(m.checkErrs, err)
		m.mu.Unlock()
		return
	}
	var violations, notes []string
	writers := 0
	for _, h := range holders {
		switch perles, why := m.classify(ctx, h); {
		case perles:
			violations = append(violations, why)
		case why != "":
			notes = append(notes, why)
		default:
			writers++
		}
	}
	m.mu.Lock()
	m.checks = append(m.checks, contentionCheck{installEnd: build.end, holders: len(holders), writers: writers})
	m.violations = append(m.violations, violations...)
	for _, note := range notes {
		if !slices.Contains(m.notes, note) {
			m.notes = append(m.notes, note)
		}
	}
	m.mu.Unlock()
}

// waitIdle waits, up to contentionIdleWait, until no build is running and
// perles runs no bd, and returns how many bd commands perles had started
// then.
func (m *contentionMonitor) waitIdle(ctx context.Context) (started int, ok bool) {
	deadline := time.Now().Add(contentionIdleWait)
	for {
		m.b.sched.mu.Lock()
		running := m.b.sched.running
		m.b.sched.mu.Unlock()
		started, idle := m.tracker.idle()
		if idle && !running {
			return started, true
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		select {
		case <-ctx.Done():
			return 0, false
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (m *contentionMonitor) discard() {
	m.mu.Lock()
	m.discarded++
	m.mu.Unlock()
}

// classify reports whether holder, a process with noms/LOCK open between two
// builds, holds it for perles, and describes it. Perles' holders are the
// test process itself with LOCK open for writing, any process it spawned
// other than the external writer's bd, and any bd or Dolt process that
// cannot be traced to the external writer: only the writer and perles run
// bd in this private workspace, so an untraceable one is most likely a
// perles bd that outlived its parent (its ancestry then reaches pid 1). An
// external writer's bd, or a descendant of one, is fine and not described.
// Any other process this test did not spawn, and the test process with LOCK
// open read-only, are fine and described.
//
// lsof cannot show flock locks on every platform (macOS shows none), so the
// test process's descriptors are told apart by access mode: Dolt opens LOCK
// read-write to lock it (bd's descriptors list as "u"), while on macOS the
// backend's manifest watcher (fsnotify's kqueue) opens every file in noms/,
// LOCK included, read-only and event-only to watch it, which locks nothing.
func (m *contentionMonitor) classify(ctx context.Context, h contentionLockHolder) (perles bool, why string) {
	self := os.Getpid()
	if h.pid == self {
		if h.writable() {
			return true, fmt.Sprintf("the test process itself (pid %d, %s) has noms/LOCK open for writing (access %q) between rebuilds",
				h.pid, h.command, h.access)
		}
		return false, fmt.Sprintf("the test process itself (pid %d, %s) has noms/LOCK open read-only (access %q): "+
			"the manifest watcher's descriptor (kqueue watches every file in noms/), which locks nothing", h.pid, h.command, h.access)
	}
	// The holder and its ancestors below the test process.
	chain := []int{h.pid}
	parent := h.ppid
	for len(chain) < 32 && parent > 1 && parent != self {
		chain = append(chain, parent)
		next, err := contentionParentPID(ctx, parent)
		if err != nil {
			parent = 0 // the ancestor has exited: not traceable to the test
			break
		}
		parent = next
	}
	if m.writer.spawned(chain) {
		return false, ""
	}
	if parent == self {
		return true, fmt.Sprintf("pid %d (%s, ancestry %v up to the test process %d) has noms/LOCK open between rebuilds", h.pid, h.command, chain, self)
	}
	if m.bdCommands[h.command] {
		return true, fmt.Sprintf("pid %d (%s, ancestry %v, not traceable to the external writer) has noms/LOCK open between rebuilds",
			h.pid, h.command, chain)
	}
	return false, fmt.Sprintf("pid %d (%s, ancestry %v) has noms/LOCK open; this test did not spawn it", h.pid, h.command, chain)
}

// contentionLockHolder is a process with noms/LOCK open, as lsof lists it.
type contentionLockHolder struct {
	pid, ppid int
	command   string
	// access is the access mode of each of its descriptors on the file:
	// "r" read, "w" write, "u" read and write, " " unknown.
	access []string
}

// writable reports whether the holder has the file open for writing, or in
// a mode lsof could not tell, or lsof listed no access mode for it at all
// (conservatively).
func (h contentionLockHolder) writable() bool {
	return len(h.access) == 0 || slices.ContainsFunc(h.access, func(mode string) bool { return mode != "r" })
}

// contentionLockHolders lists the processes that have path open.
func contentionLockHolders(ctx context.Context, lsof, path string) ([]contentionLockHolder, error) {
	cmd := exec.CommandContext(ctx, lsof, "-n", "-P", "-w", "-F", "pRca", "--", path) //nolint:gosec // G204: lsof on PATH, fixed arguments
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// lsof exits 1, printing nothing, when no process has path open.
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 &&
			stdout.Len() == 0 && strings.TrimSpace(stderr.String()) == "" {
			return nil, nil
		}
		return nil, fmt.Errorf("lsof %s: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}

	// -F output: a "p<pid>" line starts each process, followed by its
	// "R<ppid>" and "c<command>" lines, then one "f<fd>" line per
	// descriptor, each followed by its "a<access mode>" line.
	var holders []contentionLockHolder
	for line := range strings.Lines(stdout.String()) {
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		field, value := line[0], line[1:]
		if field == 'p' {
			pid, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("parsing lsof's PID %q: %w", value, err)
			}
			holders = append(holders, contentionLockHolder{pid: pid})
			continue
		}
		if len(holders) == 0 {
			return nil, fmt.Errorf("lsof printed %q before any process", line)
		}
		h := &holders[len(holders)-1]
		switch field {
		case 'R':
			ppid, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("parsing lsof's PPID %q: %w", value, err)
			}
			h.ppid = ppid
		case 'c':
			h.command = value
		case 'a':
			h.access = append(h.access, value)
		}
	}
	return holders, nil
}

// contentionParentPID returns pid's parent's PID, from ps.
func contentionParentPID(ctx context.Context, pid int) (int, error) {
	out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, fmt.Errorf("ps -p %d: %w", pid, err)
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// contentionStats summarizes latencies.
type contentionStats struct{ p50, p95, max time.Duration }

func (s contentionStats) String() string {
	return fmt.Sprintf("p50 %s / p95 %s / max %s", contentionSeconds(s.p50), contentionSeconds(s.p95), contentionSeconds(s.max))
}

// contentionSummarize returns the p50, p95 (nearest rank) and max of
// durations, which must not be empty.
func contentionSummarize(durations []time.Duration) contentionStats {
	sorted := slices.Sorted(slices.Values(durations))
	return contentionStats{
		p50: contentionPercentile(sorted, 50),
		p95: contentionPercentile(sorted, 95),
		max: sorted[len(sorted)-1],
	}
}

// contentionPercentile is the nearest-rank p-th percentile of sorted: the
// smallest value that at least p% of the values do not exceed.
func contentionPercentile(sorted []time.Duration, p int) time.Duration {
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank, 1)-1]
}

// contentionSeconds formats d in seconds with two decimals.
func contentionSeconds(d time.Duration) string {
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// contentionLogRebuilds logs each rebuild's duration and the idle time
// before it, marking those that did not overlap the write window [start,
// end), then the overlapping rebuilds' summary and perles' share of the
// window spent building.
func contentionLogRebuilds(t *testing.T, pre contentionBuild, rebuilds []contentionBuild, start, end time.Time) {
	t.Helper()
	window := end.Sub(start)
	previous := pre
	var durations []time.Duration
	var building time.Duration
	for i, r := range rebuilds {
		when := ""
		switch {
		case !r.end.After(start):
			when = " (before the writes)"
		case !r.start.Before(end):
			when = " (after the writes)"
		default:
			durations = append(durations, r.duration())
			from, to := r.start, r.end
			if from.Before(start) {
				from = start
			}
			if to.After(end) {
				to = end
			}
			building += to.Sub(from)
		}
		paced := max(minRebuildGap, 2*previous.duration())
		t.Logf("rebuild %d%s: %s, after %s idle (pacing: >= %s)",
			i+1, when, contentionSeconds(r.duration()), contentionSeconds(r.start.Sub(previous.end)), contentionSeconds(paced))
		previous = r
	}
	if len(durations) == 0 {
		t.Logf("no rebuild during the %s write window", contentionSeconds(window))
		return
	}
	t.Logf("rebuilds while the writes ran: %d in %s, %s; building %.0f%% of the window",
		len(durations), contentionSeconds(window), contentionSummarize(durations), 100*building.Seconds()/window.Seconds())
}

// contentionLogReads logs the time perles spent in each bd read.
func contentionLogReads(t *testing.T, tracker *contentionTracker) {
	t.Helper()
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	for _, command := range slices.Sorted(maps.Keys(tracker.durations)) {
		durations := tracker.durations[command]
		t.Logf("bd %s: %d runs, %s", command, len(durations), contentionSummarize(durations))
	}
}
