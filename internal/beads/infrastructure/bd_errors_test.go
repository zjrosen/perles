package infrastructure

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// bdErrorFor runs a single failed bd invocation through runBeads.
func bdErrorFor(t *testing.T, args []string, out bdOutput) error {
	t.Helper()
	executor := newRawTestExecutor(func(...string) bdOutput { return out })
	_, err := executor.runBeads(args...)
	require.Error(t, err)
	return err
}

func TestBDError_StdoutJSONWhenStderrEmpty(t *testing.T) {
	// HandleErrorRespectJSON: flag/argument errors under --json go to stdout,
	// indented, with nothing on stderr.
	stdout := "{\n  \"error\": \"--notes \\\"\\\" would clear the notes field (an empty command substitution is the usual accident); use --clear-notes to clear it deliberately\",\n  \"schema_version\": 1\n}\n"
	err := bdErrorFor(t, []string{"update", "X-1", "--notes=", "--json"}, bdFailed(1, stdout, ""))

	require.Equal(t,
		`bd update failed: --notes "" would clear the notes field (an empty command substitution is the usual accident); use --clear-notes to clear it deliberately`,
		err.Error())
}

func TestBDError_StderrProsePreferredOverExitStatus(t *testing.T) {
	err := bdErrorFor(t, []string{"show", "X-1", "--json"}, bdFailed(1, "", "Error: no issue found matching \"X-1\"\n"))
	require.Equal(t, `bd show failed: no issue found matching "X-1"`, err.Error())
}

func TestBDError_NoOutputFallsBackToExitError(t *testing.T) {
	out := bdOutput{ExitCode: 1, Err: errors.New("exit status 1")}
	err := bdErrorFor(t, []string{"update", "X-1"}, out)
	require.Equal(t, "bd update failed: exit status 1", err.Error())

	noErr := bdOutput{ExitCode: 3}
	err = bdErrorFor(t, []string{"update", "X-1"}, noErr)
	require.Equal(t, "bd update failed: exit status 3", err.Error())
}

func TestBDError_StderrIndentedJSONWithHint(t *testing.T) {
	stderr := "{\n  \"error\": \"database not initialized\",\n  \"hint\": \"run 'bd init'\",\n  \"schema_version\": 1\n}\n"
	err := bdErrorFor(t, []string{"update", "X-1", "--json"}, bdFailed(1, "", stderr))
	require.Equal(t, "bd update failed: database not initialized Hint: run 'bd init'", err.Error())
}

func TestBDError_EnvelopedErrorObject(t *testing.T) {
	stderr := `{"schema_version":1,"data":{"error":"issue X-1 not found","code":"not_found"}}`
	err := bdErrorFor(t, []string{"show", "X-1", "--json"}, bdFailed(1, "", stderr))
	require.Equal(t, "bd show failed: issue X-1 not found", err.Error())
}

func TestBDError_ProseAndJSONSummaryDeduplicated(t *testing.T) {
	// bd close/update: the refusal as prose, then a compact JSON report.
	stderr := "Error closing X-1: cannot close blocked issue: X-1 is blocked by [X-2]\n" +
		`{"error":"1 of 1 issues failed to close","failed":[{"id":"X-1","error":"cannot close blocked issue: X-1 is blocked by [X-2]"}],"schema_version":1}` + "\n"
	err := bdErrorFor(t, []string{"close", "X-1", "--reason=done", "--json"}, bdFailed(1, "", stderr))
	require.Equal(t, "bd close failed: Error closing X-1: cannot close blocked issue: X-1 is blocked by [X-2]", err.Error())
}

func TestBDError_JSONOnlyFailedEntriesReplaceSummary(t *testing.T) {
	stderr := `{"error":"1 of 1 issues failed to update","failed":[{"id":"X-1","error":"updating issue: invalid status \"bogus\""}],"schema_version":1}`
	err := bdErrorFor(t, []string{"update", "X-1", "--json"}, bdFailed(1, "", stderr))
	require.Equal(t, `bd update failed: updating issue: invalid status "bogus"`, err.Error())
}

