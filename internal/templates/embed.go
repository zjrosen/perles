package templates

import (
	"embed"
	"io/fs"
)

// registryTemplates embeds all workflow registry.yaml files and templates.
// The structure is:
//   - workflows/<workflow-name>/registry.yaml
//   - workflows/<workflow-name>/*.md (workflow-specific templates)
//   - workflows/*.md (shared templates like v1-epic-instructions.md)
//
//go:embed workflows
var registryTemplates embed.FS

// RegistryFS returns the embedded filesystem containing workflow registries and templates.
// This is used by the registry service to load workflow registrations.
func RegistryFS() fs.FS {
	return registryTemplates
}
