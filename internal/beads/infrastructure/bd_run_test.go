package infrastructure

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	domain "github.com/zjrosen/perles/internal/beads/domain"
)

func TestBDEnviron(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"BD_JSON_ENVELOPE=1",
		"BEADS_DIR=/inherited/.beads",
		"HOME=/home/me",
	}

	env := bdEnviron(base, "/abs/.beads")
	require.Equal(t, []string{
		"PATH=/usr/bin",
		"HOME=/home/me",
		"BD_JSON_ENVELOPE=0",
		"BEADS_DIR=/abs/.beads",
	}, env)

	// Without a beads dir the inherited BEADS_DIR is left alone.
	env = bdEnviron(base, "")
	require.Equal(t, []string{
		"PATH=/usr/bin",
		"BEADS_DIR=/inherited/.beads",
		"HOME=/home/me",
		"BD_JSON_ENVELOPE=0",
	}, env)
}

func TestAbsBeadsDir(t *testing.T) {
	require.Empty(t, absBeadsDir(""))
	// t.TempDir is absolute on every platform; /already/abs/.beads is not
	// absolute on Windows (no drive letter).
	abs := filepath.Join(t.TempDir(), ".beads")
	require.Equal(t, abs, absBeadsDir(abs))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cwd, ".beads"), absBeadsDir(".beads"))
}

// fakeBD is a shell stand-in for bd, answering by subcommand.
const fakeBD = `#!/bin/sh
case "$1" in
  env)
    printf 'BEADS_DIR=%s\n' "$BEADS_DIR"
    printf 'BD_JSON_ENVELOPE=%s\n' "${BD_JSON_ENVELOPE-unset}"
    printf 'CWD=%s\n' "$(pwd -P)"
    for a in "$@"; do printf 'ARG=%s\n' "$a"; done
    ;;
  update)
    echo "Error updating X-1: cannot close X-1: 1 open child issue(s); close children first or use --force to override" >&2
    echo '{"error":"1 of 1 issues failed to update","failed":[{"id":"X-1","error":"updating issue: cannot close X-1: 1 open child issue(s); close children first or use --force to override"}],"schema_version":1}' >&2
    exit 1
    ;;
  flagerr)
    printf '{\n  "error": "invalid priority \\"9\\"",\n  "schema_version": 1\n}\n'
    exit 1
    ;;
  frozen)
    echo "⛔ ERROR: workspace is frozen for migration." >&2
    exit 14
    ;;
  hang)
    exec sleep 10
    ;;
esac
`

// installFakeBD puts fakeBD first on PATH for the test.
func installFakeBD(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake bd is a POSIX shell script")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bd"), []byte(fakeBD), 0o755)) //nolint:gosec // test executable
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestBDExecutor_ExecBD_EnvironmentAndArgs(t *testing.T) {
	installFakeBD(t)
	t.Setenv("BD_JSON_ENVELOPE", "1")
	t.Setenv("BEADS_DIR", "/should/be/replaced")

	workDir := t.TempDir()
	beadsRel := filepath.Join("rel", ".beads")
	executor := NewBDExecutor(workDir, beadsRel)

	out, err := executor.runBeads("env", "--title=-dash value", "--notes=a\nb")
	require.NoError(t, err)

	lines := strings.Split(out, "\n")
	wantBeadsDir, err := filepath.Abs(beadsRel)
	require.NoError(t, err)
	wantCwd, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)

	require.Equal(t, "BEADS_DIR="+wantBeadsDir, lines[0], "BEADS_DIR is absolute and replaces the inherited one")
	require.Equal(t, "BD_JSON_ENVELOPE=0", lines[1], "inherited envelope setting is dropped")
	require.Equal(t, "CWD="+wantCwd, lines[2])
	require.Equal(t, []string{"ARG=env", "ARG=--title=-dash value", "ARG=--notes=a", "b"}, lines[3:])
}

func TestBDExecutor_ExecBD_StderrRefusal(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")

	_, err := executor.runBeads("update", "X-1", "--status=closed", "--json")
	require.Error(t, err)
	require.Equal(t,
		"bd update failed: Error updating X-1: cannot close X-1: 1 open child issue(s); close children first or use --force to override",
		err.Error())
}

func TestBDExecutor_ExecBD_StdoutJSONError(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")

	_, err := executor.runBeads("flagerr", "--json")
	require.Error(t, err)
	require.Equal(t, `bd flagerr failed: invalid priority "9"`, err.Error())
}

func TestBDExecutor_ExecBD_ExitCode14(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")

	_, err := executor.runBeads("frozen", "--json")
	require.Error(t, err)
	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok)
	require.Equal(t, bdExitMigrationFrozen, bdErr.ExitCode)
	require.Equal(t, "bd frozen failed: "+hintFrozen+" — workspace is frozen for migration.", err.Error())
}

func TestBDExecutor_ExecBD_Timeout(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")
	executor.timeout = 200 * time.Millisecond

	start := time.Now()
	_, err := executor.runBeads("hang")
	require.Error(t, err)
	require.Less(t, time.Since(start), 5*time.Second, "bd is killed at the timeout")
	require.Equal(t, "bd hang timed out after 200ms: "+hintTimeout, err.Error())
}

// TestBDExecutor_ExecBD_UpdateStatusClose runs the close refusal through the
// public method, as mark_task_complete does.
func TestBDExecutor_ExecBD_UpdateStatusClose(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")

	err := executor.UpdateStatus("X-1", domain.StatusClosed)
	require.ErrorContains(t, err, "cannot close X-1: 1 open child issue(s)")
}
