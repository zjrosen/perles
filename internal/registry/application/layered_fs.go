package registry

import (
	"errors"
	"io/fs"
)

// layeredFS is a read-only fs.FS that looks up each path in a list of filesystems
// in order and returns the first match. It lets user and community workflows use
// perles' built-in shared templates (e.g. v1-epic-instructions.md, v1-human-review.md)
// while files in earlier layers keep precedence.
//
// Directory listings are not merged: opening a directory returns the first layer's
// directory, so a layeredFS must not be used to discover workflows.
type layeredFS []fs.FS

// newLayeredFS returns fsys layered over fallback.
// If fallback is nil, fsys is returned unchanged.
func newLayeredFS(fsys, fallback fs.FS) fs.FS {
	if fallback == nil {
		return fsys
	}
	return layeredFS{fsys, fallback}
}

// Open implements fs.FS. A layer is skipped only when the path does not exist in it;
// any other error (e.g. permission denied) is returned so that an unreadable file in
// an earlier layer is never silently replaced by a later layer's copy.
func (l layeredFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	var notExistErr error
	for _, layer := range l {
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if notExistErr == nil {
			notExistErr = err
		}
	}

	if notExistErr == nil {
		notExistErr = &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return nil, notExistErr
}
