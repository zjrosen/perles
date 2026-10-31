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
