package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/task"
)

// bdOp names what a bd request does: "comments add", or the command name
// ("comments", "show", "export", ...).
func bdOp(req infrabeads.BDRequest) string {
	for i, arg := range req.Args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "comments" && i+1 < len(req.Args) && req.Args[i+1] == "add" {
			return "comments add"
		}
		return arg
	}
	return ""
}

// requestArgs returns the args of the runner's requests from the n-th on.
func requestArgs(f *fakeRunner, n int) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var args [][]string
	for _, req := range f.requests[n:] {
		args = append(args, req.Args)
	}
	return args
}

// requestCount returns how many requests the runner has seen.
func requestCount(f *fakeRunner) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// opCount returns how many of the runner's requests do op (see bdOp).
func opCount(f *fakeRunner, op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, req := range f.requests {
		if bdOp(req) == op {
			n++
		}
	}
	return n
}

// commentBD is a fake bd over a workspace of issues without dependencies,
// whose comments it keeps in memory: `comments add` appends one, and
// `--readonly comments`, `--readonly show` and `export` read the current
// state. Like the real runner's, a request whose context has ended fails
// with its cause.
type commentBD struct {
	t      *testing.T
	runner *fakeRunner
	// fail, when set and returning an error for a request, fails it.
	fail func(req infrabeads.BDRequest) error

	mu       sync.Mutex
	issues   []string
	comments map[string][]beads.Comment
	added    int
}

func newCommentBD(t *testing.T, issues ...string) *commentBD {
	t.Helper()
	bd := &commentBD{t: t, issues: issues, comments: map[string][]beads.Comment{}}
	bd.runner = &fakeRunner{answer: bd.answer}
	return bd
}

// add stores a comment the way `bd comments add` does: a bd ID and the next
// whole second, so comment order is insertion order.
func (bd *commentBD) add(issueID, author, text string) {
	bd.mu.Lock()
	defer bd.mu.Unlock()
	bd.added++
	bd.comments[issueID] = append(bd.comments[issueID], beads.Comment{
		ID:        fmt.Sprintf("bd-c-%d", bd.added),
		Author:    author,
		Text:      text,
		CreatedAt: fixtureTime.Add(time.Duration(bd.added) * time.Second),
	})
}

// texts returns the texts of an issue's stored comments.
func (bd *commentBD) texts(issueID string) []string {
	bd.mu.Lock()
	defer bd.mu.Unlock()
	var texts []string
	for _, c := range bd.comments[issueID] {
		texts = append(texts, c.Text)
	}
	return texts
}

func (bd *commentBD) answer(ctx context.Context, req infrabeads.BDRequest) (string, error) {
	if err := context.Cause(ctx); err != nil {
		return "", err
	}
	if bd.fail != nil {
		if err := bd.fail(req); err != nil {
			return "", err
		}
	}
	args := req.Args
	switch bdOp(req) {
	case "comments add": // comments add <id> --json [--actor=<author>] -- <text>
		author := ""
		for _, arg := range args {
			if v, ok := strings.CutPrefix(arg, "--actor="); ok {
				author = v
			}
		}
		bd.add(args[slices.Index(args, "add")+1], author, args[len(args)-1])
		return "{}", nil
	case "comments": // --readonly comments <id> --json
		bd.mu.Lock()
		defer bd.mu.Unlock()
		out, err := json.Marshal(append([]beads.Comment{}, bd.comments[args[2]]...))
		return string(out), err
	case "show": // --readonly show <id> --json
		return fmt.Sprintf(`[{"id":%q,"title":%q,"status":"open"}]`, args[2], args[2]), nil
	case "export":
		bd.mu.Lock()
		defer bd.mu.Unlock()
		var issues []fixtureIssue
		for _, id := range bd.issues {
			issue := fixtureIssue{ID: id, Title: id}
			for _, c := range bd.comments[id] {
				issue.Comments = append(issue.Comments, fixtureComment{ID: c.ID, Author: c.Author, Text: c.Text, CreatedAt: c.CreatedAt})
			}
			issues = append(issues, issue)
		}
		return exportJSONL(bd.t, issues...), nil
	}
	return "", fmt.Errorf("unexpected bd request %v", args)
}

