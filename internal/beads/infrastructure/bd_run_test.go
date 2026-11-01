package infrastructure

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestMergeBDEnv(t *testing.T) {
	env := bdEnviron([]string{
		"PATH=/usr/bin",
		"BD_EXPORT_EXCLUDE_OWNERS=alice",
		"HOME=/home/me",
	}, "/abs/.beads")

	merged := mergeBDEnv(env, []string{
		"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__", // replaces the inherited value
		"BD_EXPORT_EXCLUDE_OWNER=first",
		"BD_JSON_ENVELOPE=1",                      // pinned: ignored
		"beads_dir=/caller/.beads",                // pinned (keys compare case-insensitively): ignored
		"BD_EXPORT_EXCLUDE_OWNER=__perles_none__", // a later entry replaces an earlier one
	}, "/abs/.beads")
	require.Equal(t, []string{
		"PATH=/usr/bin",
		"HOME=/home/me",
		"BD_JSON_ENVELOPE=0",
		"BEADS_DIR=/abs/.beads",
		"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__",
		"BD_EXPORT_EXCLUDE_OWNER=__perles_none__",
	}, merged)

	// Without a beads dir BEADS_DIR is not pinned, so a caller's value wins.
	merged = mergeBDEnv(bdEnviron([]string{"BEADS_DIR=/inherited/.beads"}, ""), []string{"BEADS_DIR=/caller/.beads"}, "")
	require.Equal(t, []string{"BD_JSON_ENVELOPE=0", "BEADS_DIR=/caller/.beads"}, merged)

	// No extra entries: bdEnviron's output is returned as is.
	require.Equal(t, env, mergeBDEnv(env, nil, "/abs/.beads"))
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
  stdin)
    cat
    ;;
  getenv)
    shift
    for name in "$@"; do printf '%s=%s\n' "$name" "$(printenv "$name" || echo unset)"; done
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

func TestBDRunner_Stdin(t *testing.T) {
	installFakeBD(t)
	run := NewBDRunner(t.TempDir(), "")

	out, err := run(context.Background(), BDRequest{Args: []string{"stdin"}, Stdin: "X-1\nX-2\n"})
	require.NoError(t, err)
	require.Equal(t, "X-1\nX-2\n", out, "stdin reaches bd and stdout is returned untrimmed")

	out, err = run(context.Background(), BDRequest{Args: []string{"stdin"}})
	require.NoError(t, err)
	require.Empty(t, out, "no stdin by default")
}

func TestBDRunner_ExtraEnv(t *testing.T) {
	installFakeBD(t)
	t.Setenv("BD_EXPORT_EXCLUDE_OWNERS", "alice")
	t.Setenv("BD_JSON_ENVELOPE", "1")
	t.Setenv("BEADS_DIR", "/should/be/replaced")

	beadsDir := filepath.Join(t.TempDir(), ".beads")
	run := NewBDRunner(t.TempDir(), beadsDir)

	out, err := run(context.Background(), BDRequest{
		Args: []string{"getenv", "BD_EXPORT_EXCLUDE_OWNERS", "BD_EXPORT_EXCLUDE_OWNER", "BD_JSON_ENVELOPE", "BEADS_DIR"},
		Env: []string{
			"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__",
			"BD_EXPORT_EXCLUDE_OWNER=__perles_none__",
			"BD_JSON_ENVELOPE=1",
			"BEADS_DIR=/caller/.beads",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{
		"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__", // overrides the inherited value
		"BD_EXPORT_EXCLUDE_OWNER=__perles_none__",  // added
		"BD_JSON_ENVELOPE=0",                       // still pinned
		"BEADS_DIR=" + beadsDir,                    // still pinned
	}, strings.Split(strings.TrimSpace(out), "\n"))

	// Without extra entries the environment is bdEnviron's alone.
	out, err = run(context.Background(), BDRequest{Args: []string{"getenv", "BD_EXPORT_EXCLUDE_OWNER", "BD_EXPORT_EXCLUDE_OWNERS"}})
	require.NoError(t, err)
	require.Equal(t, "BD_EXPORT_EXCLUDE_OWNER=unset\nBD_EXPORT_EXCLUDE_OWNERS=alice", strings.TrimSpace(out))
}

// TestBDRunner_ExtraEnvNoDuplicateKeys checks, end to end through the real
// runner, the environment bd receives: caller entries override inherited
// values and the pinned keys stay pinned. bd is a script that execs env(1),
// which prints its environment. os/exec already drops duplicate keys from
// cmd.Env before exec, so the exact no-duplicates guarantee is pinned by
// TestMergeBDEnv; this test checks the values that reach bd.
//
// env(1) is exec'd under its own name rather than symlinked as bd: multi-call
// builds such as uutils coreutils (Ubuntu's default env) refuse to run under
// another name.
func TestBDRunner_ExtraEnvNoDuplicateKeys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env(1) stand-in for bd is POSIX only")
	}
	envPath, err := exec.LookPath("env")
	require.NoError(t, err)
	dir := t.TempDir()
	script := "#!/bin/sh\nexec '" + envPath + "'\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bd"), []byte(script), 0o755)) //nolint:gosec // test-only executable stand-in
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BD_EXPORT_EXCLUDE_OWNERS", "alice")
	t.Setenv("BD_JSON_ENVELOPE", "1")

	beadsDir := filepath.Join(t.TempDir(), ".beads")
	out, err := NewBDRunner(t.TempDir(), beadsDir)(context.Background(), BDRequest{
		Env: []string{"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__", "BD_JSON_ENVELOPE=1", "BEADS_DIR=/caller/.beads"},
	})
	require.NoError(t, err)

	values := map[string][]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[key] = append(values[key], value)
		}
	}
	require.Equal(t, []string{"__perles_none__"}, values["BD_EXPORT_EXCLUDE_OWNERS"])
	require.Equal(t, []string{"0"}, values["BD_JSON_ENVELOPE"])
	require.Equal(t, []string{beadsDir}, values["BEADS_DIR"])
}

