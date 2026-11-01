package adapter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/task"
)

func TestTranslateClientError_UnsupportedWorkspace(t *testing.T) {
	kinds := map[infrabeads.WorkspaceKind]task.UnsupportedWorkspaceKind{
		infrabeads.WorkspaceProxiedServer:  task.WorkspaceProxiedServer,
		infrabeads.WorkspaceHTTPBackend:    task.WorkspaceHTTPBackend,
		infrabeads.WorkspaceGraph:          task.WorkspaceGraph,
		infrabeads.WorkspaceLegacySQLite:   task.WorkspaceLegacySQLite,
		infrabeads.WorkspaceUnknownBackend: task.WorkspaceUnknownBackend,
	}
	for infraKind, want := range kinds {
		t.Run(string(infraKind), func(t *testing.T) {
			err := translateClientError(&infrabeads.UnsupportedWorkspaceError{Kind: infraKind, BeadsDir: "/p/.beads", Detail: "d"})

			uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
			require.True(t, ok, "got %v", err)
			require.Equal(t, &task.UnsupportedWorkspaceError{Kind: want, BeadsDir: "/p/.beads", Detail: "d"}, uw,
				"the Detail is copied unchanged")
		})
	}
}

// TestTranslateClientError_EmbeddedVersionGate: the infrastructure Detail of
// an embedded refusal carries no cause, so the mapping adds one. The bd
// version gate's refusal arrives as bd-version, not as a bd error.
func TestTranslateClientError_EmbeddedVersionGate(t *testing.T) {
	gate := infrabeads.CheckEmbeddedBDVersion("/p/.beads", "1.0.4", nil)
	require.Error(t, gate)

	err := translateClientError(gate)

	uw := requireEmbeddedUnsupported(t, err, "/p/.beads")
	require.Equal(t, "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4)", uw.Detail)
	cause, text := task.ParseEmbeddedDetail(uw.Detail)
	require.Equal(t, task.EmbeddedCauseBDVersion, cause)
	require.Equal(t, "embedded workspaces need bd >= 1.0.5 (found 1.0.4)", text)
	require.ErrorIs(t, err, gate, "the infrastructure error stays reachable")
}

func TestTranslateClientError_ServerDown(t *testing.T) {
	cause := errors.New("connection refused")
	err := translateClientError(&infrabeads.ServerUnreachableError{
		Conn: infrabeads.Connection{Host: "[::1]", Port: 3307},
		Err:  cause,
	})

	sd, ok := errors.AsType[*task.ServerDownError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, "::1", sd.Host)
	require.Equal(t, 3307, sd.Port)
	require.Equal(t, "[::1]:3307", sd.Address())
	require.False(t, sd.ManagedByBD)
	require.ErrorIs(t, err, cause)

	err = translateClientError(&infrabeads.ServerUnreachableError{
		Conn: infrabeads.Connection{Host: "127.0.0.1", Socket: "/run/dolt.sock"},
		Err:  cause,
	})
	sd, ok = errors.AsType[*task.ServerDownError](err)
	require.True(t, ok)
	require.Equal(t, "unix:/run/dolt.sock", sd.Address())

	err = translateClientError(&infrabeads.ServerUnreachableError{Conn: infrabeads.Connection{Host: "127.0.0.1"}, Err: cause})
	sd, ok = errors.AsType[*task.ServerDownError](err)
	require.True(t, ok)
	require.True(t, sd.ManagedByBD)
	require.Equal(t, "127.0.0.1", sd.Address())
}

func TestTranslateClientError_Other(t *testing.T) {
	cause := errors.New("parsing metadata.json: boom")
	err := translateClientError(cause)

	require.ErrorIs(t, err, cause)
	_, isServerDown := errors.AsType[*task.ServerDownError](err)
	require.False(t, isServerDown)
	_, isUnsupported := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.False(t, isUnsupported)
}