func TestBDError_NoiseLinesDropped(t *testing.T) {
	stderr := "NOTE: bd --json output format will change in v2.0. Set BD_JSON_ENVELOPE=1 to opt in early.\n" +
		"Error: unknown command \"reopen\" for \"bd\"\n\nRun 'bd --help' for usage.\n"
	err := bdErrorFor(t, []string{"reopen", "X-1", "--json"}, bdFailed(1, "", stderr))
	require.Equal(t, `bd reopen failed: unknown command "reopen" for "bd"`, err.Error())
	require.True(t, isUnknownCommand(err, "reopen"))
	require.False(t, isUnknownCommand(err, "close"))
}

func TestBDError_CommandName(t *testing.T) {
	err := bdErrorFor(t, []string{"comments", "add", "X-1", "--", "hi"}, bdFailed(1, "", "Error: boom"))
	require.Equal(t, "bd comments add failed: boom", err.Error())

	err = bdErrorFor(t, []string{"dep", "add", "X-1", "X-2"}, bdFailed(1, "", "Error: cycle"))
	require.Equal(t, "bd dep add failed: cycle", err.Error())
}

func TestBDCommandName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no args", args: nil, want: "command"},
		{name: "plain command", args: []string{"update", "X-1", "--json"}, want: "update"},
		{name: "readonly export", args: []string{"--readonly", "export", "--all"}, want: "export"},
		{name: "readonly show", args: []string{"--readonly", "show", "X-1", "--json"}, want: "show"},
		{name: "readonly comments", args: []string{"--readonly", "comments", "X-1", "--json"}, want: "comments"},
		{name: "readonly statuses", args: []string{"--readonly", "statuses", "--json"}, want: "statuses"},
		{name: "readonly blocked with command flags", args: []string{"--readonly", "blocked", "--flags", "--raw", "--json"}, want: "blocked"},
		{name: "several global flags", args: []string{"--json", "--readonly", "-q", "list"}, want: "list"},
		{name: "comments add", args: []string{"comments", "add", "X-1", "--", "hi"}, want: "comments add"},
		{name: "readonly comments add", args: []string{"--readonly", "comments", "add", "X-1"}, want: "comments add"},
		{name: "dep add", args: []string{"dep", "add", "X-1", "X-2"}, want: "dep add"},
		{name: "add only qualifies comments and dep", args: []string{"show", "add"}, want: "show"},
		{name: "value flag with separate value", args: []string{"--actor", "alice", "update", "X-1"}, want: "update"},
		{name: "value flag with attached value", args: []string{"--actor=alice", "update", "X-1"}, want: "update"},
		{name: "directory shorthand", args: []string{"-C", "/repo", "--readonly", "list"}, want: "list"},
		{name: "hidden format flag with separate value", args: []string{"--format", "json", "list"}, want: "list"},
		{name: "only flags", args: []string{"--version"}, want: "--version"},
		{name: "value flag missing its value", args: []string{"--readonly", "--db"}, want: "--readonly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, bdCommandName(tt.args))
		})
	}
}

// TestBDError_ReadonlyCommandName: --readonly reads name their command in
// the message, not the global flag.
func TestBDError_ReadonlyCommandName(t *testing.T) {
	err := bdErrorFor(t, []string{"--readonly", "show", "X-1", "--json"}, bdFailed(1, "", "Error: no issue found matching \"X-1\"\n"))
	require.Equal(t, `bd show failed: no issue found matching "X-1"`, err.Error())

	err = bdErrorFor(t, []string{"--readonly", "comments", "X-1", "--json"}, bdFailed(1, "", "Error: boom\n"))
	require.Equal(t, "bd comments failed: boom", err.Error())

	timedOut := newBDError([]string{"--readonly", "export", "--all"},
		bdOutput{ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true}, 30*time.Second)
	require.Equal(t, "export", timedOut.Command)
	require.Equal(t, "bd export timed out after 30s: "+hintTimeout, timedOut.Error())
}

