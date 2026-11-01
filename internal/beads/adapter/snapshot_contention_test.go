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
//   - perles rebuilt at least twice while the writes ran;
//   - between rebuilds, lsof shows no process the test process spawned,
//     other than the external writer's bd, with noms/LOCK open, nor the test
//     process itself with it open for writing (see classify for its
//     read-only manifest-watcher descriptor on macOS).
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
// The lsof check alone is skipped, with a logged reason, when lsof is not on
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
// The workspace (its bd version check and safety guard included),
// environment and backend helpers are the integration tests'
// (snapshot_integration_test.go); the helpers below are prefixed contention.

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
	require.NoError(t, b.CheckCompatibility(), "the pre-build")
	require.Len(t, integrationQuery(t, b, `id != ""`), contentionIssues, "the snapshot holds every issue")

	w.requireOwnContext(t, true)
	windowStart := time.Now()
	loaded := writer.updates(t, ids[contentionUpdates:2*contentionUpdates])
	window := time.Since(windowStart)
	m.stop()

	m.mu.Lock()
	builds := slices.Clone(m.builds)
	failures := slices.Clone(m.failures)
	m.mu.Unlock()
	require.NotEmpty(t, builds, "the pre-build installed a snapshot")
	pre, rebuilds := builds[0], builds[1:]

	base, load := contentionSummarize(baseline), contentionSummarize(loaded)
	t.Logf("machine: %s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	t.Logf("baseline bd update (%d runs, no perles):      %s", len(baseline), base)
	t.Logf("bd update while perles rebuilds (%d runs):    %s", len(loaded), load)
	t.Logf("p95 budget: <= %s (baseline p95 + %s); max budget: <= %s",
		contentionSeconds(base.p95+contentionP95Slack), contentionSeconds(contentionP95Slack), contentionSeconds(contentionMaxLatency))
	t.Logf("pre-build (CheckCompatibility): %s; the writes started %s after it ended",
		contentionSeconds(pre.duration()), contentionSeconds(windowStart.Sub(pre.end)))
	contentionLogRebuilds(t, pre, rebuilds, window)
	contentionLogReads(t, tracker)

	require.Empty(t, failures, "no snapshot build failed")
	require.GreaterOrEqual(t, len(rebuilds), contentionMinRebuilds, "perles rebuilt while the writes ran")

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
		t.Logf("lsof checks between rebuilds: %d (discarded because perles started bd during the check: %d); "+
			"external writer bd processes seen holding noms/LOCK: %d", m.checks, m.discarded, m.writerSeen)
		for _, note := range m.notes {
			t.Log(note)
		}
		require.Empty(t, m.checkErrs, "every lsof check ran")
		require.Empty(t, m.violations)
		require.Positive(t, m.checks, "at least one check ran while perles ran no bd")
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
	require.NoError(t, cmd.Start(), "starting bd update %s", id)
	cw.mu.Lock()
	cw.pids[cmd.Process.Pid] = true
	cw.mu.Unlock()
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

	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	builds   []contentionBuild
	failures []error
	// checks counts the lsof checks made while perles ran no bd; discarded
	// those thrown away because perles started bd during them. writerSeen
	// counts the external writer's bd processes the checks found holding
	// noms/LOCK.
	checks, discarded, writerSeen int
	checkErrs                     []error
	violations                    []string
	// notes describe the holders that break nothing, once each: processes
	// this test did not spawn, and the test process's read-only descriptor.
	notes []string
}

// contentionStartMonitor subscribes to b's Changes broker and starts
// watching it until stop, which also runs at cleanup.
func contentionStartMonitor(t *testing.T, b *SnapshotBackend, tracker *contentionTracker, writer *contentionWriter, lockPath string) *contentionMonitor {
	t.Helper()
	m := &contentionMonitor{b: b, tracker: tracker, writer: writer, lockPath: lockPath, done: make(chan struct{})}
	m.lsof, m.lsofErr = exec.LookPath("lsof")
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	events := b.Changes().Subscribe(ctx)
	go func() {
		defer close(m.done)
		for event := range events {
			switch event.Payload.Type {
			case watcher.DBChanged:
				if m.recordBuild() && m.lsof != "" {
					m.checkLock(ctx)
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

// stop ends the subscription and waits for the monitor to return.
func (m *contentionMonitor) stop() {
	m.cancel()
	<-m.done
}

// recordBuild records the build that was just installed, as the scheduler
// timed it, and reports whether it is new. Builds are paced seconds apart,
// so the scheduler's last build is still the installed one.
func (m *contentionMonitor) recordBuild() bool {
	s := &m.b.sched
	s.mu.Lock()
	end, duration := s.lastEnd, s.lastDuration
	s.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := len(m.builds); n > 0 && m.builds[n-1].end.Equal(end) {
		return false
	}
	m.builds = append(m.builds, contentionBuild{start: end.Add(-duration), end: end})
	return true
}

// checkLock lists the processes with noms/LOCK open once the build that was
// just installed has finished its bookkeeping, while perles runs no bd. A
// check during which perles started bd is discarded.
func (m *contentionMonitor) checkLock(ctx context.Context) {
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
	m.checks++
	m.writerSeen += writers
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
// test process itself with LOCK open for writing, and any process it
// spawned other than the external writer's bd. An external writer's bd, or
// a descendant of one, is fine and not described. A process this test did
// not spawn, and the test process with LOCK open read-only, are fine and
// described.
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
	if parent != self {
		return false, fmt.Sprintf("pid %d (%s, ancestry %v) has noms/LOCK open; this test did not spawn it", h.pid, h.command, chain)
	}
	return true, fmt.Sprintf("pid %d (%s, ancestry %v up to the test process %d) has noms/LOCK open between rebuilds", h.pid, h.command, chain, self)
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
// a mode lsof could not tell.
func (h contentionLockHolder) writable() bool {
	return slices.ContainsFunc(h.access, func(mode string) bool { return mode != "r" })
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
// before it, then the rebuild durations' summary and perles' share of the
// write window spent building.
func contentionLogRebuilds(t *testing.T, pre contentionBuild, rebuilds []contentionBuild, window time.Duration) {
	t.Helper()
	if len(rebuilds) == 0 {
		t.Logf("no rebuild during the %s write window", contentionSeconds(window))
		return
	}
	previous := pre
	durations := make([]time.Duration, 0, len(rebuilds))
	var building time.Duration
	for i, r := range rebuilds {
		paced := max(minRebuildGap, 2*previous.duration())
		t.Logf("rebuild %d: %s, after %s idle (pacing: >= %s)",
			i+1, contentionSeconds(r.duration()), contentionSeconds(r.start.Sub(previous.end)), contentionSeconds(paced))
		durations = append(durations, r.duration())
		building += r.duration()
		previous = r
	}
	t.Logf("rebuilds while the writes ran: %d in %s, %s; building %.0f%% of the window",
		len(rebuilds), contentionSeconds(window), contentionSummarize(durations), 100*building.Seconds()/window.Seconds())
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