// TestEmbeddedStartupError: each failure to start the snapshot reader maps
// to its cause, with a non-empty text, and stays reachable.
func TestEmbeddedStartupError(t *testing.T) {
	export := func() error {
		_, err := infrabeads.NewBDRunner(t.TempDir(), "/p/.beads")(context.Background(), infrabeads.BDRequest{Args: []string{"--readonly", "export", "--all"}})
		return err
	}
	t.Setenv("PATH", "") // the real runner finds no bd
	notFound := export()
	require.Error(t, notFound)
	// The real runner, and a bd that refuses the export as bd does with a
	// pending schema migration.
	installFailingBD(t, "Error: database schema v45 has pending migrations; run 'bd migrate schema' to apply them")
	refused := export()
	require.Error(t, refused)

	tests := []struct {
		name       string
		err        error
		wantDetail string
	}{
		{
			name:       "version gate",
			err:        infrabeads.CheckEmbeddedBDVersion("/p/.beads", "0.62.0", nil),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (found 0.62.0)",
		},
		{
			name:       "version gate behind a wrap",
			err:        fmt.Errorf("building: %w", infrabeads.CheckEmbeddedBDVersion("/p/.beads", "1.0.0", nil)),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.0)",
		},
		{
			name:       "bd not on PATH",
			err:        notFound,
			wantDetail: "bd-not-found: bd export failed: bd CLI not found on PATH",
		},
		{
			name: "bd refused",
			err:  refused,
			wantDetail: "bd-error: bd export failed: beads database needs a schema migration: run `bd migrate schema` — " +
				"database schema v45 has pending migrations; run 'bd migrate schema' to apply them",
		},
		{
			name:       "not found outside a bd invocation is no bd error",
			err:        fmt.Errorf("opening: %w", exec.ErrNotFound),
			wantDetail: "workspace: opening: " + exec.ErrNotFound.Error(),
		},
		{
			name:       "bd's text alone is no bd error",
			err:        errors.New("bd export failed: boom"),
			wantDetail: "workspace: bd export failed: boom",
		},
		{
			name:       "an error without text",
			err:        errors.New(" "),
			wantDetail: "workspace: the embedded Dolt database could not be opened",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uw := embeddedStartupError("/p/.beads", tt.err)

			require.Equal(t, task.WorkspaceEmbeddedDolt, uw.Kind)
			require.Equal(t, "/p/.beads", uw.BeadsDir)
			require.Equal(t, tt.wantDetail, uw.Detail)
			_, text := task.ParseEmbeddedDetail(uw.Detail)
			require.NotEmpty(t, strings.TrimSpace(text))
			require.ErrorIs(t, uw, tt.err)
		})
	}
}

// TestNewBeadsBackend_EmbeddedWorkspace: an embedded workspace gets a
// SnapshotBackend, given the version bd context reported and a runner for
// the workspace. Construction runs no bd read (perles daemon starts without
// exporting); CheckCompatibility installs the first snapshot, after which
// queries make no runner calls.
func TestNewBeadsBackend_EmbeddedWorkspace(t *testing.T) {
	ws := newEmbeddedWorkspace(t, embeddedTestMetadata)
	workDir := filepath.Dir(ws.beadsDir)
	f := newRunnerFactory(generationRunner(t).run)

	backend, err := newBeadsBackend(ws.beadsDir, workDir, backendDeps{
		bdContext:   bdContextAnswer(`{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":"1.3.2"}`),
		newBDRunner: f.new,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })

	b, ok := backend.(*SnapshotBackend)
	require.True(t, ok, "got %T", backend)
	require.Equal(t, ws.beadsDir, b.beadsDir)
	require.Equal(t, ws.manifest, b.manifestPath)
	require.Equal(t, "1.3.2", b.bdVersion)
	require.NoError(t, b.bdContextErr)
	require.Equal(t, [][2]string{{workDir, ws.beadsDir}}, f.created(), "one runner, for this workspace")
	require.Empty(t, b.Services().DBPath, "the app listens to Changes() instead of watching a file")
	require.Zero(t, f.runner.count(""), "construction runs no bd")
	require.False(t, loopsStarted(b), "nor the scheduler or the watcher")
	require.Nil(t, b.current.Load())

	require.NoError(t, b.CheckCompatibility())
	require.NotNil(t, b.current.Load(), "CheckCompatibility installs the first snapshot")
	calls := f.runner.count("")
	require.Equal(t, []string{"export", "blocked"}, f.runner.commands())

	issues, err := b.Services().QueryExecutor.Execute("type = task")
	require.NoError(t, err)
	require.Len(t, issues, 3)
	require.Equal(t, calls, f.runner.count(""), "a query after CheckCompatibility runs no bd")
}

