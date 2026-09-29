package registry

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// errFS is an fs.FS whose Open always fails with err.
type errFS struct{ err error }

func (e errFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: e.err}
}

func TestNewLayeredFS_NilFallbackReturnsPrimary(t *testing.T) {
	primary := fstest.MapFS{"a.md": {Data: []byte("primary")}}

	got := newLayeredFS(primary, nil)

	require.Equal(t, primary, got, "nil fallback should return the primary FS unchanged")
}

func TestLayeredFS_Precedence(t *testing.T) {
	primary := fstest.MapFS{
		"workflows/shared.md":    {Data: []byte("primary shared")},
		"workflows/only-user.md": {Data: []byte("primary only")},
	}
	fallback := fstest.MapFS{
		"workflows/shared.md":       {Data: []byte("fallback shared")},
		"workflows/only-builtin.md": {Data: []byte("fallback only")},
	}
	lfs := newLayeredFS(primary, fallback)

	content, err := fs.ReadFile(lfs, "workflows/shared.md")
	require.NoError(t, err)
	require.Equal(t, "primary shared", string(content), "earlier layer should win")

	content, err = fs.ReadFile(lfs, "workflows/only-user.md")
	require.NoError(t, err)
	require.Equal(t, "primary only", string(content))

	content, err = fs.ReadFile(lfs, "workflows/only-builtin.md")
	require.NoError(t, err)
	require.Equal(t, "fallback only", string(content), "missing files should fall back to later layers")

	_, err = fs.Stat(lfs, "workflows/only-builtin.md")
	require.NoError(t, err, "Stat should also fall back")
}

func TestLayeredFS_NotFoundInAnyLayer(t *testing.T) {
	lfs := newLayeredFS(fstest.MapFS{}, fstest.MapFS{})

	_, err := lfs.Open("workflows/missing.md")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestLayeredFS_InvalidPath(t *testing.T) {
	lfs := newLayeredFS(fstest.MapFS{}, fstest.MapFS{"x.md": {Data: []byte("x")}})

	_, err := lfs.Open("../x.md")
	require.ErrorIs(t, err, fs.ErrInvalid)
}

func TestLayeredFS_NonNotExistErrorDoesNotFallBack(t *testing.T) {
	// An unreadable file in the primary layer must not be silently replaced by the fallback copy.
	lfs := newLayeredFS(errFS{err: fs.ErrPermission}, fstest.MapFS{"x.md": {Data: []byte("fallback")}})

	_, err := lfs.Open("x.md")
	require.Error(t, err)
	require.True(t, errors.Is(err, fs.ErrPermission), "expected permission error, got %v", err)
}