func commentTexts(comments []task.Comment) []string {
	var texts []string
	for _, c := range comments {
		texts = append(texts, c.Text)
	}
	return texts
}

// overlayEntry returns an issue's comment overlay entry.
func overlayEntry(b *SnapshotBackend, issueID string) (commentOverlay, bool) {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	entry, ok := b.overlay[issueID]
	return entry, ok
}

func overlaySize(b *SnapshotBackend) int {
	b.overlayMu.Lock()
	defer b.overlayMu.Unlock()
	return len(b.overlay)
}

// captureLog sends perles' log to a temp file for the test and returns a
// function that reads it.
func captureLog(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "perles.log")
	closeLog, err := log.InitWithTeaLog(path, "snapshot-writes-test")
	require.NoError(t, err)
	log.SetEnabled(true)
	t.Cleanup(func() {
		log.SetEnabled(false)
		stdlog.SetOutput(os.Stderr) // tea.LogToFile redirected the standard logger
		stdlog.SetPrefix("")
		closeLog()
	})
	return func() string {
		data, err := os.ReadFile(path) //nolint:gosec // G304: test temp file
		require.NoError(t, err)
		return string(data)
	}
}

// TestSnapshotBackend_AddCommentReadAfterWrite: the TUI's AddComment,
// ShowIssue, GetComments sequence sees the new comment although the
// installed snapshot lacks it, and keeps seeing it when a snapshot exported
// before the write lands. A covering snapshot then serves it and prunes the
// overlay entry. GetComments never runs bd.
func TestSnapshotBackend_AddCommentReadAfterWrite(t *testing.T) {
	bd := newCommentBD(t, "s-a", "s-b")
	bd.add("s-a", "tester", "first")
	b := newTestBackend(t, bd.runner.run)
	require.NoError(t, b.CheckCompatibility())
	te := b.Services().TaskExecutor
	stale, err := b.build(b.ctx, func(string) {}) // exported before the write: does not cover it
	require.NoError(t, err)
	before := requestCount(bd.runner)

	require.NoError(t, te.AddComment("s-a", "alice", "hello"))
	issue, err := te.ShowIssue("s-a")
	require.NoError(t, err)
	require.Equal(t, "s-a", issue.ID)
	comments, err := te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"first", "hello"}, commentTexts(comments))
	require.Equal(t, [][]string{
		{"comments", "add", "s-a", "--json", "--actor=alice", "--", "hello"},
		{"--readonly", "comments", "s-a", "--json"},
		{"--readonly", "show", "s-a", "--json"},
	}, requestArgs(bd.runner, before), "GetComments ran no bd")
	inSnapshot, err := b.current.Load().comments("s-a")
	require.NoError(t, err)
	require.Len(t, inSnapshot, 1, "the installed snapshot lacks the new comment")
	entry, ok := overlayEntry(b, "s-a")
	require.True(t, ok)
	require.Equal(t, uint64(1), entry.seq)

	require.NoError(t, b.install(stale))
	comments, err = te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"first", "hello"}, commentTexts(comments), "a snapshot that does not cover the write keeps the overlay")
	_, ok = overlayEntry(b, "s-a")
	require.True(t, ok)

	require.NoError(t, b.rebuild(b.ctx))
	require.Equal(t, uint64(1), b.current.Load().coveredWriteSeq)
	require.Zero(t, overlaySize(b), "the covering snapshot prunes the entry")
	comments, err = te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"first", "hello"}, commentTexts(comments))

	// A comment added outside perles reaches GetComments with the next
	// snapshot: nothing overlays the snapshot any more.
	bd.add("s-a", "bob", "external")
	require.NoError(t, b.rebuild(b.ctx))
	comments, err = te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"first", "hello", "external"}, commentTexts(comments))
	require.Equal(t, 1, opCount(bd.runner, "comments"), "the only bd comment read is the one after AddComment")
}