// TestNewBeadsBackend_EmbeddedContextErrorPassedThrough: what stopped perles
// from using bd context reaches the snapshot backend, whose gate needs it.
func TestNewBeadsBackend_EmbeddedContextErrorPassedThrough(t *testing.T) {
	ws := newEmbeddedWorkspace(t, embeddedTestMetadata)
	errUnsafe := errors.New("bd context: exit status 1: Error: refusing to run in unsafe location /tmp/project")

	backend, err := newBeadsBackend(ws.beadsDir, filepath.Dir(ws.beadsDir), backendDeps{
		bdContext:   func(context.Context, string, string) ([]byte, error) { return nil, errUnsafe },
		newBDRunner: newRunnerFactory(generationRunner(t).run).new,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })

	b, ok := backend.(*SnapshotBackend)
	require.True(t, ok, "got %T", backend)
	require.Empty(t, b.bdVersion)
	require.ErrorIs(t, b.bdContextErr, errUnsafe)
}

// TestNewBeadsBackend_EmbeddedCheckCompatibility: CheckCompatibility reports
// each startup failure as an embedded unsupported workspace whose Detail
// names the cause, one case per bd version case. A version the gate admits
// gets the first snapshot installed instead.
func TestNewBeadsBackend_EmbeddedCheckCompatibility(t *testing.T) {
	errUnsafe := errors.New("bd context: exit status 1: Error: refusing to run in unsafe location /tmp/project")
	contextAnswer := func(version string) infrabeads.BDContextRunner {
		return bdContextAnswer(fmt.Sprintf(`{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":%q}`, version))
	}
	missingBD := func(context.Context, string, string) ([]byte, error) {
		return nil, fmt.Errorf("bd context: %w", &exec.Error{Name: "bd", Err: exec.ErrNotFound})
	}

	tests := []struct {
		name      string
		bdContext infrabeads.BDContextRunner
		// realBD runs the real runner with an empty PATH instead of the
		// fake bd: every bd command then fails with exec.ErrNotFound.
		realBD     bool
		wantDetail string // empty: CheckCompatibility succeeds
		wantNoBD   bool   // the gate refuses before any bd call
		// wantBDVersion is the bd version the backend keeps, as bd context
		// printed it (rows that succeed).
		wantBDVersion string
	}{
		{
			name:          "bd 1.3.2",
			bdContext:     contextAnswer("1.3.2 (6ebaacba8: main@6ebaacba8)"),
			wantBDVersion: "1.3.2 (6ebaacba8: main@6ebaacba8)",
		},
		{name: "bd 1.0.5", bdContext: contextAnswer("1.0.5"), wantBDVersion: "1.0.5"},
		{
			name:       "bd 1.0.4",
			bdContext:  contextAnswer("1.0.4"),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4)",
			wantNoBD:   true,
		},
		{
			name:       "bd 1.0.0",
			bdContext:  contextAnswer("1.0.0"),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.0)",
			wantNoBD:   true,
		},
		{
			name:       "bd 0.62.0, rejected by bd context",
			bdContext:  contextAnswer("0.62.0"),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (found 0.62.0)",
			wantNoBD:   true,
		},
		{
			name:       "bd missing",
			bdContext:  missingBD,
			realBD:     true,
			wantDetail: "bd-not-found: bd export failed: bd CLI not found on PATH",
		},
		{
			name:       "bd context failed (unsafe location)",
			bdContext:  func(context.Context, string, string) ([]byte, error) { return nil, errUnsafe },
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (could not read the bd version: " + errUnsafe.Error() + ")",
			wantNoBD:   true,
		},
		{
			name:       "empty bd_version",
			bdContext:  contextAnswer(""),
			wantDetail: "bd-version: embedded workspaces need bd >= 1.0.5 (could not read the bd version: bd context reported no bd_version)",
			wantNoBD:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := newEmbeddedWorkspace(t, embeddedTestMetadata)
			f := newRunnerFactory(generationRunner(t).run)
			if tt.realBD {
				f = newRunnerFactory(nil)
			}
			backend, err := newBeadsBackend(ws.beadsDir, filepath.Dir(ws.beadsDir), backendDeps{bdContext: tt.bdContext, newBDRunner: f.new})
			require.NoError(t, err, "construction runs no bd and does not fail")
			t.Cleanup(func() { require.NoError(t, backend.Close()) })
			require.Zero(t, f.runner.count(""))

			err = backend.CheckCompatibility()
			if tt.wantDetail == "" {
				require.NoError(t, err)
				b, ok := backend.(*SnapshotBackend)
				require.True(t, ok, "got %T", backend)
				require.Equal(t, tt.wantBDVersion, b.bdVersion)
				require.NotNil(t, b.current.Load(), "CheckCompatibility installs the first snapshot")
				require.Equal(t, []string{"export", "blocked"}, f.runner.commands())
				issues, err := b.Services().QueryExecutor.Execute("type = task")
				require.NoError(t, err)
				require.Len(t, issues, 3)
				require.Equal(t, 2, f.runner.count(""), "the query reads the installed snapshot")
				return
			}
			uw := requireEmbeddedUnsupported(t, err, ws.beadsDir)
			require.Equal(t, tt.wantDetail, uw.Detail)
			require.NotContains(t, uw.Detail, "found )", "never a found with an empty version")
			if tt.wantNoBD {
				require.Zero(t, f.runner.count(""), "the gate runs before any bd call")
			}
			if tt.realBD {
				require.NotContains(t, uw.Detail, "(found", "bd missing is not a version refusal")
				require.ErrorIs(t, err, exec.ErrNotFound)
				require.Equal(t, []string{"export"}, f.runner.commands())
			}
		})
	}
}