func TestBDRunner_TimeoutOverride(t *testing.T) {
	installFakeBD(t)
	executor := NewBDExecutor(t.TempDir(), "")
	executor.timeout = time.Hour // the per-call timeout takes precedence

	start := time.Now()
	_, err := executor.run(context.Background(), BDRequest{Args: []string{"hang"}, Timeout: 200 * time.Millisecond})
	require.Less(t, time.Since(start), 5*time.Second, "bd is killed at the per-call timeout")

	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok, "got %T: %v", err, err)
	require.True(t, bdErr.TimedOut)
	require.Equal(t, 200*time.Millisecond, bdErr.Timeout)
	require.Equal(t, "bd hang timed out after 200ms: "+hintTimeout, err.Error(), "the message names the per-call timeout")
}

func TestBDRunner_ParentContextCancelKillsBD(t *testing.T) {
	installFakeBD(t)
	run := NewBDRunner(t.TempDir(), "")

	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(200*time.Millisecond, cancel)
	defer timer.Stop()

	start := time.Now()
	_, err := run(ctx, BDRequest{Args: []string{"hang"}})
	require.Less(t, time.Since(start), 5*time.Second, "bd is killed when the context is cancelled")

	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok, "got %T: %v", err, err)
	require.False(t, bdErr.TimedOut, "a cancelled context is not a bd timeout")
	require.ErrorIs(t, err, context.Canceled)
	require.NotContains(t, err.Error(), "timed out")
}

// TestBDRunner_ParentDeadlineIsNotBDTimeout: the caller's own deadline ending
// first is reported as the caller's, not as bd timing out after its limit.
func TestBDRunner_ParentDeadlineIsNotBDTimeout(t *testing.T) {
	installFakeBD(t)
	run := NewBDRunner(t.TempDir(), "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := run(ctx, BDRequest{Args: []string{"hang"}})
	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok, "got %T: %v", err, err)
	require.False(t, bdErr.TimedOut)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotContains(t, err.Error(), "timed out after")
}

func TestBDRunner_CancelledBeforeStart(t *testing.T) {
	installFakeBD(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewBDRunner(t.TempDir(), "")(ctx, BDRequest{Args: []string{"stdin"}})
	require.ErrorIs(t, err, context.Canceled)
}

// TestBDRunner_TranslatesFailures: failures come back as the hinted *bdError,
// as for runBeads.
func TestBDRunner_TranslatesFailures(t *testing.T) {
	installFakeBD(t)
	run := NewBDRunner(t.TempDir(), "")

	_, err := run(context.Background(), BDRequest{Args: []string{"frozen", "--json"}})
	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok, "got %T: %v", err, err)
	require.Equal(t, bdExitMigrationFrozen, bdErr.ExitCode)
	require.Equal(t, defaultBDTimeout, bdErr.Timeout, "no override: the default timeout")
	require.Equal(t, "bd frozen failed: "+hintFrozen+" — workspace is frozen for migration.", err.Error())
}

func TestBDRunner_NotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := NewBDRunner(t.TempDir(), "")(context.Background(), BDRequest{Args: []string{"export", "--all"}})
	require.ErrorIs(t, err, exec.ErrNotFound)
	require.Equal(t, "bd export failed: bd CLI not found on PATH", err.Error())
}