// TestSnapshotBackend_AddCommentFetchFailure: when the comments read after
// AddComment fails, GetComments serves the snapshot's comments plus the new
// comment synthesized locally, and a warning is logged, until a covering
// snapshot replaces it with bd's copy.
func TestSnapshotBackend_AddCommentFetchFailure(t *testing.T) {
	readLog := captureLog(t)
	now := time.Date(2026, 10, 31, 9, 30, 15, 0, time.FixedZone("EST", -5*60*60))
	bd := newCommentBD(t, "s-a")
	bd.add("s-a", "tester", "first")
	bd.fail = func(req infrabeads.BDRequest) error {
		if bdOp(req) == "comments" {
			return errors.New("bd comments failed: database is locked")
		}
		return nil
	}
	b := newTestBackendWith(t, newTestWorkspace(t), SnapshotBackendConfig{
		BDVersion: "1.3.2", Runner: bd.runner.run, Clock: newFakeClock(now),
	})
	require.NoError(t, b.CheckCompatibility())
	te := b.Services().TaskExecutor

	require.NoError(t, te.AddComment("s-a", "alice", "hello"), "the write succeeded")
	_, err := te.ShowIssue("s-a")
	require.NoError(t, err)
	comments, err := te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []task.Comment{
		{ID: "bd-c-1", Author: "tester", Text: "first", CreatedAt: fixtureTime.Add(time.Second)},
		{ID: "local-1", Author: "alice", Text: "hello", CreatedAt: now.UTC()},
	}, comments)
	logged := readLog()
	require.Contains(t, logged, "[WARN] [beads] Reading comments after AddComment failed; serving the new comment from a local copy")
	require.Contains(t, logged, "commentID=local-1")
	require.Contains(t, logged, "database is locked")
	require.Equal(t, 1, opCount(bd.runner, "comments"), "GetComments ran no bd")

	require.NoError(t, b.rebuild(b.ctx))
	comments, err = te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []task.Comment{
		{ID: "bd-c-1", Author: "tester", Text: "first", CreatedAt: fixtureTime.Add(time.Second)},
		{ID: "bd-c-2", Author: "alice", Text: "hello", CreatedAt: fixtureTime.Add(2 * time.Second)},
	}, comments, "the covering snapshot replaces the local copy")
}

// TestSnapshotBackend_AddCommentFetchFailureWithoutSnapshot: with no
// snapshot to synthesize from, AddComment records no overlay and warns; the
// next GetComments reads bd, which has the comment.
func TestSnapshotBackend_AddCommentFetchFailureWithoutSnapshot(t *testing.T) {
	readLog := captureLog(t)
	bd := newCommentBD(t, "s-a")
	var failed atomic.Bool
	bd.fail = func(req infrabeads.BDRequest) error {
		if bdOp(req) == "comments" && !failed.Swap(true) {
			return errors.New("bd comments failed: database is locked")
		}
		return nil
	}
	b := newTestBackend(t, bd.runner.run)
	te := b.Services().TaskExecutor

	require.NoError(t, te.AddComment("s-a", "alice", "hello"))
	require.Zero(t, overlaySize(b))
	require.Contains(t, readLog(), "Reading comments after AddComment failed; the new comment may be missing until the next snapshot")

	comments, err := te.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"hello"}, commentTexts(comments))
	require.Equal(t, 2, opCount(bd.runner, "comments"))
	require.Zero(t, opCount(bd.runner, "export"))
}

