package adapter

// Opt-in parity tests of the SnapshotBackend against the real bd on PATH.
//
// They run real bd writes in throwaway embedded workspaces, so they are
// skipped unless PERLES_BD_INTEGRATION=1 is set: `make test` and CI never run
// them. Run them with:
//
//	PERLES_BD_INTEGRATION=1 go test ./internal/beads/adapter/ -run Integration -v
//
// Once opted in, a test skips, with a logged reason, only when:
//
//   - -short is set;
//   - bd is not on PATH;
//   - the installed bd is older than domain.MinEmbeddedBeadsVersion (1.0.5),
//     checked with the production gate (infrabeads.CheckEmbeddedBDVersion);
//   - bd refuses the workspace location as an "unsafe location".
//
// Any other failure, setup included, fails the test. Parts of the parity test
// also skip, with a logged reason, when the installed bd lacks what they
// check: the upgrade path needs `blocked --flags --raw` (no released bd up to
// v1.3.2 has it), and the export.exclude_owners and `bd schema` checks need
// bd >= 1.2.0.
//
// Safety: each workspace is a fresh os.MkdirTemp directory under the user's
// home (not t.TempDir(): on Linux that is under /tmp, which bd context
// refuses as unsafe), removed by t.Cleanup. Every BEADS_* and BD_* variable
// is cleared and BEADS_DIR is set to the workspace's .beads for the test, and
// every bd runs with its working directory in the workspace. Before the first
// bd write (bd init), bd context must not resolve any other beads directory;
// after it, bd context must report the workspace's own embedded beads dir.
// This repository's .beads is never touched.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/task"
)

// integrationEnv opts in to the real-bd integration tests.
const integrationEnv = "PERLES_BD_INTEGRATION"

// integrationTimeout bounds each bd command the tests run themselves.
const integrationTimeout = 2 * time.Minute

// integrationPickupTimeout bounds the wait for a rebuild. Rebuilds are paced
// at least minRebuildGap (2s) after the previous one, plus the build itself.
const integrationPickupTimeout = 30 * time.Second

// integrationOtherAuthor is the created_by of the issue that the parity
// workspace's export.exclude_owners excludes from bd's own exports.
const integrationOtherAuthor = "perles-it-other@example.com"

