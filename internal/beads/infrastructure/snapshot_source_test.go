package infrastructure

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// sourceRunner returns a fake BDRunner that records every request and
// answers it with respond. A failed answer is translated by newBDError, as
// NewBDRunner's runner does, so errors carry bd_errors.go's hints. Every
// request must be a --readonly read and never `bd types`.
func sourceRunner(t *testing.T, respond func(req BDRequest) bdOutput) (BDRunner, *[]BDRequest) {
	t.Helper()
	var requests []BDRequest
	run := func(_ context.Context, req BDRequest) (string, error) {
		require.NotEmpty(t, req.Args)
		require.Equal(t, "--readonly", req.Args[0], "every snapshot read is --readonly: %q", req.Args)
		require.NotContains(t, req.Args, "types", "perles never runs bd types")
		requests = append(requests, req)
		out := respond(req)
		if out.failed() {
			return "", newBDError(req.Args, out, req.Timeout)
		}
		return out.Stdout, nil
	}
	return run, &requests
}

// bdSucceeded is a successful bd run that printed stdout.
func bdSucceeded(stdout string) func(BDRequest) bdOutput {
	return func(BDRequest) bdOutput { return bdOutput{Stdout: stdout} }
}

// captureLog sends perles' log to a temp file for the test and returns a
// function that reads it.
func captureLog(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "perles.log")
	closeLog, err := log.InitWithTeaLog(path, "snapshot-source-test")
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

// exportLines is a small `bd export --all` output: two issues and a memory.
const exportLines = `{"_type":"issue","id":"a-1","title":"one","status":"open","created_by":""}
{"_type":"memory","key":"k","value":"v"}
{"_type":"issue","id":"a-2","title":"two","status":"closed","created_by":"alice"}
`

func TestReadSnapshotExport_Request(t *testing.T) {
	run, requests := sourceRunner(t, bdSucceeded(exportLines))

	out, err := ReadSnapshotExport(context.Background(), run)
	require.NoError(t, err)
	require.Equal(t, exportLines, out, "stdout is passed through as bd wrote it")

	require.Len(t, *requests, 1)
	req := (*requests)[0]
	require.Equal(t, []string{"--readonly", "export", "--all"}, req.Args)
	require.Equal(t, []string{
		"BD_EXPORT_EXCLUDE_OWNERS=__perles_none__",
		"BD_EXPORT_EXCLUDE_OWNER=__perles_none__",
	}, req.Env)
	require.Equal(t, 30*time.Second, req.Timeout, "snapshot reads are bounded at about 30s")
	require.Empty(t, req.Stdin)

	// The output is what DecodeExport reads.
	issues, err := DecodeExport(strings.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, []string{"a-1", "a-2"}, issueIDs(issues))
}

func TestReadSnapshotExport_PassesContext(t *testing.T) {
	type ctxKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "caller"))
	cancel()

	var got context.Context
	run := func(ctx context.Context, _ BDRequest) (string, error) {
		got = ctx
		return "", ctx.Err()
	}
	_, err := ReadSnapshotExport(ctx, run)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, "caller", got.Value(ctxKey{}), "the caller's context reaches bd, so cancelling it kills the export")
}

// fakeExportBD prints one export record whose fields carry the owner
// environment and the arguments bd received.
const fakeExportBD = `#!/bin/sh
printf '{"_type":"issue","id":"env-1","title":"%s","description":"%s","notes":"%s"}\n' \
  "${BD_EXPORT_EXCLUDE_OWNERS-unset}" "${BD_EXPORT_EXCLUDE_OWNER-unset}" "$*"
`

