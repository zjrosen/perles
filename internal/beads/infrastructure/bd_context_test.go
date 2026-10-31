package infrastructure

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// sampleBDContext is `bd context --json` from bd 1.3.2 on a server workspace.
const sampleBDContext = `{
  "beads_dir": "/work/perles/.beads",
  "repo_root": "/work/perles",
  "is_redirected": false,
  "is_worktree": false,
  "backend": "dolt",
  "dolt_mode": "server",
  "server_host": "127.0.0.1",
  "server_port": 50362,
  "database": "perles",
  "project_id": "1af1d931",
  "bd_version": "1.3.2",
  "schema_version": 1
}`

func TestParseBDContext(t *testing.T) {
	want := &BDContext{
		BeadsDir: "/work/perles/.beads", Backend: "dolt", DoltMode: "server",
		ServerHost: "127.0.0.1", ServerPort: 50362, Database: "perles", BdVersion: "1.3.2",
	}

	t.Run("bare", func(t *testing.T) {
		got, err := parseBDContext([]byte(sampleBDContext))
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("envelope", func(t *testing.T) {
		got, err := parseBDContext([]byte(`{"schema_version": 1, "data": ` + sampleBDContext + `}`))
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("proxied", func(t *testing.T) {
		got, err := parseBDContext([]byte(`{"backend":"dolt","dolt_mode":"proxied-server","proxied_dir":"/x/.beads/dolt","database":"beads","bd_version":"1.3.0"}`))
		require.NoError(t, err)
		require.Equal(t, "/x/.beads/dolt", got.ProxiedDir)
	})

	errorCases := []struct {
		name, out, wantErr string
	}{
		{"error object", `{"error": "cannot resolve repo context: no .beads", "schema_version": 1}`, "cannot resolve repo context"},
		{"enveloped error", `{"schema_version": 1, "data": {"error": "boom"}}`, "boom"},
		{"empty", "  \n", "printed nothing"},
		{"not json", "Error: unknown command \"context\"", "parsing bd context output"},
		{"json array", `[1, 2]`, "parsing bd context output"},
	}
	for _, tt := range errorCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseBDContext([]byte(tt.out))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestQueryBDContext(t *testing.T) {
	t.Run("current bd", func(t *testing.T) {
		got, err := queryBDContext(bdContextJSON(sampleBDContext), "/b", "/w")
		require.NoError(t, err)
		require.Equal(t, 50362, got.ServerPort)
	})

	t.Run("release candidate of 1.x", func(t *testing.T) {
		_, err := queryBDContext(bdContextJSON(`{"backend":"dolt","dolt_mode":"server","bd_version":"v1.2.0-rc.1"}`), "/b", "")
		require.NoError(t, err)
	})

	for _, version := range []string{"0.62.0", "0.60.0", ""} {
		t.Run("old bd "+version, func(t *testing.T) {
			_, err := queryBDContext(bdContextJSON(`{"backend":"dolt","dolt_mode":"server","server_port":3307,"bd_version":"`+version+`"}`), "/b", "")
			require.ErrorContains(t, err, "older than 1.0.0")
		})
	}

	t.Run("runner error", func(t *testing.T) {
		_, err := queryBDContext(noBDContext, "/b", "")
		require.ErrorContains(t, err, "not found")
	})

	t.Run("passes beads dir, work dir and a deadline", func(t *testing.T) {
		var gotBeads, gotWork string
		var hasDeadline bool
		runner := func(ctx context.Context, beadsDir, workDir string) ([]byte, error) {
			gotBeads, gotWork = beadsDir, workDir
			_, hasDeadline = ctx.Deadline()
			return []byte(sampleBDContext), nil
		}
		_, err := queryBDContext(runner, "/project/.beads", "/project")
		require.NoError(t, err)
		require.Equal(t, "/project/.beads", gotBeads)
		require.Equal(t, "/project", gotWork)
		require.True(t, hasDeadline)
	})
}

func TestDefaultBDContextRunner_DisabledInTests(t *testing.T) {
	_, err := defaultBDContextRunner(context.Background(), t.TempDir(), "")
	require.ErrorIs(t, err, errBDContextDisabled)
}

func TestBDContextEnv(t *testing.T) {
	env := bdContextEnv([]string{
		"PATH=/bin", "BD_JSON_ENVELOPE=1", "BEADS_DIR=/elsewhere", "BEADS_DOLT_SERVER_MODE=1", "BD_JSON_ENVELOPE_X=keep",
	}, "/abs/.beads")

	require.Equal(t, []string{
		"PATH=/bin", "BEADS_DOLT_SERVER_MODE=1", "BD_JSON_ENVELOPE_X=keep", "BEADS_DIR=/abs/.beads",
	}, env)
}

// TestRunBDContextCommand runs a stand-in script (not bd) to check the
// arguments, environment and working directory the runner passes.
func TestRunBDContextCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	t.Setenv("BD_JSON_ENVELOPE", "1")

	binDir := t.TempDir()
	script := writeFile(t, binDir, "fake-bd", `#!/bin/sh
printf '{"args":"%s","beads_dir":"%s","envelope":"%s","pwd":"%s","bd_version":"1.3.2"}' "$*" "$BEADS_DIR" "${BD_JSON_ENVELOPE:-unset}" "$(pwd -P)"
`)
	require.NoError(t, os.Chmod(script, 0o700)) //nolint:gosec // test script must be executable

	workDir := t.TempDir()
	relBeads, err := filepath.Rel(mustGetwd(t), filepath.Join(workDir, ".beads"))
	require.NoError(t, err)

	out, err := runBDContextCommand(context.Background(), script, relBeads, workDir)
	require.NoError(t, err)

	got := string(out)
	require.Contains(t, got, `"args":"context --json"`)
	require.Contains(t, got, `"envelope":"unset"`)
	absBeads, err := filepath.Abs(relBeads)
	require.NoError(t, err)
	require.Contains(t, got, `"beads_dir":"`+absBeads+`"`)
	realWork, err := filepath.EvalSymlinks(workDir)
	require.NoError(t, err)
	require.Contains(t, got, `"pwd":"`+realWork+`"`)
}

func TestRunBDContextCommand_FailureIncludesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script")
	}
	script := writeFile(t, t.TempDir(), "fake-bd", "#!/bin/sh\necho 'something broke' >&2\nexit 3\n")
	require.NoError(t, os.Chmod(script, 0o700)) //nolint:gosec // test script must be executable

	_, err := runBDContextCommand(context.Background(), script, t.TempDir(), "")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "something broke"), err.Error())

	_, err = runBDContextCommand(context.Background(), filepath.Join(t.TempDir(), "missing-bd"), t.TempDir(), "")
	require.Error(t, err)
	require.False(t, errors.Is(err, errBDContextDisabled))
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return wd
}