// TestSnapshotBackend_ConcurrentCommentsReadYourWrites: writers adding
// comments, readers and a goroutine swapping snapshots run concurrently
// (race-free under -race). Every writer's GetComments straight after its
// AddComment sees its comment, every comment is served at the end, and
// GetComments never runs bd. Writers are paced so that swaps keep landing
// among their writes: write n waits for n/writesPerSwap swaps.
func TestSnapshotBackend_ConcurrentCommentsReadYourWrites(t *testing.T) {
	const (
		writers       = 8
		perWriter     = 40
		writesPerSwap = 4
		readers       = 4
	)
	ids := []string{"s-a", "s-b", "s-c"}
	bd := newCommentBD(t, ids...)
	b := newTestBackend(t, bd.runner.run)
	require.NoError(t, b.CheckCompatibility())
	te := b.Services().TaskExecutor

	var (
		stop     atomic.Bool
		swaps    atomic.Int64
		written  atomic.Int64
		failMu   sync.Mutex
		failures []string
	)
	fail := func(format string, args ...any) {
		failMu.Lock()
		defer failMu.Unlock()
		if len(failures) < 20 {
			failures = append(failures, fmt.Sprintf(format, args...))
		}
	}

	var background sync.WaitGroup
	for range readers {
		background.Go(func() {
			for i := 0; !stop.Load(); i++ {
				if _, err := te.GetComments(ids[i%len(ids)]); err != nil {
					fail("GetComments: %v", err)
				}
			}
		})
	}
	background.Go(func() {
		for !stop.Load() {
			if err := b.rebuild(b.ctx); err != nil {
				fail("rebuild: %v", err)
				return
			}
			swaps.Add(1)
		}
	})

	var writing sync.WaitGroup
	for w := range writers {
		writing.Go(func() {
			for n := range perWriter {
				for swaps.Load() < int64(n/writesPerSwap) {
					time.Sleep(100 * time.Microsecond)
				}
				id := ids[(w+n)%len(ids)]
				text := fmt.Sprintf("w%d-%d", w, n)
				if err := te.AddComment(id, "alice", text); err != nil {
					fail("AddComment: %v", err)
					return
				}
				written.Add(1)
				comments, err := te.GetComments(id)
				if err != nil {
					fail("GetComments after AddComment: %v", err)
					return
				}
				if !slices.Contains(commentTexts(comments), text) {
					fail("%s is missing from %s right after AddComment", text, id)
				}
			}
		})
	}
	writing.Wait()
	stop.Store(true)
	background.Wait()
	require.Empty(t, failures)
	require.GreaterOrEqual(t, swaps.Load(), int64((perWriter-1)/writesPerSwap), "snapshots were swapped during the writes")

	requireAllComments := func() {
		t.Helper()
		for _, id := range ids {
			comments, err := te.GetComments(id)
			require.NoError(t, err)
			require.Equal(t, bd.texts(id), commentTexts(comments), id)
		}
	}
	requireAllComments()
	require.Equal(t, int(written.Load()), opCount(bd.runner, "comments"), "the only bd comment reads are those after AddComment")

	require.NoError(t, b.rebuild(b.ctx))
	require.Zero(t, overlaySize(b))
	requireAllComments()
	t.Logf("%d comments over %d swaps", written.Load(), swaps.Load())
}

// TestSnapshotBackend_GetCommentsBeforeFirstSnapshot: with no snapshot,
// GetComments runs bd --readonly comments with the backend's context and
// builds nothing.
func TestSnapshotBackend_GetCommentsBeforeFirstSnapshot(t *testing.T) {
	bd := newCommentBD(t, "s-a")
	bd.add("s-a", "tester", "first")
	var readCtx context.Context
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		readCtx = ctx
		return bd.answer(ctx, req)
	}}
	b := newTestBackend(t, runner.run)

	comments, err := b.Services().TaskExecutor.GetComments("s-a")
	require.NoError(t, err)
	require.Equal(t, []string{"first"}, commentTexts(comments))
	require.Equal(t, [][]string{{"--readonly", "comments", "s-a", "--json"}}, requestArgs(runner, 0), "no export")
	require.Nil(t, b.current.Load())
	require.Zero(t, b.lastGen.Load())

	require.NoError(t, b.Close())
	require.ErrorIs(t, context.Cause(readCtx), errSnapshotBackendClosed, "the read ran with the backend's context")
}

