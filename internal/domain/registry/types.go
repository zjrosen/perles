package registry

// Registration represents a registered workflow namespace+version
type Registration struct {
	namespace   string   // e.g., "spec-workflow"
	key         string   // e.g., "planning-standard"
	version     string   // e.g., "v1"
	name        string   // e.g., "Standard Planning Workflow"
	description string   // e.g., "Three-phase workflow: Research, Propose, Plan"
	template    string   // template filename for epic description (e.g., "v1-research-proposal-epic.md")
	dag         *Chain   // DAG-based workflow chain (replaces flat chain)
	labels      []string // e.g., ["lang:go", "category:workflow"]
}

// newRegistration creates a registration (used by builder)
func newRegistration(namespace, key, version, name, description, template string, dag *Chain, labels []string) *Registration {
	return &Registration{
		namespace:   namespace,
		key:         key,
		version:     version,
		name:        name,
		description: description,
		template:    template,
		dag:         dag,
		labels:      labels,
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