func TestUnknownFlagName(t *testing.T) {
	refusal := func(stdout, stderr string) error {
		return newBDError([]string{"--readonly", "blocked", "--flags", "--raw", "--json"}, bdFailed(1, stdout, stderr), time.Minute)
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		// bd v1.3.2 (release), stdout empty: cobra names only the first unknown flag.
		{name: "--flags", err: refusal("", "Error: unknown flag: --flags\n"), want: "--flags"},
		{name: "--raw", err: refusal("", "Error: unknown flag: --raw\n"), want: "--raw"},
		{name: "near miss is its own name", err: refusal("", "Error: unknown flag: --flagsx\n"), want: "--flagsx"},
		{name: "prefix of the name", err: refusal("", "Error: unknown flag: --flag\n"), want: "--flag"},
		{name: "clear-notes", err: refusal("", "Error: unknown flag: --clear-notes\n"), want: "--clear-notes"},
		{name: "usage line after it", err: refusal("", "Error: unknown flag: --flags\nRun 'bd blocked --help' for usage.\n"), want: "--flags"},
		{name: "JSON error object", err: refusal(`{"error":"unknown flag: --raw","schema_version":1}`, ""), want: "--raw"},
		{name: "trailing punctuation", err: refusal("", "Error: unknown flag: --flags.\n"), want: "--flags"},
		{name: "wrapped bd error", err: fmt.Errorf("reading blocked flags: %w", refusal("", "Error: unknown flag: --flags\n")), want: "--flags"},
		{name: "shorthand flag", err: refusal("", "Error: unknown shorthand flag: 'f' in -f\n"), want: ""},
		{name: "unknown command", err: refusal("", "Error: unknown command \"blocked\" for \"bd\"\n"), want: ""},
		{name: "flag without its argument", err: refusal("", "Error: flag needs an argument: --actor\n"), want: ""},
		{name: "migration refusal", err: refusal("", "Error: database schema is at v45 but this binary expects v46; run 'bd migrate schema'\n"), want: ""},
		{name: "timeout", err: newBDError([]string{"blocked"}, bdOutput{ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true}, time.Second), want: ""},
		{name: "not a bd error", err: errors.New("unknown flag: --flags"), want: ""},
		{name: "nil", err: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, unknownFlagName(tt.err))
		})
	}
}

// TestUnknownFlagName_StricterThanIsUnknownFlag: isUnknownFlag keeps its
// substring match (used for --clear-notes); unknownFlagName names the flag.
func TestUnknownFlagName_StricterThanIsUnknownFlag(t *testing.T) {
	err := bdErrorFor(t, []string{"--readonly", "blocked", "--flagsx"}, bdFailed(1, "", "Error: unknown flag: --flagsx\n"))
	require.True(t, isUnknownFlag(err, "flags"))
	require.Equal(t, "--flagsx", unknownFlagName(err))
}