// TestReadSnapshotExport_SentinelOverridesInheritedOwners runs the export
// through NewBDRunner and a fake bd: the sentinel replaces inherited owner
// settings (including the blank singular value that would drop every issue
// with an empty created_by), and the workspace's config.yaml is not touched.
func TestReadSnapshotExport_SentinelOverridesInheritedOwners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake bd is a POSIX shell script")
	}
	isolateBeadsEnv(t)
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "bd"), []byte(fakeExportBD), 0o755)) //nolint:gosec // test executable
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	beadsDir := newBeadsDir(t)
	t.Setenv("BEADS_DIR", beadsDir)
	t.Setenv("BD_EXPORT_EXCLUDE_OWNERS", "alice,bob")
	t.Setenv("BD_EXPORT_EXCLUDE_OWNER", " ")
	const config = "export.exclude_owners: alice\n"
	configPath := writeFile(t, beadsDir, "config.yaml", config)
	before, err := os.Stat(configPath)
	require.NoError(t, err)

	out, err := ReadSnapshotExport(context.Background(), NewBDRunner(filepath.Dir(beadsDir), beadsDir))
	require.NoError(t, err)

	issues, err := DecodeExport(strings.NewReader(out))
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "__perles_none__", issues[0].Title, "BD_EXPORT_EXCLUDE_OWNERS")
	require.Equal(t, "__perles_none__", issues[0].Description, "BD_EXPORT_EXCLUDE_OWNER")
	require.Equal(t, "--readonly export --all", issues[0].Notes, "bd's arguments")

	data, err := os.ReadFile(configPath) //nolint:gosec // G304: test temp file
	require.NoError(t, err)
	require.Equal(t, config, string(data), "the user's config.yaml is untouched")
	after, err := os.Stat(configPath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime())
}

func TestReadSnapshotExport_FailureIsTranslated(t *testing.T) {
	migrateConsent := `{"error":"database schema is at v65; this bd requires v74 (9 pending migration(s)), and bd no longer migrates a database without explicit consent","migrate_consent":{"pending":9},"schema_version":1}`

	tests := []struct {
		name      string
		out       bdOutput
		wantHint  string
		wantText  string
		wantErrIs error
	}{
		{
			name:     "pending migration refusal",
			out:      bdFailed(1, "", migrateConsent+"\n"),
			wantHint: hintMigrate,
			wantText: "9 pending migration(s)",
		},
		{
			name:     "timeout names the snapshot bound",
			out:      bdOutput{ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true},
			wantHint: hintTimeout,
			wantText: "timed out after 30s",
		},
		{
			name:      "bd not on PATH",
			out:       bdOutput{ExitCode: -1, Err: &exec.Error{Name: "bd", Err: exec.ErrNotFound}},
			wantText:  "bd CLI not found on PATH",
			wantErrIs: exec.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, requests := sourceRunner(t, func(BDRequest) bdOutput { return tt.out })

			out, err := ReadSnapshotExport(context.Background(), run)
			require.Empty(t, out)
			bdErr, ok := errors.AsType[*bdError](err)
			require.True(t, ok, "want the translated *bdError, got %v", err)
			require.Equal(t, tt.wantHint, bdErr.Hint)
			require.ErrorContains(t, err, tt.wantText)
			if tt.wantHint != "" {
				require.ErrorContains(t, err, tt.wantHint)
			}
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}
			require.Len(t, *requests, 1)
		})
	}
}

// issuesWithStatuses returns one export issue per status.
func issuesWithStatuses(statuses ...string) []ExportIssue {
	issues := make([]ExportIssue, len(statuses))
	for i, status := range statuses {
		issues[i] = ExportIssue{ID: fmt.Sprintf("s-%d", i+1), Status: status}
	}
	return issues
}

// bdStatusesOutput is `bd statuses --json` as bd 1.3.2 prints it, with the
// given custom_statuses member ("" omits it, as bd does when there are none).
func bdStatusesOutput(custom string) string {
	out := `{
  "built_in_statuses": [
    {"category": "active", "description": "Available to work (default)", "icon": "○", "name": "open"},
    {"category": "wip", "description": "Actively being worked on", "icon": "◐", "name": "in_progress"},
    {"category": "wip", "description": "Blocked by a dependency", "icon": "●", "name": "blocked"},
    {"category": "frozen", "description": "Deliberately put on ice for later", "icon": "❄", "name": "deferred"},
    {"category": "done", "description": "Completed", "icon": "✓", "name": "closed"},
    {"category": "frozen", "description": "Persistent, stays open indefinitely", "icon": "📌", "name": "pinned"},
    {"category": "wip", "description": "Attached to an agent's hook", "icon": "◇", "name": "hooked"}
  ],`
	if custom != "" {
		out += "\n  \"custom_statuses\": " + custom + ","
	}
	return out + "\n  \"schema_version\": 1\n}\n"
}