// TestSnapshotBackend_Integration_Parity builds every edge case of the
// proposal's E16 in a real embedded workspace and checks the snapshot's
// blocked and ready sets against bd's own answers, on the listing path
// (forced, as on a released bd) and, when the installed bd has it, on the
// --flags --raw upgrade path. It also checks that the snapshot holds exactly
// bd's issues (wisps excluded, export.exclude_owners neutralized without
// touching config.yaml), that the real export, comments included, loads into
// the snapshot DDL, metadata queries, and bd's JSON schema version.
func TestSnapshotBackend_Integration_Parity(t *testing.T) {
	w := newIntegrationWorkspace(t, "ipy")
	e := buildIntegrationEdgeCases(t, w)
	configPath := filepath.Join(w.beadsDir, "config.yaml")
	configBefore, err := os.ReadFile(configPath) //nolint:gosec // G304: the throwaway workspace's config
	require.NoError(t, err)

	// The listing path, forced: the --flags read is answered like a
	// released bd, which rejects the flag.
	b, rec := w.backend(t, integrationReleasedBD(w.run))
	require.NoError(t, b.CheckCompatibility(), "the real export, blocked listing and statuses load into the snapshot")
	require.Equal(t, 1, integrationBlockedReads(rec, true), "one --flags probe")
	require.Equal(t, 1, integrationBlockedReads(rec, false), "then the bd blocked listing")
	require.Equal(t, 1, rec.count("statuses"), "custom statuses are in use")
	require.Zero(t, rec.count("types"), "bd types is never run")

	// The issue set: bd list, which ignores export.exclude_owners, minus
	// wisps, which perles never shows (bd 1.3.2 lists none; bd 1.0.5 lists
	// them, marked ephemeral).
	var listed []integrationRow
	for _, row := range integrationDecode[[]integrationRow](t, "bd list",
		w.bd(t, "--readonly", "list", "--all", "--json", "--limit", "0")) {
		if !row.Ephemeral {
			listed = append(listed, row)
		}
	}
	all := integrationIDs(listed)
	snapshotAll := integrationQuery(t, b, `id != ""`)
	require.Equal(t, all, snapshotAll, "the snapshot holds exactly bd's non-ephemeral issues")

	t.Run("wisps excluded", func(t *testing.T) {
		require.NotContains(t, snapshotAll, e.wisp)
		require.Empty(t, integrationQuery(t, b, fmt.Sprintf("id = %q", e.wisp)))
		if slices.Contains(integrationExportIDs(t, w.bd(t, "--readonly", "export", "--all")), e.wisp) {
			t.Log("bd's export carries the wisp; the snapshot loader skips it")
		} else {
			t.Log("this bd's export omits the wisp")
		}
	})

	t.Run("exclude_owners handled", func(t *testing.T) {
		require.Contains(t, snapshotAll, e.excludedOwner)
		if slices.Contains(integrationExportIDs(t, w.bd(t, "--readonly", "export", "--all")), e.excludedOwner) {
			t.Skip("this bd's export ignores export.exclude_owners (added in bd v1.2.0): there is nothing to neutralize")
		}
		t.Log("bd's own export honours export.exclude_owners; perles' export neutralizes it")
	})

	t.Run("blocked parity, listing path", func(t *testing.T) {
		rows := integrationDecode[[]integrationRow](t, "bd blocked", w.bd(t, "--readonly", "blocked", "--json"))
		var want []string
		for _, row := range rows {
			if !integrationOnlyExternal(row.BlockedBy) {
				want = append(want, row.ID)
			}
		}
		slices.Sort(want)
		blocked := integrationQuery(t, b, "blocked = true")
		require.Equal(t, want, blocked, "bd blocked --json minus the external-only rows")

		for _, id := range []string{e.blockedEpic, e.epicChild, e.blockedByShipped, e.waiter} {
			require.Contains(t, blocked, id)
		}
		for _, id := range []string{e.externalOnly, e.blockedByClosed, e.pinned} {
			require.NotContains(t, blocked, id)
		}
	})

	t.Run("ready parity", func(t *testing.T) {
		bdReady := integrationIDs(integrationDecode[[]integrationRow](t, "bd ready",
			w.bd(t, "--readonly", "ready", "--json", "--limit", "0")))
		ready := integrationQuery(t, b, "ready = true")
		require.Subset(t, ready, bdReady, "snapshot ready includes bd ready")
		extra := integrationMinus(ready, bdReady)
		for _, id := range extra {
			t.Logf("ready = true beyond bd ready: %s %q", id, integrationIssue(t, b, id).TitleText)
		}
		// bd ready leaves out issues blocked only by an external dependency
		// (bd's external-dependency policy); perles' stored-flag semantics
		// count them as ready.
		allowed := []string{e.externalOnly}
		if !slices.Contains(bdReady, e.inReview) {
			// bd 1.0.5's ready does not count custom active statuses as
			// ready; bd 1.3.2's does, as perles does in both modes.
			t.Logf("this bd's ready leaves out the custom active status issue %s", e.inReview)
			allowed = append(allowed, e.inReview)
		}
		require.Subset(t, allowed, extra, "only the external-only issue differs")
		require.Contains(t, ready, e.inReview, "a custom active status is ready")
		require.NotContains(t, ready, e.deferredChild, "a deferred parent's child is not ready")
	})

	t.Run("metadata", func(t *testing.T) {
		require.Equal(t, []string{e.withMetadata}, integrationQuery(t, b, `metadata.team = "backend"`))
		require.Equal(t, integrationMinus(all, []string{e.withMetadata}), integrationQuery(t, b, `metadata.team = nil`),
			"every issue but the one carrying the key")
	})

	t.Run("comments load with bd's IDs", func(t *testing.T) {
		bdComments := integrationDecode[[]task.Comment](t, "bd comments",
			w.bd(t, "--readonly", "comments", e.commented, "--json"))
		require.Len(t, bdComments, 1)
		comments, err := b.Services().TaskExecutor.GetComments(e.commented)
		require.NoError(t, err)
		require.Len(t, comments, 1)
		require.Equal(t, bdComments[0].ID, comments[0].ID, "TEXT comment IDs (bd's are UUID strings)")
		require.Equal(t, bdComments[0].Text, comments[0].Text)
	})

	t.Run("blocked parity, upgrade path", func(t *testing.T) {
		up, upRec := w.backend(t, nil)
		require.NoError(t, up.CheckCompatibility())
		if integrationBlockedReads(upRec, false) > 0 {
			t.Skip("the installed bd has no blocked --flags --raw (detection fell back to the listing, as on every released bd): " +
				"the upgrade path needs a bd built after beads 4450fe99b")
		}
		require.Equal(t, 1, integrationBlockedReads(upRec, true), "the --flags --raw read on the candidates")

		out := w.bdRequest(t, infrabeads.BDRequest{
			Args:  []string{"--readonly", "blocked", "--flags", "--raw", "--json"},
			Stdin: strings.Join(all, "\n") + "\n",
		})
		var want []string
		for _, row := range integrationDecode[[]integrationRow](t, "bd blocked --flags", out) {
			if row.IsBlocked {
				want = append(want, row.ID)
			}
		}
		slices.Sort(want)
		require.Equal(t, want, integrationQuery(t, up, "blocked = true"), "bd blocked --flags --raw over every ID")
	})

	t.Run("bd JSON schema version", func(t *testing.T) {
		require.True(t, w.verified)
		ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
		defer cancel()
		out, err := w.run(ctx, infrabeads.BDRequest{Args: []string{"schema", "--json"}})
		if err != nil && strings.Contains(err.Error(), `unknown command "schema"`) {
			t.Skipf("this bd has no bd schema (added in bd v1.2.0): %v", err)
		}
		require.NoError(t, err, "bd schema --json")
		schema := integrationDecode[struct {
			SchemaVersion int `json:"schema_version"`
		}](t, "bd schema", out)
		require.Equal(t, 1, schema.SchemaVersion, "the export records perles decodes")
	})

	configAfter, err := os.ReadFile(configPath) //nolint:gosec // G304: the throwaway workspace's config
	require.NoError(t, err)
	require.Equal(t, string(configBefore), string(configAfter), "perles never edits config.yaml")
}