// TestSnapshotBackend_GetCommentsSnapshotErrorRunsNoBD: a failing snapshot
// read is returned to the caller; bd is not asked instead.
func TestSnapshotBackend_GetCommentsSnapshotErrorRunsNoBD(t *testing.T) {
	bd := newCommentBD(t, "s-a")
	b := newTestBackend(t, bd.runner.run)
	require.NoError(t, b.CheckCompatibility())
	_, err := b.current.Load().db.ExecContext(context.Background(), "DROP TABLE comments")
	require.NoError(t, err)
	before := requestCount(bd.runner)

	_, err = b.Services().TaskExecutor.GetComments("s-a")
	require.ErrorContains(t, err, "reading snapshot comments of s-a")
	require.ErrorContains(t, err, "no such table: comments")
	require.Equal(t, before, requestCount(bd.runner), "no bd on a snapshot read error")
}

// TestSnapshotBackend_ReadsAfterClose: after Close, GetComments fails with
// errSnapshotBackendClosed without running bd, whether or not a snapshot was
// built, and ShowIssue fails with the closed backend's context.
func TestSnapshotBackend_ReadsAfterClose(t *testing.T) {
	for _, built := range []bool{true, false} {
		t.Run(fmt.Sprintf("snapshot built %v", built), func(t *testing.T) {
			bd := newCommentBD(t, "s-a")
			b := newTestBackend(t, bd.runner.run)
			if built {
				require.NoError(t, b.CheckCompatibility())
			}
			te := b.Services().TaskExecutor
			require.NoError(t, b.Close())
			before := requestCount(bd.runner)

			_, err := te.GetComments("s-a")
			require.ErrorIs(t, err, errSnapshotBackendClosed)
			require.Equal(t, before, requestCount(bd.runner), "no bd after Close")

			_, err = te.ShowIssue("s-a")
			require.ErrorIs(t, err, errSnapshotBackendClosed)
		})
	}
}

