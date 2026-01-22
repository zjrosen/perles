package registry

// Source indicates where a registration originated from.
type Source int

const (
	// SourceBuiltIn indicates a registration bundled with the application.
	SourceBuiltIn Source = iota
	// SourceUser indicates a registration from the user's configuration directory.
	SourceUser
)

// String returns a human-readable representation of the Source.
func (s Source) String() string {
	switch s {
	case SourceBuiltIn:
		return "built-in"
	case SourceUser:
		return "user"
	default:
		return "unknown"
	}
}

// Registration represents a registered workflow namespace+version
type Registration struct {
	namespace    string      // e.g., "workflow"
	key          string      // e.g., "planning-standard"
	version      string      // e.g., "v1"
	name         string      // e.g., "Standard Planning Workflow"
	description  string      // e.g., "Three-phase workflow: Research, Propose, Plan"
	template     string      // template filename for epic description (e.g., "v1-research-proposal-epic.md")
	instructions string      // template filename for coordinator instructions (e.g., "epic_driven.md")
	artifactPath string      // path prefix for artifacts (default: ".spec")
	dag          *Chain      // DAG-based workflow chain (replaces flat chain)
	labels       []string    // e.g., ["lang:go", "category:workflow"]
	arguments    []*Argument // user-configurable parameters for workflow
	source       Source      // origin of registration (built-in or user)
}

// newRegistration creates a registration (used by builder)
func newRegistration(namespace, key, version, name, description, template, instructions, artifactPath string, dag *Chain, labels []string, arguments []*Argument, source Source) *Registration {
	return &Registration{
		namespace:    namespace,
		key:          key,
		version:      version,
		name:         name,
		description:  description,
		template:     template,
		instructions: instructions,
		artifactPath: artifactPath,
		dag:          dag,
		labels:       labels,
		arguments:    arguments,
		source:       source,
	}
}

// Key returns the registration key (unique identifier per type)
func (r *Registration) Key() string {
	return r.key
}

// Name returns the human-readable name
func (r *Registration) Name() string {
	return r.name
}

// Description returns the description for AI agents
func (r *Registration) Description() string {
	return r.description
}

// Namespace returns the registration namespace
func (r *Registration) Namespace() string {
	return r.namespace
}

// Version returns the registration version
func (r *Registration) Version() string {
	return r.version
}

// DAG returns the DAG-based workflow chain
func (r *Registration) DAG() *Chain {
	return r.dag
}

// Labels returns the registration labels
func (r *Registration) Labels() []string {
	return r.labels
}

// Template returns the template filename for the epic description
func (r *Registration) Template() string {
	return r.template
}

// Instructions returns the template filename for coordinator instructions
func (r *Registration) Instructions() string {
	return r.instructions
}

// ArtifactPath returns the path prefix for artifacts.
// Returns empty string if not explicitly set.
func (r *Registration) ArtifactPath() string {
	return r.artifactPath
}

// Arguments returns the workflow's user-configurable parameters.
func (r *Registration) Arguments() []*Argument {
	return r.arguments
}

// Source returns the registration's source (built-in or user).
func (r *Registration) Source() Source {
	return r.source
}