func TestBDError_Classification(t *testing.T) {
	migrateConsent := `{
  "error": "database schema is at v65; this bd requires v74 (9 pending migration(s)), and bd no longer migrates a database without explicit consent",
  "hint": "Operator decision required: this database is at schema v65 and this bd requires v74. Migrating is ONE-WAY.",
  "migrate_consent": {
    "current_version": 65,
    "required_version": 74,
    "pending": 9,
    "severity": "blocking",
    "human_decision_required": true,
    "options": [{"id": "migrate", "commands": ["bd migrate schema"]}, {"id": "keep", "commands": []}]
  },
  "schema_version": 1
}`
	schemaSkew := `{
  "error": "schema version mismatch: database is at v80, binary knows up to v74 (6 migrations ahead)",
  "hint": "BD_IGNORE_SCHEMA_SKEW=1 bd <command>  or  bd --ignore-schema-skew <command>",
  "schema_skew": {"current_version": 80, "required_version": 74, "delta": 6},
  "schema_version": 1
}`
	remoteGate := `{
  "error": "refusing to auto-apply 3 pending schema migrations to a remote-backed database (v71 -> v74): migrating clones independently forks the schema (#4259)",
  "hint": "Coordination decision required: only ONE clone may migrate a shared remote",
  "remote_migrate_gate": {"pending": 3},
  "schema_version": 1
}`
	remoteAdopt := `{
  "error": "refusing to migrate a remote-backed database (v71 -> v74): the remote is already migrated — adopt it instead of migrating here (#4259)",
  "remote_migrate_gate": {"decision": "adopt"},
  "schema_version": 1
}`
	frozenProse := "⛔ ERROR: workspace is frozen for migration (by alice).\n" +
		"   Reason: moving to dolt\n" +
		"   bd update is blocked by the freeze marker at /repo/.beads/MIGRATION-FREEZE.\n" +
		"   To resume writes, remove that file.\n"

	cases := []struct {
		name     string
		out      bdOutput
		hint     string
		contains string
		excludes string
	}{
		{
			name:     "migrate consent JSON on stderr",
			out:      bdFailed(1, "", migrateConsent),
			hint:     hintMigrate,
			contains: "database schema is at v65; this bd requires v74",
			excludes: "Operator decision required",
		},
		{
			name:     "schema behind (read-only open, beads main)",
			out:      bdFailed(1, "", "Error: schema version mismatch: database is at v65, binary expects v74; bd does not migrate a database without explicit consent — run `bd migrate schema` in that workspace to migrate"),
			hint:     hintMigrate,
			contains: "binary expects v74",
		},
		{
			name: "schema behind (bd v1.3.0 wording)",
			out:  bdFailed(1, "", "Error: schema version mismatch: database is at v60, binary expects v66, and the read-only open cannot migrate it; run any bd write command in that workspace to migrate"),
			hint: hintMigrate,
		},
		{
			name:     "schema skew: bd older than the database",
			out:      bdFailed(1, "", schemaSkew),
			hint:     hintUpgradeBD,
			contains: "binary knows up to v74",
			excludes: "BD_IGNORE_SCHEMA_SKEW",
		},
		{
			name:     "schema skew prose",
			out:      bdFailed(1, "", "Error: schema version mismatch: database is at v80, binary knows up to v74 (6 migrations ahead)\n"),
			hint:     hintUpgradeBD,
			contains: "6 migrations ahead",
		},
		{
			name:     "remote-backed migrate gate",
			out:      bdFailed(1, "", remoteGate),
			hint:     hintMigrate,
			contains: "remote-backed database",
		},
		{
			name:     "remote-backed adopt",
			out:      bdFailed(1, "", remoteAdopt),
			hint:     hintMigrateCoordinate,
			contains: "adopt it instead",
		},
		{
			name:     "migration freeze by exit code",
			out:      bdFailed(bdExitMigrationFrozen, "", frozenProse),
			hint:     hintFrozen,
			contains: "workspace is frozen for migration (by alice). Reason: moving to dolt bd update is blocked by the freeze marker at /repo/.beads/MIGRATION-FREEZE. To resume writes, remove that file.",
		},
		{
			name: "migration freeze by text",
			out:  bdFailed(1, "", "⛔ ERROR: cannot determine whether this workspace is frozen for migration.\n"),
			hint: hintFrozen,
		},
		{
			name:     "dolt server too old",
			out:      bdFailed(1, "", "Error: dolt sql-server at 127.0.0.1:3307 is version 2.5.1, but bd requires dolt 2.6.0 or newer outside embedded mode. Upgrade dolt to 2.6.0 or newer, then restart the server on it\n"),
			hint:     hintDolt,
			contains: "is version 2.5.1",
		},
		{
			name:     "gateway schema owned by the server admin",
			out:      bdFailed(1, "", "Error: schema version mismatch: database is at v65, binary expects v74; the gateway server owns this database's schema and bd never migrates it — ask the server administrator to upgrade the database"),
			hint:     "",
			contains: "ask the server administrator",
		},
		{
			name: "unclassified refusal",
			out:  bdFailed(1, "", "Error updating X-1: cannot close X-1: 1 open child issue(s); close children first or use --force to override\n"),
			hint: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := bdErrorFor(t, []string{"update", "X-1", "--json"}, tc.out)
			bdErr, ok := errors.AsType[*bdError](err)
			require.True(t, ok)
			require.Equal(t, tc.hint, bdErr.Hint)
			if tc.hint != "" {
				require.True(t, strings.HasPrefix(err.Error(), "bd update failed: "+tc.hint+" — "), err.Error())
			}
			if tc.contains != "" {
				require.Contains(t, err.Error(), tc.contains)
			}
			if tc.excludes != "" {
				require.NotContains(t, err.Error(), tc.excludes)
			}
			require.NotContains(t, err.Error(), "\n", "messages stay single-line for toasts")
		})
	}
}

