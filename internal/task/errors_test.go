package task

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerDownError(t *testing.T) {
	cause := errors.New("connection refused")

	err := &ServerDownError{Host: "127.0.0.1", Port: 50362, Err: cause}
	require.Equal(t, "127.0.0.1:50362", err.Address())
	require.Equal(t, "backend server unreachable at 127.0.0.1:50362: connection refused", err.Error())
	require.ErrorIs(t, err, cause)

	require.Equal(t, "[::1]:3307", (&ServerDownError{Host: "::1", Port: 3307}).Address())
	require.Equal(t, "unix:/run/dolt.sock", (&ServerDownError{Host: "127.0.0.1", Port: 3307, Socket: "/run/dolt.sock"}).Address())
	require.Equal(t, "127.0.0.1", (&ServerDownError{Host: "127.0.0.1"}).Address())
	require.Equal(t, "backend server unreachable at 127.0.0.1", (&ServerDownError{Host: "127.0.0.1"}).Error())
}

func TestUnsupportedWorkspaceError(t *testing.T) {
	err := &UnsupportedWorkspaceError{Kind: WorkspaceEmbeddedDolt, BeadsDir: "/p/.beads"}
	require.Equal(t, "unsupported beads workspace in /p/.beads: embedded", err.Error())

	err = &UnsupportedWorkspaceError{Kind: WorkspaceUnknownBackend, BeadsDir: "/p/.beads", Detail: "postgres"}
	require.Equal(t, "unsupported beads workspace in /p/.beads: backend (postgres)", err.Error())
}

func TestUnsupportedWorkspaceError_EmbeddedDetail(t *testing.T) {
	err := &UnsupportedWorkspaceError{
		Kind:     WorkspaceEmbeddedDolt,
		BeadsDir: "/p/.beads",
		Detail:   "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4)",
	}
	require.Equal(t, "unsupported beads workspace in /p/.beads: embedded (bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4))", err.Error())
}

func TestParseEmbeddedDetail(t *testing.T) {
	const migration = "bd export failed: beads database needs a schema migration: run `bd migrate schema` — Error: schema 45 is behind bd's 47"
	tests := []struct {
		name      string
		detail    string
		wantCause EmbeddedCause
		wantText  string
	}{
		{"bd not found", "bd-not-found: bd export failed: bd CLI not found on PATH",
			EmbeddedCauseBDNotFound, "bd export failed: bd CLI not found on PATH"},
		{"bd version found", "bd-version: embedded workspaces need bd >= 1.0.5 (found 1.0.4)",
			EmbeddedCauseBDVersion, "embedded workspaces need bd >= 1.0.5 (found 1.0.4)"},
		{"bd version unread", "bd-version: embedded workspaces need bd >= 1.0.5 (could not read the bd version: bd context failed: exit status 1)",
			EmbeddedCauseBDVersion, "embedded workspaces need bd >= 1.0.5 (could not read the bd version: bd context failed: exit status 1)"},
		{"bd error with colons in the text", "bd-error: " + migration, EmbeddedCauseBDError, migration},
		{"workspace", "workspace: /p/.beads/embeddeddolt holds no embedded Dolt repository: file does not exist",
			EmbeddedCauseWorkspace, "/p/.beads/embeddeddolt holds no embedded Dolt repository: file does not exist"},
		{"cause with empty text", "workspace: ", EmbeddedCauseWorkspace, ""},
		{"unknown prefix is a bd error", migration, EmbeddedCauseBDError, migration},
		{"no prefix is a bd error", "something broke", EmbeddedCauseBDError, "something broke"},
		{"cause needs a space after the colon", "bd-version:1.0.4", EmbeddedCauseBDError, "bd-version:1.0.4"},
		{"cause is case-sensitive", "BD-VERSION: found 1.0.4", EmbeddedCauseBDError, "BD-VERSION: found 1.0.4"},
		{"cause must lead", " bd-version: found 1.0.4", EmbeddedCauseBDError, " bd-version: found 1.0.4"},
		{"empty", "", "", ""},
		{"blank", " \t\n", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause, text := ParseEmbeddedDetail(tt.detail)
			require.Equal(t, tt.wantCause, cause)
			require.Equal(t, tt.wantText, text)
		})
	}
}
