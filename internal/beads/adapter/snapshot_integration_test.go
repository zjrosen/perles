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
//   - bd refuses the workspace location as an "unsafe location" (bd context
//     checks it once the workspace's .beads exists, so after bd init).
//
// A `bd --version` it cannot parse fails the test, as does any other failure,
// setup included. Parts of the parity test also skip, with a logged reason,
// when the installed bd lacks what they check. The upgrade path needs
// `blocked --flags --raw`, which the test asks bd for itself (no released bd
// up to v1.3.2 has it). The export.exclude_owners and `bd schema` checks need
// bd >= 1.3.0, decided by the parsed version, as is the one ready difference
// allowed on bd < 1.3.1: see integrationExcludeOwnersBD, integrationSchemaBD
// and integrationActiveReadyBD. A release candidate of one of those versions
// counts as older than it.
//
// Safety: each workspace is a fresh os.MkdirTemp directory under the user's
// home (not t.TempDir(): on Linux that is under /tmp, which bd context
// refuses as unsafe), removed by t.Cleanup (the removal uses the path
// MkdirTemp returned, before any symlink resolution). Every BEADS_* and BD_*
// variable is cleared and BEADS_DIR is set to the workspace's .beads for the
// test, and every bd runs with its working directory in the workspace. Before
// the first bd write (bd init), bd context must not resolve any other beads
// directory. After it, bd context must succeed and report exactly the
// workspace's own embedded beads dir, or the test stops. This repository's
// .beads is never touched.

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
	"regexp"
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
// at least minRebuildGap (2s) after the previous one, plus the build itself;
// the rest is room for a slow machine.
const integrationPickupTimeout = 60 * time.Second

// integrationOtherAuthor is the created_by of the issue that the parity
// workspace's export.exclude_owners excludes from bd's own exports.
const integrationOtherAuthor = "perles-it-other@example.com"

// The first bd releases with what the parity test's version-gated checks
// need. bd v1.2.1 first shipped export.exclude_owners and `bd schema`, but it
// was pulled, and v1.2.2 re-released the v1.1.2 code without them, so v1.3.0
// is the first release with both. bd ready counts custom active statuses as
// ready from v1.3.1 (beads #5918). A gate's release candidates may lack what
// it checks (see bdBefore).
const (
	integrationExcludeOwnersBD = "1.3.0"
	integrationSchemaBD        = "1.3.0"
	integrationActiveReadyBD   = "1.3.1"
)

// integrationExternalRef is the external dependency of the parity
// workspace's external-only and mixed issues.
const integrationExternalRef = "external:otherproj:somecap"

// integrationVersion finds the version, with any pre-release suffix, in
// `bd --version` output, e.g. "bd version 1.3.2 (6ebaacba8)" or
// "bd version 1.3.1-rc.1 (0a1b2c3d4)".
var integrationVersion = regexp.MustCompile(`\bversion v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)`)