// TestNewBeadsBackend_EmbeddedBDRefusal: bd refusing the export (a pending
// schema migration) is a bd-error whose text leads with perles' hint.
func TestNewBeadsBackend_EmbeddedBDRefusal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake bd is a POSIX shell script")
	}
	ws := newEmbeddedWorkspace(t, embeddedTestMetadata)
	fakeBDDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(fakeBDDir, "bd"), []byte(`#!/bin/sh
echo "Error: database schema v45 has pending migrations; run 'bd migrate schema' to apply them" >&2
exit 1
`), 0o755)) //nolint:gosec // test executable
	f := newRunnerFactory(nil)

	backend, err := newBeadsBackend(ws.beadsDir, filepath.Dir(ws.beadsDir), backendDeps{
		bdContext:   bdContextAnswer(`{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":"1.3.2"}`),
		newBDRunner: f.new,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	t.Setenv("PATH", fakeBDDir) // the fake bd only

	err = backend.CheckCompatibility()

	uw := requireEmbeddedUnsupported(t, err, ws.beadsDir)
	require.Equal(t, "bd-error: bd export failed: beads database needs a schema migration: run `bd migrate schema` — "+
		"database schema v45 has pending migrations; run 'bd migrate schema' to apply them", uw.Detail)
	require.True(t, infrabeads.IsBDError(err))
	require.NotErrorIs(t, err, exec.ErrNotFound)
	require.Equal(t, []string{"export"}, f.runner.commands())
}

// TestNewBeadsBackend_EmbeddedWorkspaceUnresolved: an embedded workspace
// whose database repository or manifest is missing is no construction
// error but an unsupported workspace with the workspace cause, so the
// screen explains it. No runner is created.
func TestNewBeadsBackend_EmbeddedWorkspaceUnresolved(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, beadsDir string)
		wantText string
	}{
		{
			name:     "no embedded repository",
			setup:    func(*testing.T, string) {},
			wantText: "holds no embedded Dolt repository",
		},
		{
			name: "repository for another database only",
			setup: func(t *testing.T, beadsDir string) {
				writeTestFile(t, filepath.Join(beadsDir, "embeddeddolt", "other", ".dolt", "noms", "manifest"), "manifest")
			},
			wantText: `embedded Dolt database "beads" has no repository`,
		},
		{
			name: "missing manifest",
			setup: func(t *testing.T, beadsDir string) {
				writeTestFile(t, filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt", "config.json"), "{}")
			},
			wantText: `embedded Dolt manifest for database "beads"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			t.Setenv("PATH", "")
			beadsDir := filepath.Join(t.TempDir(), ".beads")
			writeTestFile(t, filepath.Join(beadsDir, "metadata.json"), embeddedTestMetadata)
			tt.setup(t, beadsDir)
			f := newRunnerFactory(generationRunner(t).run)

			backend, err := newBeadsBackend(beadsDir, filepath.Dir(beadsDir), backendDeps{
				bdContext:   bdContextAnswer(`{"backend":"dolt","dolt_mode":"embedded","database":"beads","bd_version":"1.3.2"}`),
				newBDRunner: f.new,
			})

			// == nil, not require.Nil, which also accepts a typed nil.
			require.True(t, backend == nil, "no typed nil behind the interface: got %T", backend)
			uw := requireEmbeddedUnsupported(t, err, beadsDir)
			cause, text := task.ParseEmbeddedDetail(uw.Detail)
			require.Equal(t, task.EmbeddedCauseWorkspace, cause)
			require.Contains(t, text, tt.wantText)
			require.ErrorIs(t, err, fs.ErrNotExist)
			require.Empty(t, f.created(), "no runner")
		})
	}
}

// TestNewBeadsBackend_EmbeddedWithoutBDContext: with the real bd context
// runner, which never runs bd in tests, an embedded workspace still gets a
// SnapshotBackend; its build then refuses the unread bd version.
func TestNewBeadsBackend_EmbeddedWithoutBDContext(t *testing.T) {
	ws := newEmbeddedWorkspace(t, `{"backend":"dolt","dolt_mode":"embedded"}`)

	backend, err := NewBeadsBackend(ws.beadsDir, filepath.Dir(ws.beadsDir))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, backend.Close()) })
	b, ok := backend.(*SnapshotBackend)
	require.True(t, ok, "got %T", backend)
	require.Equal(t, ws.manifest, b.manifestPath, "the database defaults to bd's \"beads\"")

	uw := requireEmbeddedUnsupported(t, backend.CheckCompatibility(), ws.beadsDir)
	cause, _ := task.ParseEmbeddedDetail(uw.Detail)
	require.Equal(t, task.EmbeddedCauseBDVersion, cause)
	require.Contains(t, uw.Detail, "could not read the bd version")
}

func TestNewBeadsBackend_ServerDown(t *testing.T) {
	isolateEnv(t)
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	require.NoError(t, os.MkdirAll(beadsDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt","dolt_mode":"server"}`), 0o600))

	backend, err := NewBeadsBackend(beadsDir, filepath.Dir(beadsDir))

	// == nil, not require.Nil, which also accepts a typed nil.
	require.True(t, backend == nil, "no typed nil behind the interface: got %T", backend)
	sd, ok := errors.AsType[*task.ServerDownError](err)
	require.True(t, ok, "got %v", err)
	require.Zero(t, sd.Port, "no port file: nothing is listening")
	require.True(t, sd.ManagedByBD)
}