// TestSnapshotBackend_Integration_TrailingWrite: a real write that lands
// after the first snapshot's export read the database, but before the
// snapshot is installed, is picked up with no further trigger. The first
// snapshot's manifest hash predates the write and no watcher was running
// when it landed, so only the post-install re-hash can see it. The manifest
// watcher is disabled to make sure of that.
func TestSnapshotBackend_Integration_TrailingWrite(t *testing.T) {
	w := newIntegrationWorkspace(t, "itw")
	id := w.create(t, "trailing write target", "--priority", "2")

	var wrote atomic.Bool
	trailingWrite := func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		out, err := w.run(ctx, req)
		if err != nil || bdCommand(req) != "export" || !wrote.CompareAndSwap(false, true) {
			return out, err
		}
		// The first export has finished: land a write before handing
		// its output back.
		if _, err := w.run(ctx, infrabeads.BDRequest{Args: []string{"update", id, "--priority=0", "--json"}}); err != nil {
			return "", fmt.Errorf("injected write: %w", err)
		}
		return out, nil
	}
	b, rec := w.backend(t, trailingWrite)
	b.sched.newWatcher = func() (manifestWatcher, error) {
		return nil, errors.New("disabled by the test: the pickup must come from the post-install re-hash")
	}

	require.NoError(t, b.CheckCompatibility())
	require.True(t, wrote.Load(), "the write landed during the first build")
	require.Equal(t, task.Priority(2), integrationIssue(t, b, id).Priority, "the first snapshot predates the write")

	require.Eventually(t, func() bool {
		issues, err := b.Services().QueryExecutor.Execute(fmt.Sprintf("id = %q", id))
		return err == nil && len(issues) == 1 && issues[0].Priority == 0
	}, integrationPickupTimeout, 100*time.Millisecond, "a newer snapshot shows the trailing write")
	require.Equal(t, 2, rec.count("export"), "one follow-up build")
}

