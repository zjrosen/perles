package registry

// Artifact represents a file output produced or consumed by a workflow node.
// Artifacts are compared by filename (case-sensitive).
type Artifact struct {
	filename string // e.g., "research.md"
}

// NewArtifact creates a new artifact with the given filename.
func NewArtifact(filename string) *Artifact {
	return &Artifact{
		filename: filename,
	}
}

// Filename returns the artifact filename.
func (a *Artifact) Filename() string {
	return a.filename
}