func TestReadCustomStatuses_NotCalledForBuiltinStatuses(t *testing.T) {
	for name, issues := range map[string][]ExportIssue{
		"no issues":            nil,
		"every built-in":       issuesWithStatuses("open", "in_progress", "blocked", "deferred", "closed", "pinned", "hooked"),
		"repeated built-ins":   issuesWithStatuses("open", "open", "closed"),
		"default status, open": {{ID: "d-1", Status: "open"}},
	} {
		t.Run(name, func(t *testing.T) {
			run, requests := sourceRunner(t, func(BDRequest) bdOutput {
				t.Fatal("bd statuses must not run when only built-in statuses appear")
				return bdOutput{}
			})
			statuses, err := ReadCustomStatuses(context.Background(), run, issues)
			require.NoError(t, err)
			require.Nil(t, statuses)
			require.Empty(t, *requests)
		})
	}
}

func TestReadCustomStatuses_CalledOnceForCustomStatus(t *testing.T) {
	custom := `[
    {"name": "in_review", "category": "active"},
    {"name": "qa", "category": "wip"},
    {"name": "legacy", "category": ""},
    {"name": "", "category": "done"}
  ]`
	run, requests := sourceRunner(t, bdSucceeded(bdStatusesOutput(custom)))

	issues := issuesWithStatuses("open", "in_review", "in_review", "qa", "closed")
	statuses, err := ReadCustomStatuses(context.Background(), run, issues)
	require.NoError(t, err)
	require.Equal(t, []CustomStatus{
		{Name: "in_review", Category: "active"},
		{Name: "qa", Category: "wip"},
		{Name: "legacy", Category: ""},
	}, statuses, "custom_statuses in bd's order; a nameless entry is dropped")

	require.Len(t, *requests, 1, "statuses runs once per build")
	req := (*requests)[0]
	require.Equal(t, []string{"--readonly", "statuses", "--json"}, req.Args)
	require.Equal(t, 30*time.Second, req.Timeout)
	require.Empty(t, req.Env)
	require.Empty(t, req.Stdin)
}

func TestReadCustomStatuses_EnvelopedOutput(t *testing.T) {
	run, _ := sourceRunner(t, bdSucceeded(`{"schema_version":1,"data":{"built_in_statuses":[{"name":"open","category":"active"}],"custom_statuses":[{"name":"in_review","category":"active"}]}}`))

	statuses, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("in_review"))
	require.NoError(t, err)
	require.Equal(t, []CustomStatus{{Name: "in_review", Category: "active"}}, statuses)
}

func TestReadCustomStatuses_EmptyResultWarns(t *testing.T) {
	readLog := captureLog(t)
	run, requests := sourceRunner(t, bdSucceeded(bdStatusesOutput("")))

	statuses, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("qa", "open", "in_review", "qa"))
	require.NoError(t, err)
	require.Empty(t, statuses)
	require.Len(t, *requests, 1)

	logged := readLog()
	require.Contains(t, logged, "[WARN] [beads] bd statuses reported no custom statuses")
	require.Contains(t, logged, "statuses=in_review,qa", "the warning names the unknown statuses, sorted and deduplicated")
}

// TestReadCustomStatuses_UsesBDBuiltins: a status perles does not know as a
// built-in but bd lists in built_in_statuses (a newer bd) needs no custom
// status, so the empty result is not warned about.
func TestReadCustomStatuses_UsesBDBuiltins(t *testing.T) {
	readLog := captureLog(t)
	run, requests := sourceRunner(t, bdSucceeded(`{"built_in_statuses":[{"name":"open","category":"active"},{"name":"review","category":"wip"}],"schema_version":1}`))

	statuses, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("open", "review"))
	require.NoError(t, err)
	require.Empty(t, statuses)
	require.Len(t, *requests, 1, "perles' built-in list decides whether to ask bd")
	require.NotContains(t, readLog(), "[WARN]")
}