// TestSnapshotBackend_Integration_CommentReadAfterWrite: after AddComment,
// ShowIssue then GetComments (addCommentCmd's sequence) return the new
// comment before the next snapshot lands. The test holds that snapshot's
// export until it has read the comment, then lets it land and reads the
// comment from the snapshot itself.
func TestSnapshotBackend_Integration_CommentReadAfterWrite(t *testing.T) {
	w := newIntegrationWorkspace(t, "icm")
	id := w.create(t, "commented issue")

	release := make(chan struct{})
	var exports atomic.Int32
	holdLaterExports := func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if bdCommand(req) == "export" && exports.Add(1) > 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return "", context.Cause(ctx)
			}
		}
		return w.run(ctx, req)
	}
	b, _ := w.backend(t, holdLaterExports)
	require.NoError(t, b.CheckCompatibility())
	first := b.current.Load()
	require.NotNil(t, first)

	const text = "read after write"
	te := b.Services().TaskExecutor
	require.NoError(t, te.AddComment(id, "perles-it", text))
	issue, err := te.ShowIssue(id)
	require.NoError(t, err)
	require.Equal(t, id, issue.ID)
	comments, err := te.GetComments(id)
	require.NoError(t, err)
	require.Equal(t, []string{text}, commentTexts(comments))
	require.Same(t, first, b.current.Load(), "served before the next snapshot landed")

	close(release)
	require.Eventually(t, func() bool { return b.current.Load() != first && overlaySize(b) == 0 },
		integrationPickupTimeout, 50*time.Millisecond, "a snapshot covering the comment replaces the overlay")
	comments, err = te.GetComments(id)
	require.NoError(t, err)
	require.Equal(t, []string{text}, commentTexts(comments))
}

// TestSnapshotBackend_Integration_DaemonLazy: a backend built the way
// NewBeadsBackend builds it and never queried runs no bd export, even when
// the workspace changes, and starts no scheduler or watcher. Comments read
// before the first snapshot come from bd comments, not an export. The first
// query exports.
func TestSnapshotBackend_Integration_DaemonLazy(t *testing.T) {
	w := newIntegrationWorkspace(t, "ilz")
	id := w.create(t, "lazy issue")
	w.bd(t, "comments", "add", id, "--", "before any snapshot")

	b, rec := w.backend(t, nil)
	w.bd(t, "update", id, "--priority=1")
	time.Sleep(2 * manifestDebounce) // longer than a watcher's debounce
	require.Zero(t, rec.count(""), "constructing the backend runs no bd")
	require.False(t, loopsStarted(b), "nor the scheduler or the watcher")

	comments, err := b.Services().TaskExecutor.GetComments(id)
	require.NoError(t, err)
	require.Equal(t, []string{"before any snapshot"}, commentTexts(comments))
	require.Equal(t, []string{"comments"}, rec.commands(), "bd comments, no export")
	require.Nil(t, b.current.Load())

	require.Equal(t, task.Priority(1), integrationIssue(t, b, id).Priority)
	require.Equal(t, 1, rec.count("export"), "the first query builds the first snapshot")
}

// integrationWorkspace is a throwaway embedded beads workspace under the
// user's home. run is the production bd runner for it.
type integrationWorkspace struct {
	dir, beadsDir string
	// prefix is the workspace's issue prefix.
	prefix string
	run    infrabeads.BDRunner
	// verified is set once bd context reported the workspace's own beads
	// dir after bd init; bd refuses to run other commands before.
	verified bool
}