// TestSnapshotBackend_WritesRecordOwnWrites: every IssueWriter method, on
// success, records an own write (ownWrite bumps writeSeq and asks for a
// rebuild, see TestSnapshotScheduler_OwnWritesTrigger); on failure it does
// not. No write builds a snapshot.
func TestSnapshotBackend_WritesRecordOwnWrites(t *testing.T) {
	title, notes := "New title", "New notes"
	writes := []struct {
		name  string
		write func(task.TaskExecutor) error
	}{
		{"UpdateStatus", func(te task.TaskExecutor) error { return te.UpdateStatus("s-a", task.StatusClosed) }},
		{"UpdatePriority", func(te task.TaskExecutor) error { return te.UpdatePriority("s-a", task.PriorityHigh) }},
		{"UpdateType", func(te task.TaskExecutor) error { return te.UpdateType("s-a", task.TypeBug) }},
		{"UpdateTitle", func(te task.TaskExecutor) error { return te.UpdateTitle("s-a", title) }},
		{"UpdateDescription", func(te task.TaskExecutor) error { return te.UpdateDescription("s-a", "text") }},
		{"UpdateNotes", func(te task.TaskExecutor) error { return te.UpdateNotes("s-a", notes) }},
		{"CloseIssue", func(te task.TaskExecutor) error { return te.CloseIssue("s-a", "done") }},
		{"ReopenIssue", func(te task.TaskExecutor) error { return te.ReopenIssue("s-a") }},
		{"SetLabels", func(te task.TaskExecutor) error { return te.SetLabels("s-a", []string{"x"}) }},
		{"AddComment", func(te task.TaskExecutor) error { return te.AddComment("s-a", "alice", "hello") }},
		{"CreateEpic", func(te task.TaskExecutor) error {
			_, err := te.CreateEpic("Epic", "text", nil)
			return err
		}},
		{"CreateTask", func(te task.TaskExecutor) error {
			_, err := te.CreateTask("Task", "text", "s-a", "alice", []string{"x"})
			return err
		}},
		{"DeleteIssues", func(te task.TaskExecutor) error { return te.DeleteIssues([]string{"s-a"}) }},
		{"AddDependency", func(te task.TaskExecutor) error { return te.AddDependency("s-a", "s-b") }},
		{"UpdateIssue", func(te task.TaskExecutor) error {
			return te.UpdateIssue("s-a", task.UpdateOptions{Title: &title, Notes: &notes})
		}},
	}
	require.Len(t, writes, reflect.TypeFor[appbeads.IssueWriter]().NumMethod(), "every IssueWriter method is covered")

	for _, tt := range writes {
		t.Run(tt.name, func(t *testing.T) {
			var failing atomic.Bool
			runner := &fakeRunner{answer: func(_ context.Context, req infrabeads.BDRequest) (string, error) {
				switch {
				case bdOp(req) == "export":
					return "", errors.New("a write must not build a snapshot")
				case failing.Load():
					return "", fmt.Errorf("bd %s failed: boom", bdOp(req))
				case bdOp(req) == "create":
					return `{"id":"s-new","title":"New"}`, nil
				case bdOp(req) == "comments":
					return "[]", nil
				}
				return "", nil
			}}
			b := newTestBackend(t, runner.run)
			te := b.Services().TaskExecutor

			require.NoError(t, tt.write(te))
			require.NoError(t, tt.write(te))
			require.Equal(t, uint64(2), b.writeSeq.Load())

			failing.Store(true)
			require.ErrorContains(t, tt.write(te), "boom")
			require.Equal(t, uint64(2), b.writeSeq.Load(), "a failed write is not counted")
			require.Zero(t, runner.count("export"))
		})
	}
}

// TestSnapshotBackend_CloseDoesNotCancelWrites: writes do not run with the
// backend's context, so Close neither kills one in flight nor refuses a
// later one.
func TestSnapshotBackend_CloseDoesNotCancelWrites(t *testing.T) {
	writing := make(chan struct{})
	release := make(chan struct{})
	releaseWrite := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseWrite)
	runner := &fakeRunner{answer: func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if bdOp(req) == "close" {
			close(writing)
			<-release
		}
		if err := context.Cause(ctx); err != nil {
			return "", err
		}
		return "", nil
	}}
	b := newTestBackend(t, runner.run)
	te := b.Services().TaskExecutor

	done := make(chan error, 1)
	go func() { done <- te.CloseIssue("s-a", "done") }()
	<-writing
	require.NoError(t, b.Close())
	releaseWrite()
	require.NoError(t, <-done, "the write in flight completes")
	require.NoError(t, te.UpdateStatus("s-a", task.StatusOpen), "a write after Close runs")
	require.Equal(t, uint64(2), b.writeSeq.Load())
}

// TestSnapshotBackend_ShowIssueRunsReadonlyShow: ShowIssue always runs
// bd --readonly show, before and after the first snapshot, and builds none.
func TestSnapshotBackend_ShowIssueRunsReadonlyShow(t *testing.T) {
	bd := newCommentBD(t, "s-a")
	b := newTestBackend(t, bd.runner.run)
	te := b.Services().TaskExecutor
	show := []string{"--readonly", "show", "s-a", "--json"}

	issue, err := te.ShowIssue("s-a")
	require.NoError(t, err)
	require.Equal(t, "s-a", issue.ID)
	require.Equal(t, [][]string{show}, requestArgs(bd.runner, 0), "no export")

	require.NoError(t, b.CheckCompatibility())
	before := requestCount(bd.runner)
	_, err = te.ShowIssue("s-a")
	require.NoError(t, err)
	require.Equal(t, [][]string{show}, requestArgs(bd.runner, before), "fresh from bd once a snapshot exists")
}