func TestBDError_Timeout(t *testing.T) {
	executor := newRawTestExecutor(func(...string) bdOutput {
		return bdOutput{ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true}
	})
	executor.timeout = 90 * time.Second

	_, err := executor.runBeads("update", "X-1", "--json")
	require.Error(t, err)
	require.Equal(t, "bd update timed out after 1m30s: "+hintTimeout, err.Error())

	withStderr := newRawTestExecutor(func(...string) bdOutput {
		return bdOutput{Stderr: "waiting for workspace gate...\n", ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true}
	})
	_, err = withStderr.runBeads("update", "X-1", "--json")
	require.Equal(t, "bd update timed out after 1m30s: "+hintTimeout+" — waiting for workspace gate...", err.Error())
}

func TestBDError_NotFound(t *testing.T) {
	out := bdOutput{ExitCode: -1, Err: fmt.Errorf("exec: \"bd\": %w", exec.ErrNotFound)}
	err := bdErrorFor(t, []string{"update", "X-1"}, out)
	require.Equal(t, "bd update failed: bd CLI not found on PATH", err.Error())
	require.ErrorIs(t, err, exec.ErrNotFound)
}

func TestBDError_Truncated(t *testing.T) {
	long := "Error: " + strings.Repeat("very long refusal ", 100)
	err := bdErrorFor(t, []string{"update", "X-1"}, bdFailed(1, "", long))
	bdErr, ok := errors.AsType[*bdError](err)
	require.True(t, ok)
	require.LessOrEqual(t, len([]rune(bdErr.Message)), maxBDMessageRunes)
	require.True(t, strings.HasSuffix(bdErr.Message, "…"))
}

func TestBDError_Predicates(t *testing.T) {
	refusal := bdErrorFor(t, []string{"update", "X-1", "--notes=n", "--json"}, notesOverwriteRefusal("X-1"))
	require.True(t, isNotesOverwriteRefusal(refusal))
	require.Contains(t, refusal.Error(), "--notes would replace existing notes")

	viaServer := bdErrorFor(t, []string{"update", "X-1"}, bdFailed(1, "", `{"error":"notes_overwrite_refused: update would replace existing notes"}`))
	require.True(t, isNotesOverwriteRefusal(viaServer))

	closeRefusal := bdErrorFor(t, []string{"update", "X-1"}, bdFailed(1, "", "Error updating X-1: cannot close X-1: 1 open child issue(s); close children first or use --force to override"))
	require.False(t, isNotesOverwriteRefusal(closeRefusal), "a --force hint that is not about notes is not a notes refusal")

	unknownFlag := bdErrorFor(t, []string{"update", "X-1"}, bdFailed(1, "", "Error: unknown flag: --clear-notes\n"))
	require.True(t, isUnknownFlag(unknownFlag, "clear-notes"))
	require.False(t, isUnknownFlag(unknownFlag, "force"))

	require.False(t, isNotesOverwriteRefusal(errors.New("--force notes")), "only bd errors are inspected")
	require.False(t, isUnknownFlag(errors.New("unknown flag: --clear-notes"), "clear-notes"))
	require.False(t, isUnknownCommand(errors.New(`unknown command "reopen"`), "reopen"))
}
