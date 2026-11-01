package adapter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/task"
)

// fixtureIssue is one issue of a fake bd export.
type fixtureIssue struct {
	ID       string
	Title    string
	Status   string
	Labels   []string
	Deps     map[string]string // depends_on_id -> type
	Comments []fixtureComment
}

type fixtureComment struct {
	ID, Text  string
	Author    string // "tester" when empty
	CreatedAt time.Time
}

var fixtureTime = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

// exportJSONL renders issues as `bd export --all` JSONL.
func exportJSONL(t *testing.T, issues ...fixtureIssue) string {
	t.Helper()
	var b strings.Builder
	for _, issue := range issues {
		type dep struct {
			DependsOnID string `json:"depends_on_id"`
			Type        string `json:"type"`
		}
		type comment struct {
			ID        string    `json:"id"`
			Author    string    `json:"author"`
			Text      string    `json:"text"`
			CreatedAt time.Time `json:"created_at"`
		}
		rec := map[string]any{
			"_type": "issue", "id": issue.ID, "title": issue.Title, "status": issue.Status,
			"priority": 2, "issue_type": "task", "created_at": fixtureTime, "updated_at": fixtureTime,
			"labels": issue.Labels,
		}
		if rec["status"] == "" {
			rec["status"] = "open"
		}
		var deps []dep
		for target, typ := range issue.Deps {
			deps = append(deps, dep{DependsOnID: target, Type: typ})
		}
		rec["dependencies"] = deps
		var comments []comment
		for _, c := range issue.Comments {
			author := c.Author
			if author == "" {
				author = "tester"
			}
			comments = append(comments, comment{ID: c.ID, Author: author, Text: c.Text, CreatedAt: c.CreatedAt})
		}
		rec["comments"] = comments
		line, err := json.Marshal(rec)
		require.NoError(t, err)
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// generationTarget is the issue s-a depends on in generation gen, so a result
// whose relations mix two generations is detectable.
func generationTarget(gen int) string {
	if gen%2 == 0 {
		return "s-b"
	}
	return "s-c"
}

// generationExport is generation gen of a small workspace: s-a, s-b and s-c,
// each titled "gen-<gen> ..." and labelled gen-<gen>, with one comment whose
// text is gen-<gen>, and s-a blocked by generationTarget(gen).
func generationExport(t *testing.T, gen int) string {
	t.Helper()
	label := fmt.Sprintf("gen-%d", gen)
	issue := func(id string, deps map[string]string) fixtureIssue {
		return fixtureIssue{
			ID: id, Title: label + " " + id, Labels: []string{label}, Deps: deps,
			Comments: []fixtureComment{{ID: fmt.Sprintf("c-%d-%s", gen, id), Text: label, CreatedAt: fixtureTime}},
		}
	}
	return exportJSONL(t,
		issue("s-a", map[string]string{generationTarget(gen): "blocks"}),
		issue("s-b", nil),
		issue("s-c", nil),
	)
}

// generationFlags is `bd blocked --flags --raw --json` for generationExport.
const generationFlags = `[{"id":"s-a","is_blocked":true}]`

// bdCommand names the bd command of a request: export, blocked or statuses.
func bdCommand(req infrabeads.BDRequest) string {
	for _, arg := range req.Args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

// fakeRunner records every bd request and answers it with answer.
type fakeRunner struct {
	mu       sync.Mutex
	requests []infrabeads.BDRequest
	answer   func(ctx context.Context, req infrabeads.BDRequest) (string, error)
}

func (f *fakeRunner) run(ctx context.Context, req infrabeads.BDRequest) (string, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	f.mu.Unlock()
	return f.answer(ctx, req)
}

func (f *fakeRunner) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var commands []string
	for _, req := range f.requests {
		commands = append(commands, bdCommand(req))
	}
	return commands
}

func (f *fakeRunner) count(command string) int {
	n := 0
	for _, c := range f.commands() {
		if command == "" || c == command {
			n++
		}
	}
	return n
}

// generationRunner answers each export with the next generation of
// generationExport, starting at 1, and the flags read with generationFlags.
func generationRunner(t *testing.T) *fakeRunner {
	t.Helper()
	var gen atomic.Int64
	return &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
		switch bdCommand(req) {
		case "export":
			return generationExport(t, int(gen.Add(1))), nil
		case "blocked":
			return generationFlags, nil
		}
		return "", fmt.Errorf("unexpected bd request %v", req.Args)
	}}
}

// testWorkspace is a beads dir with an embedded manifest, under t.TempDir.
type testWorkspace struct {
	beadsDir, manifest string
}

func newTestWorkspace(t *testing.T) testWorkspace {
	t.Helper()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	noms := filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt", "noms")
	require.NoError(t, os.MkdirAll(noms, 0o750))
	manifest := filepath.Join(noms, "manifest")
	require.NoError(t, os.WriteFile(manifest, []byte("manifest v1"), 0o600))
	return testWorkspace{beadsDir: beadsDir, manifest: manifest}
}