// TestBeadsBackend_CheckCompatibility: server mode keeps MinBeadsVersion
// (1.0.0); the embedded 1.0.5 gate does not apply to it.
func TestBeadsBackend_CheckCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		version string
		err     error
		want    error
	}{
		{name: "1.0.0", version: "1.0.0"},
		{name: "1.0.4", version: "1.0.4"},
		{name: "1.0.5", version: "1.0.5"},
		{name: "1.3.2", version: "1.3.2"},
		{name: "0.62.0", version: "0.62.0", want: &task.VersionIncompatibleError{Current: "0.62.0", Required: beads.MinBeadsVersion}},
		{name: "unreadable", err: errors.New("no local_metadata"), want: &task.VersionIncompatibleError{Current: "unknown", Required: beads.MinBeadsVersion}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BeadsBackend{client: versionClient{version: tt.version, err: tt.err}}
			err := b.CheckCompatibility()
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.Equal(t, tt.want, err)
		})
	}
}

func TestDoltWatcherConfig(t *testing.T) {
	cfg := doltWatcherConfig()
	require.Equal(t, []string{"last-touched"}, cfg.RelevantFiles)
	require.Equal(t, 500*time.Millisecond, cfg.DebounceDuration)
}

// versionClient is a DBClient whose Version is fixed; nothing else is used.
type versionClient struct {
	appbeads.DBClient
	version string
	err     error
}

