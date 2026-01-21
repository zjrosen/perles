package registry

// Node represents a step in a workflow DAG with inputs, outputs, and dependencies.
type Node struct {
	key      string      // unique identifier for this node
	name     string      // display name
	template string      // template filename
	inputs   []*Artifact // artifacts consumed by this node
	outputs  []*Artifact // artifacts produced by this node
	after    []string    // node keys this node must run after (ordering-only deps)
	assignee string      // worker role to assign this task to (e.g., "research-lead", "architect")
}

// NewNode creates a new node with the given key, name, and template.
func NewNode(key, name, template string) *Node {
	return &Node{
		key:      key,
		name:     name,
		template: template,
		inputs:   []*Artifact{},
		outputs:  []*Artifact{},
		after:    []string{},
	}
}

// Key returns the node's unique identifier.
func (n *Node) Key() string {
	return n.key
}

// Name returns the node's display name.
func (n *Node) Name() string {
	return n.name
}

// Template returns the node's template filename.
func (n *Node) Template() string {
	return n.template
}

// Inputs returns the artifacts consumed by this node.
func (n *Node) Inputs() []*Artifact {
	return n.inputs
}

// Outputs returns the artifacts produced by this node.
func (n *Node) Outputs() []*Artifact {
	return n.outputs
}

// After returns the node keys this node must run after.
func (n *Node) After() []string {
	return n.after
}

// Assignee returns the worker role to assign this task to.
func (n *Node) Assignee() string {
	return n.assignee
}

// WithInputs adds input artifacts and returns the node for fluent chaining.
func (n *Node) WithInputs(artifacts ...*Artifact) *Node {
	n.inputs = append(n.inputs, artifacts...)
	return n
}

// WithOutputs adds output artifacts and returns the node for fluent chaining.
func (n *Node) WithOutputs(artifacts ...*Artifact) *Node {
	n.outputs = append(n.outputs, artifacts...)
	return n
}

// WithAfter adds ordering dependencies and returns the node for fluent chaining.
func (n *Node) WithAfter(keys ...string) *Node {
	n.after = append(n.after, keys...)
	return n
}

// WithAssignee sets the worker role and returns the node for fluent chaining.
func (n *Node) WithAssignee(assignee string) *Node {
	n.assignee = assignee
	return n
}