// TestSnapshotBackend_Integration_Parity builds every edge case of the
// proposal's E16 in a real embedded workspace and checks the snapshot's
// blocked and ready sets against bd's own answers, on the listing path
// (forced, as on a released bd) and, when the installed bd has it, on the
// --flags --raw upgrade path. It also checks that the snapshot holds exactly
// bd's issues (wisps excluded, export.exclude_owners neutralized without
// touching config.yaml), that the real export, comments included, loads into
// the snapshot DDL, metadata queries, and bd's JSON schema version.
func TestSnapshotBackend_Integration_Parity(t *testing.T) {
	w := integrationNewWorkspace(t, "ipy")
	e := integrationBuildEdgeCases(t, w)
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
		require.Equal(t, []string{e.excludedOwner}, integrationQuery(t, b, fmt.Sprintf("created_by = %q", integrationOtherAuthor)),
			"the snapshot keeps the imported created_by")
		if w.bdBefore(integrationExcludeOwnersBD) {
			t.Skipf("bd %s has no export.exclude_owners (bd >= %s has): there is nothing to neutralize", w.bdVersion, integrationExcludeOwnersBD)
		}
		require.NotContains(t, integrationExportIDs(t, w.bd(t, "--readonly", "export", "--all")), e.excludedOwner,
			"bd's own export honours export.exclude_owners; perles' export neutralizes it")
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

		// The mixed issue keeps its open blocker: only rows whose blockers
		// are all external drop out.
		for _, id := range []string{e.blockedEpic, e.epicChild, e.blockedByShipped, e.waiter, e.mixed} {
			require.Contains(t, blocked, id)
		}
		for _, id := range []string{e.externalOnly, e.blockedByClosed, e.pinned} {
			require.NotContains(t, blocked, id)
		}

		// The mixed row tests the external-only rule only when bd lists
		// external refs in blocked_by, which bd 1.0.5 leaves out; bd lists
		// them if it lists the external-only issue's.
		row := func(id string) integrationRow {
			i := slices.IndexFunc(rows, func(r integrationRow) bool { return r.ID == id })
			if i < 0 {
				return integrationRow{}
			}
			return rows[i]
		}
		mixed := row(e.mixed)
		require.Contains(t, mixed.BlockedBy, e.mixedBlocker, "bd blocked lists the mixed issue's open blocker")
		if slices.Contains(row(e.externalOnly).BlockedBy, integrationExternalRef) {
			require.Contains(t, mixed.BlockedBy, integrationExternalRef,
				"bd lists external refs (the external-only issue's), so the mixed row carries both kinds")
			t.Logf("bd %s lists external refs: the mixed row %s is blocked by %v", w.bdVersion, e.mixed, mixed.BlockedBy)
		} else {
			t.Logf("bd %s leaves external refs out of blocked_by: the mixed row %s (blocked by %v) checks only its open blocker",
				w.bdVersion, e.mixed, mixed.BlockedBy)
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
		if w.bdBefore(integrationActiveReadyBD) {
			// Before bd 1.3.1, bd ready does not count custom active
			// statuses as ready; perles does, in both modes.
			t.Logf("bd %s predates %s, whose ready counts custom active statuses: the in_review issue %s may differ",
				w.bdVersion, integrationActiveReadyBD, e.inReview)
			allowed = append(allowed, e.inReview)
		} else {
			require.Contains(t, bdReady, e.inReview, "bd ready counts a custom active status as ready")
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
		// Ask bd itself, so that a regression in the backend's detection
		// fails below instead of skipping.
		out, err := w.bdResult(t, infrabeads.BDRequest{
			Args:  []string{"--readonly", "blocked", "--flags", "--raw", "--json"},
			Stdin: strings.Join(all, "\n") + "\n",
		})
		if err != nil && (strings.Contains(err.Error(), "unknown flag: --flags") || strings.Contains(err.Error(), "unknown flag: --raw")) {
			t.Skipf("bd %s rejects blocked --flags --raw as unknown flags (no released bd up to v1.3.2 has them): "+
				"the upgrade path needs a bd built after beads 4450fe99b", w.bdVersion)
		}
		require.NoError(t, err, "bd blocked --flags --raw")
		var want []string
		for _, row := range integrationDecode[[]integrationRow](t, "bd blocked --flags", out) {
			if row.IsBlocked {
				want = append(want, row.ID)
			}
		}
		slices.Sort(want)

		up, upRec := w.backend(t, nil)
		require.NoError(t, up.CheckCompatibility())
		require.Equal(t, 1, integrationBlockedReads(upRec, true), "detection finds --flags: one --flags --raw read on the candidates")
		require.Zero(t, integrationBlockedReads(upRec, false), "and no listing")
		require.Equal(t, want, integrationQuery(t, up, "blocked = true"), "bd blocked --flags --raw over every ID")
	})

	t.Run("bd JSON schema version", func(t *testing.T) {
		if w.bdBefore(integrationSchemaBD) {
			t.Skipf("bd %s has no bd schema (bd >= %s has)", w.bdVersion, integrationSchemaBD)
		}
		schema := integrationDecode[struct {
			SchemaVersion int `json:"schema_version"`
		}](t, "bd schema", w.bd(t, "schema", "--json"))
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
// watcher is disabled to make sure of that. Rebuilds then settle: the
// follow-up's own re-hash finds the state it exported, so no build follows.
func TestSnapshotBackend_Integration_TrailingWrite(t *testing.T) {
	w := integrationNewWorkspace(t, "itw")
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

	// Wait for the follow-up to finish, then until pacing would allow
	// another build (at least minRebuildGap later), plus one build's time
	// (at least 1s) for its export to start, and count again.
	require.Eventually(t, func() bool { return !schedState(b).running },
		integrationPickupTimeout, 50*time.Millisecond, "the follow-up build finishes")
	b.sched.mu.Lock()
	nextBuild, buildTime := b.sched.nextBuildAt(), b.sched.lastDuration
	b.sched.mu.Unlock()
	time.Sleep(time.Until(nextBuild) + max(buildTime, time.Second))
	require.Equal(t, 2, rec.count("export"), "rebuilds settle: no build follows the follow-up")
}

// TestSnapshotBackend_Integration_CommentReadAfterWrite: after AddComment,
// ShowIssue then GetComments (addCommentCmd's sequence) return the new
// comment before the next snapshot lands. The test holds that snapshot's
// export until it has read the comment, then lets it land and reads the
// comment from the snapshot itself.
func TestSnapshotBackend_Integration_CommentReadAfterWrite(t *testing.T) {
	w := integrationNewWorkspace(t, "icm")
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
	w := integrationNewWorkspace(t, "ilz")
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

// TestSnapshotBackend_Integration_VersionGates needs no bd, so it is not
// opt-in: the parsed `bd --version` decides the parity test's version-gated
// checks, and a release candidate of a gate's version is older than it.
func TestSnapshotBackend_Integration_VersionGates(t *testing.T) {
	for _, tc := range []struct {
		line, version                     string
		beforeActiveReady, beforeExcluded bool
	}{
		{line: "bd version 1.3.2 (6ebaacba8)", version: "1.3.2"},
		{line: "bd version 1.3.1 (0a1b2c3d4)", version: "1.3.1"},
		{line: "bd version 1.3.1-rc.1 (0a1b2c3d4)", version: "1.3.1-rc.1", beforeActiveReady: true},
		{line: "bd version 1.3.2-rc.3", version: "1.3.2-rc.3"},
		{line: "bd version v1.3.0-rc.2+dirty", version: "1.3.0-rc.2", beforeActiveReady: true, beforeExcluded: true},
		{line: "bd version 1.0.5 (dev)", version: "1.0.5", beforeActiveReady: true, beforeExcluded: true},
	} {
		match := integrationVersion.FindStringSubmatch(tc.line)
		require.NotNil(t, match, tc.line)
		require.Equal(t, tc.version, match[1], tc.line)
		require.NoError(t, infrabeads.CheckEmbeddedBDVersion("", match[1], nil), "%s passes the production gate", tc.line)
		w := &integrationWorkspace{bdVersion: match[1]}
		require.Equal(t, tc.beforeActiveReady, w.bdBefore(integrationActiveReadyBD), "%s before %s", tc.line, integrationActiveReadyBD)
		require.Equal(t, tc.beforeExcluded, w.bdBefore(integrationExcludeOwnersBD), "%s before %s", tc.line, integrationExcludeOwnersBD)
	}
	require.Nil(t, integrationVersion.FindStringSubmatch("bd version unknown"))
}

// integrationWorkspace is a throwaway embedded beads workspace under the
// user's home. run is the production bd runner for it.
type integrationWorkspace struct {
	dir, beadsDir string
	// prefix is the workspace's issue prefix.
	prefix string
	// bdVersion is the installed bd's version, e.g. "1.3.2".
	bdVersion string
	run       infrabeads.BDRunner
	// verified is set once bd context reported the workspace's own beads
	// dir after bd init; bd refuses to run other commands before.
	verified bool
}

// integrationNewWorkspace skips unless the real-bd tests can run (see the
// file comment), then creates an embedded workspace whose issue prefix is
// prefix and isolates the environment for it.
func integrationNewWorkspace(t *testing.T, prefix string) *integrationWorkspace {
	t.Helper()
	return integrationInitWorkspace(t, "perles-embed-it-*", prefix, integrationRequireBD(t))
}

// integrationInitWorkspace creates an embedded workspace, for the installed
// bd of version bdVersion, in a fresh directory under the user's home named
// after pattern (see os.MkdirTemp), with issue prefix prefix. It isolates the
// environment for it and removes the directory when the test ends. Before
// bd init and after it, the safety guard (requireOwnContext) must pass.
func integrationInitWorkspace(t *testing.T, pattern, prefix, bdVersion string) *integrationWorkspace {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	created, err := os.MkdirTemp(home, pattern)
	require.NoError(t, err)
	// created is never reassigned: the cleanup removes exactly what
	// MkdirTemp created, even if resolving its symlinks fails below.
	t.Cleanup(func() {
		if err := os.RemoveAll(created); err != nil {
			t.Errorf("removing the workspace %s: %v", created, err)
		}
	})
	dir, err := filepath.EvalSymlinks(created)
	require.NoError(t, err)
	w := &integrationWorkspace{dir: dir, beadsDir: filepath.Join(dir, ".beads"), prefix: prefix, bdVersion: bdVersion}
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
	t.Logf("workspace %s", w.dir)
	return w
}

// bdBefore reports whether the installed bd is older than version, a final
// release. A pre-release of version ("1.3.1-rc.1") is older: the gates name
// the first release with what they check, which its release candidates may
// lack.
func (w *integrationWorkspace) bdBefore(version string) bool {
	release, prerelease, _ := strings.Cut(w.bdVersion, "-")
	cmp := domain.CompareVersions(release, version)
	return cmp < 0 || (cmp == 0 && prerelease != "")
}

// integrationRequireBD skips the test unless it is opted in and not -short,
// then checks the installed bd (see integrationBDVersion) and returns its
// version.
func integrationRequireBD(t *testing.T) string {
	t.Helper()
	if os.Getenv(integrationEnv) != "1" {
		t.Skipf("set %s=1 to run the real-bd integration tests (they run bd writes in throwaway workspaces under your home directory)", integrationEnv)
	}
	if testing.Short() {
		t.Skip("-short: skipping the real-bd integration test")
	}
	return integrationBDVersion(t)
}

// integrationBDVersion returns the version of the bd on PATH, e.g. "1.3.2",
// or "1.3.1-rc.1" for a release candidate. It skips the test when bd is not
// on PATH or is older than domain.MinEmbeddedBeadsVersion, checked with the
// production gate, and fails it when `bd --version` fails or prints no
// version it can parse.
func integrationBDVersion(t *testing.T) string {
	t.Helper()
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
	match := integrationVersion.FindStringSubmatch(line)
	if match == nil {
		t.Fatalf("cannot parse the bd version from `bd --version`: %q", line)
	}
	if infrabeads.CheckEmbeddedBDVersion("", match[1], nil) != nil {
		t.Skipf("bd %s is older than %s, the oldest bd embedded workspaces support", match[1], domain.MinEmbeddedBeadsVersion)
	}
	t.Logf("bd: %s (%s)", path, line)
	return match[1]
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
// context must succeed and report exactly the workspace's own beads dir, in
// embedded mode. Either way, a refusal of the location as unsafe skips the
// test; bd context checks the location only once a .beads exists, so the
// refusal shows after bd init (bd init itself does not check). Anything else
// stops the test.
func (w *integrationWorkspace) requireOwnContext(t *testing.T, initialized bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	out, runErr := integrationBDContext(ctx, w.beadsDir, w.dir)
	var answer struct {
		BeadsDir string `json:"beads_dir"`
		DoltMode string `json:"dolt_mode"`
	}
	parseErr := json.Unmarshal(bytes.TrimSpace(out), &answer)

	// With --json, bd prints its error on stdout.
	text := string(out)
	if runErr != nil {
		text += "\n" + runErr.Error()
	}
	integrationSkipUnsafe(t, "bd context", text)

	if initialized {
		require.NoError(t, runErr, "SAFETY: bd context in the workspace printed %q", out)
		require.NoError(t, parseErr, "SAFETY: parsing bd context output %q", out)
		require.NotEmpty(t, answer.BeadsDir, "SAFETY: bd context names no beads dir; refusing to run bd writes")
		require.Equal(t, w.beadsDir, integrationCanonical(answer.BeadsDir),
			"SAFETY: bd context resolves another beads dir; refusing to run bd writes")
		require.Equal(t, "embedded", answer.DoltMode, "bd init creates an embedded workspace")
		return
	}

	if parseErr != nil && runErr == nil {
		t.Fatalf("parsing bd context output %q: %v", out, parseErr)
	}
	if answer.BeadsDir != "" {
		require.Equal(t, w.beadsDir, integrationCanonical(answer.BeadsDir),
			"SAFETY: bd context resolves another beads dir; refusing to run bd writes")
	}
	require.True(t, runErr != nil || answer.BeadsDir != "",
		"SAFETY: bd context succeeded without naming a beads dir before bd init: %s", text)
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

// bdResult runs req with the workspace's runner and returns bd's stdout and
// the runner's error.
func (w *integrationWorkspace) bdResult(t *testing.T, req infrabeads.BDRequest) (string, error) {
	t.Helper()
	require.True(t, w.verified, "SAFETY: bd context has not confirmed the workspace")
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	defer cancel()
	return w.run(ctx, req)
}

// bdRequest runs req with the workspace's runner and returns bd's stdout.
func (w *integrationWorkspace) bdRequest(t *testing.T, req infrabeads.BDRequest) string {
	t.Helper()
	out, err := w.bdResult(t, req)
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

// integrationEdgeCases are the IDs of the parity workspace's issues that the
// checks name. The issues that only set up a case (the other blockers, the
// spawner and its child, the deferred parent) are integrationBuildEdgeCases'
// locals.
type integrationEdgeCases struct {
	blockedEpic, epicChild      string
	blockedByShipped            string
	waiter                      string
	externalOnly                string
	mixed, mixedBlocker         string
	blockedByClosed             string
	pinned                      string
	deferredChild               string
	inReview                    string
	wisp                        string
	excludedOwner, withMetadata string
	commented                   string
}

// integrationBuildEdgeCases creates the proposal's E16 edge cases, an issue
// with metadata and one with a comment.
func integrationBuildEdgeCases(t *testing.T, w *integrationWorkspace) integrationEdgeCases {
	t.Helper()
	var e integrationEdgeCases
	w.bd(t, "config", "set", "status.custom", "shipped:done,in_review:active,on_hold:frozen")

	// Blockedness flows down parent-child. The blocker is an epic too:
	// bd 1.0.5 lets only epics block epics.
	openBlocker := w.create(t, "open blocker", "--type", "epic")
	e.blockedEpic = w.create(t, "blocked epic", "--type", "epic")
	w.bd(t, "dep", "add", e.blockedEpic, openBlocker)
	e.epicChild = w.create(t, "child of the blocked epic", "--parent", e.blockedEpic)

	// A custom done status does not unblock.
	shippedBlocker := w.create(t, "shipped blocker")
	w.bd(t, "update", shippedBlocker, "--status", "shipped")
	e.blockedByShipped = w.create(t, "blocked by a shipped blocker")
	w.bd(t, "dep", "add", e.blockedByShipped, shippedBlocker)

	// A waits-for gate on a spawner with an open child.
	spawner := w.create(t, "spawner", "--type", "epic")
	w.create(t, "spawner child", "--parent", spawner)
	e.waiter = w.create(t, "waiter", "--waits-for", spawner)

	e.externalOnly = w.create(t, "external-only dependency")
	w.bd(t, "dep", "add", e.externalOnly, integrationExternalRef)

	// An open blocker and an external dependency: blocked, unlike the
	// external-only issue.
	e.mixedBlocker = w.create(t, "mixed issue's open blocker")
	e.mixed = w.create(t, "blocked by an open issue and an external dependency")
	w.bd(t, "dep", "add", e.mixed, e.mixedBlocker)
	w.bd(t, "dep", "add", e.mixed, integrationExternalRef)

	closedBlocker := w.create(t, "closed blocker")
	e.blockedByClosed = w.create(t, "blocked by a closed blocker")
	w.bd(t, "dep", "add", e.blockedByClosed, closedBlocker)
	w.bd(t, "close", closedBlocker)

	pinBlocker := w.create(t, "pinned issue's blocker")
	e.pinned = w.create(t, "pinned issue")
	w.bd(t, "dep", "add", e.pinned, pinBlocker)
	w.bd(t, "update", e.pinned, "--status", "pinned")

	deferredParent := w.create(t, "deferred parent", "--type", "epic")
	e.deferredChild = w.create(t, "child of the deferred parent", "--parent", deferredParent)
	w.bd(t, "defer", deferredParent, "--until=2099-01-01")

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
	shown := integrationDecode[[]struct {
		CreatedBy string `json:"created_by"`
	}](t, "bd show", w.bd(t, "--readonly", "show", e.excludedOwner, "--json"))
	require.Len(t, shown, 1, "bd show %s", e.excludedOwner)
	require.Equal(t, integrationOtherAuthor, shown[0].CreatedBy, "bd import keeps created_by, which export.exclude_owners matches")
	w.bd(t, "config", "set", "export.exclude_owners", integrationOtherAuthor)

	e.withMetadata = w.create(t, "has metadata")
	w.bd(t, "update", e.withMetadata, "--metadata", `{"team":"backend"}`)

	e.commented = openBlocker
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
