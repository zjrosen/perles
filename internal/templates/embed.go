package templates

import (
	"embed"
	"io/fs"
)

// registryTemplates embeds the registry.yaml and all node templates.
//
//go:embed registry.yaml
//go:embed *.md
var registryTemplates embed.FS

// RegistryFS returns the embedded filesystem containing registry.yaml and node templates.
// This is used by the registry service to load workflow registrations.
func RegistryFS() fs.FS {
	return registryTemplates
}