func TestReadCustomStatuses_Failures(t *testing.T) {
	t.Run("bd failure is the translated *bdError", func(t *testing.T) {
		run, _ := sourceRunner(t, func(BDRequest) bdOutput {
			return bdFailed(1, "", "Error: database schema is at v45 but this binary expects v46; run 'bd migrate schema'\n")
		})
		statuses, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("in_review"))
		require.Nil(t, statuses)
		bdErr, ok := errors.AsType[*bdError](err)
		require.True(t, ok, "got %v", err)
		require.Equal(t, hintMigrate, bdErr.Hint)
	})

	t.Run("malformed output", func(t *testing.T) {
		run, _ := sourceRunner(t, bdSucceeded(`{"custom_statuses": "in_review"}`))
		statuses, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("in_review"))
		require.Nil(t, statuses)
		require.ErrorContains(t, err, "parsing bd statuses output")
	})

	t.Run("no JSON", func(t *testing.T) {
		run, _ := sourceRunner(t, bdSucceeded("Built-in statuses:\n"))
		_, err := ReadCustomStatuses(context.Background(), run, issuesWithStatuses("in_review"))
		require.ErrorContains(t, err, "parsing bd statuses output")
	})
}

func TestMinBeadsVersions(t *testing.T) {
	require.Equal(t, "1.0.0", domain.MinBeadsVersion, "server mode keeps 1.0.0")
	require.Equal(t, "1.0.5", domain.MinEmbeddedBeadsVersion)
}

// requireEmbeddedVersionRefusal asserts that err is the embedded gate's
// refusal for beadsDir with the given Detail.
func requireEmbeddedVersionRefusal(t *testing.T, err error, beadsDir, detail string) {
	t.Helper()
	unsupported, ok := errors.AsType[*UnsupportedWorkspaceError](err)
	require.True(t, ok, "want *UnsupportedWorkspaceError, got %v", err)
	require.Equal(t, WorkspaceEmbeddedDolt, unsupported.Kind)
	require.Equal(t, beadsDir, unsupported.BeadsDir)
	require.Equal(t, detail, unsupported.Detail)
	require.NotContains(t, unsupported.Detail, "(found )")
}

func TestCheckEmbeddedBDVersion(t *testing.T) {
	const beadsDir = "/work/project/.beads"
	errUnsafe := errors.New("bd context: exit status 1: Error: refusing to run in unsafe location /tmp/project")
	errNotFound := fmt.Errorf("bd context: %w", &exec.Error{Name: "bd", Err: exec.ErrNotFound})
	const needs = "embedded workspaces need bd >= 1.0.5 "

	tests := []struct {
		name       string
		version    string
		contextErr error
		wantDetail string // "" means the gate passes
	}{
		{name: "1.0.4 is too old", version: "1.0.4", wantDetail: needs + "(found 1.0.4)"},
		{name: "bd before 1.0.0 with bd context's rejection", version: "0.62.0", contextErr: &bdContextTooOldError{Version: "0.62.0"}, wantDetail: needs + "(found 0.62.0)"},
		{name: "annotated old version shows its base", version: "1.0.4 (abc123: main@abc123)", wantDetail: needs + "(found 1.0.4)"},
		{name: "pre-release of 1.0.4", version: "1.0.4-rc.1", wantDetail: needs + "(found 1.0.4)"},
		{name: "1.0.5", version: "1.0.5"},
		{name: "main build of 1.3.2", version: "1.3.2 (6ebaacba8: main@6ebaacba8)"},
		{name: "1.0.5 whatever bd context said", version: "1.0.5", contextErr: errUnsafe},
		{name: "bd missing defers to the export", version: "", contextErr: errNotFound},
		{name: "unsafe location", version: "", contextErr: errUnsafe,
			wantDetail: needs + "(could not read the bd version: bd context: exit status 1: Error: refusing to run in unsafe location /tmp/project)"},
		{name: "multi-line reason is one line", version: "", contextErr: errors.New("bd context: exit status 1:\n  Error: boom\n"),
			wantDetail: needs + "(could not read the bd version: bd context: exit status 1: Error: boom)"},
		{name: "nil error", version: "", wantDetail: needs + "(could not read the bd version: bd context reported no bd_version)"},
		{name: "empty bd_version rejected by bd context", version: "", contextErr: &bdContextTooOldError{Version: ""},
			wantDetail: needs + "(could not read the bd version: bd context reported no bd_version)"},
		{name: "blank version", version: "  ", wantDetail: needs + "(could not read the bd version: bd context reported no bd_version)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckEmbeddedBDVersion(beadsDir, tt.version, tt.contextErr)
			if tt.wantDetail == "" {
				require.NoError(t, err)
				return
			}
			requireEmbeddedVersionRefusal(t, err, beadsDir, tt.wantDetail)
		})
	}
}

