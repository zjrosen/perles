package workflow

import (
	"embed"
	"io/fs"
)

// builtinTemplates embeds all built-in workflow templates from the templates directory.
//
//go:embed templates/*
var builtinTemplates embed.FS

// BuiltinTemplatesFS returns the embedded filesystem containing built-in templates.
// This is used by the registry service to load workflow registrations.
func BuiltinTemplatesFS() embed.FS {
	return builtinTemplates
}

// BuiltinTemplatesSubFS returns the templates subdirectory as a filesystem.
// This removes the "templates/" prefix so files can be accessed directly.
func BuiltinTemplatesSubFS() (fs.FS, error) {
	return fs.Sub(builtinTemplates, "templates")
}