// newIntegrationWorkspace skips unless the real-bd tests can run (see the
// file comment), then creates an embedded workspace whose issue prefix is
// prefix and isolates the environment for it.
func newIntegrationWorkspace(t *testing.T, prefix string) *integrationWorkspace {
	t.Helper()
	integrationRequireBD(t)

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dir, err := os.MkdirTemp(home, "perles-embed-it-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing the integration workspace %s: %v", dir, err)
		}
	})
	dir, err = filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	w := &integrationWorkspace{dir: dir, beadsDir: filepath.Join(dir, ".beads"), prefix: prefix}
	integrationIsolateEnv(t, w.beadsDir)
	w.run = infrabeads.NewBDRunner(w.dir, w.beadsDir)

	integrationGit(t, w.dir, "init", "-q")
	w.requireOwnContext(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	if _, err := w.run(ctx, infrabeads.BDRequest{Args: []string{"init", "--prefix", prefix, "--non-interactive"}}); err != nil {
		integrationSkipUnsafe(t, "bd init", err.Error())
		t.Fatalf("bd init in %s: %v", w.dir, err)
	}
	w.requireOwnContext(t, true)
	w.verified = true
	t.Logf("integration workspace %s", w.dir)
	return w
}

// integrationRequireBD skips the test unless it is opted in, not -short,
// and the bd on PATH is at least domain.MinEmbeddedBeadsVersion.
func integrationRequireBD(t *testing.T) {
	t.Helper()
	if os.Getenv(integrationEnv) != "1" {
		t.Skipf("set %s=1 to run the real-bd integration tests (they run bd writes in throwaway workspaces under your home directory)", integrationEnv)
	}
	if testing.Short() {
		t.Skip("-short: skipping the real-bd integration test")
	}
	path, err := exec.LookPath("bd")
	if err != nil {
		t.Skipf("bd is not on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version") //nolint:gosec // G204: the bd on PATH
	cmd.Dir = os.TempDir()
	cmd.Env = integrationBDEnv("")
	out, err := cmd.Output()
	require.NoError(t, err, "bd --version")
	line := strings.TrimSpace(string(out))
	_, version, found := strings.Cut(line, "version ")
	if !found {
		version = line
	}
	version = strings.TrimSpace(version)
	require.NotEmpty(t, version, "bd --version printed no version: %q", line)
	if infrabeads.CheckEmbeddedBDVersion("", version, nil) != nil {
		t.Skipf("bd %s is older than %s, the oldest bd embedded workspaces support", version, domain.MinEmbeddedBeadsVersion)
	}
	t.Logf("bd: %s (%s)", path, line)
}

// integrationIsolateEnv clears every BEADS_* and BD_* variable, and the git
// variables that could point git at another repository, for the test, and
// sets BEADS_DIR to beadsDir.
func integrationIsolateEnv(t *testing.T, beadsDir string) {
	t.Helper()
	gitVars := []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY"}
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") || slices.Contains(gitVars, name) {
			t.Setenv(name, "") // restores the variable after the test
			require.NoError(t, os.Unsetenv(name))
		}
	}
	t.Setenv("BEADS_DIR", beadsDir)
}

// integrationBDEnv is bd's environment: the test's, with BEADS_DIR set to
// beadsDir (removed when it is empty) and the JSON envelope off.
func integrationBDEnv(beadsDir string) []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if name != "BEADS_DIR" && name != "BD_JSON_ENVELOPE" {
			env = append(env, kv)
		}
	}
	env = append(env, "BD_JSON_ENVELOPE=0")
	if beadsDir != "" {
		env = append(env, "BEADS_DIR="+beadsDir)
	}
	return env
}