// newTestBackend returns a backend over a temp workspace, bd 1.3.2 and run.
func newTestBackend(t *testing.T, run infrabeads.BDRunner) *SnapshotBackend {
	t.Helper()
	return newTestBackendWith(t, newTestWorkspace(t), SnapshotBackendConfig{BDVersion: "1.3.2", Runner: run})
}

// newTestBackendWith returns the backend cfg configures over ws, whose
// BeadsDir and ManifestPath it sets, and closes it when the test ends. PATH
// is emptied so that nothing can reach a real bd.
func newTestBackendWith(t *testing.T, ws testWorkspace, cfg SnapshotBackendConfig) *SnapshotBackend {
	t.Helper()
	t.Setenv("PATH", "")
	cfg.BeadsDir, cfg.ManifestPath = ws.beadsDir, ws.manifest
	b, err := NewSnapshotBackend(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	return b
}

// checkBuild runs CheckCompatibility and returns the build error it reports
// (nil on success). CheckCompatibility reports a failed build as an embedded
// *task.UnsupportedWorkspaceError that wraps the build's error.
func checkBuild(t *testing.T, b *SnapshotBackend) error {
	t.Helper()
	err := b.CheckCompatibility()
	if err == nil {
		return nil
	}
	uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.True(t, ok, "CheckCompatibility: got %v", err)
	require.Equal(t, task.WorkspaceEmbeddedDolt, uw.Kind)
	require.Equal(t, b.beadsDir, uw.BeadsDir)
	require.Error(t, uw.Err)
	return uw.Err
}

// memdbRegistered reports whether a memdb database named name exists. It
// opens the name read-only, which creates no database: memdb refuses to open
// an unknown name without create.
func memdbRegistered(t *testing.T, name string) bool {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:/"+name+"?vfs=memdb&mode=ro")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	err = db.PingContext(context.Background())
	if err == nil {
		return true
	}
	require.ErrorContains(t, err, "unable to open database file")
	return false
}

// countSnapshotTables returns how many issues tables a fresh open of the
// memdb name finds: 1 for a live snapshot, 0 when nothing is reachable.
func countSnapshotTables(t *testing.T, name string) int {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:/"+name+"?vfs=memdb")
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	var n int
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'issues'`).Scan(&n))
	return n
}

// requireNoSnapshotDB asserts that nothing is left under the memdb name: it
// is not registered (an empty, rolled-back database would be), and a fresh
// open finds no issues table.
func requireNoSnapshotDB(t *testing.T, name string) {
	t.Helper()
	require.False(t, memdbRegistered(t, name), "memdb %s is still registered", name)
	require.Zero(t, countSnapshotTables(t, name), "memdb %s still holds a snapshot", name)
}

func requireSnapshotDB(t *testing.T, name string) {
	t.Helper()
	require.True(t, memdbRegistered(t, name), "memdb %s is not registered", name)
	require.Equal(t, 1, countSnapshotTables(t, name), "memdb %s holds no snapshot", name)
}

// checkGeneration returns the generation of a `type = task` result, or an
// error when it is not exactly one generation of generationExport.
func checkGeneration(issues []task.Issue) (int, error) {
	if len(issues) != 3 {
		return 0, fmt.Errorf("got %d issues, want 3", len(issues))
	}
	gen := -1
	byID := map[string]task.Issue{}
	for _, issue := range issues {
		var g int
		var id string
		if _, err := fmt.Sscanf(issue.TitleText, "gen-%d %s", &g, &id); err != nil || id != issue.ID {
			return 0, fmt.Errorf("issue %s has title %q", issue.ID, issue.TitleText)
		}
		if gen == -1 {
			gen = g
		} else if g != gen {
			return 0, fmt.Errorf("mixed generations %d and %d", gen, g)
		}
		if want := []string{fmt.Sprintf("gen-%d", gen)}; !slices.Equal(issue.Labels, want) {
			return 0, fmt.Errorf("issue %s of generation %d has labels %v", issue.ID, gen, issue.Labels)
		}
		byID[issue.ID] = issue
	}
	target := generationTarget(gen)
	other := "s-b"
	if target == "s-b" {
		other = "s-c"
	}
	if got := byID["s-a"].BlockedBy; !slices.Equal(got, []string{target}) {
		return 0, fmt.Errorf("generation %d: s-a blocked by %v, want [%s]", gen, got, target)
	}
	if got := byID[target].Blocks; !slices.Equal(got, []string{"s-a"}) {
		return 0, fmt.Errorf("generation %d: %s blocks %v, want [s-a]", gen, target, got)
	}
	if got := byID[other].Blocks; len(got) != 0 {
		return 0, fmt.Errorf("generation %d: %s blocks %v, want none", gen, other, got)
	}
	return gen, nil
}

// checkCommentGeneration returns the generation of s-a's comments.
func checkCommentGeneration(comments []task.Comment) (int, error) {
	if len(comments) != 1 {
		return 0, fmt.Errorf("got %d comments, want 1", len(comments))
	}
	var gen int
	if _, err := fmt.Sscanf(comments[0].Text, "gen-%d", &gen); err != nil {
		return 0, fmt.Errorf("comment text %q", comments[0].Text)
	}
	if want := fmt.Sprintf("c-%d-s-a", gen); comments[0].ID != want {
		return 0, fmt.Errorf("comment %s has text of generation %d", comments[0].ID, gen)
	}
	return gen, nil
}

func TestSnapshotBackend_ImplementsBackend(t *testing.T) {
	var backend task.Backend = newTestBackend(t, generationRunner(t).run)

	services := backend.Services()
	require.Empty(t, services.DBPath, "the app must create no file watcher")
	require.Equal(t, task.WatcherConfig{}, services.WatcherConfig)
	require.Equal(t, beadsCapabilities(), services.Capabilities)
	require.NotNil(t, services.TaskExecutor)
	require.NotNil(t, services.QueryExecutor)
	require.IsType(t, &BeadsQueryHelpers{}, services.QueryHelpers)
	require.IsType(t, &BeadsSyntaxHighlighter{}, services.SyntaxHighlighter)
}

func TestNewSnapshotBackend_RequiredFields(t *testing.T) {
	run := generationRunner(t).run
	valid := SnapshotBackendConfig{BeadsDir: "/w/.beads", ManifestPath: "/w/manifest", BDVersion: "1.3.2", Runner: run}
	tests := []struct {
		name    string
		mutate  func(*SnapshotBackendConfig)
		wantErr string
	}{
		{"no beads dir", func(c *SnapshotBackendConfig) { c.BeadsDir = "" }, "no beads directory"},
		{"no manifest", func(c *SnapshotBackendConfig) { c.ManifestPath = "" }, "no embedded Dolt manifest path"},
		{"no runner", func(c *SnapshotBackendConfig) { c.Runner = nil }, "no bd runner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			b, err := NewSnapshotBackend(cfg)
			require.ErrorContains(t, err, tt.wantErr)
			require.Nil(t, b)
		})
	}
}

// TestSnapshotBackend_LazyInit: construction runs no bd; the first query
// builds the snapshot, and later queries and snapshot comment reads run none.
func TestSnapshotBackend_LazyInit(t *testing.T) {
	runner := generationRunner(t)
	b := newTestBackend(t, runner.run)
	services := b.Services()
	require.NoError(t, b.FlushCaches(context.Background()))
	require.Zero(t, runner.count(""), "no bd before the first query")
	require.Nil(t, b.current.Load())

	issues, err := services.QueryExecutor.Execute("type = task")
	require.NoError(t, err)
	gen, err := checkGeneration(issues)
	require.NoError(t, err)
	require.Equal(t, 1, gen)
	require.Equal(t, []string{"export", "blocked"}, runner.commands())

	before := runner.count("")
	for range 5 {
		issues, err := services.QueryExecutor.Execute("type = task")
		require.NoError(t, err)
		_, err = checkGeneration(issues)
		require.NoError(t, err)

		comments, err := services.TaskExecutor.GetComments("s-a")
		require.NoError(t, err)
		_, err = checkCommentGeneration(comments)
		require.NoError(t, err)
	}
	require.NoError(t, b.CheckCompatibility())
	require.NoError(t, b.FlushCaches(context.Background()))
	require.Equal(t, before, runner.count(""), "once a snapshot exists, reads never run bd")
}

// TestSnapshotBackend_ConcurrentFirstQueriesBuildOnce: first queries that
// arrive while the first build runs wait for it instead of building again.
func TestSnapshotBackend_ConcurrentFirstQueriesBuildOnce(t *testing.T) {
	const callers = 8
	exporting := make(chan struct{})
	release := make(chan struct{})
	var started atomic.Int32
	inner := generationRunner(t)
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if bdCommand(req) == "export" {
			close(exporting) // a second export would panic here
			<-release
		}
		return inner.answer(ctx, req)
	}}
	b := newTestBackend(t, runner.run)
	query := b.Services().QueryExecutor

	results := make([][]task.Issue, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	call := func(i int) {
		started.Add(1)
		results[i], errs[i] = query.Execute("type = task")
	}
	wg.Go(func() { call(0) })
	<-exporting
	for i := 1; i < callers; i++ {
		wg.Go(func() { call(i) })
	}
	require.Eventually(t, func() bool { return started.Load() == callers }, 5*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // let the late callers reach the pending build
	close(release)
	wg.Wait()

	for i := range callers {
		require.NoError(t, errs[i])
		gen, err := checkGeneration(results[i])
		require.NoError(t, err)
		require.Equal(t, 1, gen)
	}
	require.Equal(t, 1, runner.count("export"))
	require.Equal(t, uint64(1), b.lastGen.Load())
}

// TestSnapshotBackend_FailedFirstBuildIsRetried: a failed first build
// installs nothing, so a later query builds again once the failure backoff
// allows (see TestSnapshotScheduler_FirstBuildBacksOff).
func TestSnapshotBackend_FailedFirstBuildIsRetried(t *testing.T) {
	inner := generationRunner(t)
	var fail atomic.Bool
	fail.Store(true)
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if fail.Load() {
			return "", errors.New("bd export failed: boom")
		}
		return inner.answer(ctx, req)
	}}
	clock := newFakeClock(schedStart)
	b := newTestBackendWith(t, newTestWorkspace(t), SnapshotBackendConfig{BDVersion: "1.3.2", Runner: runner.run, Clock: clock})
	query := b.Services().QueryExecutor

	_, err := query.Execute("type = task")
	require.EqualError(t, err, "bd export failed: boom")
	require.Nil(t, b.current.Load())

	fail.Store(false)
	_, err = query.Execute("type = task")
	require.EqualError(t, err, "bd export failed: boom", "the backoff defers the retry")
	require.Equal(t, 1, runner.count("export"))

	clock.Advance(firstFailureBackoff)
	issues, err := query.Execute("type = task")
	require.NoError(t, err)
	_, err = checkGeneration(issues)
	require.NoError(t, err)
	require.Equal(t, 2, runner.count("export"))
}

// TestSnapshotBackend_RetirementStress: readers running queries and snapshot
// comment reads in a tight loop across at least 1,000 swaps never see a
// closed database, a panic or a result mixing generations.
func TestSnapshotBackend_RetirementStress(t *testing.T) {
	const (
		readers = 16
		swaps   = 1000
	)
	b := newTestBackend(t, generationRunner(t).run)
	require.NoError(t, b.CheckCompatibility())
	services := b.Services()

	var (
		stop       atomic.Bool
		reads      atomic.Int64
		failMu     sync.Mutex
		failures   []string
		closedErrs atomic.Int64
		seenMu     sync.Mutex
		seenGens   = map[int]bool{}
	)
	fail := func(format string, args ...any) {
		failMu.Lock()
		defer failMu.Unlock()
		if len(failures) < 20 {
			failures = append(failures, fmt.Sprintf(format, args...))
		}
	}
	checkErr := func(what string, err error) bool {
		if err == nil {
			return true
		}
		if strings.Contains(err.Error(), "database is closed") {
			closedErrs.Add(1)
		}
		fail("%s: %v", what, err)
		return false
	}

	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			defer func() {
				if r := recover(); r != nil {
					fail("reader panicked: %v", r)
				}
			}()
			for !stop.Load() {
				issues, err := services.QueryExecutor.Execute("type = task")
				if checkErr("Execute", err) {
					gen, err := checkGeneration(issues)
					if checkErr("Execute result", err) {
						seenMu.Lock()
						seenGens[gen] = true
						seenMu.Unlock()
					}
				}
				comments, err := services.TaskExecutor.GetComments("s-a")
				if checkErr("GetComments", err) {
					_, err = checkCommentGeneration(comments)
					checkErr("GetComments result", err)
				}
				reads.Add(1)
			}
		})
	}

	for range swaps {
		require.NoError(t, b.rebuild(b.ctx))
	}
	stop.Store(true)
	wg.Wait()

	require.Zero(t, closedErrs.Load(), "sql: database is closed under a reader")
	require.Empty(t, failures)
	require.Equal(t, uint64(swaps+1), b.current.Load().gen)
	seenMu.Lock()
	defer seenMu.Unlock()
	require.Greater(t, len(seenGens), 1, "readers must have run across swaps")
	t.Logf("%d reads over %d swaps saw %d generations", reads.Load(), swaps, len(seenGens))
}

// TestSnapshotBackend_RetireWaitsForReader: a reader holding the old snapshot
// mid-query delays its retirement. The new snapshot serves new readers
// meanwhile, the held reader's query succeeds, and only after it releases is
// the old database closed and its memdb deleted.
func TestSnapshotBackend_RetireWaitsForReader(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	require.NoError(t, b.CheckCompatibility())
	old := b.current.Load()

	acquired := make(chan struct{})
	release := make(chan struct{})
	// A failing assertion must not leave the reader holding the snapshot,
	// or the backend's Close would wait on it forever.
	releaseReader := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseReader)
	readerDone := make(chan error, 1)
	var readerIssues []beads.Issue
	go func() {
		readerDone <- b.withCurrent(func(s *snapshot) error {
			if s != old {
				return errors.New("the reader acquired another snapshot")
			}
			close(acquired)
			<-release
			var err error
			readerIssues, err = s.exec.Execute("type = task")
			return err
		})
	}()
	<-acquired

	swapped := make(chan error, 1)
	go func() { swapped <- b.rebuild(b.ctx) }()

	// The new snapshot is stored first; its retire then waits on the reader.
	require.Eventually(t, func() bool { return b.current.Load() != old }, 5*time.Second, time.Millisecond)
	select {
	case err := <-swapped:
		t.Fatalf("the swap finished retiring while a reader held the old snapshot (err %v)", err)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, old.db.PingContext(context.Background()), "the old database stays open for its reader")
	requireSnapshotDB(t, old.memdbName)

	issues, err := b.Services().QueryExecutor.Execute("type = task")
	require.NoError(t, err)
	gen, err := checkGeneration(issues)
	require.NoError(t, err)
	require.Equal(t, 2, gen, "new readers use the new snapshot without waiting")

	releaseReader()
	require.NoError(t, <-readerDone, "the held reader's query succeeds")
	require.Len(t, readerIssues, 3)
	require.NoError(t, <-swapped)

	require.EqualError(t, old.db.PingContext(context.Background()), "sql: database is closed")
	requireNoSnapshotDB(t, old.memdbName)
	old.mu.RLock()
	defer old.mu.RUnlock()
	require.True(t, old.retired)
}

// TestSnapshotBackend_ReaderRetriesRetiredSnapshot: a reader that loaded the
// old pointer just before a swap, and gets its read lock only after the
// retire, sees retired and retries on the new snapshot instead of using the
// closed database. The test plays the retire itself, holding the old
// snapshot's write lock so that the reader parks between Load and RLock.
func TestSnapshotBackend_ReaderRetriesRetiredSnapshot(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	require.NoError(t, b.CheckCompatibility())
	old := b.current.Load()
	next, err := b.build(b.ctx, func(string) {})
	require.NoError(t, err)

	old.mu.Lock()
	done := make(chan error, 1)
	var issues []task.Issue
	go func() {
		var err error
		issues, err = b.Services().QueryExecutor.Execute("type = task")
		done <- err
	}()
	time.Sleep(50 * time.Millisecond) // the reader has loaded old and waits on its read lock

	b.current.Store(next)
	old.retired = true
	require.NoError(t, old.db.Close())
	old.mu.Unlock()

	require.NoError(t, <-done)
	gen, err := checkGeneration(issues)
	require.NoError(t, err)
	require.Equal(t, 2, gen)
	require.NoError(t, old.retire()) // already marked: frees the memdb
}

// TestSnapshotBackend_RetiredSnapshotIsDeleted: a replaced snapshot's memdb is
// gone, and the new one is reachable under its own name.
func TestSnapshotBackend_RetiredSnapshotIsDeleted(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	require.NoError(t, b.CheckCompatibility())
	first := b.current.Load()
	require.Equal(t, snapshotMemdbName(b.id, 1), first.memdbName)
	requireSnapshotDB(t, first.memdbName)

	require.NoError(t, b.rebuild(b.ctx))
	second := b.current.Load()
	require.Equal(t, snapshotMemdbName(b.id, 2), second.memdbName)
	requireNoSnapshotDB(t, first.memdbName)
	require.EqualError(t, first.db.PingContext(context.Background()), "sql: database is closed")
	requireSnapshotDB(t, second.memdbName)
}

// TestSnapshotBackend_BackendsHaveDistinctNames: memdb names are process-wide,
// so two backends' generations never share one.
func TestSnapshotBackend_BackendsHaveDistinctNames(t *testing.T) {
	one := newTestBackend(t, generationRunner(t).run)
	two := newTestBackend(t, generationRunner(t).run)
	require.NoError(t, one.CheckCompatibility())
	require.NoError(t, two.CheckCompatibility())
	require.NotEqual(t, one.current.Load().memdbName, two.current.Load().memdbName)

	name := two.current.Load().memdbName
	require.NoError(t, one.Close())
	requireSnapshotDB(t, name)
	_, err := two.Services().QueryExecutor.Execute("type = task")
	require.NoError(t, err)
}

// TestSnapshotBackend_LoadFailureFreesMemdb: a build whose load fails (a
// duplicate issue ID) leaves no reachable memdb and installs nothing.
func TestSnapshotBackend_LoadFailureFreesMemdb(t *testing.T) {
	runner := &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
		if bdCommand(req) == "export" {
			return exportJSONL(t, fixtureIssue{ID: "s-dup"}, fixtureIssue{ID: "s-dup"}), nil
		}
		return "", fmt.Errorf("unexpected bd request %v", req.Args)
	}}
	b := newTestBackend(t, runner.run)
	var loaded []*sql.DB
	b.load = func(ctx context.Context, name string, data infrabeads.SnapshotData) (*sql.DB, error) {
		db, err := infrabeads.LoadSnapshot(ctx, name, data)
		loaded = append(loaded, db)
		return db, err
	}

	err := checkBuild(t, b)
	require.ErrorContains(t, err, "loading snapshot: insert issue s-dup")
	require.Nil(t, b.current.Load())
	require.Equal(t, []*sql.DB{nil}, loaded, "LoadSnapshot returns no database on failure")
	requireNoSnapshotDB(t, snapshotMemdbName(b.id, 1))
}

// TestSnapshotBackend_LoadErrorClosesReturnedDatabase: a load that fails but
// still returns a database has it closed, and its memdb deleted.
func TestSnapshotBackend_LoadErrorClosesReturnedDatabase(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	var loaded *sql.DB
	b.load = func(ctx context.Context, name string, data infrabeads.SnapshotData) (*sql.DB, error) {
		db, err := infrabeads.LoadSnapshot(ctx, name, data)
		require.NoError(t, err)
		loaded = db
		return db, errors.New("checking the snapshot: boom")
	}

	require.EqualError(t, checkBuild(t, b), "loading snapshot: checking the snapshot: boom")
	require.Nil(t, b.current.Load())
	require.NotNil(t, loaded)
	require.EqualError(t, loaded.PingContext(context.Background()), "sql: database is closed")
	requireNoSnapshotDB(t, snapshotMemdbName(b.id, 1))
}

// TestSnapshotBackend_InstallAbortedByCloseFreesMemdb: a build that finishes
// loading after Close is discarded: its database is closed and its memdb
// deleted.
func TestSnapshotBackend_InstallAbortedByCloseFreesMemdb(t *testing.T) {
	b := newTestBackend(t, generationRunner(t).run)
	var loaded *sql.DB
	b.load = func(ctx context.Context, name string, data infrabeads.SnapshotData) (*sql.DB, error) {
		db, err := infrabeads.LoadSnapshot(ctx, name, data)
		loaded = db
		require.NoError(t, b.Close()) // Close lands between the load and the install
		return db, err
	}

	err := checkBuild(t, b)
	require.ErrorIs(t, err, errSnapshotBackendClosed)
	require.Nil(t, b.current.Load())
	require.NotNil(t, loaded)
	require.EqualError(t, loaded.PingContext(context.Background()), "sql: database is closed")
	requireNoSnapshotDB(t, snapshotMemdbName(b.id, 1))
}

// TestSnapshotBackend_BDReadFailuresCreateNoMemdb: a failing blocked-flags or
// statuses read fails the build before any memdb exists.
func TestSnapshotBackend_BDReadFailuresCreateNoMemdb(t *testing.T) {
	customStatus := exportJSONL(t,
		fixtureIssue{ID: "s-a", Deps: map[string]string{"s-b": "blocks"}},
		fixtureIssue{ID: "s-b", Status: "in_review"},
	)
	tests := []struct {
		name     string
		failing  string
		wantErr  string
		wantCmds []string
	}{
		{"blocked flags", "blocked", "bd blocked failed: boom", []string{"export", "blocked"}},
		{"custom statuses", "statuses", "bd statuses failed: boom", []string{"export", "blocked", "statuses"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
				switch cmd := bdCommand(req); {
				case cmd == tt.failing:
					return "", fmt.Errorf("bd %s failed: boom", cmd)
				case cmd == "export":
					return customStatus, nil
				case cmd == "blocked":
					return generationFlags, nil
				}
				return "", fmt.Errorf("unexpected bd request %v", req.Args)
			}}
			b := newTestBackend(t, runner.run)
			loads := 0
			b.load = func(ctx context.Context, name string, data infrabeads.SnapshotData) (*sql.DB, error) {
				loads++
				return infrabeads.LoadSnapshot(ctx, name, data)
			}

			require.EqualError(t, checkBuild(t, b), tt.wantErr)
			require.Equal(t, tt.wantCmds, runner.commands())
			require.Zero(t, loads, "no memdb is created")
			require.Zero(t, b.lastGen.Load(), "no generation is handed out")
			requireNoSnapshotDB(t, snapshotMemdbName(b.id, 1))
		})
	}
}

// TestSnapshotBackend_FailedBuildsDoNotAccumulate: repeated failed builds,
// as backoff retries or manual refreshes run them, leave no memdb behind,
// and a later successful build is the only reachable one.
func TestSnapshotBackend_FailedBuildsDoNotAccumulate(t *testing.T) {
	inner := generationRunner(t)
	var mode atomic.Value // "dup", "flags" or "ok"
	mode.Store("dup")
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		switch m := mode.Load().(string); {
		case m == "dup" && bdCommand(req) == "export":
			return exportJSONL(t, fixtureIssue{ID: "s-dup"}, fixtureIssue{ID: "s-dup"}), nil
		case m == "flags" && bdCommand(req) == "blocked":
			return "", errors.New("bd blocked failed: boom")
		}
		return inner.answer(ctx, req)
	}}
	b := newTestBackend(t, runner.run)

	for range 3 {
		require.ErrorContains(t, checkBuild(t, b), "insert issue s-dup")
	}
	mode.Store("flags")
	for range 3 {
		require.EqualError(t, checkBuild(t, b), "bd blocked failed: boom")
	}
	require.Equal(t, uint64(3), b.lastGen.Load(), "only the load attempts were handed a generation")
	mode.Store("ok")
	require.NoError(t, b.CheckCompatibility())

	for gen := uint64(1); gen <= 3; gen++ {
		requireNoSnapshotDB(t, snapshotMemdbName(b.id, gen))
	}
	require.Equal(t, snapshotMemdbName(b.id, 4), b.current.Load().memdbName)
	requireSnapshotDB(t, b.current.Load().memdbName)
}

// TestSnapshotBackend_VersionGate: every build runs the embedded bd version
// gate before any bd call.
func TestSnapshotBackend_VersionGate(t *testing.T) {
	newBackend := func(t *testing.T, ws testWorkspace, version string, contextErr error, run infrabeads.BDRunner) *SnapshotBackend {
		t.Helper()
		return newTestBackendWith(t, ws, SnapshotBackendConfig{BDVersion: version, BDContextErr: contextErr, Runner: run})
	}

	t.Run("bd 1.0.4 is refused without running bd", func(t *testing.T) {
		runner := generationRunner(t)
		b := newBackend(t, newTestWorkspace(t), "1.0.4", nil, runner.run)

		for _, build := range []func() error{
			func() error { return checkBuild(t, b) },
			func() error { _, err := b.Services().QueryExecutor.Execute("type = task"); return err },
		} {
			err := build()
			uw, ok := errors.AsType[*infrabeads.UnsupportedWorkspaceError](err)
			require.True(t, ok, "got %v", err)
			require.Equal(t, infrabeads.WorkspaceEmbeddedDolt, uw.Kind)
			require.Equal(t, b.beadsDir, uw.BeadsDir)
			require.Equal(t, "embedded workspaces need bd >= 1.0.5 (found 1.0.4)", uw.Detail)
		}
		require.Zero(t, runner.count(""), "the gate runs before any bd call")
		require.Zero(t, b.lastGen.Load(), "and before any memdb")
	})

	t.Run("version info left out is refused", func(t *testing.T) {
		runner := generationRunner(t)
		b := newBackend(t, newTestWorkspace(t), "", nil, runner.run)
		uw, ok := errors.AsType[*infrabeads.UnsupportedWorkspaceError](checkBuild(t, b))
		require.True(t, ok)
		require.Contains(t, uw.Detail, "could not read the bd version")
		require.Zero(t, runner.count(""))
	})

	t.Run("bd missing surfaces the export's not-found error", func(t *testing.T) {
		ws := newTestWorkspace(t)
		t.Setenv("PATH", "") // the real runner finds no bd
		real := infrabeads.NewBDRunner(filepath.Dir(ws.beadsDir), ws.beadsDir)
		runner := &fakeRunner{answer: real}
		b := newBackend(t, ws, "", fmt.Errorf("bd context: %w", exec.ErrNotFound), runner.run)

		err := checkBuild(t, b)
		require.ErrorIs(t, err, exec.ErrNotFound)
		require.EqualError(t, err, "bd export failed: bd CLI not found on PATH")
		require.Equal(t, []string{"export"}, runner.commands())
		require.Zero(t, b.lastGen.Load())
	})

	t.Run("bd 1.3.2 builds", func(t *testing.T) {
		runner := generationRunner(t)
		b := newBackend(t, newTestWorkspace(t), "1.3.2 (6ebaacba8: main@6ebaacba8)", nil, runner.run)
		require.NoError(t, b.CheckCompatibility())
		require.NotNil(t, b.current.Load())
	})
}

// TestSnapshotBackend_BuildReadsHashAndWriteSeqFirst: the snapshot records
// the manifest hash and writeSeq read before the export, so a write landing
// during the export is not claimed as covered.
func TestSnapshotBackend_BuildReadsHashAndWriteSeqFirst(t *testing.T) {
	inner := generationRunner(t)
	var b *SnapshotBackend
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if bdCommand(req) == "export" {
			// A write lands while bd exports.
			b.writeSeq.Add(1)
			if err := os.WriteFile(b.manifestPath, []byte("manifest v2"), 0o600); err != nil {
				return "", err
			}
		}
		return inner.answer(ctx, req)
	}}
	b = newTestBackend(t, runner.run)
	b.writeSeq.Store(7)

	require.NoError(t, b.CheckCompatibility())
	s := b.current.Load()
	sum := sha256.Sum256([]byte("manifest v1"))
	require.Equal(t, hex.EncodeToString(sum[:]), s.manifestHash)
	require.Equal(t, uint64(7), s.coveredWriteSeq)
	require.Equal(t, uint64(1), s.gen)
}

// TestSnapshotBackend_BuildComposesSources: the export, the blocked flags and
// the custom statuses all reach the snapshot.
func TestSnapshotBackend_BuildComposesSources(t *testing.T) {
	runner := &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
		switch bdCommand(req) {
		case "export":
			return exportJSONL(t,
				fixtureIssue{ID: "s-a", Deps: map[string]string{"s-b": "blocks"}},
				fixtureIssue{ID: "s-b"},
				fixtureIssue{ID: "s-r", Status: "in_review"},
				fixtureIssue{ID: "s-w", Status: "on_hold"},
			), nil
		case "blocked":
			return generationFlags, nil
		case "statuses":
			return `{"built_in_statuses":[{"name":"open"}],"custom_statuses":[` +
				`{"name":"in_review","category":"active"},{"name":"on_hold","category":"frozen"}]}`, nil
		}
		return "", fmt.Errorf("unexpected bd request %v", req.Args)
	}}
	b := newTestBackend(t, runner.run)
	query := b.Services().QueryExecutor

	ids := func(q string) []string {
		issues, err := query.Execute(q)
		require.NoError(t, err)
		var out []string
		for _, issue := range issues {
			out = append(out, issue.ID)
		}
		slices.Sort(out)
		return out
	}
	require.Equal(t, []string{"s-a"}, ids("blocked = true"))
	require.Equal(t, []string{"s-b", "s-r"}, ids("ready = true"), "in_review is an active custom status")
	require.Equal(t, []string{"export", "blocked", "statuses"}, runner.commands())
	for _, req := range runner.requests {
		require.Equal(t, "--readonly", req.Args[0])
	}
}

// TestSnapshotBackend_SnapshotComments: comments come from the snapshot's
// comments table with bd's UUID IDs, oldest first.
func TestSnapshotBackend_SnapshotComments(t *testing.T) {
	later := fixtureTime.Add(time.Hour)
	runner := &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
		if bdCommand(req) == "export" {
			return exportJSONL(t,
				fixtureIssue{ID: "s-a", Comments: []fixtureComment{
					{ID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Text: "second", CreatedAt: later},
					{ID: "16fd2706-8baf-433b-82eb-8c7fada847da", Text: "first", CreatedAt: fixtureTime},
					{ID: "886313e1-3b8a-5372-9b90-0c9aee199e5d", Text: "first, same second", CreatedAt: fixtureTime},
				}},
				fixtureIssue{ID: "s-b"},
			), nil
		}
		return "", fmt.Errorf("unexpected bd request %v", req.Args)
	}}
	b := newTestBackend(t, runner.run)

	_, err := b.snapshotComments("s-a")
	require.ErrorIs(t, err, errNoSnapshot, "snapshot comments build no snapshot")
	require.Zero(t, runner.count(""))

	require.NoError(t, b.CheckCompatibility())
	comments, err := b.Services().TaskExecutor.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []task.Comment{
		{ID: "16fd2706-8baf-433b-82eb-8c7fada847da", Author: "tester", Text: "first", CreatedAt: fixtureTime},
		{ID: "886313e1-3b8a-5372-9b90-0c9aee199e5d", Author: "tester", Text: "first, same second", CreatedAt: fixtureTime},
		{ID: "7c9e6679-7425-40de-944b-e07fc1f90ae7", Author: "tester", Text: "second", CreatedAt: later},
	}, comments)

	none, err := b.snapshotComments("s-b")
	require.NoError(t, err)
	require.Empty(t, none)
}

// TestSnapshotBackend_Close: Close retires the current snapshot, is
// idempotent, and later reads fail without running bd.
func TestSnapshotBackend_Close(t *testing.T) {
	runner := generationRunner(t)
	b := newTestBackend(t, runner.run)
	require.NoError(t, b.CheckCompatibility())
	s := b.current.Load()

	require.NoError(t, b.Close())
	require.NoError(t, b.Close())
	require.Nil(t, b.current.Load())
	require.EqualError(t, s.db.PingContext(context.Background()), "sql: database is closed")
	requireNoSnapshotDB(t, s.memdbName)

	before := runner.count("")
	_, err := b.Services().QueryExecutor.Execute("type = task")
	require.ErrorIs(t, err, errSnapshotBackendClosed)
	require.ErrorIs(t, checkBuild(t, b), errSnapshotBackendClosed)
	_, err = b.snapshotComments("s-a")
	require.ErrorIs(t, err, errSnapshotBackendClosed)
	require.Equal(t, before, runner.count(""), "no bd after Close")
}

// TestSnapshotBackend_CloseCancelsRunningBuild: Close cancels the bd read of a
// running build through the backend's context.
func TestSnapshotBackend_CloseCancelsRunningBuild(t *testing.T) {
	exporting := make(chan struct{})
	runner := &fakeRunner{answer: func(ctx context.Context, _ infrabeads.BDRequest) (string, error) {
		close(exporting)
		<-ctx.Done()
		return "", context.Cause(ctx)
	}}
	b := newTestBackend(t, runner.run)

	done := make(chan error, 1)
	go func() {
		_, err := b.Services().QueryExecutor.Execute("type = task")
		done <- err
	}()
	<-exporting
	require.NoError(t, b.Close())
	select {
	case err := <-done:
		require.ErrorIs(t, err, errSnapshotBackendClosed)
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not cancel the running export")
	}
	require.Zero(t, b.lastGen.Load())
}
