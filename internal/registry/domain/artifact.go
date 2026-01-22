package registry

// Artifact represents a file output produced or consumed by a workflow node.
// Artifacts have a key (for template access) and a filename (the actual file path).
// The filename may contain Go template syntax for dynamic paths.
type Artifact struct {
	key      string // Stable key for template access (e.g., "report")
	filename string // File path, may contain templates (e.g., "{{.Date}}-report.md")
}

// NewArtifact creates a new artifact with an explicit key and filename.
// The key is used for stable template access (e.g., {{.Outputs.report}}).
// The filename may contain Go template syntax for dynamic paths.
func NewArtifact(key, filename string) *Artifact {
	return &Artifact{
		key:      key,
		filename: filename,
	}
}

// Key returns the artifact key used for template access.
// This is a stable identifier regardless of dynamic filename content.
func (a *Artifact) Key() string {
	return a.key
}

// Filename returns the artifact filename (may contain template syntax).
func (a *Artifact) Filename() string {
	return a.filename
}