// integrationBDContext is a real infrabeads.BDContextRunner: the default one
// refuses to run inside test binaries. It needs a working directory, so bd
// never runs from the package directory.
func integrationBDContext(ctx context.Context, beadsDir, workDir string) ([]byte, error) {
	if workDir == "" {
		return nil, errors.New("integration bd context: no working directory")
	}
	cmd := exec.CommandContext(ctx, "bd", "context", "--json")
	cmd.Dir = workDir
	cmd.Env = integrationBDEnv(beadsDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("bd context: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// requireOwnContext is the safety guard around bd writes. Before bd init
// (initialized false) bd context must not resolve any beads dir but the
// workspace's: it fails with no workspace, or names this one. After it, bd
// context must report the workspace's own embedded beads dir. An unsafe
// location refusal skips the test.
func (w *integrationWorkspace) requireOwnContext(t *testing.T, initialized bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	out, runErr := integrationBDContext(ctx, w.beadsDir, w.dir)
	text := string(out)
	if runErr != nil {
		text += "\n" + runErr.Error()
	}
	integrationSkipUnsafe(t, "bd context", text)

	var answer struct {
		BeadsDir string `json:"beads_dir"`
		DoltMode string `json:"dolt_mode"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out), &answer); err != nil && runErr == nil {
		t.Fatalf("parsing bd context output %q: %v", out, err)
	}
	if answer.BeadsDir != "" {
		require.Equal(t, w.beadsDir, integrationCanonical(answer.BeadsDir),
			"SAFETY: bd context resolves another beads dir; refusing to run bd writes")
	}
	if !initialized {
		require.True(t, runErr != nil || answer.BeadsDir != "",
			"SAFETY: bd context succeeded without naming a beads dir before bd init: %s", text)
		return
	}
	require.NoError(t, runErr, "bd context after bd init")
	require.Equal(t, "embedded", answer.DoltMode, "bd init creates an embedded workspace")
}

// integrationSkipUnsafe skips the test when bd refused the workspace
// location as unsafe; text is what the step printed.
func integrationSkipUnsafe(t *testing.T, step, text string) {
	t.Helper()
	if strings.Contains(text, "unsafe location") {
		t.Skipf("%s refuses the workspace location as unsafe: %s", step, strings.TrimSpace(text))
	}
}

// integrationCanonical resolves path's symlinks, or cleans it when it does
// not exist.
func integrationCanonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

// integrationGit runs git in dir.
func integrationGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: fixed arguments
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

// bdRequest runs req with the workspace's runner and returns bd's stdout.
func (w *integrationWorkspace) bdRequest(t *testing.T, req infrabeads.BDRequest) string {
	t.Helper()
	require.True(t, w.verified, "SAFETY: bd context has not confirmed the workspace")
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	out, err := w.run(ctx, req)
	require.NoError(t, err, "bd %s", strings.Join(req.Args, " "))
	return out
}

// bd runs bd with args in the workspace and returns its stdout.
func (w *integrationWorkspace) bd(t *testing.T, args ...string) string {
	t.Helper()
	return w.bdRequest(t, infrabeads.BDRequest{Args: args})
}

// create creates an issue titled title, with bd create's flags, and returns
// its ID.
func (w *integrationWorkspace) create(t *testing.T, title string, flags ...string) string {
	t.Helper()
	id := strings.TrimSpace(w.bd(t, append([]string{"create", title, "--silent"}, flags...)...))
	require.NotEmpty(t, id, "bd create %q printed no ID", title)
	require.NotContains(t, id, " ", "bd create %q printed more than an ID", title)
	return id
}

// backend builds the workspace's backend as NewBeadsBackend does, with the
// real bd context and a recorded runner, which answers with answer or, when
// it is nil, with the production runner. The backend is closed when the test
// ends.
func (w *integrationWorkspace) backend(t *testing.T, answer infrabeads.BDRunner) (*SnapshotBackend, *fakeRunner) {
	t.Helper()
	f := newRunnerFactory(answer)
	backend, err := newBeadsBackend(w.beadsDir, w.dir, backendDeps{bdContext: integrationBDContext, newBDRunner: f.new})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	b, ok := backend.(*SnapshotBackend)
	require.True(t, ok, "an embedded workspace gets a SnapshotBackend, got %T", backend)
	require.Equal(t, [][2]string{{w.dir, w.beadsDir}}, f.created(), "the backend runs bd in the workspace")
	require.Equal(t, w.beadsDir, b.beadsDir)
	return b, f.runner
}

// integrationEdgeCases are the IDs of the parity workspace's issues.
type integrationEdgeCases struct {
	openBlocker, blockedEpic, epicChild string
	shippedBlocker, blockedByShipped    string
	spawner, spawnerChild, waiter       string
	externalOnly                        string
	closedBlocker, blockedByClosed      string
	pinBlocker, pinned                  string
	deferredParent, deferredChild       string
	inReview                            string
	wisp                                string
	excludedOwner                       string
	withMetadata                        string
	commented                           string
}

// buildIntegrationEdgeCases creates the proposal's E16 edge cases, an issue
// with metadata and one with a comment.
func buildIntegrationEdgeCases(t *testing.T, w *integrationWorkspace) integrationEdgeCases {
	t.Helper()
	var e integrationEdgeCases
	w.bd(t, "config", "set", "status.custom", "shipped:done,in_review:active,on_hold:frozen")

	// Blockedness flows down parent-child. The blocker is an epic too:
	// bd 1.0.5 lets only epics block epics.
	e.openBlocker = w.create(t, "open blocker", "--type", "epic")
	e.blockedEpic = w.create(t, "blocked epic", "--type", "epic")
	w.bd(t, "dep", "add", e.blockedEpic, e.openBlocker)
	e.epicChild = w.create(t, "child of the blocked epic", "--parent", e.blockedEpic)

	// A custom done status does not unblock.
	e.shippedBlocker = w.create(t, "shipped blocker")
	w.bd(t, "update", e.shippedBlocker, "--status", "shipped")
	e.blockedByShipped = w.create(t, "blocked by a shipped blocker")
	w.bd(t, "dep", "add", e.blockedByShipped, e.shippedBlocker)

	// A waits-for gate on a spawner with an open child.
	e.spawner = w.create(t, "spawner", "--type", "epic")
	e.spawnerChild = w.create(t, "spawner child", "--parent", e.spawner)
	e.waiter = w.create(t, "waiter", "--waits-for", e.spawner)

	e.externalOnly = w.create(t, "external-only dependency")
	w.bd(t, "dep", "add", e.externalOnly, "external:otherproj:somecap")

	e.closedBlocker = w.create(t, "closed blocker")
	e.blockedByClosed = w.create(t, "blocked by a closed blocker")
	w.bd(t, "dep", "add", e.blockedByClosed, e.closedBlocker)
	w.bd(t, "close", e.closedBlocker)

	e.pinBlocker = w.create(t, "pinned issue's blocker")
	e.pinned = w.create(t, "pinned issue")
	w.bd(t, "dep", "add", e.pinned, e.pinBlocker)
	w.bd(t, "update", e.pinned, "--status", "pinned")

	e.deferredParent = w.create(t, "deferred parent", "--type", "epic")
	e.deferredChild = w.create(t, "child of the deferred parent", "--parent", e.deferredParent)
	w.bd(t, "defer", e.deferredParent, "--until=2099-01-01")

	e.inReview = w.create(t, "in review")
	w.bd(t, "update", e.inReview, "--status", "in_review")

	e.wisp = w.create(t, "ephemeral wisp", "--ephemeral")

	// An issue by another author, excluded from bd's own exports.
	e.excludedOwner = w.prefix + "-xown"
	record, err := json.Marshal(map[string]any{
		"id": e.excludedOwner, "title": "imported from another author", "created_by": integrationOtherAuthor,
		"issue_type": "task", "priority": 2, "status": "open",
	})
	require.NoError(t, err)
	w.bdRequest(t, infrabeads.BDRequest{Args: []string{"import", "-"}, Stdin: string(record) + "\n"})
	w.bd(t, "config", "set", "export.exclude_owners", integrationOtherAuthor)

	e.withMetadata = w.create(t, "has metadata")
	w.bd(t, "update", e.withMetadata, "--metadata", `{"team":"backend"}`)

	e.commented = e.openBlocker
	w.bd(t, "comments", "add", e.commented, "--", "a comment from bd")
	return e
}

// integrationReleasedBD makes run answer like a released bd, which has no
// `blocked --flags`: the --flags read runs a bd command that has no --flags
// either instead, so bd itself rejects it with "unknown flag: --flags" and
// the backend's detection sees bd's real error.
func integrationReleasedBD(run infrabeads.BDRunner) infrabeads.BDRunner {
	return func(ctx context.Context, req infrabeads.BDRequest) (string, error) {
		if slices.Contains(req.Args, "--flags") {
			req.Args = []string{"--readonly", "list", "--flags", "--raw", "--json"}
			req.Stdin = ""
		}
		return run(ctx, req)
	}
}

// integrationBlockedReads counts the recorded bd blocked reads: the --flags
// reads when flags is set, the listing otherwise.
func integrationBlockedReads(rec *fakeRunner, flags bool) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	n := 0
	for _, req := range rec.requests {
		if bdCommand(req) == "blocked" && slices.Contains(req.Args, "--flags") == flags {
			n++
		}
	}
	return n
}

// integrationRow is one row of bd's JSON listings.
type integrationRow struct {
	ID        string   `json:"id"`
	Ephemeral bool     `json:"ephemeral"`
	BlockedBy []string `json:"blocked_by"`
	IsBlocked bool     `json:"is_blocked"`
}

// integrationDecode decodes bd's JSON output; empty output is T's zero
// value.
func integrationDecode[T any](t *testing.T, what, out string) T {
	t.Helper()
	var v T
	if strings.TrimSpace(out) == "" {
		return v
	}
	require.NoError(t, json.Unmarshal([]byte(out), &v), "parsing %s output: %s", what, out)
	return v
}

// integrationExportIDs returns the issue IDs of a bd export, sorted.
func integrationExportIDs(t *testing.T, out string) []string {
	t.Helper()
	var ids []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record struct {
			Type string `json:"_type"` //nolint:tagliatelle // bd's record type key
			ID   string `json:"id"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &record), "parsing bd export line: %s", line)
		if record.Type == "" || record.Type == "issue" {
			ids = append(ids, record.ID)
		}
	}
	require.NoError(t, scanner.Err())
	slices.Sort(ids)
	return ids
}

// integrationIDs returns the rows' IDs, sorted.
func integrationIDs(rows []integrationRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	slices.Sort(ids)
	return ids
}

// integrationQuery runs a BQL query on the backend and returns the IDs,
// sorted.
func integrationQuery(t *testing.T, b *SnapshotBackend, query string) []string {
	t.Helper()
	issues, err := b.Services().QueryExecutor.Execute(query)
	require.NoError(t, err, query)
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	slices.Sort(ids)
	return ids
}

// integrationIssue queries one issue by ID.
func integrationIssue(t *testing.T, b *SnapshotBackend, id string) task.Issue {
	t.Helper()
	issues, err := b.Services().QueryExecutor.Execute(fmt.Sprintf("id = %q", id))
	require.NoError(t, err)
	require.Len(t, issues, 1)
	return issues[0]
}

// integrationMinus returns the IDs of a that are not in b, in a's order.
func integrationMinus(a, b []string) []string {
	out := []string{}
	for _, id := range a {
		if !slices.Contains(b, id) {
			out = append(out, id)
		}
	}
	return out
}

// integrationOnlyExternal reports whether blockedBy is non-empty and holds
// only external: refs.
func integrationOnlyExternal(blockedBy []string) bool {
	if len(blockedBy) == 0 {
		return false
	}
	for _, blocker := range blockedBy {
		if !strings.HasPrefix(blocker, "external:") {
			return false
		}
	}
	return true
}