func (c versionClient) Version() (string, error) { return c.version, c.err }

var _ appbeads.DBClient = versionClient{}

// embeddedTestMetadata is metadata.json of an embedded workspace whose
// database is "beads".
const embeddedTestMetadata = `{"backend":"dolt","dolt_mode":"embedded","dolt_database":"beads"}`

// newEmbeddedWorkspace returns an embedded workspace with the given
// metadata.json and the manifest of its "beads" database, in an environment
// isolated from the user's beads settings and with an empty PATH, so that
// nothing can reach a real bd.
func newEmbeddedWorkspace(t *testing.T, metadata string) testWorkspace {
	t.Helper()
	isolateEnv(t)
	t.Setenv("PATH", "")
	ws := newTestWorkspace(t)
	writeTestFile(t, filepath.Join(ws.beadsDir, "metadata.json"), metadata)
	return ws
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// bdContextAnswer is a bd context runner that prints out.
func bdContextAnswer(out string) infrabeads.BDContextRunner {
	return func(context.Context, string, string) ([]byte, error) { return []byte(out), nil }
}

// runnerFactory is a backendDeps.newBDRunner that records the runners it
// creates. Every runner records its requests in runner; they are answered
// by answer, or by the real bd runner when answer is nil.
type runnerFactory struct {
	runner *fakeRunner
	answer infrabeads.BDRunner

	mu   sync.Mutex
	args [][2]string // workDir, beadsDir of each runner created
}

func newRunnerFactory(answer infrabeads.BDRunner) *runnerFactory {
	return &runnerFactory{runner: &fakeRunner{}, answer: answer}
}

func (f *runnerFactory) new(workDir, beadsDir string) infrabeads.BDRunner {
	f.mu.Lock()
	f.args = append(f.args, [2]string{workDir, beadsDir})
	f.mu.Unlock()
	answer := f.answer
	if answer == nil {
		answer = infrabeads.NewBDRunner(workDir, beadsDir)
	}
	f.runner.answer = answer
	return f.runner.run
}

func (f *runnerFactory) created() [][2]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][2]string(nil), f.args...)
}

// requireEmbeddedUnsupported asserts that err is an embedded unsupported
// workspace for beadsDir whose Detail parses to a cause with a non-empty
// text, and returns it.
func requireEmbeddedUnsupported(t *testing.T, err error, beadsDir string) *task.UnsupportedWorkspaceError {
	t.Helper()
	uw, ok := errors.AsType[*task.UnsupportedWorkspaceError](err)
	require.True(t, ok, "got %v", err)
	require.Equal(t, task.WorkspaceEmbeddedDolt, uw.Kind)
	require.Equal(t, beadsDir, uw.BeadsDir)
	cause, text := task.ParseEmbeddedDetail(uw.Detail)
	require.NotEmpty(t, cause, "Detail %q names a cause", uw.Detail)
	require.NotEmpty(t, strings.TrimSpace(text), "Detail %q says what failed", uw.Detail)
	require.Error(t, uw.Err)
	return uw
}

// isolateEnv clears BEADS_* and BD_* variables and redirects HOME so the
// backend sees only the workspace a test writes.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "BEADS_") || strings.HasPrefix(name, "BD_") {
			t.Setenv(name, "")
		}
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
}