// fakeHangingBD is a bd that never answers.
const fakeHangingBD = "#!/bin/sh\nexec sleep 10\n"

// TestCheckEmbeddedBDVersion_FromBDContext feeds the gate the BDVersion and
// BDContextErr that embedded classification records for each way bd context
// can answer or fail.
func TestCheckEmbeddedBDVersion_FromBDContext(t *testing.T) {
	const needs = "embedded workspaces need bd >= 1.0.5 "

	// timeoutRunner runs a bd that never answers and kills it at a short
	// deadline, as queryBDContext's timeout does.
	hangingBD := filepath.Join(t.TempDir(), "bd")
	require.NoError(t, os.WriteFile(hangingBD, []byte(fakeHangingBD), 0o755)) //nolint:gosec // test executable
	timeoutRunner := func(ctx context.Context, beadsDir, workDir string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		return runBDContextCommand(ctx, hangingBD, beadsDir, workDir)
	}

	tests := []struct {
		name      string
		runner    BDContextRunner
		posixOnly bool
		// wantDetail is the refusal's Detail; "" means the gate passes.
		// "<err>" stands for the BDContextErr text.
		wantDetail string
	}{
		{name: "bd 1.3.2", runner: bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"1.3.2 (6ebaacba8: main@6ebaacba8)"}`)},
		{name: "bd 1.0.4", runner: bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"1.0.4"}`),
			wantDetail: needs + "(found 1.0.4)"},
		{name: "bd older than 1.0.0", runner: bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":"0.62.0"}`),
			wantDetail: needs + "(found 0.62.0)"},
		{name: "empty bd_version", runner: bdContextJSON(`{"backend":"dolt","dolt_mode":"embedded","database":"ctx_db","bd_version":""}`),
			wantDetail: needs + "(could not read the bd version: bd context reported no bd_version)"},
		{name: "bd missing", runner: noBDContext},
		{name: "nil runner (bd context disabled in tests)", runner: nil,
			wantDetail: needs + "(could not read the bd version: bd context is not run in tests)"},
		{name: "malformed JSON", runner: bdContextJSON(`{"backend":"dolt","bd_version":`),
			wantDetail: needs + "(could not read the bd version: <err>)"},
		{name: "error object", runner: bdContextJSON(`{"error":"cannot resolve repo context","schema_version":1}`),
			wantDetail: needs + "(could not read the bd version: bd context: cannot resolve repo context)"},
		{name: "timeout", runner: timeoutRunner, posixOnly: true,
			wantDetail: needs + "(could not read the bd version: <err>)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posixOnly && runtime.GOOS == "windows" {
				t.Skip("fake bd is a POSIX shell script")
			}
			isolateBeadsEnv(t)
			beadsDir := newBeadsDir(t)
			writeMetadata(t, beadsDir, embeddedMetadata)

			ew := resolveEmbedded(t, beadsDir, tt.runner)
			err := CheckEmbeddedBDVersion(ew.BeadsDir, ew.BDVersion, ew.BDContextErr)
			if tt.wantDetail == "" {
				require.NoError(t, err)
				return
			}
			want := tt.wantDetail
			if strings.Contains(want, "<err>") {
				require.Error(t, ew.BDContextErr)
				want = strings.ReplaceAll(want, "<err>", ew.BDContextErr.Error())
			}
			requireEmbeddedVersionRefusal(t, err, beadsDir, want)
		})
	}
}
